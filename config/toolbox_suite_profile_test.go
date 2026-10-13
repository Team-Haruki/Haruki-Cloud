package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readToolboxConfig(t *testing.T, yaml string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return ReadConfig(path)
}

func TestToolboxSuiteProfileResolution(t *testing.T) {
	empty, cloud, spaced := "", "cloud", "  cloud "
	for _, tc := range []struct {
		name string
		cfg  *ToolboxConfig
		want string
	}{
		{"nil config", nil, ""},
		{"unset", &ToolboxConfig{}, DefaultToolboxSuiteProfile},
		{"explicit empty", &ToolboxConfig{SuiteProfile: &empty}, ""},
		{"explicit cloud", &ToolboxConfig{SuiteProfile: &cloud}, "cloud"},
		{"trimmed", &ToolboxConfig{SuiteProfile: &spaced}, "cloud"},
	} {
		if got := tc.cfg.ResolvedSuiteProfile(); got != tc.want {
			t.Fatalf("%s: ResolvedSuiteProfile() = %q, want %q", tc.name, got, tc.want)
		}
	}
	if DefaultToolboxSuiteProfile != "cloud" {
		t.Fatalf("DefaultToolboxSuiteProfile = %q, want cloud", DefaultToolboxSuiteProfile)
	}
}

func TestReadConfigToolboxSuiteProfileFromYAML(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, want string
	}{
		{"absent defaults to cloud", "profile: dev\ntoolbox:\n  base_url: http://toolbox\n", "cloud"},
		{"null defaults to cloud", "profile: dev\ntoolbox:\n  suite_profile:\n", "cloud"},
		{"empty string is the whole document", "profile: dev\ntoolbox:\n  suite_profile: \"\"\n", ""},
		{"explicit cloud", "profile: dev\ntoolbox:\n  suite_profile: cloud\n", "cloud"},
	} {
		cfg, err := readToolboxConfig(t, tc.yaml)
		if err != nil {
			t.Fatalf("%s: ReadConfig() error = %v", tc.name, err)
		}
		if got := cfg.Toolbox.ResolvedSuiteProfile(); got != tc.want {
			t.Fatalf("%s: suite profile = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestReadConfigToolboxSuiteProfileFromEnv(t *testing.T) {
	t.Run("explicit empty overrides YAML", func(t *testing.T) {
		t.Setenv("HARUKI_TOOLBOX_SUITE_PROFILE", "")
		cfg, err := readToolboxConfig(t, "profile: dev\ntoolbox:\n  suite_profile: cloud\n")
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if got := cfg.Toolbox.ResolvedSuiteProfile(); got != "" {
			t.Fatalf("suite profile = %q, want whole document", got)
		}
	})
	t.Run("value overrides YAML", func(t *testing.T) {
		t.Setenv("HARUKI_TOOLBOX_SUITE_PROFILE", "cloud")
		cfg, err := readToolboxConfig(t, "profile: dev\ntoolbox:\n  suite_profile: \"\"\n")
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if got := cfg.Toolbox.ResolvedSuiteProfile(); got != "cloud" {
			t.Fatalf("suite profile = %q, want cloud", got)
		}
	})
	t.Run("unset keeps YAML", func(t *testing.T) {
		cfg, err := readToolboxConfig(t, "profile: dev\ntoolbox:\n  suite_profile: \"\"\n")
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if got := cfg.Toolbox.ResolvedSuiteProfile(); got != "" {
			t.Fatalf("suite profile = %q, want whole document", got)
		}
	})
}

func TestReadConfigRejectsUnknownToolboxSuiteProfile(t *testing.T) {
	_, err := readToolboxConfig(t, "profile: dev\ntoolbox:\n  suite_profile: nope\n")
	if err == nil || !strings.Contains(err.Error(), "toolbox.suite_profile") {
		t.Fatalf("ReadConfig() error = %v, want an unsupported suite_profile error", err)
	}
	t.Setenv("HARUKI_TOOLBOX_SUITE_PROFILE", "nope")
	if _, err := readToolboxConfig(t, "profile: dev\n"); err == nil {
		t.Fatal("ReadConfig() accepted an unknown suite profile from the environment")
	}
}
