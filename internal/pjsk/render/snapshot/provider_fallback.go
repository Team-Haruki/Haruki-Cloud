package snapshot

import (
	"context"

	"haruki-cloud/utils/logger"
)

type FallbackSnapshotProvider struct {
	providers     []HarukiSnapshotProvider
	allowFallback bool
	logger        *logger.Logger
}

// NewFallbackSnapshotProvider creates a provider chain. When allowFallback is
// false only the first (primary) provider is tried — subsequent providers are
// skipped and the primary error is returned directly. This is the intended
// production mode where Toolbox is the sole data source.
func NewFallbackSnapshotProvider(allowFallback bool, providers ...HarukiSnapshotProvider) *FallbackSnapshotProvider {
	available := make([]HarukiSnapshotProvider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			available = append(available, provider)
		}
	}
	return &FallbackSnapshotProvider{
		providers:     available,
		allowFallback: allowFallback,
		logger:        logger.NewLoggerFromGlobal("SnapshotFallback"),
	}
}

func (p *FallbackSnapshotProvider) Resolve(ctx context.Context, selector Selector, opts ResolveOptions) (Snapshot, error) {
	if p == nil || len(p.providers) == 0 {
		return nil, ErrProviderUnavailable
	}

	var lastErr error
	for i, provider := range p.providers {
		if i > 0 && !p.allowFallback {
			break
		}
		snapshot, err := provider.Resolve(ctx, selector, opts)
		if err == nil && snapshot != nil {
			if i > 0 {
				p.logger.Warnf("primary provider failed, resolved via fallback provider %d", i)
			}
			return snapshot, nil
		}
		if err != nil {
			if i == 0 && !p.allowFallback {
				return nil, err
			}
			p.logger.Warnf("provider %d failed: %v", i, err)
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrSnapshotUnavailable
}
