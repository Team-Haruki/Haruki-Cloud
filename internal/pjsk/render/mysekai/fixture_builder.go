package mysekai

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

// BuildFixtureListRequest builds the request for rendering MySekai fixture list view.
func (c *Controller) BuildFixtureListRequest(query FixtureListQuery) (*drawing.MysekaiFixtureListRequest, error) {
	c = c.withRegion(query.Region)
	if c == nil {
		return nil, fmt.Errorf("mysekai controller is not initialized")
	}

	options := fixtureListOptionsFromQuery(query)
	merged, region, err := c.prepareFixtureListData(query.Region, options)
	if err != nil {
		return nil, err
	}

	fixturesData := c.masterdata.loadList("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	subGenreMap := c.masterdata.loadMapByID("mysekaiFixtureSubGenres.json")
	categoryFilter, err := resolveFixtureCategoryFilter(query.CategoryQuery, mainGenreMap, subGenreMap)
	if err != nil {
		return nil, err
	}
	blueprints := c.masterdata.loadMapByID("mysekaiBlueprints.json")
	characters := c.masterdata.loadMapByID("gameCharacters.json")
	obtainedFixtureIDs := map[int]struct{}{}
	if options.showObtained {
		obtainedFixtureIDs = c.fixtureListObtainedIDs(query.ObtainedSource, merged, blueprints)
	}
	collector := newFixtureListCollector(c, region, options, categoryFilter, c.craftableMysekaiFixtureIDs(blueprints), obtainedFixtureIDs, characters)
	for _, item := range fixturesData {
		collector.add(item)
	}

	var profile *drawing.ProfileCardRequest
	if options.showProfile {
		profile = c.mysekaiProfileCard(region, merged, query.Profile, false)
	}

	request := &drawing.MysekaiFixtureListRequest{
		Profile:    profile,
		ShowID:     options.showID,
		MainGenres: collector.mainGenres(mainGenreMap, subGenreMap),
	}
	if options.showProgress && collector.totalAll > 0 {
		message := fmt.Sprintf("总收集进度（不含生日家具）: %d/%d (%.1f%%)", collector.totalObtained, collector.totalAll, percent(collector.totalObtained, collector.totalAll))
		request.ProgressMessage = &message
	}
	return request, nil
}

type fixtureListOptions struct {
	showID        bool
	onlyCraftable bool
	showProfile   bool
	showProgress  bool
	showObtained  bool
}

func fixtureListOptionsFromQuery(query FixtureListQuery) fixtureListOptions {
	return fixtureListOptions{
		showID:        boolOption(query.ShowID, true),
		onlyCraftable: boolOption(query.OnlyCraftable, false),
		showProfile:   boolOption(query.ShowProfile, true),
		showProgress:  boolOption(query.ShowProgress, true),
		showObtained:  boolOption(query.ShowObtained, true),
	}
}

func boolOption(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func (c *Controller) prepareFixtureListData(regionQuery string, options fixtureListOptions) (map[string]any, renderregion.Value, error) {
	region := c.resolveRegion(regionQuery)
	if !options.showObtained && !options.showProfile && !options.showProgress {
		if c.masterdata == nil || !c.masterdata.Configured() {
			return nil, region, fmt.Errorf("mysekai masterdata is not configured")
		}
		return map[string]any{}, region, nil
	}
	return c.prepareSnapshot(regionQuery)
}

type fixtureListCollector struct {
	controller     *Controller
	region         renderregion.Value
	options        fixtureListOptions
	categoryFilter *fixtureCategoryFilter
	craftableIDs   map[int]struct{}
	obtainedIDs    map[int]struct{}
	characters     map[int]map[string]any
	grouped        map[int]map[int][]drawing.MysekaiFixture
	mainAll        map[int]int
	mainObtained   map[int]int
	subAll         map[int]map[int]int
	subObtained    map[int]map[int]int
	totalAll       int
	totalObtained  int
}

func newFixtureListCollector(
	controller *Controller,
	region renderregion.Value,
	options fixtureListOptions,
	categoryFilter *fixtureCategoryFilter,
	craftableIDs map[int]struct{},
	obtainedIDs map[int]struct{},
	characters map[int]map[string]any,
) *fixtureListCollector {
	return &fixtureListCollector{
		controller:     controller,
		region:         region,
		options:        options,
		categoryFilter: categoryFilter,
		craftableIDs:   craftableIDs,
		obtainedIDs:    obtainedIDs,
		characters:     characters,
		grouped:        map[int]map[int][]drawing.MysekaiFixture{},
		mainAll:        map[int]int{},
		mainObtained:   map[int]int{},
		subAll:         map[int]map[int]int{},
		subObtained:    map[int]map[int]int{},
	}
}

func (c *fixtureListCollector) add(item map[string]any) {
	fixtureID := intNumber(item["id"], 0)
	if !c.includesFixture(item, fixtureID) {
		return
	}
	mainGenreID, subGenreID := fixtureListGenreIDs(item, fixtureID)
	if !c.categoryFilter.allows(mainGenreID, subGenreID) {
		return
	}
	c.ensureGenre(mainGenreID)
	obtained := !c.options.showObtained || hasFixture(c.obtainedIDs, fixtureID)
	characterID := fixtureBirthdayCharacterID(c.characters, item)
	c.grouped[mainGenreID][subGenreID] = append(c.grouped[mainGenreID][subGenreID], drawing.MysekaiFixture{
		ID:          fixtureID,
		ImagePath:   fixtureThumbnailPath(func(path string) string { return c.controller.regionPath(c.region, path) }, item),
		CharacterID: characterID,
		Obtained:    obtained,
	})
	if characterID == nil {
		c.addProgress(mainGenreID, subGenreID, obtained)
	}
}

func (c *fixtureListCollector) includesFixture(item map[string]any, fixtureID int) bool {
	if fixtureID == 0 || strings.EqualFold(stringValue(item["mysekaiFixtureType"]), "gate") {
		return false
	}
	if !c.options.onlyCraftable {
		return true
	}
	_, ok := c.craftableIDs[fixtureID]
	return ok
}

func fixtureListGenreIDs(item map[string]any, fixtureID int) (int, int) {
	mainGenreID := intNumber(item["mysekaiFixtureMainGenreId"], -1)
	subGenreID := intNumber(item["mysekaiFixtureSubGenreId"], -1)
	if fixtureID == 4 {
		return mainGenreID, 14
	}
	if fixtureMainGenreHasNoSubgenre(mainGenreID) {
		return mainGenreID, -1
	}
	return mainGenreID, subGenreID
}

func fixtureMainGenreHasNoSubgenre(genreID int) bool {
	switch genreID {
	case 4, 5, 7, 8, 9, 10, 11, 12, 13:
		return true
	default:
		return false
	}
}

func fixtureBirthdayCharacterID(characters map[int]map[string]any, item map[string]any) *int {
	characterID := birthdayCharacterID(characters, stringValue(item["name"]))
	if characterID == 0 {
		return nil
	}
	return &characterID
}

func (c *fixtureListCollector) ensureGenre(mainGenreID int) {
	if c.grouped[mainGenreID] != nil {
		return
	}
	c.grouped[mainGenreID] = map[int][]drawing.MysekaiFixture{}
	c.subAll[mainGenreID] = map[int]int{}
	c.subObtained[mainGenreID] = map[int]int{}
}

func (c *fixtureListCollector) addProgress(mainGenreID, subGenreID int, obtained bool) {
	c.totalAll++
	c.mainAll[mainGenreID]++
	c.subAll[mainGenreID][subGenreID]++
	if !obtained {
		return
	}
	c.totalObtained++
	c.mainObtained[mainGenreID]++
	c.subObtained[mainGenreID][subGenreID]++
}

func (c *fixtureListCollector) mainGenres(mainGenreMap, subGenreMap map[int]map[string]any) []drawing.MysekaiFixtureMainGenre {
	genreIDs := sortedIntKeys(c.grouped)
	genres := make([]drawing.MysekaiFixtureMainGenre, 0, len(genreIDs))
	for _, genreID := range genreIDs {
		if genre, ok := c.mainGenre(genreID, mainGenreMap[genreID], subGenreMap); ok {
			genres = append(genres, genre)
		}
	}
	return genres
}

func (c *fixtureListCollector) mainGenre(genreID int, info map[string]any, subGenreMap map[int]map[string]any) (drawing.MysekaiFixtureMainGenre, bool) {
	subGenres := c.subGenres(genreID, subGenreMap)
	if len(subGenres) == 0 {
		return drawing.MysekaiFixtureMainGenre{}, false
	}
	genre := drawing.MysekaiFixtureMainGenre{
		Name:      stringValue(info["name"]),
		ImagePath: c.controller.regionPath(c.region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(info["assetbundleName"]))),
		SubGenres: subGenres,
	}
	genre.ProgressMessage = c.progressMessage(c.mainObtained[genreID], c.mainAll[genreID])
	return genre, true
}

func (c *fixtureListCollector) subGenres(genreID int, subGenreMap map[int]map[string]any) []drawing.MysekaiFixtureSubGenre {
	subGenreIDs := sortedIntKeys(c.grouped[genreID])
	subGenres := make([]drawing.MysekaiFixtureSubGenre, 0, len(subGenreIDs))
	for _, subGenreID := range subGenreIDs {
		if subGenre, ok := c.subGenre(genreID, subGenreID, subGenreMap[subGenreID]); ok {
			subGenres = append(subGenres, subGenre)
		}
	}
	return subGenres
}

func (c *fixtureListCollector) subGenre(genreID, subGenreID int, info map[string]any) (drawing.MysekaiFixtureSubGenre, bool) {
	fixtures := c.grouped[genreID][subGenreID]
	if len(fixtures) == 0 {
		return drawing.MysekaiFixtureSubGenre{}, false
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].ID < fixtures[j].ID })
	subGenre := drawing.MysekaiFixtureSubGenre{Fixtures: fixtures}
	if subGenreID == -1 || len(c.grouped[genreID]) <= 1 || len(info) == 0 {
		return subGenre, true
	}
	name := stringValue(info["name"])
	imagePath := c.controller.regionPath(c.region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(info["assetbundleName"])))
	subGenre.Name = &name
	subGenre.ImagePath = &imagePath
	subGenre.ProgressMessage = c.progressMessage(c.subObtained[genreID][subGenreID], c.subAll[genreID][subGenreID])
	return subGenre, true
}

