package honor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/bondshonor"
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/database/sekai/gamecharacterunit"
	sekaiHonor "haruki-cloud/database/sekai/honor"
	"haruki-cloud/database/sekai/honorgroup"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type CloudSource struct {
	client      *sekaiDB.Client
	region      renderregion.Value
	queryRegion renderregion.Value

	honorMu    sync.RWMutex
	honorCache map[int]*masterdata.Honor

	groupMu    sync.RWMutex
	groupCache map[int]*masterdata.HonorGroup

	bondsMu    sync.RWMutex
	bondsCache map[int]*masterdata.BondsHonor

	gcuMu    sync.RWMutex
	gcuCache map[int]*masterdata.GameCharacterUnit

	birthdayMu      sync.RWMutex
	birthdayByGroup map[int]birthdayHonorAssets
	birthdayChars   []*sekaiDB.Gamecharacter
	birthdayLoaded  bool
}

type birthdayHonorAssets struct {
	background string
	frame      string
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:          client,
		region:          region,
		queryRegion:     region,
		honorCache:      make(map[int]*masterdata.Honor),
		groupCache:      make(map[int]*masterdata.HonorGroup),
		bondsCache:      make(map[int]*masterdata.BondsHonor),
		gcuCache:        make(map[int]*masterdata.GameCharacterUnit),
		birthdayByGroup: make(map[int]birthdayHonorAssets),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) GetHonorByID(id int) (*masterdata.Honor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor id")
	}
	c.honorMu.RLock()
	if cached, ok := c.honorCache[id]; ok {
		c.honorMu.RUnlock()
		return cloneHonor(cached), nil
	}
	c.honorMu.RUnlock()

	entity, err := c.client.Honor.Query().
		Where(sekaiHonor.ServerRegionEQ(c.queryRegion.String()), sekaiHonor.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query honor %d failed: %w", id, err)
	}
	model, err := convertCloudHonor(entity)
	if err != nil {
		return nil, err
	}

	c.honorMu.Lock()
	c.honorCache[id] = model
	c.honorMu.Unlock()
	return cloneHonor(model), nil
}

func (c *CloudSource) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor group id")
	}
	c.groupMu.RLock()
	if cached, ok := c.groupCache[id]; ok {
		c.groupMu.RUnlock()
		return cloneHonorGroup(cached), nil
	}
	c.groupMu.RUnlock()

	entity, err := c.client.Honorgroup.Query().
		Where(honorgroup.ServerRegionEQ(c.queryRegion.String()), honorgroup.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query honor group %d failed: %w", id, err)
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
		if derived, ok := c.deriveBirthdayAssetsForGroup(int(entity.GameID), model.Name); ok {
			if model.BackgroundAssetBundleName == nil && derived.background != "" {
				value := derived.background
				model.BackgroundAssetBundleName = &value
			}
			if model.FrameName == nil && derived.frame != "" {
				value := derived.frame
				model.FrameName = &value
			}
		}
	}

	c.groupMu.Lock()
	c.groupCache[id] = model
	c.groupMu.Unlock()
	return cloneHonorGroup(model), nil
}

func (c *CloudSource) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid bonds honor id")
	}
	c.bondsMu.RLock()
	if cached, ok := c.bondsCache[id]; ok {
		c.bondsMu.RUnlock()
		return cloneBondsHonor(cached), nil
	}
	c.bondsMu.RUnlock()

	entity, err := c.client.Bondshonor.Query().
		Where(bondshonor.ServerRegionEQ(c.queryRegion.String()), bondshonor.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query bonds honor %d failed: %w", id, err)
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

	c.bondsMu.Lock()
	c.bondsCache[id] = model
	c.bondsMu.Unlock()
	return cloneBondsHonor(model), nil
}

func (c *CloudSource) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	if id == 0 {
		return nil, false
	}
	c.gcuMu.RLock()
	if cached, ok := c.gcuCache[id]; ok {
		c.gcuMu.RUnlock()
		return cloneGameCharacterUnit(cached), true
	}
	c.gcuMu.RUnlock()

	entity, err := c.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(c.queryRegion.String()), gamecharacterunit.GameIDEQ(int64(id))).
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

	c.gcuMu.Lock()
	c.gcuCache[id] = model
	c.gcuMu.Unlock()
	return cloneGameCharacterUnit(model), true
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
			return nil, fmt.Errorf("unmarshal honor levels failed: %w", err)
		}
	}
	return model, nil
}

func cloneHonor(src *masterdata.Honor) *masterdata.Honor {
	if src == nil {
		return nil
	}
	copy := *src
	if src.Levels != nil {
		copy.Levels = append([]masterdata.HonorLevel(nil), src.Levels...)
	}
	return &copy
}

func (c *CloudSource) deriveBirthdayAssetsForGroup(groupID int, groupName string) (birthdayHonorAssets, bool) {
	c.birthdayMu.RLock()
	if assets, ok := c.birthdayByGroup[groupID]; ok {
		c.birthdayMu.RUnlock()
		return assets, true
	}
	c.birthdayMu.RUnlock()

	c.birthdayMu.Lock()
	defer c.birthdayMu.Unlock()
	if assets, ok := c.birthdayByGroup[groupID]; ok {
		return assets, true
	}

	if !c.birthdayLoaded {
		rows, err := c.client.Gamecharacter.Query().
			Where(gamecharacter.ServerRegionEQ(c.queryRegion.String())).
			All(context.Background())
		if err == nil {
			c.birthdayChars = rows
		}
		c.birthdayLoaded = true
	}

	for _, row := range c.birthdayChars {
		gameID := int(row.GameID)
		if gameID <= 0 {
			continue
		}
		if !birthdayGroupMatchesCharacter(groupName, row) {
			continue
		}
		suffix := fmt.Sprintf("01_%02d", gameID)
		assets := birthdayHonorAssets{
			background: "honor_bg_birthday_" + suffix,
			frame:      "honor_frame_birthday_" + suffix,
		}
		c.birthdayByGroup[groupID] = assets
		return assets, true
	}
	return birthdayHonorAssets{}, false
}

func birthdayGroupMatchesCharacter(groupName string, row *sekaiDB.Gamecharacter) bool {
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

func cloneHonorGroup(src *masterdata.HonorGroup) *masterdata.HonorGroup {
	if src == nil {
		return nil
	}
	copy := *src
	return &copy
}

func cloneBondsHonor(src *masterdata.BondsHonor) *masterdata.BondsHonor {
	if src == nil {
		return nil
	}
	copy := *src
	return &copy
}

func cloneGameCharacterUnit(src *masterdata.GameCharacterUnit) *masterdata.GameCharacterUnit {
	if src == nil {
		return nil
	}
	copy := *src
	return &copy
}

// jsonString extracts a plain string from a json.RawMessage value.
func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return string(raw)
	}
	return s
}
