package provider

import (
	"context"
	"fmt"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	sekaiStamp "haruki-cloud/database/sekai/stamp"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbStampProvider struct {
	client *sekaiDB.Client
	region renderregion.Value

	mu     sync.RWMutex
	loaded bool
	stamps []masterdata.Stamp
}

func (p *dbStampProvider) GetAll() ([]masterdata.Stamp, error) {
	return p.getAll(nil)
}

func (p *dbStampProvider) getAll(ctx context.Context) ([]masterdata.Stamp, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.RLock()
	if p.loaded {
		out := append([]masterdata.Stamp(nil), p.stamps...)
		p.mu.RUnlock()
		return out, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.loaded {
		items, err := p.client.Stamp.Query().
			Where(sekaiStamp.ServerRegionEQ(p.region.String())).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("query stamps: %w", err)
		}
		stamps := make([]masterdata.Stamp, 0, len(items))
		for _, item := range items {
			stamps = append(stamps, masterdata.Stamp{
				ID:              int(item.GameID),
				AssetBundleName: item.AssetbundleName,
				CharacterID:     int(item.CharacterId1),
			})
		}
		p.stamps = stamps
		p.loaded = true
	}
	return append([]masterdata.Stamp(nil), p.stamps...), nil
}
