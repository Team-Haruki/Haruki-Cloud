package mysekai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type Controller struct {
	drawing       *drawing.HarukiDrawingClient
	snapshot      *userdata.Service
	masterdata    *localMasterdataStore
	defaultRegion renderregion.Value
	nicknames     map[string]int
}

func NewController(drawingClient *drawing.HarukiDrawingClient, snapshot *userdata.Service, masterdataDir string, defaultRegion renderregion.Value) *Controller {
	return &Controller{
		drawing:       drawingClient,
		snapshot:      snapshot,
		masterdata:    newLocalMasterdataStore(masterdataDir),
		defaultRegion: renderregion.WithDefault(defaultRegion),
		nicknames:     cloneNicknames(defaultNicknames),
	}
}

func (c *Controller) BuildResourceRequest(query ResourceQuery) (*drawing.MysekaiResourceRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	profile := c.mysekaiProfileCard(region, merged)
	if profile == nil {
		return nil, fmt.Errorf("mysekai resource requires profile data")
	}

	gateID, gateLevel := extractMysekaiGate(merged)
	return &drawing.MysekaiResourceRequest{
		Profile:             *profile,
		Phenoms:             extractMysekaiPhenoms(merged),
		GateID:              gateID,
		GateLevel:           gateLevel,
		GateIconPath:        fmt.Sprintf("mysekai/gate_icon/gate_%d.png", gateID),
		VisitCharacters:     c.extractVisitCharacters(merged),
		SiteResourceNumbers: c.extractSiteResourceNumbers(merged),
	}, nil
}

func (c *Controller) RenderResource(query ResourceQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildResourceRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiResource(payload)
}

func (c *Controller) BuildFixtureListRequest(query FixtureListQuery) (*drawing.MysekaiFixtureListRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	showID := true
	if query.ShowID != nil {
		showID = *query.ShowID
	}

	fixturesData := c.masterdata.loadList("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	subGenreMap := c.masterdata.loadMapByID("mysekaiFixtureSubGenres.json")
	blueprints := c.masterdata.loadMapByID("mysekaiBlueprints.json")
	characters := c.masterdata.loadMapByID("gameCharacters.json")
	obtainedFixtureIDs := c.obtainedMysekaiFixtureIDs(merged, blueprints)

	type fixtureRow struct {
		fixture drawing.MysekaiFixture
	}

	grouped := map[int]map[int][]fixtureRow{}
	mainProgressAll := map[int]int{}
	mainProgressObtained := map[int]int{}
	subProgressAll := map[int]map[int]int{}
	subProgressObtained := map[int]map[int]int{}
	totalAll := 0
	totalObtained := 0

	for _, item := range fixturesData {
		fixtureID := intNumber(item["id"], 0)
		if fixtureID == 0 || strings.EqualFold(stringValue(item["mysekaiFixtureType"]), "gate") {
			continue
		}

		mainGenreID := intNumber(item["mysekaiFixtureMainGenreId"], -1)
		subGenreID := intNumber(item["mysekaiFixtureSubGenreId"], -1)
		if fixtureID == 4 {
			subGenreID = 14
		}
		if _, ok := map[int]struct{}{4: {}, 5: {}, 7: {}, 8: {}, 9: {}, 10: {}, 11: {}, 12: {}, 13: {}}[mainGenreID]; ok {
			subGenreID = -1
		}

		if _, ok := grouped[mainGenreID]; !ok {
			grouped[mainGenreID] = map[int][]fixtureRow{}
			subProgressAll[mainGenreID] = map[int]int{}
			subProgressObtained[mainGenreID] = map[int]int{}
		}

		obtained := hasFixture(obtainedFixtureIDs, fixtureID)
		var characterID *int
		if charID := birthdayCharacterID(characters, stringValue(item["name"])); charID != 0 {
			characterID = &charID
		}

		grouped[mainGenreID][subGenreID] = append(grouped[mainGenreID][subGenreID], fixtureRow{
			fixture: drawing.MysekaiFixture{
				ID:          fixtureID,
				ImagePath:   fixtureThumbnailPath(item),
				CharacterID: characterID,
				Obtained:    obtained,
			},
		})

		if characterID == nil {
			totalAll++
			if obtained {
				totalObtained++
			}
			mainProgressAll[mainGenreID]++
			subProgressAll[mainGenreID][subGenreID]++
			if obtained {
				mainProgressObtained[mainGenreID]++
				subProgressObtained[mainGenreID][subGenreID]++
			}
		}
	}

	mainGenreIDs := make([]int, 0, len(grouped))
	for genreID := range grouped {
		mainGenreIDs = append(mainGenreIDs, genreID)
	}
	sort.Ints(mainGenreIDs)

	mainGenres := make([]drawing.MysekaiFixtureMainGenre, 0, len(mainGenreIDs))
	for _, genreID := range mainGenreIDs {
		subGenreIDs := make([]int, 0, len(grouped[genreID]))
		for subID := range grouped[genreID] {
			subGenreIDs = append(subGenreIDs, subID)
		}
		sort.Ints(subGenreIDs)

		subGenres := make([]drawing.MysekaiFixtureSubGenre, 0, len(subGenreIDs))
		for _, subID := range subGenreIDs {
			rows := grouped[genreID][subID]
			if len(rows) == 0 {
				continue
			}
			fixtures := make([]drawing.MysekaiFixture, 0, len(rows))
			for _, row := range rows {
				fixtures = append(fixtures, row.fixture)
			}
			subGenre := drawing.MysekaiFixtureSubGenre{
				Fixtures: fixtures,
			}
			if subID != -1 && len(grouped[genreID]) > 1 {
				if info := subGenreMap[subID]; len(info) > 0 {
					name := stringValue(info["name"])
					imagePath := fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(info["assetbundleName"]))
					subGenre.Name = &name
					subGenre.ImagePath = &imagePath
					if total := subProgressAll[genreID][subID]; total > 0 {
						message := fmt.Sprintf("%d/%d (%.1f%%)", subProgressObtained[genreID][subID], total, percent(subProgressObtained[genreID][subID], total))
						subGenre.ProgressMessage = &message
					}
				}
			}
			subGenres = append(subGenres, subGenre)
		}
		if len(subGenres) == 0 {
			continue
		}

		mainInfo := mainGenreMap[genreID]
		mainGenre := drawing.MysekaiFixtureMainGenre{
			Name:      stringValue(mainInfo["name"]),
			ImagePath: fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(mainInfo["assetbundleName"])),
			SubGenres: subGenres,
		}
		if total := mainProgressAll[genreID]; total > 0 {
			message := fmt.Sprintf("%d/%d (%.1f%%)", mainProgressObtained[genreID], total, percent(mainProgressObtained[genreID], total))
			mainGenre.ProgressMessage = &message
		}
		mainGenres = append(mainGenres, mainGenre)
	}

	request := &drawing.MysekaiFixtureListRequest{
		Profile:    c.mysekaiProfileCard(region, merged),
		ShowID:     showID,
		MainGenres: mainGenres,
	}
	if totalAll > 0 {
		message := fmt.Sprintf("总收集进度（不含生日家具）: %d/%d (%.1f%%)", totalObtained, totalAll, percent(totalObtained, totalAll))
		request.ProgressMessage = &message
	}
	return request, nil
}

