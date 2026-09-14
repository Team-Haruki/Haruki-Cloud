package assets

import "context"

// UsesStore reports whether r reads object keys from a configured store rather
// than probing the local AssetHelper roots.
func (r *AssetReader) UsesStore() bool {
	return r.usesStore()
}

// ProbeExisting is the existence seam of the five C1 forks (addendum B2 step 1;
// deleted with them in T15). With a store-backed reader it answers through
// AssetReader.Stat and returns no local path, because no Cloud-local file
// backs the object. Otherwise it is today's helper.FirstExisting probe on the
// caller's (request-bound) helper, falling back to the reader's own helper when
// helper is nil; localPath is the resolved local path of the first hit.
func ProbeExisting(ctx context.Context, reader *AssetReader, helper *AssetHelper, paths ...string) (localPath string, ok bool) {
	if reader.UsesStore() {
		_, ok = reader.Stat(ctx, paths...)
		return "", ok
	}
	if helper == nil && reader != nil {
		helper = reader.helper.WithContext(readerContext(ctx))
	}
	if helper == nil {
		return "", false
	}
	resolved := helper.FirstExisting(paths...)
	return resolved, resolved != ""
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
