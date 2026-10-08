// Package dbpool bounds and tunes database/sql connection pools.
//
// database/sql defaults keep only two idle connections and never recycle
// them, so a burst opens (and then closes) a fresh backend per extra
// concurrent query. Every Cloud pool is opened through Open so its limits are
// explicit, configurable and visible in /debug/vars.
package dbpool

import (
	"database/sql"
	"expvar"
	"sort"
	"sync"
	"time"
)

// Config is one pool's limits (YAML block "pool"). A zero field takes the
// default the caller passes to WithDefaults; a negative MaxIdle disables idle
// connections and a negative duration disables that limit.
type Config struct {
	MaxOpen         int           `yaml:"max_open"`
	MaxIdle         int           `yaml:"max_idle"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time"`
}

// Shared defaults: connections are recycled every 30 minutes and an idle
// connection is closed after 5 minutes, so a quiet pool shrinks back without
// dropping below what a burst just needed.
const (
	DefaultConnMaxLifetime = 30 * time.Minute
	DefaultConnMaxIdleTime = 5 * time.Minute
)

// Defaults returns the recommended limits for a pool of maxOpen connections:
// MaxIdle equal to MaxOpen (idle churn is what costs a fresh backend per
// burst) and the shared lifetimes.
func Defaults(maxOpen int) Config {
	return Config{
		MaxOpen:         maxOpen,
		MaxIdle:         maxOpen,
		ConnMaxLifetime: DefaultConnMaxLifetime,
		ConnMaxIdleTime: DefaultConnMaxIdleTime,
	}
}

// WithDefaults fills the zero fields of c from def, except that an unset
// MaxIdle follows MaxOpen. MaxIdle is clamped to MaxOpen when MaxOpen is
// bounded.
func (c Config) WithDefaults(def Config) Config {
	if c.MaxOpen == 0 {
		c.MaxOpen = def.MaxOpen
	}
	if c.MaxIdle == 0 {
		// An unset MaxIdle follows MaxOpen, so raising only max_open keeps
		// the pool free of idle churn.
		c.MaxIdle = c.MaxOpen
		if c.MaxIdle <= 0 {
			c.MaxIdle = def.MaxIdle
		}
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = def.ConnMaxLifetime
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = def.ConnMaxIdleTime
	}
	if c.MaxOpen > 0 && c.MaxIdle > c.MaxOpen {
		c.MaxIdle = c.MaxOpen
	}
	return c
}

// Apply sets c on db. Non-positive MaxOpen leaves the pool unbounded; a
// negative MaxIdle keeps no idle connection; non-positive durations leave
// that limit off.
func Apply(db *sql.DB, c Config) {
	if db == nil {
		return
	}
	if c.MaxOpen > 0 {
		db.SetMaxOpenConns(c.MaxOpen)
	} else {
		db.SetMaxOpenConns(0)
	}
	switch {
	case c.MaxIdle < 0:
		db.SetMaxIdleConns(-1)
	case c.MaxIdle > 0:
		db.SetMaxIdleConns(c.MaxIdle)
	}
	if c.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(c.ConnMaxLifetime)
	}
	if c.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(c.ConnMaxIdleTime)
	}
}

// Open opens driverName/dsn, applies c and registers the pool under name so
// its sql.DBStats appear in expvar "db_pools". database/sql opens lazily, so
// Open does not dial.
func Open(name, driverName, dsn string, c Config) (*sql.DB, error) {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	Apply(db, c)
	Register(name, db)
	return db, nil
}

var registry = struct {
	sync.Mutex
	pools map[string][]*sql.DB
}{pools: make(map[string][]*sql.DB)}

func init() {
	expvar.Publish("db_pools", expvar.Func(Stats))
}

// Register exposes db's statistics under name. Several pools may share a
// name (one per region); their stats are summed.
func Register(name string, db *sql.DB) {
	if db == nil || name == "" {
		return
	}
	registry.Lock()
	registry.pools[name] = append(registry.pools[name], db)
	registry.Unlock()
}

// Unregister removes db, for pools closed before process exit.
func Unregister(name string, db *sql.DB) {
	registry.Lock()
	defer registry.Unlock()
	pools := registry.pools[name]
	for i, candidate := range pools {
		if candidate == db {
			registry.pools[name] = append(pools[:i:i], pools[i+1:]...)
			break
		}
	}
	if len(registry.pools[name]) == 0 {
		delete(registry.pools, name)
	}
}

// PoolStats is the expvar view of one named pool: MaxIdleClosed and
// MaxIdleTimeClosed count connections closed by the idle limits (the churn
// these settings exist to reduce), WaitCount/WaitDurationMS how often a
// query queued behind MaxOpen.
type PoolStats struct {
	Pools             int     `json:"pools"`
	MaxOpen           int     `json:"max_open"`
	Open              int     `json:"open"`
	InUse             int     `json:"in_use"`
	Idle              int     `json:"idle"`
	WaitCount         int64   `json:"wait_count"`
	WaitDurationMS    float64 `json:"wait_duration_ms"`
	MaxIdleClosed     int64   `json:"max_idle_closed"`
	MaxIdleTimeClosed int64   `json:"max_idle_time_closed"`
	MaxLifetimeClosed int64   `json:"max_lifetime_closed"`
}

// Stats returns the summed sql.DBStats of every registered pool by name.
func Stats() any {
	registry.Lock()
	names := make([]string, 0, len(registry.pools))
	for name := range registry.pools {
		names = append(names, name)
	}
	sort.Strings(names)
	snapshot := make(map[string][]*sql.DB, len(names))
	for _, name := range names {
		snapshot[name] = append([]*sql.DB(nil), registry.pools[name]...)
	}
	registry.Unlock()

	out := make(map[string]PoolStats, len(snapshot))
	for name, pools := range snapshot {
		var total PoolStats
		for _, db := range pools {
			s := db.Stats()
			total.Pools++
			total.MaxOpen += s.MaxOpenConnections
			total.Open += s.OpenConnections
			total.InUse += s.InUse
			total.Idle += s.Idle
			total.WaitCount += s.WaitCount
			total.WaitDurationMS += float64(s.WaitDuration) / float64(time.Millisecond)
			total.MaxIdleClosed += s.MaxIdleClosed
			total.MaxIdleTimeClosed += s.MaxIdleTimeClosed
			total.MaxLifetimeClosed += s.MaxLifetimeClosed
		}
		out[name] = total
	}
	return out
}
