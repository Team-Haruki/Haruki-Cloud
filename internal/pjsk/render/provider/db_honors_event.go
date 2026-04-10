package provider

import (
	"context"
	"encoding/json"

	"haruki-cloud/database/sekai/event"
)

func (p *dbHonorProvider) GetEventIDByHonorID(ctx context.Context, honorID int) int {
	if honorID == 0 {
		return 0
	}
	p.init()

	p.eventHonorMu.RLock()
	if p.eventByHonorLoaded {
		id := p.eventByHonorID[honorID]
		p.eventHonorMu.RUnlock()
		return id
	}
	p.eventHonorMu.RUnlock()

	p.eventHonorMu.Lock()
	defer p.eventHonorMu.Unlock()
	if p.eventByHonorLoaded {
		return p.eventByHonorID[honorID]
	}

	items, err := p.client.Event.Query().
		Where(event.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return 0
	}

	for _, item := range items {
		var ranges []honorRewardRange
		if err := json.Unmarshal(item.EventRankingRewardRanges, &ranges); err != nil {
			continue
		}
		eventID := int(item.GameID)
		for _, rr := range ranges {
			for _, detail := range rr.EventRankingRewardDetails {
				if detail.ResourceType == "honor" && detail.ResourceID > 0 {
					p.eventByHonorID[detail.ResourceID] = eventID
				}
			}
		}
	}

	p.eventByHonorLoaded = true
	return p.eventByHonorID[honorID]
}

type honorRewardRange struct {
	EventRankingRewardDetails []honorRewardDetail `json:"eventRankingRewardDetails"`
}

type honorRewardDetail struct {
	ResourceType string `json:"resourceType"`
	ResourceID   int    `json:"resourceId"`
}
