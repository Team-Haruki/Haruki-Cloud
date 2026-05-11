package education

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

func (c *Controller) BuildCharacterMissionOverviewRequestFromSnapshot(query CharacterMissionQuery) (*drawing.CharacterMissionOverviewRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}
	if query.Cid <= 0 {
		return nil, fmt.Errorf("character mission request requires character id")
	}

	overview, err := c.buildCharacterMissionOverview(ctx, query.Cid)
	if err != nil {
		return nil, err
	}
	return overview, nil
}

func (c *Controller) BuildCharacterMissionAllRequestFromSnapshot(query CharacterMissionQuery) (*drawing.CharacterMissionAllRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}
	if query.Cid <= 0 {
		return nil, fmt.Errorf("character mission request requires character id")
	}
	if strings.TrimSpace(query.MissionType) == "" {
		return nil, fmt.Errorf("character mission all request requires mission type")
	}

	request, err := c.buildCharacterMissionAll(ctx, query.Cid, strings.TrimSpace(query.MissionType))
	if err != nil {
		return nil, err
	}
	return request, nil
}

func (c *Controller) RenderCharacterMissionOverview(req drawing.CharacterMissionOverviewRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	return c.drawing.GenerateCharacterMissionOverview(&req)
}

func (c *Controller) RenderCharacterMissionAll(req drawing.CharacterMissionAllRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	return c.drawing.GenerateCharacterMissionAll(&req)
}

func (c *Controller) buildCharacterMissionOverview(
	ctx *resolvedSnapshotContext,
	cid int,
) (*drawing.CharacterMissionOverviewRequest, error) {
	rows, name, iconPath, level, exp, pendingExp, finalLevel, finalExp, err := c.buildCharacterMissionRows(ctx, cid)
	if err != nil {
		return nil, err
	}

	byType := make(map[string]drawing.CharacterMissionOverviewRow, len(rows))
	for _, row := range rows {
		byType[row.MissionType] = row
	}

	basicRows := make([]drawing.CharacterMissionOverviewRow, 0)
	for _, missionType := range []string{
		"collect_member",
		"collect_stamp",
		"collect_costume_3d",
		"collect_character_archive_voice",
		"collect_another_vocal",
		"read_mysekai_fixture_unique_character_talk",
		"read_area_talk",
	} {
		if row, ok := byType[missionType]; ok {
			basicRows = append(basicRows, characterMissionOverviewRowClone(row))
		}
	}

	achievementRows := make([]drawing.CharacterMissionOverviewRow, 0)
	for _, missionType := range []string{
		"play_live",
		"play_live_ex",
		"waiting_room",
		"waiting_room_ex",
		"read_card_episode_first",
		"read_card_episode_second",
		"area_item_level_up_character",
		"area_item_level_up_unit",
		"area_item_level_up_reality_world",
		"skill_level_up_rare",
		"skill_level_up_standard",
		"master_rank_up_rare",
		"master_rank_up_standard",
		"collect_mysekai_fixture",
		"collect_mysekai_canvas",
	} {
		if row, ok := byType[missionType]; ok {
			achievementRows = append(achievementRows, characterMissionOverviewRowClone(row))
		}
	}

	return &drawing.CharacterMissionOverviewRequest{
		Profile:           *ctx.profile,
		CharacterID:       cid,
		CharacterName:     name,
		CharacterIconPath: iconPath,
		CurrentLevel:      level,
		CurrentExp:        exp,
		PendingExp:        pendingExp,
		FinalLevel:        finalLevel,
		FinalExp:          finalExp,
		BasicRows:         basicRows,
		AchievementRows:   achievementRows,
	}, nil
}

