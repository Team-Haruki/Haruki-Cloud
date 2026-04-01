package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	sekaidb "haruki-cloud/database/sekai"
	bonddb "haruki-cloud/database/sekai/bond"
	charactermissionv2parametergroupdb "haruki-cloud/database/sekai/charactermissionv2parametergroup"
	gamecharacterunitdb "haruki-cloud/database/sekai/gamecharacterunit"
	leveldb "haruki-cloud/database/sekai/level"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/education"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
	sekaiutils "haruki-cloud/utils/sekai"
)

func executeEducation(rc *RequestContext) (message onebot11.Message, err error) {
	var data []byte
	region := renderregion.Value(rc.Cmd.Region)
	regionStr := rc.RegionStr
	publicDetailedProfile := rc.GetDetailedProfile()

	// Resolve user binding and fetch suite data from Toolbox.
	platform := rc.Platform
	platformUserID := rc.PlatformUserID
	var suiteUID int64
	var suitePlatform, suitePlatformUserID string
	var suiteBinding *accountdata.ResolvedBinding

	_, binding, _ := resolveBindingWithFallback(
		rc.Ctx, rc.App.Bindings, platform, platformUserID, regionStr, rc.Cmd.RegionExplicit,
		bindingResolutionOptions{RequireSuite: true},
	)
	if binding != nil {
		if uid, convErr := strconv.ParseInt(binding.PJSKUserID, 10, 64); convErr == nil {
			suiteUID = uid
			suitePlatform = platform
			suitePlatformUserID = platformUserID
			suiteBinding = binding
		}
	}

	switch rc.Cmd.Mode {
	case "education-challenge":
		q := education.ChallengeLiveQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = publicDetailedProfile
		if suiteUID > 0 {
			suiteJSON, suiteErr := sekaiutils.GetToolboxClient().GetSuiteData(
				regionStr, suiteUID, suitePlatform, suitePlatformUserID)
			if suiteErr == nil && len(suiteJSON) > 0 {
				snapshot, buildErr := buildEducationSnapshot(rc.App, region, suiteJSON)
				if buildErr == nil {
					q.Snapshot = snapshot
				}
			}
		}
		data, err = rc.App.Edu.RenderChallengeLiveDetails(q)

	case "education-bonds":
		query := education.BondsQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
			query.Cid, err = resolveEducationBondsCharacterID(rc.Ctx, rc.App, region, query.CharacterQuery)
			if err != nil {
				return nil, err
			}
		}

		req := drawing.BondsRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.Bonds) == 0 && suiteUID > 0 {
			bondsReq, buildErr := buildBondsRequestFromSuite(
				rc.Ctx, rc.App, regionStr, suiteUID, suitePlatform, suitePlatformUserID, publicDetailedProfile, query.Cid)
			if buildErr == nil {
				req = *bondsReq
			}
		}
		data, err = rc.App.Edu.RenderBonds(req)

	case "education-leader":
		req := drawing.LeaderCountRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.LeaderCounts) == 0 && suiteUID > 0 {
			leaderReq, buildErr := buildLeaderCountRequestFromSuite(
				rc.Ctx, rc.App, regionStr, suiteUID, suitePlatform, suitePlatformUserID, publicDetailedProfile)
			if buildErr == nil {
				req = *leaderReq
			}
		}
		data, err = rc.App.Edu.RenderLeaderCount(req)

	case "education-power":
		req := drawing.PowerBonusDetailRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.CharaBonuses) == 0 && len(req.UnitBonuses) == 0 && len(req.AttrBonuses) == 0 && suiteUID > 0 {
			var powerMysekaiJSON []byte
			if hasUsableMySekaiData(suiteBinding) {
				powerMysekaiJSON, _ = sekaiutils.GetToolboxClient().GetMySekaiData(regionStr, suiteUID, suitePlatform, suitePlatformUserID)
			}
			builtReq, buildErr := buildPowerBonusRequestFromSuite(
				rc.App, region, regionStr, suiteUID, suitePlatform, suitePlatformUserID, powerMysekaiJSON, publicDetailedProfile)
			if buildErr != nil {
				return nil, buildErr
			}
			req = *builtReq
		}
		data, err = rc.App.Edu.RenderPowerBonusDetail(req)

	case "education-area":
		query := education.AreaItemQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
			query.Cid, err = resolveEducationAreaCharacterID(rc.Ctx, rc.App, region, query.CharacterQuery)
			if err != nil {
				return nil, err
			}
		}
		query.Profile = publicDetailedProfile
		if suiteUID > 0 {
			builtReq, buildErr := buildAreaItemUpgradeMaterialsRequestFromSuite(
				rc.App, query, regionStr, suiteUID, suitePlatform, suitePlatformUserID)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = rc.App.Edu.RenderAreaItemUpgradeMaterials(*builtReq)
			break
		}
		data, err = rc.App.Edu.RenderAreaItemUpgradeMaterials(drawing.AreaItemUpgradeMaterialsRequest{})

	default:
		return nil, unsupportedModeError("education", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

// buildEducationSnapshot creates a userdata.Service from live Toolbox suite data.
func buildEducationSnapshot(app *renderapp.App, region renderregion.Value, suiteJSON []byte) (*userdata.Service, error) {
	return userdata.NewFromBytes(app.Sekai, app.Assets, region, suiteJSON, nil, nil)
}

func buildPowerBonusRequestFromSuite(
	app *renderapp.App, region renderregion.Value, regionStr string, uid int64, platform, platformUserID string,
	mysekaiJSON []byte,
	profile *drawing.DetailedProfileCardRequest,
) (*drawing.PowerBonusDetailRequest, error) {
	suiteJSON, err := sekaiutils.GetToolboxClient().GetSuiteData(regionStr, uid, platform, platformUserID)
	if err != nil {
		return nil, fmt.Errorf("fetch suite data: %w", err)
	}
	if len(suiteJSON) == 0 {
		return nil, fmt.Errorf("suite data is empty")
	}
	snapshot, err := userdata.NewFromBytes(app.Sekai, app.Assets, region, suiteJSON, mysekaiJSON, nil)
	if err != nil {
		return nil, err
	}
	return app.Edu.BuildPowerBonusDetailRequestFromSnapshot(education.PowerBonusQuery{
		Region:   region,
		Profile:  profile,
		Snapshot: snapshot,
	})
}

func buildAreaItemUpgradeMaterialsRequestFromSuite(
	app *renderapp.App, query education.AreaItemQuery, regionStr string, uid int64, platform, platformUserID string,
) (*drawing.AreaItemUpgradeMaterialsRequest, error) {
	snapshot, err := buildEducationSnapshotFromSuite(app, query.Region, regionStr, uid, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	query.Snapshot = snapshot
	return app.Edu.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(query)
}

func buildEducationSnapshotFromSuite(
	app *renderapp.App, region renderregion.Value, regionStr string, uid int64, platform, platformUserID string,
) (*userdata.Service, error) {
	suiteJSON, err := sekaiutils.GetToolboxClient().GetSuiteData(regionStr, uid, platform, platformUserID)
	if err != nil {
		return nil, fmt.Errorf("fetch suite data: %w", err)
	}
	if len(suiteJSON) == 0 {
		return nil, fmt.Errorf("suite data is empty")
	}
	snapshot, err := buildEducationSnapshot(app, region, suiteJSON)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// buildBondsRequestFromSuite fetches bonds data from the Toolbox and builds a BondsRequest.
func buildBondsRequestFromSuite(
	ctx context.Context, app *renderapp.App, region string, uid int64, platform, platformUserID string,
	profile *drawing.DetailedProfileCardRequest,
	targetCharacterID int,
) (*drawing.BondsRequest, error) {
	tc := sekaiutils.GetToolboxClient()

	bondsRaw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userBonds")
	if err != nil {
		return nil, fmt.Errorf("fetch userBonds: %w", err)
	}
	charsRaw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userCharacters")
	if err != nil {
		return nil, fmt.Errorf("fetch userCharacters: %w", err)
	}

	var suiteBonds []struct {
		BondsGroupID int `json:"bondsGroupId"`
		Rank         int `json:"rank"`
		Exp          int `json:"exp"`
	}
	if err := json.Unmarshal(bondsRaw, &suiteBonds); err != nil {
		return nil, fmt.Errorf("decode userBonds: %w", err)
	}

	var suiteChars []struct {
		CharacterID   int `json:"characterId"`
		CharacterRank int `json:"characterRank"`
	}
	if err := json.Unmarshal(charsRaw, &suiteChars); err != nil {
		return nil, fmt.Errorf("decode userCharacters: %w", err)
	}

	charRankMap := make(map[int]int, len(suiteChars))
	for _, c := range suiteChars {
		charRankMap[c.CharacterID] = c.CharacterRank
	}

	// Look up bonds master data to map group IDs to character pairs.
	normalizedRegion := regionWithDefault(region)

	bondsMaster, err := app.Sekai.Bond.Query().
		Where(bonddb.ServerRegionEQ(normalizedRegion)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query bonds master: %w", err)
	}
	type bondPair struct {
		CharID1 int
		CharID2 int
	}
	groupToPair := make(map[int]bondPair, len(bondsMaster))
	for _, b := range bondsMaster {
		groupToPair[int(b.GroupID)] = bondPair{CharID1: int(b.CharacterId1), CharID2: int(b.CharacterId2)}
	}

	type suiteBondEntry struct {
		BondsGroupID int
		Rank         int
		Exp          int
	}
	userBondByGroupID := make(map[int]suiteBondEntry, len(suiteBonds))
	for _, sb := range suiteBonds {
		userBondByGroupID[sb.BondsGroupID] = suiteBondEntry{
			BondsGroupID: sb.BondsGroupID,
			Rank:         sb.Rank,
			Exp:          sb.Exp,
		}
	}

	bondLevels, err := app.Sekai.Level.Query().
		Where(
			leveldb.ServerRegionEQ(normalizedRegion),
			leveldb.LevelTypeEQ("bonds"),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query bonds levels: %w", err)
	}
	levelTotalExp := make(map[int]int, len(bondLevels))
	maxLevel := 0
	for _, item := range bondLevels {
		levelValue := int(item.Level)
		levelTotalExp[levelValue] = int(item.TotalExp)
		if levelValue > maxLevel {
			maxLevel = levelValue
		}
	}

	selectedPairs := make([]bondPair, 0, len(suiteBonds))
	selectedState := make([]suiteBondEntry, 0, len(suiteBonds))
	requiredCharIDs := make(map[int]struct{}, len(suiteBonds)*2)
	if targetCharacterID > 0 {
		for _, master := range bondsMaster {
			if master == nil {
				continue
			}
			pair := bondPair{CharID1: int(master.CharacterId1), CharID2: int(master.CharacterId2)}
			if pair.CharID1 != targetCharacterID && pair.CharID2 != targetCharacterID {
				continue
			}
			if pair.CharID1 != targetCharacterID {
				pair.CharID1, pair.CharID2 = pair.CharID2, pair.CharID1
			}
			selectedPairs = append(selectedPairs, pair)
			selectedState = append(selectedState, userBondByGroupID[int(master.GroupID)])
			requiredCharIDs[pair.CharID1] = struct{}{}
			requiredCharIDs[pair.CharID2] = struct{}{}
		}
	} else {
		for _, sb := range suiteBonds {
			pair, ok := groupToPair[sb.BondsGroupID]
			if !ok {
				continue
			}
			selectedPairs = append(selectedPairs, pair)
			selectedState = append(selectedState, userBondByGroupID[sb.BondsGroupID])
			requiredCharIDs[pair.CharID1] = struct{}{}
			requiredCharIDs[pair.CharID2] = struct{}{}
		}
	}

	// Map game_id to game_character_id for icon paths (e.g. 46 to actual 1-26 range ID).
	gameIDToCharID := make(map[int]int, len(requiredCharIDs))
	charColorMap := make(map[int][]int, len(requiredCharIDs))
	if len(requiredCharIDs) > 0 {
		charIDs := make([]int64, 0, len(requiredCharIDs))
		for charID := range requiredCharIDs {
			charIDs = append(charIDs, int64(charID))
		}
		sort.Slice(charIDs, func(i, j int) bool { return charIDs[i] < charIDs[j] })

		colorRows, err := app.Sekai.Gamecharacterunit.Query().
			Where(
				gamecharacterunitdb.ServerRegionEQ(normalizedRegion),
				gamecharacterunitdb.GameIDIn(charIDs...),
			).
			Order(gamecharacterunitdb.ByID()).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("query gamecharacterunit colors: %w", err)
		}
		for _, row := range colorRows {
			gameID := int(row.GameID)
			charID := int(row.GameCharacterID)
			if charID > 0 {
				gameIDToCharID[gameID] = charID
			}
			if _, ok := charColorMap[gameID]; ok {
				continue
			}
			charColorMap[gameID] = parseBondColorCode(row.ColorCode)
		}
	}

	// resolveCharIcon maps a game_id to its icon path via game_character_id
	resolveCharIcon := func(gameID int) string {
		if mapped, ok := gameIDToCharID[gameID]; ok {
			return charaIconPath(app.Assets, mapped)
		}
		return charaIconPath(app.Assets, gameID)
	}
	resolveBondSortCharacterID := func(gameID int) int {
		if mapped, ok := gameIDToCharID[gameID]; ok && mapped > 0 {
			return mapped
		}
		return gameID
	}

	bonds := make([]drawing.BondInfo, 0, len(selectedPairs))
	userMaxLevel := 0
	for idx, pair := range selectedPairs {
		state := selectedState[idx]
		if state.Rank > userMaxLevel {
			userMaxLevel = state.Rank
		}

		info := drawing.BondInfo{
			CharaID1:       pair.CharID1,
			CharaID2:       pair.CharID2,
			CharaIconPath1: resolveCharIcon(pair.CharID1),
			CharaIconPath2: resolveCharIcon(pair.CharID2),
			CharaRank1:     charRankMap[pair.CharID1],
			CharaRank2:     charRankMap[pair.CharID2],
			BondLevel:      state.Rank,
			HasBond:        state.BondsGroupID != 0,
			Color1:         defaultBondColor(),
			Color2:         defaultBondColor(),
		}
		if color, ok := charColorMap[pair.CharID1]; ok {
			info.Color1 = color
		}
		if color, ok := charColorMap[pair.CharID2]; ok {
			info.Color2 = color
		}
		if state.Rank > 0 && state.Rank < maxLevel {
			currentTotalExp, okCurrent := levelTotalExp[state.Rank]
			nextTotalExp, okNext := levelTotalExp[state.Rank+1]
			if okCurrent && okNext {
				needExp := nextTotalExp - currentTotalExp - state.Exp
				if needExp < 0 {
					needExp = 0
				}
				info.NeedExp = &needExp
			}
		}
		bonds = append(bonds, info)
	}
	if targetCharacterID > 0 {
		deduped := make([]drawing.BondInfo, 0, len(bonds))
		indexByDisplayRight := make(map[int]int, len(bonds))
		betterBondInfo := func(current, candidate drawing.BondInfo) bool {
			if candidate.BondLevel != current.BondLevel {
				return candidate.BondLevel > current.BondLevel
			}
			if candidate.HasBond != current.HasBond {
				return candidate.HasBond
			}
			rightCurrent := resolveBondSortCharacterID(current.CharaID2)
			rightCandidate := resolveBondSortCharacterID(candidate.CharaID2)
			if rightCandidate != rightCurrent {
				return rightCandidate < rightCurrent
			}
			return candidate.CharaID2 < current.CharaID2
		}
		for _, bond := range bonds {
			displayRight := resolveBondSortCharacterID(bond.CharaID2)
			if idx, ok := indexByDisplayRight[displayRight]; ok {
				if betterBondInfo(deduped[idx], bond) {
					deduped[idx] = bond
				}
				continue
			}
			indexByDisplayRight[displayRight] = len(deduped)
			deduped = append(deduped, bond)
		}
		bonds = deduped
	}
	if maxLevel == 0 {
		maxLevel = userMaxLevel
	}
	sort.Slice(bonds, func(i, j int) bool {
		if targetCharacterID > 0 {
			if bonds[i].BondLevel != bonds[j].BondLevel {
				return bonds[i].BondLevel > bonds[j].BondLevel
			}
			if bonds[i].HasBond != bonds[j].HasBond {
				return bonds[i].HasBond
			}
			rightI := resolveBondSortCharacterID(bonds[i].CharaID2)
			rightJ := resolveBondSortCharacterID(bonds[j].CharaID2)
			if rightI != rightJ {
				return rightI < rightJ
			}
			if bonds[i].CharaID2 != bonds[j].CharaID2 {
				return bonds[i].CharaID2 < bonds[j].CharaID2
			}
			return bonds[i].CharaID1 < bonds[j].CharaID1
		}
		if bonds[i].BondLevel != bonds[j].BondLevel {
			return bonds[i].BondLevel > bonds[j].BondLevel
		}
		if bonds[i].CharaID1 != bonds[j].CharaID1 {
			return bonds[i].CharaID1 < bonds[j].CharaID1
		}
		return bonds[i].CharaID2 < bonds[j].CharaID2
	})

	req := &drawing.BondsRequest{
		Bonds:    bonds,
		MaxLevel: maxLevel,
	}
	if profile != nil {
		req.Profile = *profile
	}
	return req, nil
}

// buildLeaderCountRequestFromSuite fetches leader usage data from Toolbox and builds a LeaderCountRequest.
func buildLeaderCountRequestFromSuite(
	ctx context.Context, app *renderapp.App, region string, uid int64, platform, platformUserID string,
	profile *drawing.DetailedProfileCardRequest,
) (*drawing.LeaderCountRequest, error) {
	tc := sekaiutils.GetToolboxClient()
	playCountByCharacter := make(map[int]int, 26)

	var missionGroups []leaderMissionRequirement
	maxPlayLimit := 0
	if app != nil && app.Sekai != nil {
		var missionErr error
		missionGroups, maxPlayLimit, missionErr = loadLeaderMissionRequirements(ctx, app.Sekai, region)
		if missionErr != nil {
			return nil, missionErr
		}
	}

	exCountByCharacter := make(map[int]int)
	exLevelByCharacter := make(map[int]int)
	hasPlayLiveMission := false

	if raw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userCharacterMissionV2s"); err == nil {
		var missions []struct {
			CharacterMissionType string `json:"characterMissionType"`
			CharacterID          int    `json:"characterId"`
			Progress             int    `json:"progress"`
		}
		if err := json.Unmarshal(raw, &missions); err != nil {
			return nil, fmt.Errorf("decode userCharacterMissionV2s: %w", err)
		}
		for _, item := range missions {
			if item.CharacterID <= 0 {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(item.CharacterMissionType)) {
			case "play_live":
				playCountByCharacter[item.CharacterID] = item.Progress
				hasPlayLiveMission = true
			case "play_live_ex":
				exCountByCharacter[item.CharacterID] = item.Progress
				if _, ok := exLevelByCharacter[item.CharacterID]; !ok {
					exLevelByCharacter[item.CharacterID] = 0
				}
			}
		}
	}

	if !hasPlayLiveMission {
		raw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userCharacterLiveUsageCounts")
		if err != nil {
			return nil, fmt.Errorf("fetch userCharacterMissionV2s: %w", err)
		}

		var usageCounts []struct {
			CharacterID            int    `json:"characterId"`
			CharacterLiveUsageType string `json:"characterLiveUsageType"`
			UsageCount             int    `json:"usageCount"`
		}
		if err := json.Unmarshal(raw, &usageCounts); err != nil {
			return nil, fmt.Errorf("decode userCharacterLiveUsageCounts: %w", err)
		}
		for _, item := range usageCounts {
			if item.CharacterID <= 0 || !strings.EqualFold(item.CharacterLiveUsageType, "leader") {
				continue
			}
			playCountByCharacter[item.CharacterID] = item.UsageCount
		}
	}

	if raw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userCharacterMissionV2Statuses"); err == nil {
		var statuses []struct {
			ParameterGroupID int `json:"parameterGroupId"`
			Seq              int `json:"seq"`
			CharacterID      int `json:"characterId"`
		}
		if err := json.Unmarshal(raw, &statuses); err != nil {
			return nil, fmt.Errorf("decode userCharacterMissionV2Statuses: %w", err)
		}
		for _, item := range statuses {
			if item.CharacterID <= 0 || item.ParameterGroupID != 101 {
				continue
			}
			if item.Seq > exLevelByCharacter[item.CharacterID] {
				exLevelByCharacter[item.CharacterID] = item.Seq
			}
			exCountByCharacter[item.CharacterID] += leaderMissionRequirementForSeq(missionGroups, item.Seq)
		}
	}

	leaders := make([]drawing.LeaderCountInfo, 0, 26)
	for charID := 1; charID <= 26; charID++ {
		playCount := playCountByCharacter[charID]
		leaders = append(leaders, drawing.LeaderCountInfo{
			CharaID:       charID,
			CharaIconPath: charaIconPath(app.Assets, charID),
			PlayCount:     playCount,
			ExLevel:       exLevelByCharacter[charID],
			ExCount:       exCountByCharacter[charID],
		})
	}
	sort.SliceStable(leaders, func(i, j int) bool {
		totalI := leaders[i].PlayCount + leaders[i].ExCount
		totalJ := leaders[j].PlayCount + leaders[j].ExCount
		if totalI == totalJ {
			return leaders[i].CharaID < leaders[j].CharaID
		}
		return totalI > totalJ
	})

	maxPlay := maxPlayLimit
	if maxPlay <= 0 {
		for _, item := range leaders {
			if item.PlayCount > maxPlay {
				maxPlay = item.PlayCount
			}
		}
	}

	req := &drawing.LeaderCountRequest{
		LeaderCounts: leaders,
		MaxPlayCount: maxPlay,
	}
	if profile != nil {
		req.Profile = *profile
	}
	return req, nil
}

