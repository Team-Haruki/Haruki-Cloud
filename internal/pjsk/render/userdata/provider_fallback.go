package userdata

import "context"

type FallbackSnapshotProvider struct {
	providers []SnapshotProvider
}

func NewFallbackSnapshotProvider(providers ...SnapshotProvider) *FallbackSnapshotProvider {
	available := make([]SnapshotProvider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			available = append(available, provider)
		}
	}
	return &FallbackSnapshotProvider{providers: available}
}

func (p *FallbackSnapshotProvider) Resolve(ctx context.Context, selector Selector, opts ResolveOptions) (Snapshot, error) {
	if p == nil || len(p.providers) == 0 {
		return nil, ErrProviderUnavailable
	}

	var lastErr error
	for _, provider := range p.providers {
		snapshot, err := provider.Resolve(ctx, selector, opts)
		if err == nil && snapshot != nil {
			return snapshot, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrSnapshotUnavailable
}
