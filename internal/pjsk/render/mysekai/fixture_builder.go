package mysekai

import (
	"fmt"
	"sort"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

// BuildFixtureListRequest builds the request for rendering MySekai fixture list view.
func (c *Controller) BuildFixtureListRequest(query FixtureListQuery) (*drawing.MysekaiFixtureListRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	showID := true
	if query.ShowID != nil {
		showID = *query.ShowID
	}
	onlyCraftable := query.OnlyCraftable != nil && *query.OnlyCraftable

	fixturesData := c.masterdata.loadList("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	subGenreMap := c.masterdata.loadMapByID("mysekaiFixtureSubGenres.json")
	blueprints := c.masterdata.loadMapByID("mysekaiBlueprints.json")
	characters := c.masterdata.loadMapByID("gameCharacters.json")
	obtainedFixtureIDs := c.obtainedMysekaiFixtureIDs(merged, blueprints)
	craftableFixtureIDs := c.craftableMysekaiFixtureIDs(blueprints)

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
		if onlyCraftable {
			if _, ok := craftableFixtureIDs[fixtureID]; !ok {
				continue
			}
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
				ImagePath:   fixtureThumbnailPath(func(p string) string { return c.regionPath(region, p) }, item),
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
					imagePath := c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(info["assetbundleName"])))
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
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(mainInfo["assetbundleName"]))),
			SubGenres: subGenres,
		}
		if total := mainProgressAll[genreID]; total > 0 {
			message := fmt.Sprintf("%d/%d (%.1f%%)", mainProgressObtained[genreID], total, percent(mainProgressObtained[genreID], total))
			mainGenre.ProgressMessage = &message
		}
		mainGenres = append(mainGenres, mainGenre)
	}

	request := &drawing.MysekaiFixtureListRequest{
		Profile:    c.mysekaiProfileCard(region, merged, query.Profile),
		ShowID:     showID,
		MainGenres: mainGenres,
	}
	if totalAll > 0 {
		message := fmt.Sprintf("总收集进度（不含生日家具）: %d/%d (%.1f%%)", totalObtained, totalAll, percent(totalObtained, totalAll))
		request.ProgressMessage = &message
	}
	return request, nil
}

// RenderFixtureList renders the MySekai fixture list view.
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

// BuildFixtureDetailRequests builds the requests for rendering MySekai fixture detail views.
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
			Images:             fixtureColorImages(func(p string) string { return c.regionPath(region, p) }, fixture),
			MainGenreName:      stringValue(mainGenre["name"]),
			MainGenreImagePath: c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(mainGenre["assetbundleName"]))),
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
			RecycleMaterials:        c.fixtureRecycleMaterials(region, fixtureID, onlyDisassemble),
		}

		if subGenreID != 0 {
			subName := stringValue(subGenre["name"])
			subPath := c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(subGenre["assetbundleName"])))
			request.SubGenreName = &subName
			request.SubGenreImagePath = &subPath
		}
		if blueprint := findFixtureBlueprint(blueprints, fixtureID); blueprint != nil {
			request.BasicInfo = append(request.BasicInfo, fixtureBlueprintInfo(blueprint)...)
			request.CostMaterials = c.fixtureCostMaterials(region, intNumber(blueprint["id"], 0), blueprintCosts)
		}
		requests = append(requests, request)
	}

	if len(requests) == 0 {
		return nil, fmt.Errorf("mysekai fixture detail found no valid fixtures")
	}
	return requests, nil
}

// RenderFixtureDetail renders the MySekai fixture detail view.
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

// fixtureCostMaterials builds the cost materials list for a fixture blueprint.
func (c *Controller) fixtureCostMaterials(region renderregion.Value, blueprintID int, costs []map[string]any) []drawing.MysekaiFixtureMaterial {
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
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon)),
			Quantity:  intNumber(item["quantity"], 0),
		})
	}
	return result
}

// fixtureRecycleMaterials builds the recycle materials list for a fixture.
func (c *Controller) fixtureRecycleMaterials(region renderregion.Value, fixtureID int, items []map[string]any) []drawing.MysekaiFixtureMaterial {
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
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon)),
			Quantity:  intNumber(item["quantity"], 0),
		})
	}
	return result
}

// fixtureReactionCharacterGroups builds the reaction character groups for a fixture.
func (c *Controller) fixtureReactionCharacterGroups(fixtureID int) []drawing.MysekaiReactionCharacterGroups {
	var parsed map[string]any
	if !c.masterdata.loadObject("mysekai/system/fixture_reaction_data/fixture_reaction_data.json", &parsed) {
		return nil
	}

	rawItems, _ := parsed["FixturerRactions"].([]any)
	grouped := map[int][][]int{}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok || intNumber(item["FixtureId"], 0) != fixtureID {
			continue
		}
		reactions, _ := item["ReactionCharacter"].([]any)
		for _, rawReaction := range reactions {
			entry, ok := rawReaction.(map[string]any)
			if !ok {
				continue
			}
			characters, _ := entry["CharacterUnitIds"].([]any)
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
				icons = append(icons, c.staticPath(fmt.Sprintf("chara_icon/%s.png", charaIconName(id))))
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
