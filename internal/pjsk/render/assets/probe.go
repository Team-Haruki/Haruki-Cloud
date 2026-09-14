package assets

// UsesStore reports whether r reads object keys from a configured store rather
// than probing the local AssetHelper roots.
func (r *AssetReader) UsesStore() bool {
	return r.usesStore()
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
