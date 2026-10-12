package config

import "testing"

func TestDatabaseAutoMigrateDefaultsOnAndFollowsEnv(t *testing.T) {
	var cfg Config
	if !cfg.Database.AutoMigrateEnabled() {
		t.Fatal("unset database.auto_migrate must be enabled")
	}

	t.Setenv("HARUKI_DB_AUTO_MIGRATE", "false")
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Database.AutoMigrateEnabled() {
		t.Fatal("HARUKI_DB_AUTO_MIGRATE=false did not disable auto-migrate")
	}

	t.Setenv("HARUKI_DB_AUTO_MIGRATE", "true")
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Database.AutoMigrateEnabled() {
		t.Fatal("HARUKI_DB_AUTO_MIGRATE=true did not enable auto-migrate")
	}

	t.Setenv("HARUKI_DB_AUTO_MIGRATE", "not-a-bool")
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Database.AutoMigrateEnabled() {
		t.Fatal("an invalid value must leave the previous setting")
	}
}
