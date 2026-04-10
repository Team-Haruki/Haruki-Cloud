package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localStampProvider
// ===========================================================================

type localStampProvider struct {
	store  *localStore
	stamps lazyValue[[]masterdata.Stamp]
}

func (p *localStampProvider) GetAll(_ context.Context) ([]masterdata.Stamp, error) {
	if err := p.stamps.init(func() ([]masterdata.Stamp, error) {
		return loadJSON[masterdata.Stamp](p.store, "stamps.json")
	}); err != nil {
		return nil, err
	}
	return append([]masterdata.Stamp(nil), p.stamps.v()...), nil
}