func (c *Controller) buildCharacterMissionAll(
	ctx *resolvedSnapshotContext,
	cid int,
	missionType string,
) (*drawing.CharacterMissionAllRequest, error) {
	rows, name, iconPath, _, _, _, _, _, err := c.buildCharacterMissionRows(ctx, cid)
	if err != nil {
		return nil, err
	}
	rowByType := make(map[string]drawing.CharacterMissionOverviewRow, len(rows))
	for _, row := range rows {
		rowByType[row.MissionType] = row
	}

	sectionTypes := []string{missionType}
	switch missionType {
	case "play_live":
		sectionTypes = []string{"play_live", "play_live_ex"}
	case "waiting_room":
		sectionTypes = []string{"waiting_room", "waiting_room_ex"}
	}

	sections := make([]drawing.CharacterMissionAllSection, 0, len(sectionTypes))
	for _, sectionType := range sectionTypes {
		base, ok := rowByType[sectionType]
		if !ok {
			return nil, fmt.Errorf("character mission type not found: %s", sectionType)
		}
		missionDef := findCharacterMissionByType(ctx.source.GetCharacterMissions(cid), sectionType)
		if missionDef == nil {
			return nil, fmt.Errorf("character mission definition not found: %s", sectionType)
		}
		groupRows := cloneCharacterMissionParameterGroups(ctx.source.GetCharacterMissionParameterGroups(missionDef.ParameterGroupID))
		sort.Slice(groupRows, func(i, j int) bool { return groupRows[i].Seq < groupRows[j].Seq })
		displayRows := make([]drawing.CharacterMissionAllTableRow, 0, len(groupRows))
		accRequirement := 0
		accExp := 0
		if _, ok := CharacterMissionExTypes[sectionType]; ok {
			maxRound := maxInt(derefInt(base.CurrentRound), characterMissionMaxExplicitSeq(groupRows))
			displayRows = make([]drawing.CharacterMissionAllTableRow, 0, maxRound)
			for roundNo := 1; roundNo <= maxRound; roundNo++ {
				requirement := characterMissionRequirementForRound(groupRows, roundNo)
				exp := characterMissionExpForRound(groupRows, roundNo)
				accRequirement += requirement
				accExp += exp
				displayRows = append(displayRows, drawing.CharacterMissionAllTableRow{
					Seq:            roundNo,
					Requirement:    requirement,
					AccRequirement: accRequirement,
					Exp:            exp,
					AccExp:         accExp,
				})
			}
		} else {
			for _, groupRow := range groupRows {
				if groupRow == nil {
					continue
				}
				accRequirement = groupRow.Requirement
				accExp += groupRow.Exp
				displayRows = append(displayRows, drawing.CharacterMissionAllTableRow{
					Seq:            groupRow.Seq,
					Requirement:    groupRow.Requirement,
					AccRequirement: accRequirement,
					Exp:            groupRow.Exp,
					AccExp:         accExp,
				})
			}
		}
		sections = append(sections, drawing.CharacterMissionAllSection{
			MissionType:          base.MissionType,
			Title:                base.Title,
			IsEx:                 base.IsEx,
			CurrentTotal:         base.Current,
			ReachedSeq:           calcCharacterMissionReachedSeq(displayRows, base.Current, base.IsEx, derefInt(base.CurrentRound)),
			CurrentRoundNo:       characterMissionUpperPtr(base.CurrentRound),
			CurrentRoundProgress: characterMissionUpperPtr(base.CurrentRoundProgress),
			CurrentRoundNeed:     characterMissionUpperPtr(base.CurrentRoundNeed),
			Upper:                characterMissionUpperPtr(base.Upper),
			Ratio:                base.Ratio,
			NextNeed:             characterMissionUpperPtr(base.NextNeed),
			NextExp:              characterMissionUpperPtr(base.NextExp),
			DisplayRows:          displayRows,
		})
	}

	title := CharacterMissionShortName(missionType)
	return &drawing.CharacterMissionAllRequest{
		Profile:           *ctx.profile,
		CharacterID:       cid,
		CharacterName:     name,
		CharacterIconPath: iconPath,
		Title:             title,
		Sections:          sections,
	}, nil
}

