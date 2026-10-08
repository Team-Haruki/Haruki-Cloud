package dbpool

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
	"time"
)

// stubDriver lets database/sql open pools without a real database.
type stubDriver struct{}

func (stubDriver) Open(string) (driver.Conn, error) { return nil, errors.New("stub: no connections") }

var registerStub sync.Once

func openStub(t *testing.T) *sql.DB {
	t.Helper()
	registerStub.Do(func() { sql.Register("dbpool-stub", stubDriver{}) })
	db, err := sql.Open("dbpool-stub", "")
	if err != nil {
		t.Fatalf("open stub: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestWithDefaultsFillsZeroFieldsOnly(t *testing.T) {
	got := Config{MaxOpen: 4}.WithDefaults(Defaults(16))
	want := Config{MaxOpen: 4, MaxIdle: 4, ConnMaxLifetime: DefaultConnMaxLifetime, ConnMaxIdleTime: DefaultConnMaxIdleTime}
	if got != want {
		t.Fatalf("WithDefaults = %+v, want %+v", got, want)
	}

	got = Config{}.WithDefaults(Defaults(10))
	if got.MaxOpen != 10 || got.MaxIdle != 10 {
		t.Fatalf("zero config should take defaults, got %+v", got)
	}

	got = Config{MaxOpen: 3, MaxIdle: 8, ConnMaxLifetime: time.Minute, ConnMaxIdleTime: -1}.WithDefaults(Defaults(16))
	if got.MaxIdle != 3 {
		t.Fatalf("MaxIdle must be clamped to MaxOpen, got %d", got.MaxIdle)
	}
	if got.ConnMaxLifetime != time.Minute || got.ConnMaxIdleTime != -1 {
		t.Fatalf("explicit durations must be kept, got %+v", got)
	}

	got = Config{MaxIdle: -1}.WithDefaults(Defaults(6))
	if got.MaxIdle != -1 || got.MaxOpen != 6 {
		t.Fatalf("negative MaxIdle must be kept, got %+v", got)
	}
}

func TestApplySetsPoolLimits(t *testing.T) {
	db := openStub(t)
	Apply(db, Defaults(12))
	if got := db.Stats().MaxOpenConnections; got != 12 {
		t.Fatalf("MaxOpenConnections = %d, want 12", got)
	}

	Apply(db, Config{MaxOpen: 0, MaxIdle: -1})
	if got := db.Stats().MaxOpenConnections; got != 0 {
		t.Fatalf("MaxOpen 0 must leave the pool unbounded, got %d", got)
	}
	Apply(nil, Defaults(1))
}

func TestOpenRegistersStats(t *testing.T) {
	registerStub.Do(func() { sql.Register("dbpool-stub", stubDriver{}) })
	db, err := Open("test_pool", "dbpool-stub", "", Defaults(7))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	second := openStub(t)
	Apply(second, Defaults(3))
	Register("test_pool", second)
	Register("", second)
	Register("ignored", nil)

	stats, ok := Stats().(map[string]PoolStats)
	if !ok {
		t.Fatalf("Stats returned %T", Stats())
	}
	pool, ok := stats["test_pool"]
	if !ok {
		t.Fatalf("test_pool missing from %v", stats)
	}
	if pool.Pools != 2 || pool.MaxOpen != 10 {
		t.Fatalf("summed stats = %+v, want 2 pools with max_open 10", pool)
	}

	Unregister("test_pool", second)
	if got := Stats().(map[string]PoolStats)["test_pool"]; got.Pools != 1 || got.MaxOpen != 7 {
		t.Fatalf("after Unregister = %+v", got)
	}
	Unregister("test_pool", db)
	if _, ok := Stats().(map[string]PoolStats)["test_pool"]; ok {
		t.Fatal("empty pool name should be removed")
	}

	if _, err := Open("bad", "no-such-driver", "", Defaults(1)); err == nil {
		t.Fatal("unknown driver should fail")
	}
}
