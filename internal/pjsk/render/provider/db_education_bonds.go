package provider

import (
	"context"
	"slices"
	"strings"

	"haruki-cloud/database/sekai/bond"
	"haruki-cloud/database/sekai/charactermissionv2parametergroup"
	"haruki-cloud/database/sekai/gamecharacterunit"
	"haruki-cloud/database/sekai/level"
)

func (p *dbEducationProvider) GetBonds(ctx context.Context) []*Bond {
	if !p.ensureBondMasterLoaded(ctx) {
		return nil
	}

	p.bondMu.RLock()
	defer p.bondMu.RUnlock()
	return cloneEdBonds(p.bonds)
}

func (p *dbEducationProvider) GetBondLevels(ctx context.Context) []*BondLevel {
	if !p.ensureBondMasterLoaded(ctx) {
		return nil
	}

	p.bondMu.RLock()
	defer p.bondMu.RUnlock()
	return cloneEdBondLevels(p.bondLevels)
}

func (p *dbEducationProvider) GetGameCharacterStyle(ctx context.Context, gameID int) *GameCharacterStyle {
	if gameID <= 0 || !p.ensureGameCharacterStylesLoaded(ctx) {
		return nil
	}

	p.styleMu.RLock()
	defer p.styleMu.RUnlock()
	return cloneEdGameCharacterStyle(p.stylesByGameID[gameID])
}

func (p *dbEducationProvider) GetCharacterMissions(ctx context.Context, characterID int) []*CharacterMission {
	if characterID <= 0 || !p.ensureLeaderMissionsLoaded(ctx) {
		return nil
	}

	p.missionMu.RLock()
	defer p.missionMu.RUnlock()
	return cloneEdCharacterMissions(p.characterMissionsByCharacter[characterID])
}

func (p *dbEducationProvider) GetCharacterMissionParameterGroups(ctx context.Context, parameterGroupID int) []*CharacterMissionParameterGroup {
	if parameterGroupID <= 0 || !p.ensureLeaderMissionsLoaded(ctx) {
		return nil
	}

	p.missionMu.RLock()
	defer p.missionMu.RUnlock()
	return cloneEdCharacterMissionParameterGroups(p.characterMissionGroupsByID[parameterGroupID])
}

func (p *dbEducationProvider) GetLeaderMissionRequirements(ctx context.Context) ([]LeaderMissionRequirement, int) {
	if !p.ensureLeaderMissionsLoaded(ctx) {
		return nil, 0
	}

	p.missionMu.RLock()
	defer p.missionMu.RUnlock()
	return cloneEdLeaderMissionRequirements(p.leaderRequirements), p.leaderMaxPlayLimit
}