func (c *Controller) buildCharacterMissionRows(
	ctx *resolvedSnapshotContext,
	cid int,
) ([]drawing.CharacterMissionOverviewRow, string, string, int, int, int, int, int, error) {
	missions := cloneCharacterMissions(ctx.source.GetCharacterMissions(cid))
	if len(missions) == 0 {
		return nil, "", "", 0, 0, 0, 0, 0, fmt.Errorf("character mission data not found for character %d", cid)
	}
	sort.Slice(missions, func(i, j int) bool { return missions[i].ID < missions[j].ID })

	userCharacter := findRawUserCharacter(ctx.raw.UserCharacters, cid)
	currentLevel := 0
	currentExp := 0
	currentTotalExp := 0
	if userCharacter != nil {
		currentLevel = userCharacter.CharacterRank
		currentExp = userCharacter.Exp
		currentTotalExp = userCharacter.TotalExp
	}
	characterLevels := cloneCharacterLevels(ctx.source.GetCharacterLevels())
	levelTotalExpByLevel := make(map[int]int, len(characterLevels))
	for _, item := range characterLevels {
		if item == nil || item.Level <= 0 {
			continue
		}
		levelTotalExpByLevel[item.Level] = item.TotalExp
	}
	if currentLevel > 0 && currentTotalExp > 0 {
		if baseTotalExp, ok := levelTotalExpByLevel[currentLevel]; ok && currentTotalExp >= baseTotalExp {
			currentExp = currentTotalExp - baseTotalExp
		}
	}

	statuses := characterMissionStatusesForCharacter(ctx.raw, cid)
	pendingExp := 0
	for _, item := range statuses {
		if strings.EqualFold(strings.TrimSpace(item.MissionStatus), "achieved") {
			pendingExp += characterMissionGroupExp(ctx.source.GetCharacterMissionParameterGroups(item.ParameterGroupID), item.Seq)
		}
	}
	finalLevel := currentLevel
	finalExp := currentExp + pendingExp
	baseTotalExp := currentTotalExp
	if baseTotalExp <= 0 && currentLevel > 0 {
		if levelStart, ok := levelTotalExpByLevel[currentLevel]; ok {
			baseTotalExp = levelStart + currentExp
		}
	}
	if len(characterLevels) > 0 {
		finalTotalExp := maxInt(baseTotalExp, 0) + pendingExp
		finalLevel = 1
		levelStart := 0
		for _, item := range characterLevels {
			if item == nil || item.Level <= 0 {
				continue
			}
			if item.TotalExp <= finalTotalExp {
				finalLevel = item.Level
				levelStart = item.TotalExp
				continue
			}
			break
		}
		finalExp = finalTotalExp - levelStart
	}

	userByTypeProgress := make(map[string]int)
	for _, item := range ctx.raw.UserCharacterMissionV2s {
		if item.CharacterID != cid {
			continue
		}
		userByTypeProgress[item.CharacterMissionType] = maxInt(userByTypeProgress[item.CharacterMissionType], item.Progress)
	}

	statusSeqByMissionID := make(map[int]int)
	statusSeqByGroupID := make(map[int]int)
	for _, item := range statuses {
		statusSeqByMissionID[item.MissionID] = maxInt(statusSeqByMissionID[item.MissionID], item.Seq)
		statusSeqByGroupID[item.ParameterGroupID] = maxInt(statusSeqByGroupID[item.ParameterGroupID], item.Seq)
	}

	rows := make([]drawing.CharacterMissionOverviewRow, 0, len(missions))
	for _, mission := range missions {
		groups := cloneCharacterMissionParameterGroups(ctx.source.GetCharacterMissionParameterGroups(mission.ParameterGroupID))
		sort.Slice(groups, func(i, j int) bool { return groups[i].Seq < groups[j].Seq })
		isEx := isCharacterMissionExType(mission.CharacterMissionType)

		current := userByTypeProgress[mission.CharacterMissionType]
		if isEx {
			receivedSeq := maxInt(statusSeqByMissionID[mission.ID], statusSeqByGroupID[mission.ParameterGroupID])
			clearedTotal := characterMissionClearedTotal(groups, receivedSeq)
			if current > 0 {
				if current < clearedTotal {
					current = clearedTotal + current
				}
			} else {
				current = clearedTotal
			}
		} else if current <= 0 {
			receivedSeq := maxInt(statusSeqByMissionID[mission.ID], statusSeqByGroupID[mission.ParameterGroupID])
			if receivedSeq > 0 {
				current = characterMissionRequirementBySeq(groups, receivedSeq)
			}
		}

		upper := characterMissionUpper(groups, isEx)
		ratio := 0.0
		if upper != nil && *upper > 0 {
			if current > *upper {
				ratio = 1.0
			} else {
				ratio = float64(current) / float64(*upper)
			}
		}

		nextNeed, nextExp := characterMissionNextTarget(groups, current, isEx)
		currentRound, currentRoundProgress, currentRoundNeed := 0, 0, 0
		exRoundText := ""
		if isEx {
			currentRound, currentRoundProgress, currentRoundNeed = characterMissionCurrentRound(groups, current)
			exRoundText = fmt.Sprintf("EX %d 回目", currentRound)
		}

		row := drawing.CharacterMissionOverviewRow{
			MissionID:            mission.ID,
			MissionType:          mission.CharacterMissionType,
			Title:                CharacterMissionShortName(mission.CharacterMissionType),
			IsAchievement:        mission.IsAchievementMission,
			IsEx:                 isEx,
			Current:              current,
			Upper:                upper,
			Ratio:                ratio,
			NextNeed:             nextNeed,
			NextExp:              nextExp,
			CurrentRound:         characterMissionRoundPtr(currentRound),
			CurrentRoundProgress: characterMissionRoundPtr(currentRoundProgress),
			CurrentRoundNeed:     characterMissionRoundPtr(currentRoundNeed),
			ExDisplayRoundText:   characterMissionRoundTextPtr(exRoundText),
		}
		rows = append(rows, row)
	}

	iconPath := c.characterIconPath(cid)
	return rows, characterMissionDisplayName(cid), iconPath, currentLevel, currentExp, pendingExp, finalLevel, finalExp, nil
}

