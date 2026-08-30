package mysekai

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type talkRead struct {
	fixtureIDs []int
	read       int
	total      int
	cuidsSet   [][]int
	hasRead    bool
	cuids      []int
}

type talkListMasterdata struct {
	blueprints          map[int]map[string]any
	fixtures            []map[string]any
	fixtureMap          map[int]map[string]any
	mainGenreMap        map[int]map[string]any
	characterUnitGroups map[int]map[string]any
	archiveGroups       map[int]map[string]any
	conditions          []map[string]any
	conditionGroups     []map[string]any
	talks               []map[string]any
}

type talkListIndexes struct {
	conditionIDsByFixture map[int][]int
	groupIDsByCondition   map[int][]int
	talksByGroup          map[int][]map[string]any
}

// BuildTalkListRequest builds the request for rendering MySekai talk list view.
func (c *Controller) BuildTalkListRequest(query TalkListQuery) (*drawing.MysekaiTalkListRequest, error) {
	c = c.withRegion(query.Region)
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(query.Query) == "" {
		return nil, fmt.Errorf("mysekai talk list requires character query")
	}
	showAllTalks := query.ShowAllTalks != nil && *query.ShowAllTalks

	_, characterUnitID, err := c.resolveTalkCharacter(query.Query)
	if err != nil {
		return nil, err
	}
	if characterUnitID == 0 {
		return nil, fmt.Errorf("mysekai talk list invalid character query: %s", query.Query)
	}

	masterdata := c.loadTalkListMasterdata()
	obtainedFixtureIDs := userMysekaiBlueprintFixtureIDs(merged, masterdata.blueprints)
	userTalkReads := collectUserTalkReads(merged, showAllTalks)
	indexes := buildTalkListIndexes(masterdata)
	archiveReads := collectTalkArchiveReads(characterUnitID, masterdata, indexes, userTalkReads)
	singleReads, multiReadsMap := groupTalkReads(archiveReads)

	singleMainGenres := c.buildSingleTalkGenres(region, singleReads, masterdata, obtainedFixtureIDs)
	multiReads := c.buildMultiTalkReads(region, multiReadsMap, masterdata.fixtureMap, obtainedFixtureIDs)
	totalTalks, totalReads := countTalkProgress(singleReads, multiReadsMap)
	progressMessage, promptMessage := talkProgressMessages(showAllTalks, totalReads, totalTalks)

	return &drawing.MysekaiTalkListRequest{
		Profile:          c.mysekaiProfileCard(region, merged, query.Profile, true),
		SdImagePath:      c.regionPath(region, fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", characterUnitID)),
		ProgressMessage:  &progressMessage,
		PromptMessage:    promptMessage,
		ShowID:           true,
		SingleMainGenres: singleMainGenres,
		MultiReads:       multiReads,
	}, nil
}

func (c *Controller) loadTalkListMasterdata() talkListMasterdata {
	return talkListMasterdata{
		blueprints:          c.masterdata.loadMapByID("mysekaiBlueprints.json"),
		fixtures:            c.masterdata.loadList("mysekaiFixtures.json"),
		fixtureMap:          c.masterdata.loadMapByID("mysekaiFixtures.json"),
		mainGenreMap:        c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json"),
		characterUnitGroups: c.masterdata.loadMapByID("mysekaiGameCharacterUnitGroups.json"),
		archiveGroups:       c.masterdata.loadMapByID("characterArchiveMysekaiCharacterTalkGroups.json"),
		conditions:          c.masterdata.loadList("mysekaiCharacterTalkConditions.json"),
		conditionGroups:     c.masterdata.loadList("mysekaiCharacterTalkConditionGroups.json"),
		talks:               c.masterdata.loadList("mysekaiCharacterTalks.json"),
	}
}

func collectUserTalkReads(merged map[string]any, showAllTalks bool) map[int]bool {
	reads := map[int]bool{}
	if showAllTalks {
		return reads
	}
	for _, raw := range nestedList(merged, "userMysekaiCharacterTalks") {
		item, _ := raw.(map[string]any)
		talkID := intNumberFrom(item, 0, "mysekaiCharacterTalkId", "mysekai_character_talk_id", "mysekaiCharacterTalkID")
		if talkID != 0 {
			reads[talkID] = boolValueFrom(item, "isRead", "is_read")
		}
	}
	return reads
}

func buildTalkListIndexes(masterdata talkListMasterdata) talkListIndexes {
	return talkListIndexes{
		conditionIDsByFixture: indexTalkConditionsByFixture(masterdata.conditions),
		groupIDsByCondition:   indexTalkGroupsByCondition(masterdata.conditionGroups),
		talksByGroup:          indexTalksByGroup(masterdata.talks),
	}
}

func indexTalkConditionsByFixture(conditions []map[string]any) map[int][]int {
	result := map[int][]int{}
	for _, condition := range conditions {
		if stringValueFrom(condition, "mysekaiCharacterTalkConditionType", "mysekai_character_talk_condition_type") != "mysekai_fixture_id" {
			continue
		}
		fixtureID := intNumberFrom(condition, 0, "mysekaiCharacterTalkConditionTypeValue", "mysekai_character_talk_condition_type_value")
		if fixtureID != 0 {
			result[fixtureID] = append(result[fixtureID], intNumberFrom(condition, 0, "id", "game_id"))
		}
	}
	return result
}

func indexTalkGroupsByCondition(groups []map[string]any) map[int][]int {
	result := map[int][]int{}
	for _, group := range groups {
		conditionID := intNumberFrom(group, 0, "mysekaiCharacterTalkConditionId", "mysekai_character_talk_condition_id")
		result[conditionID] = append(result[conditionID], intNumberFrom(group, 0, "id", "game_id"))
	}
	return result
}

func indexTalksByGroup(talks []map[string]any) map[int][]map[string]any {
	result := map[int][]map[string]any{}
	for _, talk := range talks {
		groupID := intNumberFrom(talk, 0, "mysekaiCharacterTalkConditionGroupId", "mysekai_character_talk_condition_group_id")
		result[groupID] = append(result[groupID], talk)
	}
	return result
}

func collectTalkArchiveReads(
	characterUnitID int,
	masterdata talkListMasterdata,
	indexes talkListIndexes,
	userTalkReads map[int]bool,
) map[int]*talkRead {
	archiveReads := map[int]*talkRead{}
	for _, fixture := range masterdata.fixtures {
		fixtureID := intNumberFrom(fixture, 0, "id", "game_id")
		if fixtureID == 0 || stringValueFrom(fixture, "mysekaiFixtureType", "mysekai_fixture_type") == "gate" {
			continue
		}
		collectFixtureArchiveReads(
			fixtureID,
			characterUnitID,
			masterdata,
			indexes,
			userTalkReads,
			archiveReads,
		)
	}
	return archiveReads
}

func collectFixtureArchiveReads(
	fixtureID int,
	characterUnitID int,
	masterdata talkListMasterdata,
	indexes talkListIndexes,
	userTalkReads map[int]bool,
	archiveReads map[int]*talkRead,
) {
	for _, groupID := range talkGroupIDsForFixture(fixtureID, indexes) {
		for _, talk := range indexes.talksByGroup[groupID] {
			addTalkArchiveRead(fixtureID, characterUnitID, talk, masterdata, userTalkReads, archiveReads)
		}
	}
}

func talkGroupIDsForFixture(fixtureID int, indexes talkListIndexes) []int {
	groupIDs := map[int]struct{}{}
	for _, conditionID := range indexes.conditionIDsByFixture[fixtureID] {
		for _, groupID := range indexes.groupIDsByCondition[conditionID] {
			groupIDs[groupID] = struct{}{}
		}
	}
	result := make([]int, 0, len(groupIDs))
	for groupID := range groupIDs {
		result = append(result, groupID)
	}
	return result
}

func addTalkArchiveRead(
	fixtureID int,
	characterUnitID int,
	talk map[string]any,
	masterdata talkListMasterdata,
	userTalkReads map[int]bool,
	archiveReads map[int]*talkRead,
) {
	group := masterdata.characterUnitGroups[intNumberFrom(talk, 0, "mysekaiGameCharacterUnitGroupId", "mysekai_game_character_unit_group_id")]
	if len(group) == 0 {
		return
	}
	groupCuids := extractGroupCuids(group)
	if !containsInt(groupCuids, characterUnitID) {
		return
	}
	archiveID := intNumberFrom(talk, 0, "characterArchiveMysekaiCharacterTalkGroupId", "character_archive_mysekai_character_talk_group_id")
	archive := masterdata.archiveGroups[archiveID]
	if len(archive) > 0 && stringValueFrom(archive, "archiveDisplayType", "archive_display_type") != "normal" {
		return
	}
	read := archiveReads[archiveID]
	if read == nil {
		read = &talkRead{}
		archiveReads[archiveID] = read
	}
	if !containsInt(read.fixtureIDs, fixtureID) {
		read.fixtureIDs = append(read.fixtureIDs, fixtureID)
	}
	read.cuids = groupCuids
	talkID := intNumberFrom(talk, 0, "id", "game_id")
	read.hasRead = read.hasRead || userTalkReads[talkID]
}

func groupTalkReads(archiveReads map[int]*talkRead) (map[string]*talkRead, map[string]*talkRead) {
	singleReads := map[string]*talkRead{}
	multiReads := map[string]*talkRead{}
	for _, item := range archiveReads {
		sort.Ints(item.fixtureIDs)
		key := talkFixtureKey(item.fixtureIDs)
		target := singleReads
		if len(item.cuids) > 1 {
			target = multiReads
		}
		mergeTalkRead(target, key, item)
	}
	return singleReads, multiReads
}

func talkFixtureKey(fixtureIDs []int) string {
	parts := make([]string, 0, len(fixtureIDs))
	for _, fixtureID := range fixtureIDs {
		parts = append(parts, strconv.Itoa(fixtureID))
	}
	return strings.Join(parts, " ")
}

func mergeTalkRead(target map[string]*talkRead, key string, item *talkRead) {
	if target[key] == nil {
		target[key] = &talkRead{fixtureIDs: item.fixtureIDs}
	}
	grouped := target[key]
	grouped.total++
	if item.hasRead {
		grouped.read++
		return
	}
	if len(item.cuids) > 1 && !containsCUIDGroup(grouped.cuidsSet, item.cuids) {
		grouped.cuidsSet = append(grouped.cuidsSet, item.cuids)
	}
}

func containsCUIDGroup(groups [][]int, candidate []int) bool {
	for _, existing := range groups {
		if intsEqual(existing, candidate) {
			return true
		}
	}
	return false
}

func (c *Controller) buildSingleTalkGenres(
	region renderregion.Value,
	singleReads map[string]*talkRead,
	masterdata talkListMasterdata,
	obtainedFixtureIDs map[int]struct{},
) []drawing.MysekaiSingleTalkMainGenre {
	grouped := make(map[int][]drawing.MysekaiTalkFixtures)
	for key, item := range singleReads {
		fixtureIDs := unreadTalkFixtureIDs(key, item)
		if len(fixtureIDs) == 0 {
			continue
		}
		mainGenreID := intNumberFrom(masterdata.fixtureMap[fixtureIDs[0]], 0, "mysekaiFixtureMainGenreId", "mysekai_fixture_main_genre_id")
		grouped[mainGenreID] = append(grouped[mainGenreID], drawing.MysekaiTalkFixtures{
			Fixtures:  c.buildTalkFixtures(region, fixtureIDs, masterdata.fixtureMap, obtainedFixtureIDs),
			NoreadNum: item.total - item.read,
		})
	}
	return c.assembleSingleTalkGenres(region, grouped, masterdata.mainGenreMap)
}

func unreadTalkFixtureIDs(key string, item *talkRead) []int {
	if item.total == item.read {
		return nil
	}
	return parseIntTokens(key)
}

func (c *Controller) buildTalkFixtures(
	region renderregion.Value,
	fixtureIDs []int,
	fixtureMap map[int]map[string]any,
	obtainedFixtureIDs map[int]struct{},
) []drawing.MysekaiFixture {
	fixtures := make([]drawing.MysekaiFixture, 0, len(fixtureIDs))
	for _, fixtureID := range fixtureIDs {
		fixture := fixtureMap[fixtureID]
		fixtures = append(fixtures, drawing.MysekaiFixture{
			ID:        fixtureID,
			ImagePath: fixtureThumbnailPath(func(path string) string { return c.regionPath(region, path) }, fixture),
			Obtained:  hasFixture(obtainedFixtureIDs, fixtureID),
		})
	}
	return fixtures
}

func (c *Controller) assembleSingleTalkGenres(
	region renderregion.Value,
	grouped map[int][]drawing.MysekaiTalkFixtures,
	mainGenreMap map[int]map[string]any,
) []drawing.MysekaiSingleTalkMainGenre {
	mainGenreIDs := make([]int, 0, len(grouped))
	for mainGenreID := range grouped {
		mainGenreIDs = append(mainGenreIDs, mainGenreID)
	}
	sort.Ints(mainGenreIDs)
	result := make([]drawing.MysekaiSingleTalkMainGenre, 0, len(mainGenreIDs))
	for _, mainGenreID := range mainGenreIDs {
		info := mainGenreMap[mainGenreID]
		fixtures := grouped[mainGenreID]
		sortSingleTalkFixtures(fixtures)
		result = append(result, drawing.MysekaiSingleTalkMainGenre{
			Name:      stringValueFrom(info, "name"),
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValueFrom(info, "assetbundleName", "assetbundle_name"))),
			SubGenres: [][]drawing.MysekaiTalkFixtures{fixtures},
		})
	}
	return result
}

