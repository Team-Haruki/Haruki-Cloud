package mysekai

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
)

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

	blueprints := c.masterdata.loadMapByID("mysekaiBlueprints.json")
	obtainedFixtureIDs := userMysekaiBlueprintFixtureIDs(merged, blueprints)
	fixturesData := c.masterdata.loadList("mysekaiFixtures.json")
	fixtureMap := c.masterdata.loadMapByID("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	gameCharacterUnitGroups := c.masterdata.loadMapByID("mysekaiGameCharacterUnitGroups.json")
	archiveGroups := c.masterdata.loadMapByID("characterArchiveMysekaiCharacterTalkGroups.json")
	conditions := c.masterdata.loadList("mysekaiCharacterTalkConditions.json")
	conditionGroups := c.masterdata.loadList("mysekaiCharacterTalkConditionGroups.json")
	talks := c.masterdata.loadList("mysekaiCharacterTalks.json")

	userTalkReads := map[int]bool{}
	if !showAllTalks {
		for _, raw := range nestedList(merged, "userMysekaiCharacterTalks") {
			item, _ := raw.(map[string]any)
			talkID := intNumberFrom(item, 0, "mysekaiCharacterTalkId", "mysekai_character_talk_id", "mysekaiCharacterTalkID")
			if talkID == 0 {
				continue
			}
			userTalkReads[talkID] = boolValueFrom(item, "isRead", "is_read")
		}
	}

	type talkRead struct {
		fixtureIDs []int
		read       int
		total      int
		cuidsSet   [][]int
		hasRead    bool
		cuids      []int
	}

	conditionIDsByFixture := map[int][]int{}
	for _, condition := range conditions {
		if stringValueFrom(condition, "mysekaiCharacterTalkConditionType", "mysekai_character_talk_condition_type") != "mysekai_fixture_id" {
			continue
		}
		fixtureID := intNumberFrom(condition, 0, "mysekaiCharacterTalkConditionTypeValue", "mysekai_character_talk_condition_type_value")
		if fixtureID != 0 {
			conditionIDsByFixture[fixtureID] = append(conditionIDsByFixture[fixtureID], intNumberFrom(condition, 0, "id", "game_id"))
		}
	}

	groupIDsByCondition := map[int][]int{}
	for _, group := range conditionGroups {
		conditionID := intNumberFrom(group, 0, "mysekaiCharacterTalkConditionId", "mysekai_character_talk_condition_id")
		groupIDsByCondition[conditionID] = append(groupIDsByCondition[conditionID], intNumberFrom(group, 0, "id", "game_id"))
	}

	talksByGroup := map[int][]map[string]any{}
	for _, talk := range talks {
		groupID := intNumberFrom(talk, 0, "mysekaiCharacterTalkConditionGroupId", "mysekai_character_talk_condition_group_id")
		talksByGroup[groupID] = append(talksByGroup[groupID], talk)
	}

	archiveReads := map[int]*talkRead{}
	for _, fixture := range fixturesData {
		fixtureID := intNumberFrom(fixture, 0, "id", "game_id")
		if fixtureID == 0 || stringValueFrom(fixture, "mysekaiFixtureType", "mysekai_fixture_type") == "gate" {
			continue
		}

		groupIDs := map[int]struct{}{}
		for _, conditionID := range conditionIDsByFixture[fixtureID] {
			for _, groupID := range groupIDsByCondition[conditionID] {
				groupIDs[groupID] = struct{}{}
			}
		}
		for groupID := range groupIDs {
			for _, talk := range talksByGroup[groupID] {
				talkID := intNumberFrom(talk, 0, "id", "game_id")
				group := gameCharacterUnitGroups[intNumberFrom(talk, 0, "mysekaiGameCharacterUnitGroupId", "mysekai_game_character_unit_group_id")]
				if len(group) == 0 {
					continue
				}
				groupCuids := extractGroupCuids(group)
				if !containsInt(groupCuids, characterUnitID) {
					continue
				}

				archiveID := intNumberFrom(talk, 0, "characterArchiveMysekaiCharacterTalkGroupId", "character_archive_mysekai_character_talk_group_id")
				archive := archiveGroups[archiveID]
				if len(archive) > 0 && stringValueFrom(archive, "archiveDisplayType", "archive_display_type") != "normal" {
					continue
				}
				if _, ok := archiveReads[archiveID]; !ok {
					archiveReads[archiveID] = &talkRead{}
				}
				read := archiveReads[archiveID]
				if !containsInt(read.fixtureIDs, fixtureID) {
					read.fixtureIDs = append(read.fixtureIDs, fixtureID)
				}
				read.cuids = groupCuids
				if userTalkReads[talkID] {
					read.hasRead = true
				}
			}
		}
	}

	singleReads := map[string]*talkRead{}
	multiReadsMap := map[string]*talkRead{}
	for _, item := range archiveReads {
		sort.Ints(item.fixtureIDs)
		keyParts := make([]string, 0, len(item.fixtureIDs))
		for _, fixtureID := range item.fixtureIDs {
			keyParts = append(keyParts, strconv.Itoa(fixtureID))
		}
		key := strings.Join(keyParts, " ")
		target := singleReads
		if len(item.cuids) > 1 {
			target = multiReadsMap
		}
		if _, ok := target[key]; !ok {
			target[key] = &talkRead{}
		}
		target[key].fixtureIDs = item.fixtureIDs
		target[key].total++
		if item.hasRead {
			target[key].read++
			continue
		}
		if len(item.cuids) > 1 {
			duplicate := false
			for _, existing := range target[key].cuidsSet {
				if intsEqual(existing, item.cuids) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				target[key].cuidsSet = append(target[key].cuidsSet, item.cuids)
			}
		}
	}

	groupedSingle := map[int][]drawing.MysekaiTalkFixtures{}
	for key, item := range singleReads {
		if item.total == item.read {
			continue
		}
		fixtureIDs := parseIntTokens(key)
		if len(fixtureIDs) == 0 {
			continue
		}
		mainGenreID := intNumberFrom(fixtureMap[fixtureIDs[0]], 0, "mysekaiFixtureMainGenreId", "mysekai_fixture_main_genre_id")
		fixtures := make([]drawing.MysekaiFixture, 0, len(fixtureIDs))
		for _, fixtureID := range fixtureIDs {
			fixture := fixtureMap[fixtureID]
			fixtures = append(fixtures, drawing.MysekaiFixture{
				ID:        fixtureID,
				ImagePath: fixtureThumbnailPath(func(p string) string { return c.regionPath(region, p) }, fixture),
				Obtained:  hasFixture(obtainedFixtureIDs, fixtureID),
			})
		}
		groupedSingle[mainGenreID] = append(groupedSingle[mainGenreID], drawing.MysekaiTalkFixtures{
			Fixtures:  fixtures,
			NoreadNum: item.total - item.read,
		})
	}

	mainGenreIDs := make([]int, 0, len(groupedSingle))
	for mainGenreID := range groupedSingle {
		mainGenreIDs = append(mainGenreIDs, mainGenreID)
	}
	sort.Ints(mainGenreIDs)

	singleMainGenres := make([]drawing.MysekaiSingleTalkMainGenre, 0, len(mainGenreIDs))
	for _, mainGenreID := range mainGenreIDs {
		info := mainGenreMap[mainGenreID]
		fixtures := groupedSingle[mainGenreID]
		sortSingleTalkFixtures(fixtures)
		singleMainGenres = append(singleMainGenres, drawing.MysekaiSingleTalkMainGenre{
			Name:      stringValueFrom(info, "name"),
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValueFrom(info, "assetbundleName", "assetbundle_name"))),
			SubGenres: [][]drawing.MysekaiTalkFixtures{fixtures},
		})
	}

	totalTalks := 0
	totalReads := 0
	for _, item := range singleReads {
		totalTalks += item.total
		totalReads += item.read
	}

	multiReads := make([]drawing.MysekaiTalkFixtures, 0, len(multiReadsMap))
	for key, item := range multiReadsMap {
		totalTalks += item.total
		totalReads += item.read
		if item.total == item.read {
			continue
		}
		fixtureIDs := parseIntTokens(key)
		if len(fixtureIDs) == 0 {
			continue
		}

		iconGroups := make([][]string, 0, len(item.cuidsSet))
		for _, cuids := range item.cuidsSet {
			icons := make([]string, 0, len(cuids))
			for _, cuid := range cuids {
				icons = append(icons, c.staticPath(fmt.Sprintf("chara_icon/%s.png", charaIconName(cuid))))
			}
			iconGroups = append(iconGroups, icons)
		}

		fixtures := make([]drawing.MysekaiFixture, 0, len(fixtureIDs))
		for _, fixtureID := range fixtureIDs {
			fixture := fixtureMap[fixtureID]
			fixtures = append(fixtures, drawing.MysekaiFixture{
				ID:        fixtureID,
				ImagePath: fixtureThumbnailPath(func(p string) string { return c.regionPath(region, p) }, fixture),
				Obtained:  hasFixture(obtainedFixtureIDs, fixtureID),
			})
		}
		multiReads = append(multiReads, drawing.MysekaiTalkFixtures{
			Fixtures:            fixtures,
			NoreadNum:           item.total - item.read,
			CharacterIDs:        item.cuidsSet,
			CharaIconPathGroups: iconGroups,
		})
	}
	sort.SliceStable(multiReads, func(i, j int) bool {
		if len(multiReads[i].Fixtures) != len(multiReads[j].Fixtures) {
			return len(multiReads[i].Fixtures) > len(multiReads[j].Fixtures)
		}
		if len(multiReads[i].Fixtures) == 0 || len(multiReads[j].Fixtures) == 0 {
			return false
		}
		return multiReads[i].Fixtures[0].ID < multiReads[j].Fixtures[0].ID
	})

	var progressMessage string
	var promptMessage *string
	if showAllTalks {
		progressMessage = fmt.Sprintf("对话家具列表 - 共 %d 条对话", totalTalks)
	} else {
		progressMessage = fmt.Sprintf("未读对话家具列表 - 进度: %d/%d (%.1f%%)", totalReads, totalTalks, percent(totalReads, totalTalks))
		promptMessage = new("*仅展示未读对话家具，灰色表示未获得蓝图")
	}
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
	payload, err := c.BuildTalkListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiTalkList(payload)
}