func isCharacterMissionExType(missionType string) bool {
	_, ok := CharacterMissionExTypes[missionType]
	return ok
}

func characterMissionDisplayName(cid int) string {
	if nickname, ok := characterIDDisplayNames[cid]; ok {
		return nickname
	}
	if nickname, ok := characterNicknameFallbacks[cid]; ok {
		return nickname
	}
	return fmt.Sprintf("角色%d", cid)
}

var characterIDDisplayNames = map[int]string{
	1:  "星乃一歌",
	2:  "天马咲希",
	3:  "望月穗波",
	4:  "日野森志步",
	5:  "花里实乃理",
	6:  "桐谷遥",
	7:  "桃井爱莉",
	8:  "日野森雫",
	9:  "小豆泽心羽",
	10: "白石杏",
	11: "东云彰人",
	12: "青柳冬弥",
	13: "天马司",
	14: "凤笑梦",
	15: "草薙宁宁",
	16: "神代类",
	17: "宵崎奏",
	18: "朝比奈真冬",
	19: "东云绘名",
	20: "晓山瑞希",
	21: "初音未来",
	22: "镜音铃",
	23: "镜音连",
	24: "巡音流歌",
	25: "MEIKO",
	26: "KAITO",
}

var characterNicknameFallbacks = map[int]string{
	1:  "ick",
	2:  "saki",
	3:  "hnm",
	4:  "shiho",
	5:  "mnr",
	6:  "hrk",
	7:  "airi",
	8:  "szk",
	9:  "khn",
	10: "an",
	11: "akt",
	12: "toya",
	13: "tks",
	14: "emu",
	15: "nene",
	16: "rui",
	17: "knd",
	18: "mfy",
	19: "ena",
	20: "mzk",
	21: "miku",
	22: "rin",
	23: "len",
	24: "luka",
	25: "meiko",
	26: "kaito",
}

func cloneCharacterMissions(source []*CharacterMission) []*CharacterMission {
	if len(source) == 0 {
		return nil
	}
	out := make([]*CharacterMission, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, &CharacterMission{
			ID:                   item.ID,
			CharacterID:          item.CharacterID,
			CharacterMissionType: item.CharacterMissionType,
			ParameterGroupID:     item.ParameterGroupID,
			IsAchievementMission: item.IsAchievementMission,
		})
	}
	return out
}

func cloneCharacterMissionParameterGroups(source []*CharacterMissionParameterGroup) []*CharacterMissionParameterGroup {
	if len(source) == 0 {
		return nil
	}
	out := make([]*CharacterMissionParameterGroup, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, &CharacterMissionParameterGroup{
			GameID:      item.GameID,
			Seq:         item.Seq,
			Requirement: item.Requirement,
			Exp:         item.Exp,
			Quantity:    item.Quantity,
		})
	}
	return out
}

func cloneCharacterLevels(source []*CharacterLevel) []*CharacterLevel {
	if len(source) == 0 {
		return nil
	}
	out := make([]*CharacterLevel, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, &CharacterLevel{
			Level:    item.Level,
			TotalExp: item.TotalExp,
		})
	}
	return out
}

func findRawUserCharacter(items []rendersnapshot.RawUserCharacter, cid int) *rendersnapshot.RawUserCharacter {
	for i := range items {
		if items[i].CharacterID == cid {
			return &items[i]
		}
	}
	return nil
}

func characterMissionStatusesForCharacter(raw *rendersnapshot.RawUserData, cid int) []rendersnapshot.RawUserCharacterMissionV2Status {
	if raw == nil || cid <= 0 {
		return nil
	}
	items := rendersnapshot.ResolveCharacterMissionV2Statuses(raw)
	result := make([]rendersnapshot.RawUserCharacterMissionV2Status, 0, len(items))
	for _, item := range items {
		if item.CharacterID == cid {
			result = append(result, item)
		}
	}
	return result
}

