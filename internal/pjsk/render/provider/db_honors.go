package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/bondshonor"
	"haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/database/sekai/gamecharacterunit"
	sekaiHonor "haruki-cloud/database/sekai/honor"
	"haruki-cloud/database/sekai/honorgroup"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbHonorProvider struct {
	client *sekaiDB.Client
	region renderregion.Value

	honorMu    sync.RWMutex
	honorCache map[int]*masterdata.Honor

	groupMu    sync.RWMutex
	groupCache map[int]*masterdata.HonorGroup

	bondsMu    sync.RWMutex
	bondsCache map[int]*masterdata.BondsHonor

	gcuMu    sync.RWMutex
	gcuCache map[int]*masterdata.GameCharacterUnit

	birthdayMu      sync.RWMutex
	birthdayByGroup map[int]honorBirthdayAssets
	birthdayChars   []*sekaiDB.Gamecharacter
	birthdayLoaded  bool

	eventHonorMu       sync.RWMutex
	eventByHonorID     map[int]int
	eventByHonorLoaded bool
}

type honorBirthdayAssets struct {
	background string
	frame      string
}

func (p *dbHonorProvider) init() {
	if p.honorCache == nil {
		p.honorCache = make(map[int]*masterdata.Honor)
	}
	if p.groupCache == nil {
		p.groupCache = make(map[int]*masterdata.HonorGroup)
	}
	if p.bondsCache == nil {
		p.bondsCache = make(map[int]*masterdata.BondsHonor)
	}
	if p.gcuCache == nil {
		p.gcuCache = make(map[int]*masterdata.GameCharacterUnit)
	}
	if p.birthdayByGroup == nil {
		p.birthdayByGroup = make(map[int]honorBirthdayAssets)
	}
	if p.eventByHonorID == nil {
		p.eventByHonorID = make(map[int]int)
	}
}

func (p *dbHonorProvider) GetByID(id int) (*masterdata.Honor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor id")
	}
	p.init()

	p.honorMu.RLock()
	if cached, ok := p.honorCache[id]; ok {
		p.honorMu.RUnlock()
		return common.CloneHonor(cached), nil
	}
	p.honorMu.RUnlock()

	entity, err := p.client.Honor.Query().
		Where(sekaiHonor.ServerRegionEQ(p.region.String()), sekaiHonor.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query honor %d: %w", id, err)
	}
	model, err := convertCloudHonor(entity)
	if err != nil {
		return nil, err
	}

	p.honorMu.Lock()
	p.honorCache[id] = model
	p.honorMu.Unlock()
	return common.CloneHonor(model), nil
}

func (p *dbHonorProvider) GetGroupByID(id int) (*masterdata.HonorGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor group id")
	}
	p.init()

	p.groupMu.RLock()
	if cached, ok := p.groupCache[id]; ok {
		p.groupMu.RUnlock()
		return common.CloneHonorGroup(cached), nil
	}
	p.groupMu.RUnlock()

	entity, err := p.client.Honorgroup.Query().
		Where(honorgroup.ServerRegionEQ(p.region.String()), honorgroup.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query honor group %d: %w", id, err)
	}

	model := &masterdata.HonorGroup{
		ID:          int(entity.GameID),
		HonorType:   entity.HonorType,
		Name:        entity.Name,
		Description: "",
	}
	if value := entity.BackgroundAssetbundleName; value != "" {
		model.BackgroundAssetBundleName = &value
	}
	if value := entity.FrameName; value != "" {
		model.FrameName = &value
	}
	if model.HonorType == "birthday" && (model.BackgroundAssetBundleName == nil || model.FrameName == nil) {
		if derived, ok := p.deriveBirthdayAssetsForGroup(int(entity.GameID), model.Name); ok {
			if model.BackgroundAssetBundleName == nil && derived.background != "" {
				value := derived.background
				model.BackgroundAssetBundleName = &value
			}
			if model.FrameName == nil && derived.frame != "" {
				value := derived.frame
				model.FrameName = &value
			}
			slog.Info(
				"honor birthday derive trace",
				"group_id", entity.GameID,
				"group_name", model.Name,
				"derived_background_assetbundle_name", derived.background,
				"derived_frame_name", derived.frame,
			)
		}
	}

	p.groupMu.Lock()
	p.groupCache[id] = model
	p.groupMu.Unlock()
	return common.CloneHonorGroup(model), nil
}

