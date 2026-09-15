package assets

import "context"

// UsesStore reports whether r reads object keys from a configured store rather
// than probing the local AssetHelper roots.
func (r *AssetReader) UsesStore() bool {
	return r.usesStore()
}

// ProbeExisting is the existence seam kept after T15 for the honor overlays
// that are not C1 candidate forks (scroll, birthday level icon, and the event
// rank-overlay suppression; pending contract item C1-overlays). Delete it once Drawing
// tolerates those as missing. While the caller's (request-bound) helper - or the
// reader's own helper when helper is nil - still has real local roots, it runs
// today's helper.FirstExisting probe first and a hit returns the resolved
// local path exactly as before. Only when the local probe misses (or no local
// root is left, after E1) does a store-backed reader answer through
// AssetReader.Stat; a store hit has no Cloud-local file, so localPath is "".
func ProbeExisting(ctx context.Context, reader *AssetReader, helper *AssetHelper, paths ...string) (localPath string, ok bool) {
	if helper == nil && reader != nil && reader.helper != nil {
		helper = reader.helper.WithContext(readerContext(ctx))
	}
	storeBacked := reader.UsesStore()
	if helper != nil && (!storeBacked || helperHasLocalRoots(helper)) {
		if resolved := helper.FirstExisting(paths...); resolved != "" {
			return resolved, true
		}
	}
	if !storeBacked {
		return "", false
	}
	_, ok = reader.Stat(ctx, paths...)
	return "", ok
}

// ReaderOr returns reader, or a legacy (store-less) reader over helper when
// reader is nil, so components built without the composition root keep
// today's local probing.
func ReaderOr(reader *AssetReader, helper *AssetHelper) *AssetReader {
	if reader != nil {
		return reader
	}
	return NewAssetReader(helper, nil)
}
