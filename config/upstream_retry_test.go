package config

import (
	"slices"
	"testing"
	"time"

	"haruki-cloud/internal/core/upstream"

	"gopkg.in/yaml.v3"
)

func TestUpstreamRetryEnvOverrides(t *testing.T) {
	t.Setenv("HARUKI_SEKAI_API_RETRY_MAX_RETRIES", "2")
	t.Setenv("HARUKI_TOOLBOX_RETRY_BUDGET", "1s")
	t.Setenv("HARUKI_TRACKER_RETRY_MAX_RETRIES", "-1")
	t.Setenv("HARUKI_TRACKER_RETRY_WAIT", "50ms")
	t.Setenv("HARUKI_TRACKER_RETRY_MAX_WAIT", "500ms")
	var cfg Config
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SekaiAPI.Retry.MaxRetries != 2 || cfg.Toolbox.Retry.Budget != time.Second {
		t.Fatalf("sekai %+v toolbox %+v", cfg.SekaiAPI.Retry, cfg.Toolbox.Retry)
	}
	tracker := cfg.Tracker.Retry.WithDefaults()
	if tracker.MaxRetries != -1 || tracker.Wait != 50*time.Millisecond || tracker.MaxWait != 500*time.Millisecond || tracker.Budget != 0 {
		t.Fatalf("tracker %+v", tracker)
	}
}

func TestUpstreamRetryDefaultsMatchPre38Behaviour(t *testing.T) {
	got := UpstreamRetryConfig{}.WithDefaults()
	want := UpstreamRetryConfig{MaxRetries: 4, Wait: time.Second, MaxWait: 2 * time.Second}
	if got != want {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}
	if got := (UpstreamRetryConfig{Wait: 3 * time.Second}).WithDefaults(); got.MaxWait != 3*time.Second {
		t.Fatalf("max_wait must not be below wait: %+v", got)
	}
}

func TestSekaiAPIBackupTargetWiring(t *testing.T) {
	t.Setenv("HARUKI_SEKAI_API_BASE_URL", "https://primary.example.com/")
	t.Setenv("HARUKI_SEKAI_API_BACKUP_BASE_URL", " https://standby.example.com ")
	var cfg Config
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	got := cfg.SekaiAPI.EffectiveTargets()
	want := []upstream.TargetConfig{
		{Name: "sekai-api-1", BaseURL: "https://primary.example.com"},
		{Name: SekaiAPIBackupTargetName, BaseURL: "https://standby.example.com", Backup: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("env targets = %+v, want %+v", got, want)
	}

	cases := []struct {
		name string
		cfg  SekaiAPIConfig
		want []upstream.TargetConfig
	}{
		{
			name: "base_url only",
			cfg:  SekaiAPIConfig{BaseURL: "https://primary.example.com"},
			want: []upstream.TargetConfig{{Name: "sekai-api-1", BaseURL: "https://primary.example.com"}},
		},
		{
			name: "yaml targets win over backup_base_url",
			cfg: SekaiAPIConfig{
				BaseURL:       "https://primary.example.com",
				BackupBaseURL: "https://standby.example.com",
				Targets:       []upstream.TargetConfig{{Name: "a", BaseURL: "https://a.example.com"}, {Name: "b", BaseURL: "https://b.example.com", Backup: true}},
			},
			want: []upstream.TargetConfig{{Name: "a", BaseURL: "https://a.example.com"}, {Name: "b", BaseURL: "https://b.example.com", Backup: true}},
		},
		{
			name: "blank targets fall back to base_url plus backup",
			cfg: SekaiAPIConfig{
				BaseURL:       "https://primary.example.com",
				BackupBaseURL: "https://standby.example.com",
				Targets:       []upstream.TargetConfig{{BaseURL: " "}},
			},
			want: []upstream.TargetConfig{
				{Name: "sekai-api-1", BaseURL: "https://primary.example.com"},
				{Name: SekaiAPIBackupTargetName, BaseURL: "https://standby.example.com", Backup: true},
			},
		},
		{name: "nothing configured", cfg: SekaiAPIConfig{}, want: nil},
		{
			name: "duplicate names are made unique",
			cfg: SekaiAPIConfig{Targets: []upstream.TargetConfig{
				{Name: "entry", BaseURL: "https://p.example.com"},
				{Name: "entry", BaseURL: "https://s.example.com", Backup: true},
			}},
			want: []upstream.TargetConfig{
				{Name: "entry", BaseURL: "https://p.example.com"},
				{Name: "entry#2", BaseURL: "https://s.example.com", Backup: true},
			},
		},
		{
			name: "renaming skips names in use",
			cfg: SekaiAPIConfig{Targets: []upstream.TargetConfig{
				{Name: "a", BaseURL: "https://1.example.com"},
				{Name: "a", BaseURL: "https://2.example.com"},
				{Name: "a#2", BaseURL: "https://3.example.com"},
				{BaseURL: "https://4.example.com"},
				{Name: "sekai-api-4", BaseURL: "https://5.example.com"},
			}},
			want: []upstream.TargetConfig{
				{Name: "a", BaseURL: "https://1.example.com"},
				{Name: "a#3", BaseURL: "https://2.example.com"},
				{Name: "a#2", BaseURL: "https://3.example.com"},
				{Name: "sekai-api-4", BaseURL: "https://4.example.com"},
				{Name: "sekai-api-4#2", BaseURL: "https://5.example.com"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EffectiveTargets(); !slices.Equal(got, tc.want) {
				t.Fatalf("targets = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSekaiAPIBackupYAML(t *testing.T) {
	var cfg Config
	raw := []byte("sekai_api:\n  targets:\n    - name: primary\n      base_url: https://p.example.com\n    - name: standby\n      base_url: https://s.example.com\n      backup: true\n  backup_base_url: https://x.example.com\n")
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SekaiAPI.BackupBaseURL != "https://x.example.com" || len(cfg.SekaiAPI.Targets) != 2 || cfg.SekaiAPI.Targets[0].Backup || !cfg.SekaiAPI.Targets[1].Backup {
		t.Fatalf("parsed sekai_api = %+v", cfg.SekaiAPI)
	}
}
