package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/virtuallive"
	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/usererror"
)

type dbVLiveProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	// rows serves the columns and tables the ent schema does not model yet
	// (solo cheer-point columns, virtualLiveGroups); nil disables them.
	rows MasterRowSource

	// lives caches each queried region's virtual lives (sorted, with the
	// solo fields applied) until the next masterdata reset. The table
	// carries large JSON columns, so it is read once per generation.
	livesMu sync.Mutex
	lives   map[renderregion.Value]*dbMasterIndex[[]*VLive]
}

func (p *dbVLiveProvider) GetLives(ctx context.Context, region renderregion.Value) ([]*VLive, error) {
	if p.client == nil {
		return nil, usererror.Misconfigured(errors.New("vlive provider is not configured"))
	}

	queryRegion := renderregion.WithDefault(region)
	lives, err := p.livesIndex(queryRegion).get(ctx, "vlives.index", func(loadCtx context.Context) ([]*VLive, error) {
		return p.loadLives(loadCtx, queryRegion)
	})
	if err != nil {
		return nil, err
	}
	return cloneVLives(lives), nil
}

func (p *dbVLiveProvider) livesIndex(region renderregion.Value) *dbMasterIndex[[]*VLive] {
	p.livesMu.Lock()
	defer p.livesMu.Unlock()
	if p.lives == nil {
		p.lives = make(map[renderregion.Value]*dbMasterIndex[[]*VLive])
	}
	index, ok := p.lives[region]
	if !ok {
		index = &dbMasterIndex[[]*VLive]{}
		p.lives[region] = index
	}
	return index
}

func (p *dbVLiveProvider) loadLives(ctx context.Context, region renderregion.Value) ([]*VLive, error) {
	entities, err := p.client.Virtuallive.Query().
		Where(virtuallive.ServerRegionEQ(region.String())).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query virtual lives: %w", err)
	}

	lives := make([]*VLive, 0, len(entities))
	for _, entity := range entities {
		lives = append(lives, convertDBVLive(entity))
	}
	p.applySoloFields(ctx, lives)

	sort.Slice(lives, func(i, j int) bool {
		if lives[i].StartAt == lives[j].StartAt {
			return lives[i].ID < lives[j].ID
		}
		return lives[i].StartAt < lives[j].StartAt
	})
	return lives, nil
}

func (p *dbVLiveProvider) resetMasterdataCache() {
	if p == nil {
		return
	}
	p.livesMu.Lock()
	defer p.livesMu.Unlock()
	for _, index := range p.lives {
		index.reset()
	}
}

// cloneVLives copies the cached lives so callers can mutate their result.
func cloneVLives(lives []*VLive) []*VLive {
	result := make([]*VLive, 0, len(lives))
	for _, live := range lives {
		if live == nil {
			continue
		}
		clone := *live
		clone.Schedules = slices.Clone(live.Schedules)
		clone.Rewards = slices.Clone(live.Rewards)
		clone.Characters = slices.Clone(live.Characters)
		clone.TotalCheerPointRewards = slices.Clone(live.TotalCheerPointRewards)
		if live.TotalCheerPointSurplusReward != nil {
			surplus := *live.TotalCheerPointSurplusReward
			clone.TotalCheerPointSurplusReward = &surplus
		}
		if live.VirtualItemOverrideCost != nil {
			cost := *live.VirtualItemOverrideCost
			clone.VirtualItemOverrideCost = &cost
		}
		result = append(result, &clone)
	}
	return result
}

func convertDBVLive(entity *sekaiDB.Virtuallive) *VLive {
	return &VLive{
		ID:              int(entity.GameID),
		Name:            entity.Name,
		AssetBundleName: entity.AssetbundleName,
		StartAt:         entity.StartAt,
		EndAt:           entity.EndAt,
		Schedules:       decodeVLiveSchedules(entity.VirtualLiveSchedules),
		Rewards:         decodeVLiveRewards(entity.VirtualLiveRewards),
		Characters:      decodeVLiveCharacters(entity.VirtualLiveCharacters),

		VirtualLiveType:    entity.VirtualLiveType,
		VirtualLiveGroupID: int(entity.VirtualLiveGroupID),
	}
}

// applySoloFields merges the solo cheer-point columns, read through the raw
// row store, into the region's solo lives. Regions without solo lives never
// query them.
func (p *dbVLiveProvider) applySoloFields(ctx context.Context, lives []*VLive) {
	if p.rows == nil || !hasSoloVLive(lives) {
		return
	}
	rows, ok := p.rows.LoadMasterRows(ctx, soloVirtualLivesFile)
	if !ok {
		return
	}
	for _, live := range lives {
		if live.VirtualLiveType == VLiveTypeSolo {
			applyVLiveSoloFields(live, rows[live.ID])
		}
	}
}

// GetGroups implements VLiveGroupProvider through the raw row store.
func (p *dbVLiveProvider) GetGroups(ctx context.Context, _ renderregion.Value) (map[int]*VLiveGroup, bool) {
	if p == nil || p.rows == nil {
		return nil, false
	}
	rows, ok := p.rows.LoadMasterRows(ctx, "virtualLiveGroups.json")
	if !ok {
		return nil, false
	}
	return vliveGroupsFromRows(rows), true
}

func decodeVLiveSchedules(data []byte) []VLiveSchedule {
	var items []map[string]any
	_ = json.Unmarshal(data, &items)
	schedules := make([]VLiveSchedule, 0, len(items))
	for _, item := range items {
		startAt := vliveInt64Number(item["startAt"])
		endAt := vliveInt64Number(item["endAt"])
		if startAt > 0 && endAt > 0 {
			schedules = append(schedules, VLiveSchedule{StartAt: startAt, EndAt: endAt})
		}
	}
	return schedules
}

func decodeVLiveRewards(data []byte) []VLiveReward {
	var items []map[string]any
	_ = json.Unmarshal(data, &items)
	rewards := make([]VLiveReward, 0, len(items))
	for _, item := range items {
		resourceBoxID := vliveIntNumber(item["resourceBoxId"])
		if resourceBoxID > 0 {
			rewards = append(rewards, VLiveReward{
				VirtualLiveType: vliveString(item["virtualLiveType"]),
				ResourceBoxID:   resourceBoxID,
			})
		}
	}
	return rewards
}

func decodeVLiveCharacters(data []byte) []VLiveCharacter {
	var items []map[string]any
	_ = json.Unmarshal(data, &items)
	characters := make([]VLiveCharacter, 0, len(items))
	for _, item := range items {
		gameCharacterUnitID := vliveIntNumber(item["gameCharacterUnitId"])
		if gameCharacterUnitID > 0 {
			characters = append(characters, VLiveCharacter{
				GameCharacterUnitID:        gameCharacterUnitID,
				VirtualLivePerformanceType: vliveString(item["virtualLivePerformanceType"]),
			})
		}
	}
	return characters
}

func vliveInt64Number(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	default:
		return 0
	}
}

func vliveIntNumber(value any) int {
	return int(vliveInt64Number(value))
}

func vliveString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

// Ensure the entity type is used to avoid import issues.
var _ *sekaiDB.Virtuallive
