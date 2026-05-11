package provider

import (
	"context"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *localEducationProvider) ensureBondMaster() error {
	return p.bonds.init(func() (bondData, error) {
		items, err := loadJSON[localBondJSON](p.store, "bonds.json")
		if err != nil {
			return bondData{}, err
		}
		data := bondData{bonds: make([]*Bond, 0, len(items))}
		for _, item := range items {
			data.bonds = append(data.bonds, &Bond{
				GroupID:      item.GroupID,
				CharacterID1: item.CharacterID1,
				CharacterID2: item.CharacterID2,
			})
		}

		levels, err := loadJSON[localLevelJSON](p.store, "levels.json")
		if err != nil {
			return bondData{}, err
		}
		data.bondLevels = make([]*BondLevel, 0)
		data.characterLevels = make([]*CharacterLevel, 0)
		for _, item := range levels {
			if item.Level <= 0 {
				continue
			}
			switch {
			case strings.EqualFold(item.LevelType, "bonds"):
				data.bondLevels = append(data.bondLevels, &BondLevel{
					Level:    item.Level,
					TotalExp: item.TotalExp,
				})
			case strings.EqualFold(item.LevelType, "character"):
				data.characterLevels = append(data.characterLevels, &CharacterLevel{
					Level:    item.Level,
					TotalExp: item.TotalExp,
				})
			}
		}
		sort.Slice(data.bondLevels, func(i, j int) bool { return data.bondLevels[i].Level < data.bondLevels[j].Level })
		sort.Slice(data.characterLevels, func(i, j int) bool {
			return data.characterLevels[i].Level < data.characterLevels[j].Level
		})
		return data, nil
	})
}

func (p *localEducationProvider) ensureGameCharacterStyles() error {
	return p.styles.init(func() (map[int]*GameCharacterStyle, error) {
		items, err := loadJSON[masterdata.GameCharacterUnit](p.store, "gameCharacterUnits.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*GameCharacterStyle, len(items))
		for i := range items {
			byID[items[i].ID] = &GameCharacterStyle{
				GameID:      items[i].ID,
				CharacterID: items[i].GameCharacterID,
				ColorCode:   strings.TrimSpace(items[i].ColorCode),
			}
		}
		return byID, nil
	})
}

func (p *localEducationProvider) ensureLeaderMissionRequirements() error {
	return p.missions.init(func() (missionData, error) {
		data := missionData{
			missions:   make(map[int][]*CharacterMission),
			groupsByID: make(map[int][]*CharacterMissionParameterGroup),
		}
		missions, err := loadJSON[localCharacterMissionJSON](p.store, "characterMissionV2s.json")
		if err == nil {
			for _, item := range missions {
				mission := &CharacterMission{
					ID:                   item.ID,
					CharacterID:          item.CharacterID,
					CharacterMissionType: item.CharacterMissionType,
					ParameterGroupID:     item.ParameterGroupID,
					IsAchievementMission: item.IsAchievementMission,
				}
				data.missions[mission.CharacterID] = append(data.missions[mission.CharacterID], mission)
			}
		}

		items, err := loadJSON[localLeaderMissionRequirementJSON](p.store, "characterMissionV2ParameterGroups.json")
		if err != nil {
			return missionData{}, err
		}
		for _, item := range items {
			data.groupsByID[item.ID] = append(data.groupsByID[item.ID], &CharacterMissionParameterGroup{
				GameID:      item.GameID,
				Seq:         item.Seq,
				Requirement: item.Requirement,
				Exp:         item.Exp,
				Quantity:    item.Quantity,
			})
			switch item.GameID {
			case 1:
				if item.Requirement > data.maxPlayLimit {
					data.maxPlayLimit = item.Requirement
				}
			case 101:
				data.requirements = append(data.requirements, LeaderMissionRequirement{
					Seq:         item.Seq,
					Requirement: item.Requirement,
				})
			}
		}
		sort.Slice(data.requirements, func(i, j int) bool { return data.requirements[i].Seq < data.requirements[j].Seq })
		return data, nil
	})
}

func (p *localEducationProvider) GetBonds(_ context.Context) []*Bond {
	if err := p.ensureBondMaster(); err != nil {
		return nil
	}
	return cloneEdBonds(p.bonds.v().bonds)
}

func (p *localEducationProvider) GetBondLevels(_ context.Context) []*BondLevel {
	if err := p.ensureBondMaster(); err != nil {
		return nil
	}
	return cloneEdBondLevels(p.bonds.v().bondLevels)
}

func (p *localEducationProvider) GetCharacterLevels(_ context.Context) []*CharacterLevel {
	if err := p.ensureBondMaster(); err != nil {
		return nil
	}
	return cloneEdCharacterLevels(p.bonds.v().characterLevels)
}

func (p *localEducationProvider) GetGameCharacterStyle(_ context.Context, gameID int) *GameCharacterStyle {
	if gameID <= 0 {
		return nil
	}
	if err := p.ensureGameCharacterStyles(); err != nil {
		return nil
	}
	return cloneEdGameCharacterStyle(p.styles.v()[gameID])
}

func (p *localEducationProvider) GetCharacterMissions(_ context.Context, characterID int) []*CharacterMission {
	if characterID <= 0 {
		return nil
	}
	if err := p.ensureLeaderMissionRequirements(); err != nil {
		return nil
	}
	return cloneEdCharacterMissions(p.missions.v().missions[characterID])
}

func (p *localEducationProvider) GetCharacterMissionParameterGroups(_ context.Context, parameterGroupID int) []*CharacterMissionParameterGroup {
	if parameterGroupID <= 0 {
		return nil
	}
	if err := p.ensureLeaderMissionRequirements(); err != nil {
		return nil
	}
	return cloneEdCharacterMissionParameterGroups(p.missions.v().groupsByID[parameterGroupID])
}

func (p *localEducationProvider) GetLeaderMissionRequirements(_ context.Context) ([]LeaderMissionRequirement, int) {
	if err := p.ensureLeaderMissionRequirements(); err != nil {
		return nil, 0
	}
	return cloneEdLeaderMissionRequirements(p.missions.v().requirements), p.missions.v().maxPlayLimit
}

type localBondJSON struct {
	GroupID      int `json:"groupId"`
	CharacterID1 int `json:"characterId1"`
	CharacterID2 int `json:"characterId2"`
}

type localLevelJSON struct {
	LevelType string `json:"levelType"`
	Level     int    `json:"level"`
	TotalExp  int    `json:"totalExp"`
}

type localLeaderMissionRequirementJSON struct {
	GameID      int `json:"gameId"`
	Seq         int `json:"seq"`
	Requirement int `json:"requirement"`
	Exp         int `json:"exp"`
	Quantity    int `json:"quantity"`
	ID          int `json:"id"`
}

type localCharacterMissionJSON struct {
	ID                   int    `json:"id"`
	CharacterID          int    `json:"characterId"`
	CharacterMissionType string `json:"characterMissionType"`
	ParameterGroupID     int    `json:"parameterGroupId"`
	IsAchievementMission bool   `json:"isAchievementMission"`
}