func (c *fixtureListCollector) progressMessage(obtained, total int) *string {
	if !c.options.showProgress || total <= 0 {
		return nil
	}
	message := fmt.Sprintf("%d/%d (%.1f%%)", obtained, total, percent(obtained, total))
	return &message
}

func sortedIntKeys[T any](values map[int]T) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func (c *Controller) fixtureListObtainedIDs(source string, merged map[string]any, blueprints map[int]map[string]any) map[int]struct{} {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "fixture", "fixtures", "crafted", "craft":
		return userMysekaiFixtureIDs(nestedList(merged, "userMysekaiFixtures"))
	case "blueprint", "blueprints":
		return userMysekaiBlueprintFixtureIDs(merged, blueprints)
	default:
		return c.obtainedMysekaiFixtureIDs(merged, blueprints)
	}
}

// RenderFixtureList renders the MySekai fixture list view.
func (c *Controller) RenderFixtureList(query FixtureListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildFixtureListRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiFixtureList(payload)
}

// BuildFixtureDetailRequests builds the requests for rendering MySekai fixture detail views.
func (c *Controller) BuildFixtureDetailRequests(query FixtureDetailQuery) ([]drawing.MysekaiFixtureDetailRequest, error) {
	c = c.withRegion(query.Region)
	if err := c.ensureMasterdata(); err != nil {
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
			request.Friendcodes, request.FriendcodeSource = c.fixtureFriendcodes(
				region,
				fixtureID,
				stringValue(fixture["name"]),
				boolValue(blueprint["isEnableSketch"]),
			)
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
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	requests, err := c.BuildFixtureDetailRequests(query)
	finishBuild()
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
	if !c.loadFixtureReactionObject(c.defaultRegion, &parsed) {
		return nil
	}

	rawItems, _ := parsed["FixturerRactions"].([]any)
	grouped := groupFixtureReactionCharacters(rawItems, fixtureID)
	return c.buildFixtureReactionCharacterGroups(grouped)
}

func groupFixtureReactionCharacters(rawItems []any, fixtureID int) map[int][][]int {
	grouped := map[int][][]int{}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok || intNumber(item["FixtureId"], 0) != fixtureID {
			continue
		}
		reactions, _ := item["ReactionCharacter"].([]any)
		for _, rawReaction := range reactions {
			characterIDs := fixtureReactionCharacterIDs(rawReaction)
			if len(characterIDs) > 0 {
				grouped[len(characterIDs)] = append(grouped[len(characterIDs)], characterIDs)
			}
		}
	}
	return grouped
}

func fixtureReactionCharacterIDs(raw any) []int {
	entry, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	characters, _ := entry["CharacterUnitIds"].([]any)
	characterIDs := make([]int, 0, len(characters))
	for _, character := range characters {
		if id := intNumber(character, 0); id != 0 {
			characterIDs = append(characterIDs, id)
		}
	}
	return characterIDs
}

func (c *Controller) buildFixtureReactionCharacterGroups(grouped map[int][][]int) []drawing.MysekaiReactionCharacterGroups {
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
