package drawingcache

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

const (
	DefaultDBFilename = "cache.db"
	DefaultGCInterval = time.Hour
)

type Config struct {
	StorageDir string
	DBPath     string
	GCInterval time.Duration
}

type Service struct {
	db  *sql.DB
	api *API
	cfg Config
}

func NewService(ctx context.Context, cfg Config) (*Service, error) {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cfg.StorageDir) == "" {
		return nil, errors.New("storage dir is empty")
	}

	db, err := InitDB(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	api := NewAPI(NewDAO(db), cfg.StorageDir)
	StartGCWorker(ctx, db, cfg.StorageDir, cfg.GCInterval)
	return &Service{db: db, api: api, cfg: cfg}, nil
}

func (s *Service) RegisterRoutes(router fiber.Router) {
	if s == nil || s.api == nil {
		return
	}
	s.api.RegisterRoutes(router)
}

func (s *Service) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Service) Config() Config {
	if s == nil {
		return Config{}
	}
	return s.cfg
}

func normalizeConfig(cfg Config) Config {
	cfg.StorageDir = strings.TrimSpace(cfg.StorageDir)
	cfg.DBPath = strings.TrimSpace(cfg.DBPath)
	if cfg.DBPath == "" && cfg.StorageDir != "" {
		cfg.DBPath = filepath.Join(cfg.StorageDir, DefaultDBFilename)
	}
	if cfg.GCInterval == 0 {
		cfg.GCInterval = DefaultGCInterval
	}
	if cfg.GCInterval < 0 {
		cfg.GCInterval = 0
	}
	return cfg
}
