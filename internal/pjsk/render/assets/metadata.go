package assets

import "haruki-cloud/internal/storage"

// MetadataIndex provides complete published resource listings. An incomplete
// region returns authoritative=false so normal storage probing still works.
// Implementations must support concurrent Lookup calls and atomic replacement.
type MetadataIndex interface {
	Lookup(key storage.Key) (resolved storage.Key, found bool, authoritative bool)
}

// WithMetadataIndex attaches an index at application construction time. Context
// clones share the index, whose contents may be replaced atomically at runtime.
func (h *AssetHelper) WithMetadataIndex(index MetadataIndex) *AssetHelper {
	if h == nil {
		return nil
	}
	h.metadata = index
	if h.store != nil {
		h.store.metadata = index
	}
	return h
}