func (p *dbEducationProvider) ensureBondMasterLoaded(ctx context.Context) bool {
	p.init()
	p.bondMu.RLock()
	if p.bondsLoaded {
		p.bondMu.RUnlock()
		return true
	}
	p.bondMu.RUnlock()

	p.bondMu.Lock()
	defer p.bondMu.Unlock()

	if p.bondsLoaded {
		return true
	}

	items, err := p.client.Bond.Query().
		Where(bond.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	p.bonds = make([]*Bond, 0, len(items))
	for _, item := range items {
		p.bonds = append(p.bonds, &Bond{
			GroupID:      int(item.GroupID),
			CharacterID1: int(item.CharacterId1),
			CharacterID2: int(item.CharacterId2),
		})
	}

	levels, err := p.client.Level.Query().
		Where(level.ServerRegionEQ(p.region.String()), level.LevelTypeEQ("bonds")).
		All(ctx)
	if err != nil {
		return false
	}
	p.bondLevels = make([]*BondLevel, 0, len(levels))
	for _, item := range levels {
		p.bondLevels = append(p.bondLevels, &BondLevel{
			Level:    int(item.Level),
			TotalExp: int(item.TotalExp),
		})
	}

	p.bondsLoaded = true
	return true
}

func (p *dbEducationProvider) ensureGameCharacterStylesLoaded(ctx context.Context) bool {
	p.init()
	p.styleMu.RLock()
	if p.stylesLoaded {
		p.styleMu.RUnlock()
		return true
	}
	p.styleMu.RUnlock()

	p.styleMu.Lock()
	defer p.styleMu.Unlock()

	if p.stylesLoaded {
		return true
	}

	items, err := p.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range items {
		p.stylesByGameID[int(item.GameID)] = &GameCharacterStyle{
			GameID:      int(item.GameID),
			CharacterID: int(item.GameCharacterID),
			ColorCode:   strings.TrimSpace(item.ColorCode),
		}
	}

	p.stylesLoaded = true
	return true
}

func (p *dbEducationProvider) ensureLeaderMissionsLoaded(ctx context.Context) bool {
	p.init()
	p.missionMu.RLock()
	if p.leaderMissionsLoaded {
		p.missionMu.RUnlock()
		return true
	}
	p.missionMu.RUnlock()

	p.missionMu.Lock()
	defer p.missionMu.Unlock()

	if p.leaderMissionsLoaded {
		return true
	}

	if p.store != nil && p.store.Configured() {
		if missions, err := p.store.loadJSON[localCharacterMissionJSON]("characterMissionV2s.json"); err == nil {
			for _, item := range missions {
				mission := &CharacterMission{
					ID:                   item.ID,
					CharacterID:          item.CharacterID,
					CharacterMissionType: item.CharacterMissionType,
					ParameterGroupID:     item.ParameterGroupID,
					IsAchievementMission: item.IsAchievementMission,
				}
				p.characterMissionsByCharacter[mission.CharacterID] = append(p.characterMissionsByCharacter[mission.CharacterID], mission)
			}
		}
	}

	items, err := p.client.Charactermissionv2Parametergroup.Query().
		Where(
			charactermissionv2parametergroup.ServerRegionEQ(p.region.String()),
		).
		Order(charactermissionv2parametergroup.ByID(), charactermissionv2parametergroup.ByGameID(), charactermissionv2parametergroup.BySeq()).
		All(ctx)
	if err != nil {
		return false
	}
	p.leaderRequirements = make([]LeaderMissionRequirement, 0)
	for _, item := range items {
		group := &CharacterMissionParameterGroup{
			GameID:      int(item.GameID),
			Seq:         int(item.Seq),
			Requirement: int(item.Requirement),
			Exp:         int(item.Exp),
			Quantity:    int(item.Quantity),
		}
		p.characterMissionGroupsByID[int(item.GameID)] = append(p.characterMissionGroupsByID[int(item.GameID)], group)
		switch item.GameID {
		case 1:
			if requirement := int(item.Requirement); requirement > p.leaderMaxPlayLimit {
				p.leaderMaxPlayLimit = requirement
			}
		case 101:
			p.leaderRequirements = append(p.leaderRequirements, LeaderMissionRequirement{
				Seq:         int(item.Seq),
				Requirement: int(item.Requirement),
			})
		}
	}

	p.leaderMissionsLoaded = true
	return true
}

func cloneEdBonds(source []*Bond) []*Bond {
	if len(source) == 0 {
		return nil
	}
	out := make([]*Bond, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, new(*item))
	}
	return out
}

func cloneEdBondLevels(source []*BondLevel) []*BondLevel {
	if len(source) == 0 {
		return nil
	}
	out := make([]*BondLevel, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, new(*item))
	}
	return out
}

func cloneEdGameCharacterStyle(source *GameCharacterStyle) *GameCharacterStyle {
	if source == nil {
		return nil
	}
	return new(*source)
}

func cloneEdLeaderMissionRequirements(source []LeaderMissionRequirement) []LeaderMissionRequirement {
	if len(source) == 0 {
		return nil
	}
	return slices.Clone(source)
}

func cloneEdCharacterMissions(source []*CharacterMission) []*CharacterMission {
	if len(source) == 0 {
		return nil
	}
	out := make([]*CharacterMission, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, new(*item))
	}
	return out
}

func cloneEdCharacterMissionParameterGroups(source []*CharacterMissionParameterGroup) []*CharacterMissionParameterGroup {
	if len(source) == 0 {
		return nil
	}
	out := make([]*CharacterMissionParameterGroup, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, new(*item))
	}
	return out
}
