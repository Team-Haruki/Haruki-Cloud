package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"haruki-cloud/internal/core/dbpool"
)

func TestDBPoolDefaults(t *testing.T) {
	var cfg Config
	cases := []struct {
		name string
		got  dbpool.Config
		want int
	}{
		{"sekai", cfg.Sekai.DBPool(), DefaultSekaiPoolMaxOpen},
		{"mysekai", cfg.Sekai.MySekaiDBPool(), DefaultSekaiMySekaiPoolMaxOpen},
		{"provider", cfg.Sekai.ProviderDBPool(), DefaultSekaiProviderPoolMaxOpen},
		{"pjsk", cfg.PJSK.DBPool(), DefaultPJSKPoolMaxOpen},
		{"users", cfg.UsersDB.DBPool(), DefaultUsersPoolMaxOpen},
		{"bot", cfg.HarukiBotDB.DBPool(), DefaultBotPoolMaxOpen},
		{"censor", cfg.Censor.DBPool(), DefaultCensorPoolMaxOpen},
		{"chunithm music", cfg.Chunithm.MusicDBPoolConfig(), DefaultChunithmPoolMaxOpen},
		{"chunithm binding", cfg.Chunithm.BindingDBPoolConfig(), DefaultChunithmPoolMaxOpen},
	}
	total := 0
	for _, tc := range cases {
		if tc.got != dbpool.Defaults(tc.want) {
			t.Fatalf("%s pool = %+v, want %+v", tc.name, tc.got, dbpool.Defaults(tc.want))
		}
		total += tc.got.MaxOpen
	}
	// Keep the documented worst case (five regions, image cache index 8)
	// well under the shared PostgreSQL max_connections of 200.
	worst := total + 4*(DefaultSekaiMySekaiPoolMaxOpen+DefaultSekaiProviderPoolMaxOpen) + 8
	if worst != 84 {
		t.Fatalf("documented pool total changed: %d (update db_pools.go and the example config)", worst)
	}
}

func TestDBPoolFromYAMLAndEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	yaml := `sekai:
  pool:
    max_open: 20
    conn_max_idle_time: 2m
  mysekai_pool:
    max_open: 2
pjsk:
  pool:
    max_open: 6
    max_idle: 3
censor:
  censor_db_pool:
    max_open: 2
chunithm:
  music_db_pool:
    max_open: 1
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARUKI_BOT_DB_MAX_OPEN", "5")
	t.Setenv("HARUKI_USERS_DB_CONN_MAX_LIFETIME", "10m")
	t.Setenv("HARUKI_SEKAI_PROVIDER_DB_MAX_IDLE", "-1")
	cfg, err := ReadConfig(path)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if got := cfg.Sekai.DBPool(); got.MaxOpen != 20 || got.MaxIdle != 20 || got.ConnMaxIdleTime != 2*time.Minute || got.ConnMaxLifetime != dbpool.DefaultConnMaxLifetime {
		t.Fatalf("sekai pool = %+v", got)
	}
	if got := cfg.Sekai.MySekaiDBPool(); got.MaxOpen != 2 {
		t.Fatalf("mysekai pool = %+v", got)
	}
	if got := cfg.Sekai.ProviderDBPool(); got.MaxIdle != -1 || got.MaxOpen != DefaultSekaiProviderPoolMaxOpen {
		t.Fatalf("provider pool = %+v", got)
	}
	if got := cfg.PJSK.DBPool(); got.MaxOpen != 6 || got.MaxIdle != 3 {
		t.Fatalf("pjsk pool = %+v", got)
	}
	if got := cfg.HarukiBotDB.DBPool(); got.MaxOpen != 5 {
		t.Fatalf("bot pool = %+v", got)
	}
	if got := cfg.UsersDB.DBPool(); got.ConnMaxLifetime != 10*time.Minute {
		t.Fatalf("users pool = %+v", got)
	}
	if got := cfg.Censor.DBPool(); got.MaxOpen != 2 {
		t.Fatalf("censor pool = %+v", got)
	}
	if got := cfg.Chunithm.MusicDBPoolConfig(); got.MaxOpen != 1 {
		t.Fatalf("chunithm music pool = %+v", got)
	}
}
