package assetindex

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/storage"
)

// Inventory must be called after the producer has completed all object writes.
// A second inventory before Commit rejects concurrent mutations observed by List.
func Inventory(ctx context.Context, store storage.Store, region string) ([]Object, string, error) {
	if !validRegion(region) {
		return nil, "", fmt.Errorf("asset index: invalid region")
	}
	prefix := region + "-assets/"
	var objects []Object
	err := store.List(ctx, storage.Key(prefix), func(o storage.Object) error {
		if !strings.HasPrefix(string(o.Key), prefix) {
			return fmt.Errorf("asset index: object outside region")
		}
		if _, err := storage.CleanKey(string(o.Key)); err != nil {
			return err
		}
		if len(objects) >= 500_000 {
			return fmt.Errorf("asset index: inventory exceeds object limit")
		}
		objects = append(objects, Object{Key: o.Key, Size: o.Size, ETag: o.ETag, Modified: o.ModTime.UTC()})
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	for i := 1; i < len(objects); i++ {
		if objects[i-1].Key == objects[i].Key {
			return nil, "", fmt.Errorf("asset index: duplicate object")
		}
	}
	data, err := json.Marshal(objects)
	if err != nil {
		return nil, "", err
	}
	return objects, digest(data), nil
}

func objectGroup(key storage.Key) string {
	parts := strings.Split(string(key), "/")
	// Split the usual startapp/ondemand layout by resource domain. Other files
	// stay under the region root, so the complete inventory also covers them.
	if len(parts) >= 4 {
		return strings.Join(parts[:3], "/") + "/"
	}
	return parts[0] + "/"
}

// Prepare writes immutable shards but does not expose them as the current version.
func Prepare(ctx context.Context, store storage.Store, region string) (Manifest, error) {
	objects, revision, err := Inventory(ctx, store, region)
	if err != nil {
		return Manifest{}, err
	}
	if len(objects) == 0 {
		return Manifest{}, fmt.Errorf("asset index: refusing to publish empty region")
	}
	groups := make(map[string][]Object)
	for _, o := range objects {
		p := objectGroup(o.Key)
		groups[p] = append(groups[p], o)
	}
	prefixes := make([]string, 0, len(groups))
	for p := range groups {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	m := Manifest{Version: Version, Region: region, sourceRevision: revision, Complete: true, PublishedAt: time.Now().UTC()}
	for _, p := range prefixes {
		blob, err := writeBlob(ctx, store, Root+region+"/shards/", Shard{Version: Version, Region: region, Prefix: p, Objects: groups[p]})
		if err != nil {
			return Manifest{}, err
		}
		m.Shards = append(m.Shards, ShardRef{Prefix: p, Blob: blob})
	}
	m.Revision = manifestRevision(m.Shards)
	return m, nil
}

// Commit only moves the pointer after all shards (and an optional BPM index)
// exist. Producers must serialize publishing for a region with asset mutations.
func Commit(ctx context.Context, store storage.Store, manifest Manifest) error {
	if manifest.Version != Version || !validRegion(manifest.Region) || !manifest.Complete || !validDigest(manifest.Revision) {
		return fmt.Errorf("asset index: invalid manifest")
	}
	// Validate the bytes consumers will load before advancing the pointer.
	loaded, err := loadRegion(ctx, store, manifest, 500_000)
	if err != nil {
		return err
	}
	if loaded.inventoryRevision != manifest.sourceRevision {
		return fmt.Errorf("asset index: complete manifest does not cover source inventory")
	}
	_, revision, err := Inventory(ctx, store, manifest.Region)
	if err != nil {
		return err
	}
	if manifest.sourceRevision == "" || revision != manifest.sourceRevision {
		return fmt.Errorf("asset index: assets changed during publication")
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return store.Put(ctx, PointerKey(manifest.Region), data, storage.PutOptions{ContentType: "application/json"})
}
