package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	"haruki-cloud/internal/core/dbpool"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestOpenEntDriverAppliesPoolLimits(t *testing.T) {
	dsn := fmt.Sprintf("file:ent_pool_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	drv, err := openEntDriver("pool_test", "sqlite3", dsn, dbpool.Config{MaxOpen: 5, MaxIdle: 5, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("openEntDriver: %v", err)
	}
	client := pjskDB.NewClient(pjskDB.Driver(drv))
	defer func() {
		dbpool.Unregister("pool_test", drv.DB())
		_ = client.Close()
	}()
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if got := drv.DB().Stats().MaxOpenConnections; got != 5 {
		t.Fatalf("MaxOpenConnections = %d, want 5", got)
	}
	if _, ok := dbpool.Stats().(map[string]dbpool.PoolStats)["pool_test"]; !ok {
		t.Fatal("pool should be registered for /debug/vars")
	}
}

func TestOpenEntDriverRejectsUnsupportedDriver(t *testing.T) {
	if _, err := openEntDriver("bad", "oracle", "dsn", dbpool.Defaults(1)); err == nil {
		t.Fatal("unsupported driver should be rejected")
	}
}

func TestWireAliasCacheInvalidation(t *testing.T) {
	wireAliasCacheInvalidation(nil, nil)
	wireAliasCacheInvalidation(&renderapp.App{}, nil)

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	wireAliasCacheInvalidation(&renderapp.App{Aliases: &pjskalias.Service{}}, client)
}
