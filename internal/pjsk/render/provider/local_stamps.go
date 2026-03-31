package provider

import (
	"sync"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localStampProvider
// ===========================================================================

type localStampProvider struct {
	store *localStore

	once   sync.Once
	stamps []masterdata.Stamp
	err    error
}

func (p *localStampProvider) GetAll() ([]masterdata.Stamp, error) {
	p.once.Do(func() {
		items, err := loadJSON[masterdata.Stamp](p.store, "stamps.json")
		if err != nil {
			p.err = err
			return
		}
		p.stamps = items
	})
	if p.err != nil {
		return nil, p.err
	}
	return append([]masterdata.Stamp(nil), p.stamps...), nil
}