func (p *dbHonorProvider) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid bonds honor id")
	}
	p.init()

	p.bondsMu.RLock()
	if cached, ok := p.bondsCache[id]; ok {
		p.bondsMu.RUnlock()
		return common.CloneBondsHonor(cached), nil
	}
	p.bondsMu.RUnlock()

	entity, err := p.client.Bondshonor.Query().
		Where(bondshonor.ServerRegionEQ(p.region.String()), bondshonor.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query bonds honor %d: %w", id, err)
	}
	model := &masterdata.BondsHonor{
		ID:                   int(entity.GameID),
		GameCharacterUnitID1: int(entity.GameCharacterUnitId1),
		GameCharacterUnitID2: int(entity.GameCharacterUnitId2),
		HonorRarity:          entity.HonorRarity,
		Name:                 entity.Name,
		Description:          entity.Description,
		BondsGroupID:         int(entity.BondsGroupID),
	}

	p.bondsMu.Lock()
	p.bondsCache[id] = model
	p.bondsMu.Unlock()
	return common.CloneBondsHonor(model), nil
}

func (p *dbHonorProvider) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	if id == 0 {
		return nil, false
	}
	p.init()

	p.gcuMu.RLock()
	if cached, ok := p.gcuCache[id]; ok {
		p.gcuMu.RUnlock()
		return common.CloneGameCharacterUnit(cached), true
	}
	p.gcuMu.RUnlock()

	entity, err := p.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(p.region.String()), gamecharacterunit.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, false
	}
	model := &masterdata.GameCharacterUnit{
		ID:              int(entity.GameID),
		GameCharacterID: int(entity.GameCharacterID),
		Unit:            entity.Unit,
		ColorCode:       entity.ColorCode,
	}

	p.gcuMu.Lock()
	p.gcuCache[id] = model
	p.gcuMu.Unlock()
	return common.CloneGameCharacterUnit(model), true
}

func (p *dbHonorProvider) GetEventIDByHonorID(honorID int) int {
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
		All(context.Background())
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

func (p *dbHonorProvider) deriveBirthdayAssetsForGroup(groupID int, groupName string) (honorBirthdayAssets, bool) {
	p.birthdayMu.RLock()
	if cached, ok := p.birthdayByGroup[groupID]; ok {
		p.birthdayMu.RUnlock()
		return cached, true
	}
	p.birthdayMu.RUnlock()

	p.birthdayMu.Lock()
	defer p.birthdayMu.Unlock()
	if cached, ok := p.birthdayByGroup[groupID]; ok {
		return cached, true
	}

	if !p.birthdayLoaded {
		rows, err := p.client.Gamecharacter.Query().
			Where(gamecharacter.ServerRegionEQ(p.region.String())).
			All(context.Background())
		if err == nil {
			p.birthdayChars = rows
		}
		p.birthdayLoaded = true
	}

	for _, row := range p.birthdayChars {
		gameID := int(row.GameID)
		if gameID <= 0 {
			continue
		}
		if !honorBirthdayGroupMatchesCharacter(groupName, row) {
			continue
		}
		suffix := fmt.Sprintf("01_%02d", gameID)
		derived := honorBirthdayAssets{
			background: "honor_bg_birthday_" + suffix,
			frame:      "honor_frame_birthday_" + suffix,
		}
		p.birthdayByGroup[groupID] = derived
		slog.Info(
			"honor birthday match trace",
			"group_id", groupID,
			"group_name", groupName,
			"character_id", row.GameID,
			"first_name", row.FirstName,
			"given_name", row.GivenName,
			"first_name_english", row.FirstNameEnglish,
			"given_name_english", row.GivenNameEnglish,
			"background_assetbundle_name", derived.background,
			"frame_name", derived.frame,
		)
		return derived, true
	}
	return honorBirthdayAssets{}, false
}

func honorBirthdayGroupMatchesCharacter(groupName string, row *sekaiDB.Gamecharacter) bool {
	if row == nil {
		return false
	}
	name := strings.TrimSpace(groupName)
	if name == "" {
		return false
	}
	candidates := []string{
		strings.TrimSpace(row.FirstName),
		strings.TrimSpace(row.GivenName),
		strings.TrimSpace(row.FirstName + row.GivenName),
		strings.TrimSpace(row.FirstNameEnglish),
		strings.TrimSpace(row.GivenNameEnglish),
		strings.TrimSpace(row.FirstNameEnglish + row.GivenNameEnglish),
	}
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(name, candidate) {
			return true
		}
	}
	if nickname, ok := assets.CharacterIDToNickname[int(row.GameID)]; ok && nickname != "" {
		if strings.Contains(strings.ToLower(name), strings.ToLower(strings.TrimSpace(nickname))) {
			return true
		}
	}
	return false
}

func convertCloudHonor(entity *sekaiDB.Honor) (*masterdata.Honor, error) {
	model := &masterdata.Honor{
		ID:              int(entity.GameID),
		GroupID:         int(entity.GroupID),
		HonorRarity:     entity.HonorRarity,
		Name:            entity.Name,
		Description:     "",
		AssetBundleName: entity.AssetbundleName,
	}
	if len(entity.Levels) > 0 {
		if err := json.Unmarshal(entity.Levels, &model.Levels); err != nil {
			return nil, fmt.Errorf("unmarshal honor levels: %w", err)
		}
	}
	return model, nil
}
