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
	legacyBackfillSQL,
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
	`CREATE TABLE IF NOT EXISTS image_cache_object_deletions (
    content_hash TEXT NOT NULL,
    cdn_path TEXT NOT NULL,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (content_hash, cdn_path)
)`,
	`CREATE INDEX IF NOT EXISTS idx_icod_next_attempt ON image_cache_object_deletions (next_attempt_at)`,
	`ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS writer_node TEXT NULL`,
	`ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS written_at TIMESTAMPTZ NULL`,
}

// legacyBackfillSQL is the one-shot storage_backend backfill of the canonical
// DDL. Init runs it through backfillLegacyBackend: the statement is a full
// scan of image_cache_entries that updates nothing once the backfill has
// happened, so it is gated on legacyBackfillPendingSQL, which the partial
// index below answers from an (empty) index instead of the heap. The index
// is Cloud-side only and not part of the cross-repo contract text.
//
// Outside a transaction the index is built CONCURRENTLY so Drawing's writes
// are not blocked during the one-off build; an interrupted concurrent build
// leaves an invalid index that IF NOT EXISTS would keep forever, so Init
// checks pg_index and rebuilds it once. Inside a transaction (lifecycle
// callers) CONCURRENTLY is not allowed and the plain form runs.
const (
	legacyBackfillSQL                  = `UPDATE image_cache_entries SET storage_backend = 'legacy_disk' WHERE storage_backend IS NULL`
	legacyBackfillIndexSQL             = `CREATE INDEX IF NOT EXISTS idx_ice_storage_backend_pending ON image_cache_entries (hash) WHERE storage_backend IS NULL`
	legacyBackfillIndexConcurrentlySQL = `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ice_storage_backend_pending ON image_cache_entries (hash) WHERE storage_backend IS NULL`
	legacyBackfillIndexInvalidSQL      = `SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname = 'idx_ice_storage_backend_pending' AND NOT i.indisvalid`
	legacyBackfillIndexDropSQL         = `DROP INDEX CONCURRENTLY IF EXISTS idx_ice_storage_backend_pending`
	legacyBackfillPendingSQL           = `SELECT 1 FROM image_cache_entries WHERE storage_backend IS NULL LIMIT 1`
)

// RenderIndexDDL returns a copy of the canonical render index DDL, one
// statement per element, in execution order.
func RenderIndexDDL() []string {
	return append([]string(nil), renderIndexDDL...)
}