type leaderMissionRequirement struct {
	Seq         int
	Requirement int
}

func loadLeaderMissionRequirements(ctx context.Context, client *sekaidb.Client, region string) ([]leaderMissionRequirement, int, error) {
	if client == nil {
		return nil, 0, nil
	}

	normalizedRegion := strings.ToLower(strings.TrimSpace(region))
	if normalizedRegion == "" {
		normalizedRegion = DefaultRegionStr
	}

	groups, err := client.Charactermissionv2Parametergroup.Query().
		Where(
			charactermissionv2parametergroupdb.ServerRegionEQ(normalizedRegion),
			charactermissionv2parametergroupdb.GameIDIn(1, 101),
		).
		Order(charactermissionv2parametergroupdb.ByGameID(), charactermissionv2parametergroupdb.BySeq()).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query charactermissionv2parametergroups: %w", err)
	}

	requirements := make([]leaderMissionRequirement, 0)
	maxPlayLimit := 0
	for _, item := range groups {
		switch item.GameID {
		case 1:
			if requirement := int(item.Requirement); requirement > maxPlayLimit {
				maxPlayLimit = requirement
			}
		case 101:
			requirements = append(requirements, leaderMissionRequirement{
				Seq:         int(item.Seq),
				Requirement: int(item.Requirement),
			})
		}
	}
	return requirements, maxPlayLimit, nil
}

func leaderMissionRequirementForSeq(requirements []leaderMissionRequirement, seq int) int {
	if seq <= 0 || len(requirements) == 0 {
		return 0
	}

	result := 0
	for _, item := range requirements {
		if item.Seq > seq {
			break
		}
		result = item.Requirement
	}
	return result
}

// charaIconPath resolves a character icon path using the asset helper.
func charaIconPath(helper *assets.AssetHelper, charID int) string {
	if nickname, ok := assets.CharacterIDToNickname[charID]; ok {
		return assets.ResolveAssetPath(helper, assets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
	}
	return assets.ResolveAssetPath(helper, assets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
}

func defaultBondColor() []int {
	return []int{100, 100, 100}
}

func parseBondColorCode(code string) []int {
	colorCode := strings.TrimSpace(strings.TrimPrefix(code, "#"))
	if len(colorCode) != 6 {
		return defaultBondColor()
	}

	result := make([]int, 3)
	for idx := 0; idx < 3; idx++ {
		value, err := strconv.ParseInt(colorCode[idx*2:idx*2+2], 16, 64)
		if err != nil {
			return defaultBondColor()
		}
		result[idx] = int(value)
	}
	return result
}
