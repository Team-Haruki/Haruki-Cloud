package provider

import "context"

// LoadMasterRows serves a whole master table from a provider's generic row
// store (database first, its local store second). ok is false when the
// provider has no row store or none of its sources could answer, which is
// how a region that has not adopted a table (or a database that does not
// have it yet) reads.
func LoadMasterRows(ctx context.Context, p MasterDataProvider, filename string) (map[int]map[string]any, bool) {
	if p == nil {
		return nil, false
	}
	store := p.MySekai()
	if store == nil {
		return nil, false
	}
	if rowSource, ok := store.(MasterRowSource); ok {
		return rowSource.LoadMasterRows(ctx, filename)
	}
	rows := store.LoadMapByID(filename)
	return rows, rows != nil
}
