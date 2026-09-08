package drawingcache

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
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
	stop      context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
	db        *sql.DB
	api       *API
	cfg       Config
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
	if ctx == nil {
		ctx = context.Background()
	}
	workerCtx, stop := context.WithCancel(ctx)
	s := &Service{db: db, api: api, cfg: cfg, stop: stop, done: make(chan struct{})}
	go func() { defer close(s.done); api.maintain(workerCtx, cfg.GCInterval) }()
	return s, nil
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
	s.closeOnce.Do(func() {
		if s.stop != nil {
			s.stop()
			<-s.done
		}
		if s.api != nil {
			s.closeErr = s.api.flushTouches()
		}
		s.closeErr = errors.Join(s.closeErr, s.db.Close())
	})
	return s.closeErr
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
