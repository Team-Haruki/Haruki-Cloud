package storage

import (
	"context"
	"errors"

	"haruki-cloud/utils/logger"
)

// DualMode selects how a DualStore serves reads.
type DualMode string

const (
	// DualWrite writes to both stores and reads the primary, falling back to
	// the mirror when the primary does not have the object.
	DualWrite DualMode = "write"
	// DualWriteOnly writes to both stores and reads the primary only.
	DualWriteOnly DualMode = "write_only"
)

type dualStore struct {
	primary Store
	mirror  Store
	mode    DualMode
	log     *logger.Logger
}

// NewDual returns a Store that mirrors writes from primary to mirror. It
// exists only for the user_upload slot's local-to-Garage migration. The
// primary is the source of truth: its errors are returned, mirror write
// failures are logged at Warn and swallowed. An unknown mode behaves as
// DualWriteOnly; a nil store is treated as Disabled.
func NewDual(primary, mirror Store, mode DualMode, log *logger.Logger) Store {
	if primary == nil {
		primary = Disabled()
	}
	if mirror == nil {
		mirror = Disabled()
	}
	if mode != DualWrite {
		mode = DualWriteOnly
	}
	return &dualStore{primary: primary, mirror: mirror, mode: mode, log: log}
}

func (d *dualStore) Get(ctx context.Context, key Key) ([]byte, error) {
	data, err := d.primary.Get(ctx, key)
	if d.readThrough(err) {
		return d.mirror.Get(ctx, key)
	}
	return data, err
}

func (d *dualStore) Put(ctx context.Context, key Key, data []byte, opts PutOptions) error {
	if err := d.primary.Put(ctx, key, data, opts); err != nil {
		return err
	}
	if err := d.mirror.Put(ctx, key, data, opts); err != nil {
		d.log.WarnContext(ctx, "storage mirror put failed", "key", string(key), "mode", string(d.mode), "error", err)
	}
	return nil
}

func (d *dualStore) Stat(ctx context.Context, key Key) (Object, error) {
	object, err := d.primary.Stat(ctx, key)
	if d.readThrough(err) {
		return d.mirror.Stat(ctx, key)
	}
	return object, err
}

func (d *dualStore) Delete(ctx context.Context, key Key) error {
	primaryErr := d.primary.Delete(ctx, key)
	mirrorErr := d.mirror.Delete(ctx, key)
	if primaryErr != nil && !errors.Is(primaryErr, ErrNotExist) {
		return primaryErr
	}
	if mirrorErr != nil && !errors.Is(mirrorErr, ErrNotExist) {
		return mirrorErr
	}
	return nil
}

func (d *dualStore) List(ctx context.Context, prefix Key, fn func(Object) error) error {
	return d.primary.List(ctx, prefix, fn)
}

func (d *dualStore) ListDir(ctx context.Context, prefix Key, fn func(DirEntry) error) error {
	return d.primary.ListDir(ctx, prefix, fn)
}

func (d *dualStore) readThrough(err error) bool {
	return d.mode == DualWrite && errors.Is(err, ErrNotExist)
}
