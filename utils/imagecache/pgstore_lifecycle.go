package imagecache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

// ContentLockSQL is shared by Cloud and Drawing. The transaction spans lookup,
// object I/O and index writes; lock failure must not cause an unlocked upload.
const ContentLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *PGStore) executor() sqlExecutor {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// WithContentLock serializes ownership across participating writers and GC.
// Init enables it on widened schemas. Legacy stores cannot run garage GC.
// Callers bound the context for the entire operation, including object I/O.
func (s *PGStore) WithContentLock(ctx context.Context, hash string, work func(context.Context, *PGStore) error) error {
	if s == nil || !s.lifecycle.Load() {
		return work(ctx, s)
	}
	if s.tx != nil {
		return errors.New("imagecache: nested content lock")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	finish := commandtrace.MeasureOperation(ctx, "image.content_lock")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		finish()
		return fmt.Errorf("imagecache: begin content lock: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, ContentLockSQL, hash); err != nil {
		finish()
		return fmt.Errorf("imagecache: content lock: %w", err)
	}
	finish()
	scoped := &PGStore{db: s.db, tx: tx}
	scoped.widened.Store(s.Widened())
	scoped.metadata.Store(s.metadata.Load())
	if err = work(ctx, scoped); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("imagecache: commit content lock: %w", err)
	}
	return nil
}

// InspectSchema initializes an operator's read-only client without running DDL.
// Repair and GC require the deployed writer/outbox schema, not just old tables.
func (s *PGStore) InspectSchema(ctx context.Context) error {
	if s == nil {
		return errors.New("imagecache: index unavailable")
	}
	if err := s.probeSchema(ctx); err != nil {
		return err
	}
	return s.probeLifecycleSchema(ctx)
}

func (s *PGStore) probeLifecycleSchema(ctx context.Context) error {
	if !s.Widened() {
		s.metadata.Store(false)
		s.lifecycle.Store(false)
		return nil
	}
	var ready bool
	if err := s.executor().QueryRowContext(ctx, inspectLifecycleSQL).Scan(&ready); err != nil {
		return err
	}
	s.metadata.Store(ready)
	s.lifecycle.Store(ready)
	return nil
}

const inspectLifecycleSQL = `SELECT
 (SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'image_cache_entries' AND column_name IN ('writer_node', 'written_at')) = 2
 AND EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'image_cache_object_deletions')`
