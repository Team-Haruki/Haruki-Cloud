package imagecache

// renderIndexDDL is the single canonical render index DDL for the whole
// programme (contract addendum A5). Cloud is the only side that executes it;
// Drawing carries a verbatim copy for its preflight check, so any change here
// is a cross-repo change.
//
// Every statement is idempotent. Init runs them in order, after initSQL and
// migrateSQL, only when PGStoreOptions.RenderIndexDDL is set. The four
// ADD COLUMN statements and the ALTER COLUMN take a brief ACCESS EXCLUSIVE
// lock on image_cache_entries.
//
// Fixed points: storage_backend is nullable with no default (writers must set
// it; the UPDATE is a one-shot backfill), and the foreign key has no
// ON DELETE CASCADE, so deleting a still-referenced entry fails loudly.
var renderIndexDDL = []string{
	`ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS storage_backend TEXT`,
	`ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS media_type TEXT`,
	`ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL`,
	`ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS last_referenced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
	`ALTER TABLE image_cache_entries ALTER COLUMN file_path DROP NOT NULL`,
	`UPDATE image_cache_entries SET storage_backend = 'legacy_disk' WHERE storage_backend IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_ice_group_created ON image_cache_entries (group_name, created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_ice_expires ON image_cache_entries (expires_at) WHERE expires_at IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_ice_last_referenced ON image_cache_entries (last_referenced_at)`,
	`CREATE TABLE IF NOT EXISTS render_cache_index (
    request_key  TEXT PRIMARY KEY,
    content_hash TEXT NOT NULL REFERENCES image_cache_entries(hash),
    api_path     TEXT NOT NULL,
    user_id      TEXT NOT NULL DEFAULT 'public',
    group_name   TEXT NOT NULL DEFAULT 'pjsk',
    key_version  INT NOT NULL DEFAULT 3,
    ttl_seconds  BIGINT NOT NULL DEFAULT 0,
    expires_at   TIMESTAMPTZ NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,
	`CREATE INDEX IF NOT EXISTS idx_rci_expires ON render_cache_index (expires_at) WHERE expires_at IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_rci_api_path_user ON render_cache_index (api_path, user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_rci_content_hash ON render_cache_index (content_hash)`,
}

// RenderIndexDDL returns a copy of the canonical render index DDL, one
// statement per element, in execution order.
func RenderIndexDDL() []string {
	return append([]string(nil), renderIndexDDL...)
}