func (c *Controller) buildMultiTalkReads(
	region renderregion.Value,
	multiReadsMap map[string]*talkRead,
	fixtureMap map[int]map[string]any,
	obtainedFixtureIDs map[int]struct{},
) []drawing.MysekaiTalkFixtures {
	result := make([]drawing.MysekaiTalkFixtures, 0, len(multiReadsMap))
	for key, item := range multiReadsMap {
		fixtureIDs := unreadTalkFixtureIDs(key, item)
		if len(fixtureIDs) == 0 {
			continue
		}
		result = append(result, drawing.MysekaiTalkFixtures{
			Fixtures:            c.buildTalkFixtures(region, fixtureIDs, fixtureMap, obtainedFixtureIDs),
			NoreadNum:           item.total - item.read,
			CharacterIDs:        item.cuidsSet,
			CharaIconPathGroups: c.buildTalkCharacterIconGroups(item.cuidsSet),
		})
	}
	sortMultiTalkReads(result)
	return result
}

func (c *Controller) buildTalkCharacterIconGroups(cuidsSet [][]int) [][]string {
	groups := make([][]string, 0, len(cuidsSet))
	for _, cuids := range cuidsSet {
		icons := make([]string, 0, len(cuids))
		for _, cuid := range cuids {
			icons = append(icons, c.staticPath(fmt.Sprintf("chara_icon/%s.png", charaIconName(cuid))))
		}
		groups = append(groups, icons)
	}
	return groups
}