func findCharacterMissionByType(items []*CharacterMission, missionType string) *CharacterMission {
	for _, item := range items {
		if item != nil && item.CharacterMissionType == missionType {
			return item
		}
	}
	return nil
}

func characterMissionRequirementBySeq(groups []*CharacterMissionParameterGroup, seq int) int {
	if seq <= 0 {
		return 0
	}
	value := 0
	for _, item := range groups {
		if item == nil {
			continue
		}
		if item.Seq > seq {
			break
		}
		value = item.Requirement
	}
	return value
}

func characterMissionGroupExp(groups []*CharacterMissionParameterGroup, seq int) int {
	if seq <= 0 {
		return 0
	}
	value := 0
	for _, item := range groups {
		if item == nil {
			continue
		}
		if item.Seq > seq {
			break
		}
		value = item.Exp
	}
	return value
}

func characterMissionClearedTotal(groups []*CharacterMissionParameterGroup, seq int) int {
	if seq <= 0 {
		return 0
	}
	total := 0
	for roundNo := 1; roundNo <= seq; roundNo++ {
		total += characterMissionRequirementForRound(groups, roundNo)
	}
	return total
}

func characterMissionUpper(groups []*CharacterMissionParameterGroup, isEx bool) *int {
	if len(groups) == 0 {
		return nil
	}
	if isEx {
		total := 0
		for roundNo := 1; roundNo <= 30; roundNo++ {
			total += characterMissionRequirementForRound(groups, roundNo)
		}
		return characterMissionNextPtr(total)
	}
	maxRequirement := 0
	for _, item := range groups {
		if item == nil || item.Requirement <= maxRequirement {
			continue
		}
		maxRequirement = item.Requirement
	}
	return characterMissionNextPtr(maxRequirement)
}

func characterMissionCurrentRound(groups []*CharacterMissionParameterGroup, total int) (int, int, int) {
	total = maxInt(total, 0)
	roundNo := 1
	for {
		requirement := characterMissionRequirementForRound(groups, roundNo)
		if requirement <= 0 || total < requirement {
			return roundNo, total, requirement
		}
		total -= requirement
		roundNo++
	}
}

func characterMissionNextTarget(groups []*CharacterMissionParameterGroup, current int, isEx bool) (*int, *int) {
	if isEx {
		roundNo, inRoundProgress, roundNeed := characterMissionCurrentRound(groups, current)
		if roundNeed <= 0 {
			return nil, nil
		}
		nextNeed := current + maxInt(roundNeed-inRoundProgress, 0)
		nextExp := characterMissionExpForRound(groups, roundNo)
		return characterMissionNextPtr(nextNeed), characterMissionNextPtr(nextExp)
	}

	for _, item := range groups {
		if item == nil {
			continue
		}
		if item.Requirement > current {
			return characterMissionNextPtr(item.Requirement), characterMissionNextPtr(item.Exp)
		}
	}
	return nil, nil
}

func characterMissionRequirementForRound(groups []*CharacterMissionParameterGroup, roundNo int) int {
	if roundNo <= 0 {
		return 0
	}
	value := 0
	for _, item := range groups {
		if item == nil {
			continue
		}
		if item.Seq > roundNo {
			break
		}
		value = item.Requirement
	}
	return value
}

func characterMissionExpForRound(groups []*CharacterMissionParameterGroup, roundNo int) int {
	if roundNo <= 0 {
		return 0
	}
	value := 0
	for _, item := range groups {
		if item == nil {
			continue
		}
		if item.Seq > roundNo {
			break
		}
		value = item.Exp
	}
	return value
}

func characterMissionMaxExplicitSeq(groups []*CharacterMissionParameterGroup) int {
	maxSeq := 0
	for _, item := range groups {
		if item == nil || item.Seq <= maxSeq {
			continue
		}
		maxSeq = item.Seq
	}
	return maxSeq
}

func calcCharacterMissionReachedSeq(rows []drawing.CharacterMissionAllTableRow, current int, isEx bool, currentRound int) int {
	if isEx && currentRound > 0 {
		return currentRound
	}
	reached := 0
	for _, row := range rows {
		if row.Requirement <= current {
			reached = row.Seq
			continue
		}
		break
	}
	return reached
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
