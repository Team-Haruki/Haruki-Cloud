package snapshot

import (
	"context"
	"fmt"

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

	providers := p.providers
	if !p.allowFallback {
		providers = providers[:1]
	}
	var lastErr error
	for i, provider := range providers {
		snapshot, err := provider.Resolve(ctx, selector, opts)
		if err == nil && snapshot != nil {
			p.logFallbackSuccess(ctx, i)
			return snapshot, nil
		}
		if err != nil {
			p.logProviderFailure(ctx, i, err)
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrSnapshotUnavailable
}

func (p *FallbackSnapshotProvider) logFallbackSuccess(ctx context.Context, providerIndex int) {
	if providerIndex > 0 {
		p.logger.WarnContext(ctx, "snapshot resolved through fallback provider", "provider_index", providerIndex)
	}
}

func (p *FallbackSnapshotProvider) logProviderFailure(ctx context.Context, providerIndex int, err error) {
	p.logger.WarnContext(ctx, "snapshot provider failed",
		"provider_index", providerIndex,
		"error_type", fmt.Sprintf("%T", err),
	)
}
