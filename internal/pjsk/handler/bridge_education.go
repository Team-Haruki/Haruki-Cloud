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
	bonddb "haruki-cloud/database/sekai/bond"
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
	// Try global default binding first when no explicit region prefix.
	platform := rc.Platform
	platformUserID := rc.PlatformUserID
	var suiteUID int64
	var suitePlatform, suitePlatformUserID string
	var suiteBinding *accountdata.ResolvedBinding

	if platform != "" && platformUserID != "" && rc.App.Bindings != nil {
		ctx := rc.Ctx
		var binding *accountdata.ResolvedBinding
		var resolveErr error
		if !rc.Cmd.RegionExplicit {
			_, binding, resolveErr = rc.App.Bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
			if resolveErr != nil || binding == nil || !hasUsableSuiteData(binding) {
				_, binding, resolveErr = rc.App.Bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
			}
		} else {
			_, binding, resolveErr = rc.App.Bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
		}
		if resolveErr == nil && binding != nil && hasUsableSuiteData(binding) {
			if uid, convErr := strconv.ParseInt(binding.PJSKUserID, 10, 64); convErr == nil {
				suiteUID = uid
				suitePlatform = platform
				suitePlatformUserID = platformUserID
				suiteBinding = binding
			}
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
		req := drawing.BondsRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.Bonds) == 0 && suiteUID > 0 {
			bondsReq, buildErr := buildBondsRequestFromSuite(
				rc.Ctx, rc.App, regionStr, suiteUID, suitePlatform, suitePlatformUserID, publicDetailedProfile)
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
				rc.App, regionStr, suiteUID, suitePlatform, suitePlatformUserID, publicDetailedProfile)
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
		return nil, fmt.Errorf("bridge: unsupported education mode %q", rc.Cmd.Mode)
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

	requiredCharIDs := make(map[int]struct{}, len(suiteBonds)*2)
	for _, sb := range suiteBonds {
		pair, ok := groupToPair[sb.BondsGroupID]
		if !ok {
			continue
		}
		requiredCharIDs[pair.CharID1] = struct{}{}
		requiredCharIDs[pair.CharID2] = struct{}{}
	}

	// Map game_id → game_character_id for icon paths (e.g., 46 → actual 1-26 range ID)
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

	bonds := make([]drawing.BondInfo, 0, len(suiteBonds))
	userMaxLevel := 0
	for _, sb := range suiteBonds {
		pair, ok := groupToPair[sb.BondsGroupID]
		if !ok {
			continue
		}

		if sb.Rank > userMaxLevel {
			userMaxLevel = sb.Rank
		}

		info := drawing.BondInfo{
			CharaID1:       pair.CharID1,
			CharaID2:       pair.CharID2,
			CharaIconPath1: resolveCharIcon(pair.CharID1),
			CharaIconPath2: resolveCharIcon(pair.CharID2),
			CharaRank1:     charRankMap[pair.CharID1],
			CharaRank2:     charRankMap[pair.CharID2],
			BondLevel:      sb.Rank,
			HasBond:        true,
			Color1:         defaultBondColor(),
			Color2:         defaultBondColor(),
		}
		if color, ok := charColorMap[pair.CharID1]; ok {
			info.Color1 = color
		}
		if color, ok := charColorMap[pair.CharID2]; ok {
			info.Color2 = color
		}
		if sb.Rank > 0 && sb.Rank < maxLevel {
			currentTotalExp, okCurrent := levelTotalExp[sb.Rank]
			nextTotalExp, okNext := levelTotalExp[sb.Rank+1]
			if okCurrent && okNext {
				needExp := nextTotalExp - currentTotalExp - sb.Exp
				if needExp < 0 {
					needExp = 0
				}
				info.NeedExp = &needExp
			}
		}
		bonds = append(bonds, info)
	}
	if maxLevel == 0 {
		maxLevel = userMaxLevel
	}
	sort.Slice(bonds, func(i, j int) bool {
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
	app *renderapp.App, region string, uid int64, platform, platformUserID string,
	profile *drawing.DetailedProfileCardRequest,
) (*drawing.LeaderCountRequest, error) {
	tc := sekaiutils.GetToolboxClient()

	raw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userCharacterLiveUsageCounts")
	if err != nil {
		return nil, fmt.Errorf("fetch userCharacterLiveUsageCounts: %w", err)
	}

	var usageCounts []struct {
		CharacterID            int    `json:"characterId"`
		CharacterLiveUsageType string `json:"characterLiveUsageType"`
		UsageCount             int    `json:"usageCount"`
	}
	if err := json.Unmarshal(raw, &usageCounts); err != nil {
		return nil, fmt.Errorf("decode userCharacterLiveUsageCounts: %w", err)
	}

	// Group by character, pick leader counts.
	type charEntry struct {
		LeaderCount int
		MemberCount int
	}
	charMap := make(map[int]*charEntry)
	for _, u := range usageCounts {
		entry, ok := charMap[u.CharacterID]
		if !ok {
			entry = &charEntry{}
			charMap[u.CharacterID] = entry
		}
		switch u.CharacterLiveUsageType {
		case "leader":
			entry.LeaderCount = u.UsageCount
		case "member":
			entry.MemberCount = u.UsageCount
		}
	}

	leaders := make([]drawing.LeaderCountInfo, 0, len(charMap))
	maxPlay := 0
	for charID, entry := range charMap {
		if entry.LeaderCount > maxPlay {
			maxPlay = entry.LeaderCount
		}
		leaders = append(leaders, drawing.LeaderCountInfo{
			CharaID:       charID,
			CharaIconPath: charaIconPath(app.Assets, charID),
			PlayCount:     entry.LeaderCount,
		})
	}
	sort.Slice(leaders, func(i, j int) bool { return leaders[i].PlayCount > leaders[j].PlayCount })

	req := &drawing.LeaderCountRequest{
		LeaderCounts: leaders,
		MaxPlayCount: maxPlay,
	}
	if profile != nil {
		req.Profile = *profile
	}
	return req, nil
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
