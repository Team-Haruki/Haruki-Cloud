package stamp

import (
	"context"
	"fmt"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	sekaiStamp "haruki-cloud/database/sekai/stamp"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type CloudSource struct {
	client      *sekaiDB.Client
	region      renderregion.Value
	queryRegion renderregion.Value

	mu     sync.RWMutex
	loaded bool
	stamps []masterdata.Stamp
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:      client,
		region:      region,
		queryRegion: region,
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) GetStamps() ([]masterdata.Stamp, error) {
	c.mu.RLock()
	if c.loaded {
		out := append([]masterdata.Stamp(nil), c.stamps...)
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loaded {
		items, err := c.client.Stamp.Query().
			Where(sekaiStamp.ServerRegionEQ(c.queryRegion.String())).
			All(context.Background())
		if err != nil {
			return nil, fmt.Errorf("query stamps failed: %w", err)
		}
		stamps := make([]masterdata.Stamp, 0, len(items))
		for _, item := range items {
			stamps = append(stamps, masterdata.Stamp{
				ID:              int(item.GameID),
				AssetBundleName: item.AssetbundleName,
			})
		}
		c.stamps = stamps
		c.loaded = true
	}
	return append([]masterdata.Stamp(nil), c.stamps...), nil
}