func (c *Controller) RenderFixtureList(query FixtureListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildFixtureListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiFixtureList(payload)
}

func (c *Controller) BuildFixtureDetailRequests(query FixtureDetailQuery) ([]drawing.MysekaiFixtureDetailRequest, error) {
	if err := c.ensure(); err != nil {
		return nil, err
	}

	fixtureIDs := parseIntTokens(query.Query)
	if len(fixtureIDs) == 0 {
		return nil, fmt.Errorf("mysekai fixture detail invalid query: %s", query.Query)
	}

	fixtureMap := c.masterdata.loadMapByID("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	subGenreMap := c.masterdata.loadMapByID("mysekaiFixtureSubGenres.json")
	blueprints := c.masterdata.loadList("mysekaiBlueprints.json")
	blueprintCosts := c.masterdata.loadList("mysekaiBlueprintMysekaiMaterialCosts.json")
	onlyDisassemble := c.masterdata.loadList("mysekaiFixtureOnlyDisassembleMaterials.json")
	tags := c.masterdata.loadMapByID("mysekaiFixtureTags.json")
	region := c.resolveRegion(query.Region)

	requests := make([]drawing.MysekaiFixtureDetailRequest, 0, len(fixtureIDs))
	for _, fixtureID := range fixtureIDs {
		fixture := fixtureMap[fixtureID]
		if len(fixture) == 0 {
			continue
		}
		mainGenreID := intNumber(fixture["mysekaiFixtureMainGenreId"], 0)
		subGenreID := intNumber(fixture["mysekaiFixtureSubGenreId"], 0)
		mainGenre := mainGenreMap[mainGenreID]
		subGenre := subGenreMap[subGenreID]

		request := drawing.MysekaiFixtureDetailRequest{
			Title:              fmt.Sprintf("【%s-%d】%s", strings.ToUpper(region.String()), fixtureID, stringValue(fixture["name"])),
			Images:             fixtureColorImages(fixture),
			MainGenreName:      stringValue(mainGenre["name"]),
			MainGenreImagePath: fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(mainGenre["assetbundleName"])),
			Size: map[string]int{
				"width":  nestedInt(fixture, "gridSize", "width"),
				"depth":  nestedInt(fixture, "gridSize", "depth"),
				"height": nestedInt(fixture, "gridSize", "height"),
			},
			FirstPutCost:            intNumber(fixture["firstPutCost"], 0),
			SecondPutCost:           intNumber(fixture["secondPutCost"], 0),
			BasicInfo:               fixtureBasicInfo(fixture),
			Tags:                    fixtureTags(fixture, tags),
			ReactionCharacterGroups: c.fixtureReactionCharacterGroups(fixtureID),
			RecycleMaterials:        c.fixtureRecycleMaterials(fixtureID, onlyDisassemble),
		}

		if subGenreID != 0 {
			subName := stringValue(subGenre["name"])
			subPath := fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(subGenre["assetbundleName"]))
			request.SubGenreName = &subName
			request.SubGenreImagePath = &subPath
		}
		if blueprint := findFixtureBlueprint(blueprints, fixtureID); blueprint != nil {
			request.BasicInfo = append(request.BasicInfo, fixtureBlueprintInfo(blueprint)...)
			request.CostMaterials = c.fixtureCostMaterials(intNumber(blueprint["id"], 0), blueprintCosts)
		}
		requests = append(requests, request)
	}

	if len(requests) == 0 {
		return nil, fmt.Errorf("mysekai fixture detail found no valid fixtures")
	}
	return requests, nil
}

