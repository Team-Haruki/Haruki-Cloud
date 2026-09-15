package s3

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/storage"
)

// Open builds an S3 store from a resolved provider block. It is the
// storage.Opener the composition root injects into storage.BuildSet; the
// typed options (request_timeout, dial_timeout, stat_timeout,
// failover_cooldown, max_attempts, max_object_bytes, proxy) are parsed here.
func Open(resolved storage.Resolved) (storage.Store, error) {
	cfg, err := ConfigFromResolved(resolved)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

// ConfigFromResolved translates a resolved provider into Config.
func ConfigFromResolved(resolved storage.Resolved) (Config, error) {
	options := resolved.Options
	cfg := Config{
		Endpoints: resolved.Endpoints,
		Bucket:    resolved.Bucket,
		Root:      resolved.Root,
		Region:    resolved.Region,
		AccessKey: options[storage.OptionAccessKeyID],
		SecretKey: options[storage.OptionSecretAccessKey],
		PathStyle: resolved.PathStyle,
		Proxy:     strings.TrimSpace(options[storage.OptionProxy]),
	}
	if resolved.PublicRead {
		cfg.DefaultACL = "public-read"
	}
	durations := []struct {
		key string
		dst *time.Duration
	}{
		{storage.OptionRequestTimeout, &cfg.RequestTimeout},
		{storage.OptionDialTimeout, &cfg.DialTimeout},
		{storage.OptionStatTimeout, &cfg.StatTimeout},
		{storage.OptionFailoverCooldown, &cfg.FailoverCooldown},
	}
	for _, item := range durations {
		if err := parseDurationOption(options, item.key, item.dst); err != nil {
			return Config{}, err
		}
	}
	if raw := strings.TrimSpace(options[storage.OptionMaxAttempts]); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("option %s: %q is not a positive integer", storage.OptionMaxAttempts, raw)
		}
		cfg.MaxAttempts = value
	}
	if raw := strings.TrimSpace(options[storage.OptionMaxObjectBytes]); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("option %s: %q is not a positive integer", storage.OptionMaxObjectBytes, raw)
		}
		cfg.MaxObjectBytes = value
	}
	return cfg, nil
}

func parseDurationOption(options map[string]string, key string, dst *time.Duration) error {
	raw := strings.TrimSpace(options[key])
	if raw == "" {
		return nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fmt.Errorf("option %s: %q is not a positive duration", key, raw)
	}
	*dst = value
	return nil
}