func sortMultiTalkReads(items []drawing.MysekaiTalkFixtures) {
	sort.SliceStable(items, func(i, j int) bool {
		if len(items[i].Fixtures) != len(items[j].Fixtures) {
			return len(items[i].Fixtures) > len(items[j].Fixtures)
		}
		if len(items[i].Fixtures) == 0 || len(items[j].Fixtures) == 0 {
			return false
		}
		return items[i].Fixtures[0].ID < items[j].Fixtures[0].ID
	})
}

func countTalkProgress(groups ...map[string]*talkRead) (int, int) {
	totalTalks := 0
	totalReads := 0
	for _, group := range groups {
		for _, item := range group {
			totalTalks += item.total
			totalReads += item.read
		}
	}
	return totalTalks, totalReads
}

func talkProgressMessages(showAllTalks bool, totalReads, totalTalks int) (string, *string) {
	if showAllTalks {
		return fmt.Sprintf("对话家具列表 - 共 %d 条对话", totalTalks), nil
	}
	message := fmt.Sprintf("未读对话家具列表 - 进度: %d/%d (%.1f%%)", totalReads, totalTalks, percent(totalReads, totalTalks))
	return message, new("*仅展示未读对话家具，灰色表示未获得蓝图")
}

func sortSingleTalkFixtures(items []drawing.MysekaiTalkFixtures) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i].Fixtures
		right := items[j].Fixtures
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		for idx := 0; idx < len(left) && idx < len(right); idx++ {
			if left[idx].ID != right[idx].ID {
				return left[idx].ID > right[idx].ID
			}
		}
		return items[i].NoreadNum > items[j].NoreadNum
	})
}

// RenderTalkList renders the MySekai talk list view.
func (c *Controller) RenderTalkList(query TalkListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildTalkListRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiTalkList(payload)
}
