package provider

import (
	"context"
	"slices"
	"strings"

	"haruki-cloud/database/sekai/bond"
	"haruki-cloud/database/sekai/charactermissionv2"
	"haruki-cloud/database/sekai/charactermissionv2parametergroup"
	"haruki-cloud/database/sekai/gamecharacterunit"
	"haruki-cloud/database/sekai/level"
	"haruki-cloud/internal/pjsk/render/cachefill"
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

// The mission getters serve the database cache; when the fill failed (not
// when the table is empty, which the fill handles) they serve the local
// files for the request instead, when a store is configured, and otherwise
// report the failure wrapped in cachefill.ErrUnavailable so the command
// fails instead of rendering no missions.

func (p *dbEducationProvider) GetCharacterMissions(ctx context.Context, characterID int) ([]*CharacterMission, error) {
	if characterID <= 0 {
		return nil, nil
	}
	if err := p.ensureLeaderMissionsLoaded(ctx); err != nil {
		if local := p.localFallback(); local != nil {
			return local.GetCharacterMissions(ctx, characterID)
		}
		return nil, cachefill.Unavailable(err)
	}

	p.missionMu.RLock()
	defer p.missionMu.RUnlock()
	return cloneEdCharacterMissions(p.characterMissionsByCharacter[characterID]), nil
}

func (p *dbEducationProvider) GetCharacterMissionParameterGroups(ctx context.Context, parameterGroupID int) ([]*CharacterMissionParameterGroup, error) {
	if parameterGroupID <= 0 {
		return nil, nil
	}
	if err := p.ensureLeaderMissionsLoaded(ctx); err != nil {
		if local := p.localFallback(); local != nil {
			return local.GetCharacterMissionParameterGroups(ctx, parameterGroupID)
		}
		return nil, cachefill.Unavailable(err)
	}

	p.missionMu.RLock()
	defer p.missionMu.RUnlock()
	return cloneEdCharacterMissionParameterGroups(p.characterMissionGroupsByID[parameterGroupID]), nil
}

func (p *dbEducationProvider) GetLeaderMissionRequirements(ctx context.Context) ([]LeaderMissionRequirement, int, error) {
	if err := p.ensureLeaderMissionsLoaded(ctx); err != nil {
		if local := p.localFallback(); local != nil {
			return local.GetLeaderMissionRequirements(ctx)
		}
		return nil, 0, cachefill.Unavailable(err)
	}

	p.missionMu.RLock()
	defer p.missionMu.RUnlock()
	return cloneEdLeaderMissionRequirements(p.leaderRequirements), p.leaderMaxPlayLimit, nil
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

// ensureLeaderMissionsLoaded fills the mission caches once, sharing the
// fill between concurrent callers; a failed fill is reported to the caller,
// leaves nothing loaded and is not retried until the backoff elapses.
func (p *dbEducationProvider) ensureLeaderMissionsLoaded(ctx context.Context) error {
	p.init()
	p.missionMu.RLock()
	loaded := p.leaderMissionsLoaded
	p.missionMu.RUnlock()
	if loaded {
		return nil
	}
	return p.fill.Do(ctx, "leaderMissions", p.loadLeaderMissions)
}

func (p *dbEducationProvider) loadLeaderMissions(ctx context.Context) error {
	p.missionMu.Lock()
	defer p.missionMu.Unlock()
	if p.leaderMissionsLoaded {
		return nil
	}

	missionsByCharacter := make(map[int][]*CharacterMission)
	missionsFromDB, err := p.loadCharacterMissionsFromDB(ctx, missionsByCharacter)
	if err != nil {
		return err
	}
	if !missionsFromDB && p.store != nil && p.store.Configured() {
		if missions, err := p.store.loadJSON[localCharacterMissionJSON]("characterMissionV2s.json"); err == nil {
			for _, item := range missions {
				mission := &CharacterMission{
					ID:                   item.ID,
					CharacterID:          item.CharacterID,
					CharacterMissionType: item.CharacterMissionType,
					ParameterGroupID:     item.ParameterGroupID,
					IsAchievementMission: item.IsAchievementMission,
				}
				missionsByCharacter[mission.CharacterID] = append(missionsByCharacter[mission.CharacterID], mission)
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
		return err
	}
	groupsByID := make(map[int][]*CharacterMissionParameterGroup)
	requirements := make([]LeaderMissionRequirement, 0)
	maxPlayLimit := 0
	for _, item := range items {
		group := &CharacterMissionParameterGroup{
			GameID:      int(item.GameID),
			Seq:         int(item.Seq),
			Requirement: int(item.Requirement),
			Exp:         int(item.Exp),
			Quantity:    int(item.Quantity),
		}
		groupsByID[int(item.GameID)] = append(groupsByID[int(item.GameID)], group)
		switch item.GameID {
		case 1:
			if requirement := int(item.Requirement); requirement > maxPlayLimit {
				maxPlayLimit = requirement
			}
		case 101:
			requirements = append(requirements, LeaderMissionRequirement{
				Seq:         int(item.Seq),
				Requirement: int(item.Requirement),
			})
		}
	}

	p.characterMissionsByCharacter = missionsByCharacter
	p.characterMissionGroupsByID = groupsByID
	p.leaderRequirements = requirements
	p.leaderMaxPlayLimit = maxPlayLimit
	p.leaderMissionsLoaded = true
	return nil
}

// loadCharacterMissionsFromDB fills the per-character mission index from
// charactermissionv2s. It reports false when the table has no rows for the
// region (not ingested yet) so the caller can fall back to the local
// characterMissionV2s.json; a query error is returned so nothing is marked
// loaded.
func (p *dbEducationProvider) loadCharacterMissionsFromDB(ctx context.Context, byCharacter map[int][]*CharacterMission) (bool, error) {
	items, err := p.client.Charactermissionv2.Query().
		Where(charactermissionv2.ServerRegionEQ(p.region.String())).
		Order(charactermissionv2.ByGameID()).
		All(ctx)
	if err != nil {
		return false, err
	}
	if len(items) == 0 {
		return false, nil
	}
	for _, item := range items {
		mission := &CharacterMission{
			ID:                   int(item.GameID),
			CharacterID:          int(item.CharacterID),
			CharacterMissionType: item.CharacterMissionType,
			ParameterGroupID:     int(item.ParameterGroupID),
			IsAchievementMission: item.IsAchievementMission,
		}
		byCharacter[mission.CharacterID] = append(byCharacter[mission.CharacterID], mission)
	}
	return true, nil
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
