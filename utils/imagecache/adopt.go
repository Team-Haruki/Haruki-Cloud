package imagecache

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

// adoptedKeyPattern is the image-cache key layout of an immutable write:
// <group>/<sha256>-<generation>.<ext>, the shape storeHashed produces.
var adoptedKeyPattern = regexp.MustCompile(`^([A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)*)/([0-9a-f]{64})-([A-Za-z0-9]{1,64})\.(jpg|png)$`)

// AdoptedObject is an image another writer already stored in the image-cache
// bucket under this client's own key layout (Drawing's store-ref mode). Only
// its index row is missing.
type AdoptedObject struct {
	Hash       string // sha256 hex of the bytes, lowercase
	Group      string // image group, e.g. "pjsk"
	CDNPath    string // <group>/<hash>-<generation>.<ext>
	MediaType  string
	SizeBytes  int64
	WriterNode string // image host that accepted the write ("" = none)
}

// AdoptedLocation is where an adopted image is served from.
type AdoptedLocation struct {
	CDNPath string
	// WriterNode is the fresh writer of the recorded row: the host to prefer
	// while replicas converge ("" = any host).
	WriterNode string
	// Duplicate reports that the hash was already recorded under another path.
	// That row is kept and the adopted object is queued for deletion.
	Duplicate bool
}

// ValidateAdoptedObject checks that obj names a fresh write in the
// image-cache key layout for its own hash and group.
func ValidateAdoptedObject(obj AdoptedObject) error {
	match := adoptedKeyPattern.FindStringSubmatch(obj.CDNPath)
	if match == nil {
		return fmt.Errorf("imagecache: adopted path %q is not <group>/<sha256>-<generation>.<ext>", obj.CDNPath)
	}
	if path.Clean(obj.CDNPath) != obj.CDNPath {
		return fmt.Errorf("imagecache: adopted path %q is not canonical", obj.CDNPath)
	}
	if match[1] != obj.Group {
		return fmt.Errorf("imagecache: adopted path %q is outside group %q", obj.CDNPath, obj.Group)
	}
	if match[2] != obj.Hash {
		return fmt.Errorf("imagecache: adopted path %q does not name hash %q", obj.CDNPath, obj.Hash)
	}
	if want := mediaTypeFromPath(obj.CDNPath); obj.MediaType != "" && !strings.EqualFold(obj.MediaType, want) {
		return fmt.Errorf("imagecache: adopted media type %q does not match %q", obj.MediaType, want)
	}
	if obj.SizeBytes <= 0 {
		return fmt.Errorf("imagecache: adopted object has no size")
	}
	return nil
}

// AdoptedRender is the render_cache_index row recorded together with an
// adopted object (Drawing's cache-ref mode): the caller's own directive for
// the render the object answers.
type AdoptedRender struct {
	RequestKey string
	APIPath    string
	UserID     string
	GroupName  string
	KeyVersion int
	// TTLSeconds <= 0 is infinite; otherwise expires_at = now() + TTL.
	TTLSeconds int64
}

// ValidateAdoptedRender checks the fields render_cache_index requires.
func ValidateAdoptedRender(render AdoptedRender) error {
	switch {
	case strings.TrimSpace(render.RequestKey) == "":
		return fmt.Errorf("imagecache: adopted render has no request key")
	case strings.TrimSpace(render.APIPath) == "":
		return fmt.Errorf("imagecache: adopted render has no api path")
	case strings.TrimSpace(render.UserID) == "" || strings.TrimSpace(render.GroupName) == "":
		return fmt.Errorf("imagecache: adopted render has no user or group")
	}
	return nil
}

// AdoptObject records the image_cache_entries row for an object another
// writer uploaded. It writes the same row as storeHashed, under the same
// content lock, so retention and GC treat the object like Cloud's own
// write. It does no object I/O.
//
// If the hash already has a row under another path, that row is kept. Its
// location is returned and the adopted object is queued for deletion through
// the upload-intent queue, so GC removes the duplicate after its grace period.
// Without an index it only validates obj and returns its own location.
func (s *PGStore) AdoptObject(ctx context.Context, obj AdoptedObject, now time.Time) (AdoptedLocation, error) {
	finish := commandtrace.MeasureOperation(ctx, "image.ref_index")
	defer finish()
	return s.adopt(ctx, obj, nil, now)
}

// AdoptRender is AdoptObject plus the render_cache_index row for render, in
// the same content-lock transaction: GC (which retires only rows no render
// index row references, under the same lock) never sees the content row
// without its reference. It is the write Drawing's artifact mode makes, done
// by the caller instead (cache-ref). A legacy_disk row for the hash is
// upgraded to the adopted object, as Drawing's own write does; a garage row
// keeps its path and the render row points at it.
func (s *PGStore) AdoptRender(ctx context.Context, obj AdoptedObject, render AdoptedRender, now time.Time) (AdoptedLocation, error) {
	finish := commandtrace.MeasureOperation(ctx, "image.render_index")
	defer finish()
	if err := ValidateAdoptedRender(render); err != nil {
		return AdoptedLocation{CDNPath: obj.CDNPath, WriterNode: obj.WriterNode}, err
	}
	return s.adopt(ctx, obj, &render, now)
}

func (s *PGStore) adopt(ctx context.Context, obj AdoptedObject, render *AdoptedRender, now time.Time) (AdoptedLocation, error) {
	if err := ValidateAdoptedObject(obj); err != nil {
		return AdoptedLocation{}, err
	}
	own := AdoptedLocation{CDNPath: obj.CDNPath, WriterNode: obj.WriterNode}
	if s == nil {
		return own, nil
	}
	entry := ImageEntry{
		Hash: obj.Hash, GroupName: obj.Group, CDNPath: obj.CDNPath, StorageBackend: BackendGarage,
		MediaType: mediaTypeFromPath(obj.CDNPath), SizeBytes: obj.SizeBytes,
		WriterNode: obj.WriterNode, WrittenAt: now.UTC(),
	}
	location := own
	location.WriterNode = entry.FreshWriterNode(now)
	err := s.WithContentLock(ctx, obj.Hash, func(ctx context.Context, index *PGStore) error {
		finishLookup := commandtrace.MeasureOperation(ctx, "image.lookup")
		existing, found, err := index.Lookup(ctx, obj.Hash)
		finishLookup()
		if err != nil {
			return err
		}
		if found && existing.CDNPath != obj.CDNPath && (render == nil || existing.StorageBackend == BackendGarage) {
			location = AdoptedLocation{CDNPath: existing.CDNPath, WriterNode: existing.FreshWriterNode(now), Duplicate: true}
			commandtrace.RecordOperation(ctx, "image.ref_duplicate", 0)
			if existing.StorageBackend == BackendGarage {
				if err := index.TouchEntry(ctx, obj.Hash); err != nil {
					return err
				}
			}
			if index.tx != nil {
				// Not a recorded path: GC deletes it once the intent is due.
				if err := index.registerUpload(ctx, obj.Hash, obj.CDNPath); err != nil {
					return err
				}
			}
		} else {
			finishIndex := commandtrace.MeasureOperation(ctx, "image.index")
			err = index.InsertEntry(ctx, entry)
			finishIndex()
			if err != nil {
				return err
			}
		}
		if render == nil {
			return nil
		}
		return index.upsertRender(ctx, obj.Hash, *render)
	})
	if err != nil {
		return own, err
	}
	return location, nil
}
