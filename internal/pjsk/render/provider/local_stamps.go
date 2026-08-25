package provider

import (
	"context"
	"slices"

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
		return p.store.loadJSON[masterdata.Stamp]("stamps.json")
	}); err != nil {
		return nil, err
	}
	return slices.Clone(p.stamps.v()), nil
}