func (c *Controller) RenderFixtureDetail(query FixtureDetailQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	requests, err := c.BuildFixtureDetailRequests(query)
	if err != nil {
		return nil, err
	}
	if len(requests) != 1 {
		return nil, fmt.Errorf("mysekai fixture detail render requires exactly one fixture id")
	}
	return c.drawing.GenerateMysekaiFixtureDetail(&requests[0])
}

func (c *Controller) BuildDoorUpgradeRequest(query DoorUpgradeQuery) (*drawing.MysekaiDoorUpgradeRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	specGateID := 0
	if ids := parseIntTokens(query.Query); len(ids) > 0 {
		specGateID = ids[0]
	}

	userMaterials := map[int]int{}
	for _, raw := range nestedList(merged, "userMysekaiMaterials") {
		item, _ := raw.(map[string]interface{})
		userMaterials[intNumber(item["mysekaiMaterialId"], 0)] = intNumber(item["quantity"], 0)
	}

	specLevels := map[int]int{}
	for _, raw := range nestedList(merged, "userMysekaiGates") {
		item, _ := raw.(map[string]interface{})
		gateID := intNumber(item["mysekaiGateId"], 0)
		if gateID != 0 {
			specLevels[gateID] = intNumber(item["mysekaiGateLevel"], 0)
		}
	}

	const gateMaxLevel = 40

	type tempItem struct {
		MaterialID int
		Quantity   int
	}

	gateTemp := map[int][][]tempItem{}
	for _, item := range c.masterdata.loadList("mysekaiGateMaterialGroups.json") {
		groupID := intNumber(item["groupId"], 0)
		if groupID == 0 {
			continue
		}
		gateID := groupID / 1000
		level := groupID % 1000
		if gateID == 0 || level <= 0 || level > gateMaxLevel {
			continue
		}
		if _, ok := gateTemp[gateID]; !ok {
			gateTemp[gateID] = make([][]tempItem, gateMaxLevel)
		}
		gateTemp[gateID][level-1] = append(gateTemp[gateID][level-1], tempItem{
			MaterialID: intNumber(item["mysekaiMaterialId"], 0),
			Quantity:   intNumber(item["quantity"], 0),
		})
	}

	if specGateID == 0 {
		bestLevel := 0
		for gateID, level := range specLevels {
			if level == gateMaxLevel || level <= bestLevel {
				continue
			}
			bestLevel = level
			specGateID = gateID
		}
	}
	if specGateID != 0 {
		if level := specLevels[specGateID]; level == gateMaxLevel {
			return nil, fmt.Errorf("queried gate already max level")
		}
		if mats, ok := gateTemp[specGateID]; ok {
			gateTemp = map[int][][]tempItem{specGateID: mats}
		}
	}

	materialIcons := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	gateIDs := make([]int, 0, len(gateTemp))
	for gateID := range gateTemp {
		gateIDs = append(gateIDs, gateID)
	}
	sort.Ints(gateIDs)

	green := []int{0, 200, 0}
	red := []int{200, 0, 0}

	gateMaterials := make([]drawing.MysekaiGateMaterials, 0, len(gateIDs))
	for _, gateID := range gateIDs {
		levelMats := gateTemp[gateID]
		currentLevel := specLevels[gateID]
		if currentLevel > 0 && currentLevel < len(levelMats) {
			levelMats = levelMats[currentLevel:]
		}

		sumMaterials := map[int]int{}
		outLevels := make([]drawing.MysekaiGateLevelMaterials, 0, len(levelMats))
		for index, items := range levelMats {
			if len(items) == 0 {
				continue
			}
			levelColor := []int{50, 50, 50}
			outItems := make([]drawing.MysekaiGateMaterialItem, 0, len(items))
			for _, item := range items {
				sumMaterials[item.MaterialID] += item.Quantity
				userQty := userMaterials[item.MaterialID]
				color := green
				if userQty < sumMaterials[item.MaterialID] {
					color = red
					levelColor = red
				}
				outItems = append(outItems, drawing.MysekaiGateMaterialItem{
					ImagePath:   fmt.Sprintf("mysekai/thumbnail/material/%s.png", materialIcons[item.MaterialID]),
					Quantity:    item.Quantity,
					Color:       color,
					SumQuantity: fmt.Sprintf("%s/%d", formatMysekaiQuantity(userQty), sumMaterials[item.MaterialID]),
				})
			}
			outLevels = append(outLevels, drawing.MysekaiGateLevelMaterials{
				Level: currentLevel + index + 1,
				Color: levelColor,
				Items: outItems,
			})
		}
		levelCopy := currentLevel
		gateMaterials = append(gateMaterials, drawing.MysekaiGateMaterials{
			ID:             gateID,
			Level:          &levelCopy,
			LevelMaterials: outLevels,
		})
	}

	return &drawing.MysekaiDoorUpgradeRequest{
		Profile:       c.mysekaiProfileCard(region, merged),
		GateMaterials: gateMaterials,
	}, nil
}

