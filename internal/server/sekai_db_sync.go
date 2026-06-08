package server

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"
)

const (
	defaultSekaiDBSyncInterval = 30 * time.Minute
	defaultSekaiDBSyncTimeout  = 15 * time.Minute
)

type sekaiDBSyncSettings struct {
	SourceDBURL   string
	TargetDBURL   string
	PgDumpPath    string
	PgRestorePath string
	Timeout       time.Duration
}

func startSekaiDBRemoteSync(ctx context.Context, mainLogger *harukiLogger.Logger) {
	cfg := harukiConfig.Cfg.Sekai.RemoteSync
	if !harukiConfig.Cfg.Sekai.Enabled || !cfg.Enabled {
		return
	}
	ctx = ensureContext(ctx)

	settings, err := buildSekaiDBSyncSettings()
	if err != nil {
		mainLogger.Errorf("Sekai DB remote sync config is invalid: %v", err)
		os.Exit(1)
	}

	if cfg.Initial {
		mainLogger.Infof("Sekai DB remote sync initial run started")
		if err := runSekaiDBRemoteSync(ctx, settings); err != nil {
			if cfg.FailStartup {
				mainLogger.Errorf("Sekai DB remote sync initial run failed: %v", err)
				os.Exit(1)
			}
			mainLogger.Warnf("Sekai DB remote sync initial run failed: %v", err)
		} else {
			mainLogger.Infof("Sekai DB remote sync initial run finished")
		}
	}

	interval := cfg.Interval
	if interval == 0 {
		interval = defaultSekaiDBSyncInterval
	}
	if interval < 0 {
		mainLogger.Infof("Sekai DB remote sync background loop disabled")
		return
	}

	mainLogger.Infof("Sekai DB remote sync background loop started (interval=%s)", interval)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := runSekaiDBRemoteSync(ctx, settings); err != nil {
					mainLogger.Warnf("Sekai DB remote sync failed: %v", err)
					continue
				}
				mainLogger.Infof("Sekai DB remote sync finished")
			}
		}
	}()
}

func buildSekaiDBSyncSettings() (sekaiDBSyncSettings, error) {
	sekaiCfg := harukiConfig.Cfg.Sekai
	syncCfg := sekaiCfg.RemoteSync

	sourceType := strings.TrimSpace(syncCfg.SourceDBType)
	if sourceType == "" {
		sourceType = sekaiCfg.DBType
	}
	if sourceType == "" {
		sourceType = "postgres"
	}
	if !isPostgresDBType(sourceType) {
		return sekaiDBSyncSettings{}, fmt.Errorf("source_db_type %q is not supported; only postgres is supported", sourceType)
	}
	if !isPostgresDBType(sekaiCfg.DBType) {
		return sekaiDBSyncSettings{}, fmt.Errorf("target sekai db_type %q is not supported; only postgres is supported", sekaiCfg.DBType)
	}

	sourceURL := strings.TrimSpace(syncCfg.SourceDBURL)
	if sourceURL == "" {
		return sekaiDBSyncSettings{}, fmt.Errorf("source_db_url is required")
	}
	targetURL := strings.TrimSpace(sekaiCfg.DBURL)
	if targetURL == "" {
		return sekaiDBSyncSettings{}, fmt.Errorf("sekai.db_url is required")
	}
	if sourceURL == targetURL {
		return sekaiDBSyncSettings{}, fmt.Errorf("source_db_url and sekai.db_url must point to different databases")
	}

	timeout := syncCfg.Timeout
	if timeout <= 0 {
		timeout = defaultSekaiDBSyncTimeout
	}

	pgDumpPath := strings.TrimSpace(syncCfg.PgDumpPath)
	if pgDumpPath == "" {
		pgDumpPath = "pg_dump"
	}
	pgRestorePath := strings.TrimSpace(syncCfg.PgRestorePath)
	if pgRestorePath == "" {
		pgRestorePath = "pg_restore"
	}

	return sekaiDBSyncSettings{
		SourceDBURL:   sourceURL,
		TargetDBURL:   targetURL,
		PgDumpPath:    pgDumpPath,
		PgRestorePath: pgRestorePath,
		Timeout:       timeout,
	}, nil
}

func runSekaiDBRemoteSync(ctx context.Context, cfg sekaiDBSyncSettings) error {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	dumpFile, err := os.CreateTemp("", "haruki-sekai-db-sync-*.dump")
	if err != nil {
		return fmt.Errorf("create temporary dump file: %w", err)
	}
	dumpPath := dumpFile.Name()
	_ = dumpFile.Close()
	defer func() { _ = os.Remove(dumpPath) }()

	if err := runCommand(runCtx, "pg_dump", cfg.PgDumpPath,
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--file", dumpPath,
		cfg.SourceDBURL,
	); err != nil {
		return err
	}

	if err := runCommand(runCtx, "pg_restore", cfg.PgRestorePath,
		"--clean",
		"--if-exists",
		"--no-owner",
		"--no-privileges",
		"--single-transaction",
		"--exit-on-error",
		"--dbname", cfg.TargetDBURL,
		dumpPath,
	); err != nil {
		return err
	}
	return nil
}

func runCommand(ctx context.Context, label, path string, args ...string) error {
	cmd := exec.CommandContext(ctx, path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w: %s", label, err, truncateCommandOutput(stderr.String()))
	}
	return nil
}

func isPostgresDBType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "postgres", "postgresql", "pg":
		return true
	default:
		return false
	}
}

func truncateCommandOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "<no stderr>"
	}
	const max = 4096
	if len(output) <= max {
		return output
	}
	return output[:max] + "...(truncated)"
}
