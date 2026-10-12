package provider

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

const masterdataIngestVersionTimeout = 5 * time.Second

// master_ingest_version is written by the ingest service: one row per
// (target, region) that records the registry contentHash whose files it
// last committed, with status "ok" once the whole version is in. Rows in
// any other state keep the previous hash, so they read as unknown.
const (
	masterdataIngestVersionPostgresQuery = `SELECT content_hash FROM master_ingest_version WHERE region = $1 AND status = 'ok' AND content_hash IS NOT NULL ORDER BY finished_at DESC NULLS LAST LIMIT 1`
	masterdataIngestVersionGenericQuery  = `SELECT content_hash FROM master_ingest_version WHERE region = ? AND status = 'ok' AND content_hash IS NOT NULL ORDER BY finished_at DESC LIMIT 1`
)

// MasterdataIngestHash reports the master content hash the ingest last
// committed for this region, or ok=false when it cannot be known: no raw
// pool is configured, the table is missing, or the region's ingest is not
// in the "ok" state. Callers treat an unknown hash as "may have changed".
func (p *DatabaseProvider) MasterdataIngestHash(ctx context.Context) (string, bool) {
	if p == nil || p.mysekai == nil || p.mysekai.db == nil {
		return "", false
	}
	return masterdataIngestHash(ctx, p.mysekai.db, p.mysekai.dbType, p.region.String())
}

func masterdataIngestHash(ctx context.Context, db *sql.DB, dbType, region string) (string, bool) {
	if db == nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, masterdataIngestVersionTimeout)
	defer cancel()
	query := masterdataIngestVersionGenericQuery
	if strings.EqualFold(strings.TrimSpace(dbType), "postgres") {
		query = masterdataIngestVersionPostgresQuery
	}
	var hash sql.NullString
	if err := db.QueryRowContext(ctx, query, region).Scan(&hash); err != nil {
		return "", false
	}
	value := strings.TrimSpace(hash.String)
	if !hash.Valid || value == "" {
		return "", false
	}
	return value, true
}
