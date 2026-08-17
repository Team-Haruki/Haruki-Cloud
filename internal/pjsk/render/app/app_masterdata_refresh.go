package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/utils/logger"
)

const defaultLocalMasterdataRefreshInterval = 5 * time.Minute

type masterdataCacheResetter interface {
	ResetMasterdataCache()
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

	additionalResetters := make([]masterdataCacheResetter, 0, 2)
	if a.MySekai != nil {
		additionalResetters = append(additionalResetters, a.MySekai)
	}
	if a.Inventory != nil {
		additionalResetters = append(additionalResetters, a.Inventory)
	}
	state := newLocalMasterdataRefreshState(root, a.Providers, additionalResetters...)
	if len(state.providers) == 0 {
		return
	}
	state.captureInitial()

	logger.Info("local masterdata refresh loop started", "refresh_interval", interval)
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
	root                string
	providers           map[renderregion.Value]masterdataCacheResetter
	additionalResetters []masterdataCacheResetter
	signatures          map[renderregion.Value]string
}

func newLocalMasterdataRefreshState(root string, providers map[renderregion.Value]provider.MasterDataProvider, additionalResetters ...masterdataCacheResetter) *localMasterdataRefreshState {
	state := &localMasterdataRefreshState{
		root:                root,
		providers:           make(map[renderregion.Value]masterdataCacheResetter, len(providers)),
		additionalResetters: additionalResetters,
		signatures:          make(map[renderregion.Value]string, len(providers)),
	}
	for region, src := range providers {
		resetter, ok := src.(masterdataCacheResetter)
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
			logger.Warn("local masterdata signature failed",
				"operation", "capture_initial",
				"region", region,
				"error_type", fmt.Sprintf("%T", err),
			)
			continue
		}
		s.signatures[region] = signature.Hash
	}
}

func (s *localMasterdataRefreshState) refresh() {
	if s == nil {
		return
	}
	changed := false
	for region, resetter := range s.providers {
		signature, err := provider.LocalMasterdataDirSignature(s.root, region)
		if err != nil {
			logger.Warn("local masterdata signature failed",
				"operation", "refresh",
				"region", region,
				"error_type", fmt.Sprintf("%T", err),
			)
			continue
		}
		previous := s.signatures[region]
		if previous == "" {
			s.signatures[region] = signature.Hash
			resetter.ResetMasterdataCache()
			changed = true
			logger.Info("masterdata cache reset",
				"reason", "source_appeared",
				"region", region,
				"files", signature.Files,
			)
			continue
		}
		if previous == signature.Hash {
			continue
		}
		s.signatures[region] = signature.Hash
		resetter.ResetMasterdataCache()
		changed = true
		logger.Info("masterdata cache reset",
			"reason", "source_changed",
			"region", region,
			"files", signature.Files,
		)
	}
	if !changed {
		return
	}
	for _, resetter := range s.additionalResetters {
		if resetter != nil {
			resetter.ResetMasterdataCache()
		}
	}
}