func (c *Controller) RenderDoorUpgrade(query DoorUpgradeQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildDoorUpgradeRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiDoorUpgrade(payload)
}

func (c *Controller) BuildMusicRecordRequest(query MusicRecordQuery) (*drawing.MysekaiMusicrecordRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	showID := false
	if query.ShowID != nil {
		showID = *query.ShowID
	}

	obtainedRecords := map[int]int64{}
	for _, raw := range nestedList(merged, "userMysekaiMusicRecords") {
		item, _ := raw.(map[string]interface{})
		obtainedRecords[intNumber(item["mysekaiMusicRecordId"], 0)] = int64Number(item["obtainedAt"], 0)
	}

	records := c.masterdata.loadList("mysekaiMusicRecords.json")
	musicTags := c.masterdata.loadList("musicTags.json")
	musics := c.masterdata.loadMapByID("musics.json")
	limitedTimes := c.masterdata.loadList("limitedTimeMusics.json")

	categoryMusicIDs := map[string][]int{
		"light_music_club": {},
		"idol":             {},
		"street":           {},
		"theme_park":       {},
		"school_refusal":   {},
		"vocaloid":         {},
		"other":            {},
	}

	limitedByMusic := map[int][]map[string]interface{}{}
	for _, item := range limitedTimes {
		musicID := intNumber(item["musicId"], 0)
		if musicID != 0 {
			limitedByMusic[musicID] = append(limitedByMusic[musicID], item)
		}
	}

	tagByMusicID := map[int]string{}
	for _, item := range musicTags {
		musicID := intNumber(item["musicId"], 0)
		tag := stringValue(item["musicTag"])
		if musicID == 0 || tag == "" || tag == "all" || tag == "vocaloid" {
			continue
		}
		if _, ok := tagByMusicID[musicID]; !ok {
			tagByMusicID[musicID] = tag
		}
	}

	musicObtainedAt := map[int]int64{}
	nowMs := time.Now().UnixMilli()
	for _, record := range records {
		if stringValue(record["mysekaiMusicTrackType"]) != "music" {
			continue
		}
		recordID := intNumber(record["id"], 0)
		musicID := intNumber(record["externalId"], 0)
		if recordID == 0 || musicID == 0 {
			continue
		}
		if musicID == 241 || musicID == 290 {
			continue
		}

		music := musics[musicID]
		if len(music) == 0 {
			continue
		}
		if int64Number(music["publishedAt"], 0) > nowMs {
			continue
		}
		if windows := limitedByMusic[musicID]; len(windows) > 0 && !isMusicAvailableNow(windows, nowMs) {
			continue
		}

		if ts, ok := obtainedRecords[recordID]; ok {
			musicObtainedAt[musicID] = ts
		}

		tag := tagByMusicID[musicID]
		if tag == "" {
			tag = "vocaloid"
		}
		categoryMusicIDs[tag] = append(categoryMusicIDs[tag], musicID)
	}

	tagIcons := map[string]string{
		"light_music_club": "icon_light_sound.png",
		"idol":             "icon_idol.png",
		"street":           "icon_street.png",
		"theme_park":       "icon_theme_park.png",
		"school_refusal":   "icon_school_refusal.png",
		"vocaloid":         "icon_piapro.png",
		"other":            "",
	}
	order := []string{"light_music_club", "street", "idol", "theme_park", "school_refusal", "vocaloid", "other"}

	totalCount := 0
	obtainedCount := 0
	categories := make([]drawing.MysekaiCategoryMusicrecord, 0, len(order))
	for _, tag := range order {
		musicIDs := categoryMusicIDs[tag]
		sort.Slice(musicIDs, func(i, j int) bool {
			left, leftObtained := musicObtainedAt[musicIDs[i]]
			right, rightObtained := musicObtainedAt[musicIDs[j]]
			if leftObtained && rightObtained {
				return left < right
			}
			if leftObtained != rightObtained {
				return leftObtained
			}
			return musicIDs[i] < musicIDs[j]
		})

		categoryTotal := len(musicIDs)
		categoryObtained := 0
		records := make([]drawing.MysekaiMusicrecord, 0, len(musicIDs))
		for _, musicID := range musicIDs {
			totalCount++
			if musicObtainedAt[musicID] != 0 {
				obtainedCount++
				categoryObtained++
			}
			assetbundleName := stringValue(musics[musicID]["assetbundleName"])
			if assetbundleName == "" {
				continue
			}
			record := drawing.MysekaiMusicrecord{
				ImagePath: fmt.Sprintf("music/jacket/%s/%s.png", assetbundleName, assetbundleName),
				Obtained:  musicObtainedAt[musicID] != 0,
			}
			if showID {
				idCopy := musicID
				record.ID = &idCopy
			}
			records = append(records, record)
		}
		if categoryTotal == 0 {
			continue
		}
		message := fmt.Sprintf("%d/%d (%.1f%%)", categoryObtained, categoryTotal, percent(categoryObtained, categoryTotal))
		categories = append(categories, drawing.MysekaiCategoryMusicrecord{
			Tag:             tag,
			TagIconPath:     tagIcons[tag],
			ProgressMessage: &message,
			Musicrecords:    records,
		})
	}

	profile := c.mysekaiProfileCard(region, merged)
	if profile == nil {
		return nil, fmt.Errorf("mysekai music record requires profile data")
	}
	request := &drawing.MysekaiMusicrecordRequest{
		Profile:              *profile,
		CategoryMusicrecords: categories,
	}
	if totalCount > 0 {
		message := fmt.Sprintf("总收集进度: %d/%d (%.1f%%)", obtainedCount, totalCount, percent(obtainedCount, totalCount))
		request.ProgressMessage = &message
	}
	return request, nil
}

