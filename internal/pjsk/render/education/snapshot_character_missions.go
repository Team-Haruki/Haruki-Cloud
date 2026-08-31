package education

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

func (c *Controller) BuildCharacterMissionOverviewRequestFromSnapshot(query CharacterMissionQuery) (*drawing.CharacterMissionOverviewRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.traceContext(), "payload.build")
	defer finishBuild()
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
	finishBuild := commandtrace.MeasureOperation(c.traceContext(), "payload.build")
	defer finishBuild()
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

	sectionTypes := characterMissionSectionTypes(missionType)
	sections := make([]drawing.CharacterMissionAllSection, 0, len(sectionTypes))
	for _, sectionType := range sectionTypes {
		section, err := buildCharacterMissionAllSection(ctx, cid, sectionType, rowByType)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
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

func characterMissionSectionTypes(missionType string) []string {
	switch missionType {
	case "play_live":
		return []string{"play_live", "play_live_ex"}
	case "waiting_room":
		return []string{"waiting_room", "waiting_room_ex"}
	default:
		return []string{missionType}
	}
}

func buildCharacterMissionAllSection(ctx *resolvedSnapshotContext, cid int, sectionType string, rowByType map[string]drawing.CharacterMissionOverviewRow) (drawing.CharacterMissionAllSection, error) {
	base, ok := rowByType[sectionType]
	if !ok {
		return drawing.CharacterMissionAllSection{}, fmt.Errorf("character mission type not found: %s", sectionType)
	}
	missionDef := findCharacterMissionByType(ctx.source.GetCharacterMissions(cid), sectionType)
	if missionDef == nil {
		return drawing.CharacterMissionAllSection{}, fmt.Errorf("character mission definition not found: %s", sectionType)
	}
	groups := cloneCharacterMissionParameterGroups(ctx.source.GetCharacterMissionParameterGroups(missionDef.ParameterGroupID))
	sort.Slice(groups, func(i, j int) bool { return groups[i].Seq < groups[j].Seq })
	displayRows := buildCharacterMissionAllDisplayRows(groups, base)
	return drawing.CharacterMissionAllSection{
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
	}, nil
}

func buildCharacterMissionAllDisplayRows(groups []*CharacterMissionParameterGroup, base drawing.CharacterMissionOverviewRow) []drawing.CharacterMissionAllTableRow {
	if base.IsEx {
		return buildCharacterMissionExDisplayRows(groups, maxInt(derefInt(base.CurrentRound), characterMissionMaxExplicitSeq(groups)))
	}
	result := make([]drawing.CharacterMissionAllTableRow, 0, len(groups))
	accExp := 0
	for _, group := range groups {
		if group == nil {
			continue
		}
		accExp += group.Exp
		result = append(result, drawing.CharacterMissionAllTableRow{
			Seq: group.Seq, Requirement: group.Requirement, AccRequirement: group.Requirement,
			Exp: group.Exp, AccExp: accExp,
		})
	}
	return result
}

func buildCharacterMissionExDisplayRows(groups []*CharacterMissionParameterGroup, maxRound int) []drawing.CharacterMissionAllTableRow {
	result := make([]drawing.CharacterMissionAllTableRow, 0, maxRound)
	accRequirement, accExp := 0, 0
	for roundNo := 1; roundNo <= maxRound; roundNo++ {
		requirement := characterMissionRequirementForRound(groups, roundNo)
		exp := characterMissionExpForRound(groups, roundNo)
		accRequirement += requirement
		accExp += exp
		result = append(result, drawing.CharacterMissionAllTableRow{
			Seq: roundNo, Requirement: requirement, AccRequirement: accRequirement,
			Exp: exp, AccExp: accExp,
		})
	}
	return result
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

	statuses := characterMissionStatusesForCharacter(ctx.raw, cid)
	levelProgress := resolveCharacterMissionLevelProgress(ctx, cid, statuses)
	progressIndex := newCharacterMissionProgressIndex(ctx.raw, cid, statuses)

	rows := make([]drawing.CharacterMissionOverviewRow, 0, len(missions))
	for _, mission := range missions {
		rows = append(rows, buildCharacterMissionOverviewRow(ctx.source, mission, progressIndex))
	}

	iconPath := c.characterIconPath(cid)
	return rows, characterMissionDisplayName(cid), iconPath,
		levelProgress.currentLevel, levelProgress.currentExp, levelProgress.pendingExp,
		levelProgress.finalLevel, levelProgress.finalExp, nil
}

type characterMissionLevelProgress struct {
	currentLevel int
	currentExp   int
	pendingExp   int
	finalLevel   int
	finalExp     int
}

func resolveCharacterMissionLevelProgress(ctx *resolvedSnapshotContext, cid int, statuses []rendersnapshot.RawUserCharacterMissionV2Status) characterMissionLevelProgress {
	levels := cloneCharacterLevels(ctx.source.GetCharacterLevels())
	levelStarts := characterLevelStarts(levels)
	currentLevel, currentExp, currentTotalExp := rawCharacterLevelProgress(ctx.raw.UserCharacters, cid)
	if start, ok := levelStarts[currentLevel]; ok && currentTotalExp >= start && currentTotalExp > 0 {
		currentExp = currentTotalExp - start
	}
	pendingExp := characterMissionPendingExp(ctx.source, statuses)
	baseTotalExp := currentTotalExp
	if baseTotalExp <= 0 && currentLevel > 0 {
		if levelStart, ok := levelStarts[currentLevel]; ok {
			baseTotalExp = levelStart + currentExp
		}
	}
	finalLevel, finalExp := characterMissionFinalLevel(levels, maxInt(baseTotalExp, 0)+pendingExp, currentLevel, currentExp+pendingExp)
	return characterMissionLevelProgress{currentLevel, currentExp, pendingExp, finalLevel, finalExp}
}

func characterLevelStarts(levels []*CharacterLevel) map[int]int {
	result := make(map[int]int, len(levels))
	for _, level := range levels {
		if level != nil && level.Level > 0 {
			result[level.Level] = level.TotalExp
		}
	}
	return result
}

func rawCharacterLevelProgress(characters []rendersnapshot.RawUserCharacter, cid int) (int, int, int) {
	character := findRawUserCharacter(characters, cid)
	if character == nil {
		return 0, 0, 0
	}
	return character.CharacterRank, character.Exp, character.TotalExp
}

func characterMissionPendingExp(source DataSource, statuses []rendersnapshot.RawUserCharacterMissionV2Status) int {
	result := 0
	for _, status := range statuses {
		if strings.EqualFold(strings.TrimSpace(status.MissionStatus), "achieved") {
			result += characterMissionGroupExp(source.GetCharacterMissionParameterGroups(status.ParameterGroupID), status.Seq)
		}
	}
	return result
}

func characterMissionFinalLevel(levels []*CharacterLevel, totalExp, fallbackLevel, fallbackExp int) (int, int) {
	if len(levels) == 0 {
		return fallbackLevel, fallbackExp
	}
	finalLevel, levelStart := 1, 0
	for _, level := range levels {
		if level == nil || level.Level <= 0 {
			continue
		}
		if level.TotalExp > totalExp {
			break
		}
		finalLevel, levelStart = level.Level, level.TotalExp
	}
	return finalLevel, totalExp - levelStart
}

type characterMissionProgressIndex struct {
	byType      map[string]int
	byMissionID map[int]int
	byGroupID   map[int]int
}

func newCharacterMissionProgressIndex(raw *rendersnapshot.RawUserData, cid int, statuses []rendersnapshot.RawUserCharacterMissionV2Status) characterMissionProgressIndex {
	result := characterMissionProgressIndex{make(map[string]int), make(map[int]int), make(map[int]int)}
	for _, mission := range raw.UserCharacterMissionV2s {
		if mission.CharacterID == cid {
			result.byType[mission.CharacterMissionType] = maxInt(result.byType[mission.CharacterMissionType], mission.Progress)
		}
	}
	for _, status := range statuses {
		result.byMissionID[status.MissionID] = maxInt(result.byMissionID[status.MissionID], status.Seq)
		result.byGroupID[status.ParameterGroupID] = maxInt(result.byGroupID[status.ParameterGroupID], status.Seq)
	}
	return result
}

func buildCharacterMissionOverviewRow(source DataSource, mission *CharacterMission, index characterMissionProgressIndex) drawing.CharacterMissionOverviewRow {
	groups := cloneCharacterMissionParameterGroups(source.GetCharacterMissionParameterGroups(mission.ParameterGroupID))
	sort.Slice(groups, func(i, j int) bool { return groups[i].Seq < groups[j].Seq })
	isEx := isCharacterMissionExType(mission.CharacterMissionType)
	current := resolveCharacterMissionCurrent(mission, groups, isEx, index)
	upper := characterMissionUpper(groups, isEx)
	nextNeed, nextExp := characterMissionNextTarget(groups, current, isEx)
	currentRound, roundProgress, roundNeed, roundText := characterMissionRoundDetails(groups, current, isEx)
	return drawing.CharacterMissionOverviewRow{
		MissionID: mission.ID, MissionType: mission.CharacterMissionType,
		Title: CharacterMissionShortName(mission.CharacterMissionType), IsAchievement: mission.IsAchievementMission,
		IsEx: isEx, Current: current, Upper: upper, Ratio: characterMissionRatio(current, upper),
		NextNeed: nextNeed, NextExp: nextExp, CurrentRound: characterMissionRoundPtr(currentRound),
		CurrentRoundProgress: characterMissionRoundPtr(roundProgress), CurrentRoundNeed: characterMissionRoundPtr(roundNeed),
		ExDisplayRoundText: characterMissionRoundTextPtr(roundText),
	}
}

func resolveCharacterMissionCurrent(mission *CharacterMission, groups []*CharacterMissionParameterGroup, isEx bool, index characterMissionProgressIndex) int {
	current := index.byType[mission.CharacterMissionType]
	receivedSeq := maxInt(index.byMissionID[mission.ID], index.byGroupID[mission.ParameterGroupID])
	if isEx {
		clearedTotal := characterMissionClearedTotal(groups, receivedSeq)
		if current > 0 && current < clearedTotal {
			return clearedTotal + current
		}
		return maxInt(current, clearedTotal)
	}
	if current <= 0 && receivedSeq > 0 {
		return characterMissionRequirementBySeq(groups, receivedSeq)
	}
	return current
}

func characterMissionRatio(current int, upper *int) float64 {
	if upper == nil || *upper <= 0 {
		return 0
	}
	if current > *upper {
		return 1
	}
	return float64(current) / float64(*upper)
}

func characterMissionRoundDetails(groups []*CharacterMissionParameterGroup, current int, isEx bool) (int, int, int, string) {
	if !isEx {
		return 0, 0, 0, ""
	}
	round, progress, need := characterMissionCurrentRound(groups, current)
	return round, progress, need, fmt.Sprintf("EX %d 回目", round)
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
