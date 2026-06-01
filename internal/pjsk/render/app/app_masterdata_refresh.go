package app

import (
	"context"
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/utils/logger"
)

const defaultLocalMasterdataRefreshInterval = 5 * time.Minute

type localMasterdataCacheResetter interface {
	ResetLocalMasterdataCache()
}

func (a *App) startLocalMasterdataRefresh(ctx context.Context, root string, interval time.Duration) {
	if a == nil || len(a.Providers) == 0 {
		return
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	if interval == 0 {
		interval = defaultLocalMasterdataRefreshInterval
	}
	if interval < 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	state := newLocalMasterdataRefreshState(root, a.Providers)
	if len(state.providers) == 0 {
		return
	}
	state.captureInitial()

	logger.Infof("local masterdata refresh loop started (refresh=%s root=%q)", interval, root)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state.refresh()
			}
		}
	}()
}

type localMasterdataRefreshState struct {
	root       string
	providers  map[renderregion.Value]localMasterdataCacheResetter
	signatures map[renderregion.Value]string
}

func newLocalMasterdataRefreshState(root string, providers map[renderregion.Value]provider.MasterDataProvider) *localMasterdataRefreshState {
	state := &localMasterdataRefreshState{
		root:       root,
		providers:  make(map[renderregion.Value]localMasterdataCacheResetter, len(providers)),
		signatures: make(map[renderregion.Value]string, len(providers)),
	}
	for region, src := range providers {
		resetter, ok := src.(localMasterdataCacheResetter)
		if !ok || resetter == nil {
			continue
		}
		state.providers[renderregion.WithDefault(region)] = resetter
	}
	return state
}

func (s *localMasterdataRefreshState) captureInitial() {
	if s == nil {
		return
	}
	for region := range s.providers {
		signature, err := provider.LocalMasterdataDirSignature(s.root, region)
		if err != nil {
			logger.Warnf("local masterdata: failed to capture initial signature for %s: %v", region, err)
			continue
		}
		s.signatures[region] = signature.Hash
	}
}

func (s *localMasterdataRefreshState) refresh() {
	if s == nil {
		return
	}
	for region, resetter := range s.providers {
		signature, err := provider.LocalMasterdataDirSignature(s.root, region)
		if err != nil {
			logger.Warnf("local masterdata: failed to check update for %s: %v", region, err)
			continue
		}
		previous := s.signatures[region]
		if previous == "" {
			s.signatures[region] = signature.Hash
			resetter.ResetLocalMasterdataCache()
			logger.Infof("local masterdata appeared; reset render caches (region=%s dir=%s files=%d)", region, signature.Dir, signature.Files)
			continue
		}
		if previous == signature.Hash {
			continue
		}
		s.signatures[region] = signature.Hash
		resetter.ResetLocalMasterdataCache()
		logger.Infof("local masterdata changed; reset render caches (region=%s dir=%s files=%d)", region, signature.Dir, signature.Files)
	}
}