func (c *Controller) RenderMusicRecord(query MusicRecordQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicRecordRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiMusicRecord(payload)
}

func (c *Controller) BuildTalkListRequest(query TalkListQuery) (*drawing.MysekaiTalkListRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(query.Query) == "" {
		return nil, fmt.Errorf("mysekai talk list requires character query")
	}

	_, characterUnitID := c.resolveTalkCharacter(query.Query)
	if characterUnitID == 0 {
		return nil, fmt.Errorf("mysekai talk list invalid character query: %s", query.Query)
	}

	obtainedFixtureIDs := c.obtainedMysekaiFixtureIDs(merged, c.masterdata.loadMapByID("mysekaiBlueprints.json"))
	fixturesData := c.masterdata.loadList("mysekaiFixtures.json")
	fixtureMap := c.masterdata.loadMapByID("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	gameCharacterUnitGroups := c.masterdata.loadMapByID("mysekaiGameCharacterUnitGroups.json")
	archiveGroups := c.masterdata.loadMapByID("characterArchiveMysekaiCharacterTalkGroups.json")
	conditions := c.masterdata.loadList("mysekaiCharacterTalkConditions.json")
	conditionGroups := c.masterdata.loadList("mysekaiCharacterTalkConditionGroups.json")
	talks := c.masterdata.loadList("mysekaiCharacterTalks.json")

	userTalkReads := map[int]bool{}
	for _, raw := range nestedList(merged, "userMysekaiCharacterTalks") {
		item, _ := raw.(map[string]interface{})
		userTalkReads[intNumber(item["mysekaiCharacterTalkId"], 0)] = boolValue(item["isRead"])
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
		if stringValue(condition["mysekaiCharacterTalkConditionType"]) != "mysekai_fixture_id" {
			continue
		}
		fixtureID := intNumber(condition["mysekaiCharacterTalkConditionTypeValue"], 0)
		if fixtureID != 0 {
			conditionIDsByFixture[fixtureID] = append(conditionIDsByFixture[fixtureID], intNumber(condition["id"], 0))
		}
	}

	groupIDsByCondition := map[int][]int{}
	for _, group := range conditionGroups {
		conditionID := intNumber(group["mysekaiCharacterTalkConditionId"], 0)
		groupIDsByCondition[conditionID] = append(groupIDsByCondition[conditionID], intNumber(group["id"], 0))
	}

	talksByGroup := map[int][]map[string]interface{}{}
	for _, talk := range talks {
		groupID := intNumber(talk["mysekaiCharacterTalkConditionGroupId"], 0)
		talksByGroup[groupID] = append(talksByGroup[groupID], talk)
	}

	archiveReads := map[int]*talkRead{}
	for _, fixture := range fixturesData {
		fixtureID := intNumber(fixture["id"], 0)
		if fixtureID == 0 || stringValue(fixture["mysekaiFixtureType"]) == "gate" {
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
				talkID := intNumber(talk["id"], 0)
				group := gameCharacterUnitGroups[intNumber(talk["mysekaiGameCharacterUnitGroupId"], 0)]
				if len(group) == 0 {
					continue
				}
				groupCuids := extractGroupCuids(group)
				if !containsInt(groupCuids, characterUnitID) {
					continue
				}

				archiveID := intNumber(talk["characterArchiveMysekaiCharacterTalkGroupId"], 0)
				archive := archiveGroups[archiveID]
				if len(archive) > 0 && stringValue(archive["archiveDisplayType"]) != "normal" {
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
		mainGenreID := intNumber(fixtureMap[fixtureIDs[0]]["mysekaiFixtureMainGenreId"], 0)
		fixtures := make([]drawing.MysekaiFixture, 0, len(fixtureIDs))
		for _, fixtureID := range fixtureIDs {
			fixture := fixtureMap[fixtureID]
			fixtures = append(fixtures, drawing.MysekaiFixture{
				ID:        fixtureID,
				ImagePath: fixtureThumbnailPath(fixture),
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
		singleMainGenres = append(singleMainGenres, drawing.MysekaiSingleTalkMainGenre{
			Name:      stringValue(info["name"]),
			ImagePath: fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(info["assetbundleName"])),
			SubGenres: [][]drawing.MysekaiTalkFixtures{groupedSingle[mainGenreID]},
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
				icons = append(icons, fmt.Sprintf("chara_icon/%s.png", charaIconName(cuid)))
			}
			iconGroups = append(iconGroups, icons)
		}

		fixtures := make([]drawing.MysekaiFixture, 0, len(fixtureIDs))
		for _, fixtureID := range fixtureIDs {
			fixture := fixtureMap[fixtureID]
			fixtures = append(fixtures, drawing.MysekaiFixture{
				ID:        fixtureID,
				ImagePath: fixtureThumbnailPath(fixture),
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

	progressMessage := fmt.Sprintf("未读对话家具列表 - 进度: %d/%d (%.1f%%)", totalReads, totalTalks, percent(totalReads, totalTalks))
	promptMessage := "*仅展示未读对话家具，灰色表示未获得蓝图"
	return &drawing.MysekaiTalkListRequest{
		Profile:          c.mysekaiProfileCard(region, merged),
		SdImagePath:      fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", characterUnitID),
		ProgressMessage:  &progressMessage,
		PromptMessage:    &promptMessage,
		ShowID:           true,
		SingleMainGenres: singleMainGenres,
		MultiReads:       multiReads,
	}, nil
}

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

func (c *Controller) ensure() error {
	if c == nil {
		return fmt.Errorf("mysekai controller is not initialized")
	}
	if c.snapshot == nil {
		return fmt.Errorf("local user snapshot is not configured")
	}
	if err := c.snapshot.Require(); err != nil {
		return err
	}
	if c.masterdata == nil || !c.masterdata.Configured() {
		return fmt.Errorf("local mysekai masterdata is not configured")
	}
	return nil
}

func (c *Controller) resolveRegion(region string) renderregion.Value {
	normalized := renderregion.Normalize(region)
	if !normalized.IsZero() {
		return normalized
	}
	if !c.defaultRegion.IsZero() {
		return c.defaultRegion
	}
	return renderregion.JP
}

func (c *Controller) prepareSnapshot(region string) (map[string]interface{}, renderregion.Value, error) {
	if err := c.ensure(); err != nil {
		return nil, renderregion.Unknown, err
	}
	rawBytes, err := c.snapshot.RawBytes()
	if err != nil {
		return nil, renderregion.Unknown, err
	}
	var merged map[string]interface{}
	if err := json.Unmarshal(rawBytes, &merged); err != nil {
		return nil, renderregion.Unknown, fmt.Errorf("decode merged mysekai snapshot: %w", err)
	}
	return merged, c.resolveRegion(region), nil
}

func (c *Controller) mysekaiProfileCard(region renderregion.Value, merged map[string]interface{}) *drawing.ProfileCardRequest {
	if c == nil || c.snapshot == nil {
		return nil
	}
	profile := c.snapshot.ProfileCard(region)
	if profile == nil {
		return nil
	}
	if updated, ok := merged["userMysekaiGamedata"].(map[string]interface{}); ok {
		if level := intNumber(updated["mysekaiRank"], 0); level > 0 {
			profile.MysekaiLevel = &level
		}
	}
	return profile
}

func (c *Controller) obtainedMysekaiFixtureIDs(merged map[string]interface{}, blueprints map[int]map[string]interface{}) map[int]struct{} {
	result := map[int]struct{}{}
	for _, raw := range nestedList(merged, "userMysekaiBlueprints") {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		blueprintID := intNumber(item["mysekaiBlueprintId"], 0)
		blueprint := blueprints[blueprintID]
		if len(blueprint) == 0 || stringValue(blueprint["mysekaiCraftType"]) != "mysekai_fixture" {
			continue
		}
		targetID := intNumber(blueprint["craftTargetId"], 0)
		if targetID != 0 {
			result[targetID] = struct{}{}
		}
	}
	return result
}

func (c *Controller) extractVisitCharacters(merged map[string]interface{}) []drawing.MysekaiVisitCharacter {
	visit, ok := merged["userMysekaiGateCharacterVisit"].(map[string]interface{})
	if !ok {
		return []drawing.MysekaiVisitCharacter{}
	}

	groupMap := c.masterdata.loadMapByID("mysekaiGameCharacterUnitGroups.json")
	characters, ok := visit["userMysekaiGateCharacters"].([]interface{})
	if !ok {
		return []drawing.MysekaiVisitCharacter{}
	}

	result := make([]drawing.MysekaiVisitCharacter, 0, len(characters))
	seen := map[int]struct{}{}
	for _, item := range characters {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		groupID := intNumber(entry["mysekaiGameCharacterUnitGroupId"], 0)
		group := groupMap[groupID]
		if len(group) == 0 || intNumber(group["gameCharacterUnitId2"], 0) != 0 {
			continue
		}
		displayUnitID := intNumber(group["gameCharacterUnitId1"], 0)
		if displayUnitID == 0 {
			continue
		}
		if _, ok := seen[displayUnitID]; ok {
			continue
		}
		seen[displayUnitID] = struct{}{}

		var memoriaPath *string
		if gameCharacterID := c.gameCharacterIDByUnitID(displayUnitID); gameCharacterID > 0 {
			path := fmt.Sprintf("mysekai/item_preview/material/item_memoria_%d.png", gameCharacterID)
			memoriaPath = &path
		}
		var reservationIconPath *string
		if boolValue(entry["isReservation"]) {
			path := "mysekai/invitationcard.png"
			reservationIconPath = &path
		}

		result = append(result, drawing.MysekaiVisitCharacter{
			SdImagePath:         fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", displayUnitID),
			MemoriaImagePath:    memoriaPath,
			IsRead:              false,
			IsReservation:       boolValue(entry["isReservation"]),
			ReservationIconPath: reservationIconPath,
		})
		if len(result) >= 6 {
			break
		}
	}
	return result
}

func (c *Controller) extractSiteResourceNumbers(merged map[string]interface{}) []drawing.MysekaiSiteResourceNumber {
	updated := nestedList(merged, "userMysekaiHarvestMaps")
	if len(updated) == 0 {
		return []drawing.MysekaiSiteResourceNumber{}
	}

	counts := map[int]map[string]int{5: {}, 7: {}, 6: {}, 8: {}}
	for _, item := range updated {
		siteMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		siteID := intNumber(siteMap["mysekaiSiteId"], 0)
		if _, ok := counts[siteID]; !ok {
			counts[siteID] = map[string]int{}
		}

		drops, _ := siteMap["userMysekaiSiteHarvestResourceDrops"].([]interface{})
		for _, rawDrop := range drops {
			drop, ok := rawDrop.(map[string]interface{})
			if !ok {
				continue
			}
			if stringValue(drop["mysekaiSiteHarvestResourceDropStatus"]) != "before_drop" {
				continue
			}
			key := fmt.Sprintf("%s_%d", drop["resourceType"], intNumber(drop["resourceId"], 0))
			counts[siteID][key] += intNumber(drop["quantity"], 0)
		}
	}

	materialMap := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	materialRarityMap := c.loadFieldMap("mysekaiMaterials.json", "mysekaiMaterialRarityType")
	itemMap := c.loadIconNameMap("mysekaiItems.json", "iconAssetbundleName")
	musicRecordMap := c.loadMusicRecordJacketMap()

	order := []int{5, 7, 6, 8}
	result := make([]drawing.MysekaiSiteResourceNumber, 0, len(order))
	for _, siteID := range order {
		resMap := counts[siteID]
		keys := sortKeysByResource(resMap, materialRarityMap)
		resources := make([]drawing.MysekaiResourceNumber, 0, len(keys))
		for _, key := range keys {
			imagePath, hasRecord := c.resourceImagePath(key, materialMap, itemMap, musicRecordMap, merged)
			if imagePath == "" {
				continue
			}
			resources = append(resources, drawing.MysekaiResourceNumber{
				ImagePath:           imagePath,
				Number:              resMap[key],
				TextColor:           resourceTextColor(key, materialRarityMap),
				HasMusicRecord:      hasRecord,
				MusicRecordIconPath: musicRecordIconPath(hasRecord),
			})
		}
		if len(resources) == 0 {
			continue
		}
		result = append(result, drawing.MysekaiSiteResourceNumber{
			ImagePath:       fmt.Sprintf("mysekai/site/sitemap/texture/img_harvest_site_%d.png", siteID),
			ResourceNumbers: resources,
		})
	}
	return result
}

func (c *Controller) resourceImagePath(key string, materialMap, itemMap, musicRecordMap map[int]string, merged map[string]interface{}) (string, bool) {
	parts := strings.Split(key, "_")
	if len(parts) < 2 {
		return "", false
	}
	id := intNumber(parts[len(parts)-1], 0)
	typeKey := strings.TrimSuffix(key, fmt.Sprintf("_%d", id))
	switch typeKey {
	case "mysekai_material":
		if icon := materialMap[id]; icon != "" {
			return fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon), false
		}
	case "material":
		return fmt.Sprintf("thumbnail/material_rip/material%d.png", id), false
	case "mysekai_item":
		if icon := itemMap[id]; icon != "" {
			return fmt.Sprintf("mysekai/thumbnail/item/%s.png", icon), false
		}
	case "mysekai_music_record":
		if jacket := musicRecordMap[id]; jacket != "" {
			return fmt.Sprintf("music/jacket/%s/%s.png", jacket, jacket), c.hasMysekaiMusicRecord(merged, id)
		}
	}
	return "", false
}

func (c *Controller) hasMysekaiMusicRecord(merged map[string]interface{}, recordID int) bool {
	for _, item := range nestedList(merged, "userMysekaiMusicRecords") {
		entry, ok := item.(map[string]interface{})
		if ok && intNumber(entry["mysekaiMusicRecordId"], 0) == recordID {
			return true
		}
	}
	return false
}

func (c *Controller) loadIconNameMap(filename, field string) map[int]string {
	items := c.masterdata.loadMapByID(filename)
	result := make(map[int]string, len(items))
	for id, item := range items {
		if value := stringValue(item[field]); value != "" {
			result[id] = value
		}
	}
	return result
}

func (c *Controller) loadFieldMap(filename, field string) map[int]string {
	items := c.masterdata.loadMapByID(filename)
	result := make(map[int]string, len(items))
	for id, item := range items {
		if value := stringValue(item[field]); value != "" {
			result[id] = value
		}
	}
	return result
}

func (c *Controller) loadMusicRecordJacketMap() map[int]string {
	records := c.masterdata.loadMapByID("mysekaiMusicRecords.json")
	musics := c.masterdata.loadMapByID("musics.json")
	result := make(map[int]string, len(records))
	for id, record := range records {
		externalID := intNumber(record["externalId"], 0)
		if externalID == 0 {
			continue
		}
		if music := musics[externalID]; len(music) > 0 {
			if assetbundleName := stringValue(music["assetbundleName"]); assetbundleName != "" {
				result[id] = assetbundleName
			}
		}
	}
	return result
}

func (c *Controller) gameCharacterIDByUnitID(unitID int) int {
	if item := c.masterdata.loadMapByID("gameCharacterUnits.json")[unitID]; len(item) > 0 {
		return intNumber(item["gameCharacterId"], 0)
	}
	return 0
}

func (c *Controller) fixtureCostMaterials(blueprintID int, costs []map[string]interface{}) []drawing.MysekaiFixtureMaterial {
	var result []drawing.MysekaiFixtureMaterial
	iconMap := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	for _, item := range costs {
		if intNumber(item["mysekaiBlueprintId"], 0) != blueprintID {
			continue
		}
		materialID := intNumber(item["mysekaiMaterialId"], 0)
		icon := iconMap[materialID]
		if icon == "" {
			continue
		}
		result = append(result, drawing.MysekaiFixtureMaterial{
			ImagePath: fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon),
			Quantity:  intNumber(item["quantity"], 0),
		})
	}
	return result
}

func (c *Controller) fixtureRecycleMaterials(fixtureID int, items []map[string]interface{}) []drawing.MysekaiFixtureMaterial {
	var result []drawing.MysekaiFixtureMaterial
	iconMap := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	for _, item := range items {
		if intNumber(item["mysekaiFixtureId"], 0) != fixtureID {
			continue
		}
		materialID := intNumber(item["mysekaiMaterialId"], 0)
		icon := iconMap[materialID]
		if icon == "" {
			continue
		}
		result = append(result, drawing.MysekaiFixtureMaterial{
			ImagePath: fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon),
			Quantity:  intNumber(item["quantity"], 0),
		})
	}
	return result
}

func (c *Controller) fixtureReactionCharacterGroups(fixtureID int) []drawing.MysekaiReactionCharacterGroups {
	var parsed map[string]interface{}
	if !c.masterdata.loadObject("mysekai/system/fixture_reaction_data/fixture_reaction_data.json", &parsed) {
		return nil
	}

	rawItems, _ := parsed["FixturerRactions"].([]interface{})
	grouped := map[int][][]int{}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok || intNumber(item["FixtureId"], 0) != fixtureID {
			continue
		}
		reactions, _ := item["ReactionCharacter"].([]interface{})
		for _, rawReaction := range reactions {
			entry, ok := rawReaction.(map[string]interface{})
			if !ok {
				continue
			}
			characters, _ := entry["CharacterUnitIds"].([]interface{})
			var characterIDs []int
			for _, character := range characters {
				id := intNumber(character, 0)
				if id != 0 {
					characterIDs = append(characterIDs, id)
				}
			}
			if len(characterIDs) > 0 {
				grouped[len(characterIDs)] = append(grouped[len(characterIDs)], characterIDs)
			}
		}
	}

	counts := make([]int, 0, len(grouped))
	for count := range grouped {
		counts = append(counts, count)
	}
	sort.Ints(counts)

	result := make([]drawing.MysekaiReactionCharacterGroups, 0, len(counts))
	for _, count := range counts {
		iconGroups := make([][]string, 0, len(grouped[count]))
		for _, ids := range grouped[count] {
			icons := make([]string, 0, len(ids))
			for _, id := range ids {
				icons = append(icons, fmt.Sprintf("chara_icon/%s.png", charaIconName(id)))
			}
			iconGroups = append(iconGroups, icons)
		}
		result = append(result, drawing.MysekaiReactionCharacterGroups{
			Number:                count,
			CharacterUintIDGroups: grouped[count],
			CharaIconPathGroups:   iconGroups,
		})
	}
	return result
}

func (c *Controller) resolveTalkCharacter(query string) (int, int) {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return 0, 0
	}

	gameCharacterUnits := c.masterdata.loadList("gameCharacterUnits.json")
	if ids := parseIntTokens(normalized); len(ids) > 0 {
		target := ids[0]
		for _, item := range gameCharacterUnits {
			if intNumber(item["id"], 0) == target {
				return intNumber(item["gameCharacterId"], 0), target
			}
			if intNumber(item["gameCharacterId"], 0) == target {
				return target, intNumber(item["id"], 0)
			}
		}
	}

	characters := c.masterdata.loadMapByID("gameCharacters.json")
	for _, token := range strings.Fields(normalized) {
		if characterID, ok := c.nicknames[token]; ok {
			for _, item := range gameCharacterUnits {
				if intNumber(item["gameCharacterId"], 0) == characterID {
					return characterID, intNumber(item["id"], 0)
				}
			}
		}
		for characterID, item := range characters {
			candidates := []string{
				strings.ToLower(stringValue(item["firstName"])),
				strings.ToLower(stringValue(item["givenName"])),
				strings.ToLower(stringValue(item["firstName"]) + stringValue(item["givenName"])),
			}
			for _, candidate := range candidates {
				if candidate == "" || candidate != token {
					continue
				}
				for _, unit := range gameCharacterUnits {
					if intNumber(unit["gameCharacterId"], 0) == characterID {
						return characterID, intNumber(unit["id"], 0)
					}
				}
			}
		}
	}
	return 0, 0
}
