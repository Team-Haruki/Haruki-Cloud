package mysekai

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/usererror"
)

// BuildFixtureListRequest builds the request for rendering MySekai fixture list view.
func (c *Controller) BuildFixtureListRequest(query FixtureListQuery) (*drawing.MysekaiFixtureListRequest, error) {
	c = c.withRegion(query.Region)
	if c == nil {
		return nil, usererror.Misconfigured(errors.New("mysekai controller is not initialized"))
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
	var firstCraft *firstCraftTally
	source := fixtureListObtainedSource(query.ObtainedSource, merged)
	if options.showObtained {
		obtainedFixtureIDs = c.fixtureListObtainedIDs(source, merged, blueprints)
		firstCraft = c.newFirstCraftTally(merged, blueprints, time.Now())
	}
	collector := newFixtureListCollector(c, region, options, categoryFilter, c.craftableMysekaiFixtureIDs(blueprints), obtainedFixtureIDs, characters)
	collector.firstCraft = firstCraft
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
		message := c.fixtureListProgressMessage(collector).String()
		request.ProgressMessage = &message
	}
	request.Labels = c.fixtureListLabels(fixtureListScope(options, source), collector.hasFirstCraft)
	return request, nil
}

// Drawing label keys of the fixture list (Drawing AGENTS.md "Caller labels
// and raw keys").
const (
	fixtureListLegendLabel     = "mysekai.fixture_list.legend"
	fixtureListFirstCraftLabel = "mysekai.fixture_list.first_craft_badge"
)

// legendSeparator joins the legend's sentences; it is punctuation, not copy.
const legendSeparator = "；"

// fixtureListScope is the legend of a fixture list: what it lists and what
// grey means for its obtained source. A static catalog greys nothing.
func fixtureListScope(options fixtureListOptions, source string) []i18n.Message {
	scope := make([]i18n.Message, 0, 2)
	if options.onlyCraftable {
		scope = append(scope, i18n.M("mysekai.image.fixture_list.legend.only_craftable"))
	}
	if options.showObtained {
		if source == obtainedSourceBlueprint {
			scope = append(scope, i18n.M("mysekai.image.fixture_list.legend.grey_no_blueprint"))
		} else {
			scope = append(scope, i18n.M("mysekai.image.fixture_list.legend.grey_never_held"))
		}
	}
	return scope
}

// fixtureListLabels are the Drawing labels of a fixture list: the legend
// (the view's scope, the ribbon sentence when ribbons are drawn and need
// explaining, the birthday icon) and the first-craft ribbon text.
func (c *Controller) fixtureListLabels(scope []i18n.Message, explainRibbon bool) map[string]string {
	badge := c.firstCraftBadge()
	legend := make([]string, 0, len(scope)+2)
	for _, sentence := range scope {
		legend = append(legend, sentence.String())
	}
	if explainRibbon {
		legend = append(legend, i18n.T("mysekai.image.fixture_list.legend.first_craft_badge", i18n.Data{"Badge": badge}))
	}
	legend = append(legend, i18n.T("mysekai.image.fixture_list.legend.birthday_icon"))
	return map[string]string{
		fixtureListLegendLabel:     strings.Join(legend, legendSeparator),
		fixtureListFirstCraftLabel: badge,
	}
}

// firstCraftBadge is the ribbon text of a tile whose first craft is
// available now: the exp it pays, or a plain word when master lacks it.
func (c *Controller) firstCraftBadge() string {
	if exp := c.firstCraftBonusExp(); exp > 0 {
		return i18n.T("mysekai.image.fixture_list.first_craft_badge", i18n.Data{"Exp": exp})
	}
	return i18n.T("mysekai.image.fixture_list.first_craft_badge_no_exp")
}

// fixtureListProgressMessage is the total progress line, followed by the
// first-craft bonuses the user can still collect when there are any.
func (c *Controller) fixtureListProgressMessage(collector *fixtureListCollector) i18n.Message {
	data := i18n.Data{"Obtained": collector.totalObtained, "Total": collector.totalAll, "Percent": i18n.Percent(percent(collector.totalObtained, collector.totalAll))}
	if collector.firstCraft == nil || collector.firstCraft.Craftable == 0 {
		return i18n.M("mysekai.image.fixture.progress", data)
	}
	bonusExp := c.firstCraftBonusExp()
	if bonusExp <= 0 {
		return i18n.M("mysekai.image.fixture.progress", data)
	}
	data["Count"] = collector.firstCraft.Craftable
	data["Exp"] = i18n.Wan(float64(collector.firstCraft.Craftable * bonusExp))
	return i18n.M("mysekai.image.fixture.progress_first_craft", data)
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
			return nil, region, usererror.Misconfigured(errors.New("mysekai masterdata is not configured"))
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
	// firstCraft counts the first-craft bonuses still available among the
	// listed fixtures; nil without user data.
	firstCraft *firstCraftTally
	// firstCraftOnly lists only fixtures whose first-craft bonus is still
	// available, bright when craftable now; firstCraftShowAll adds the ones
	// whose blueprint is missing.
	firstCraftOnly    bool
	firstCraftShowAll bool
	// fixtureImage resolves each listed thumbnail (fixtureImageResolver).
	fixtureImage pathResolver
	// hasFirstCraft is set once a listed fixture carries the first-craft
	// ribbon, so the legend explains it only when it is drawn.
	hasFirstCraft bool
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
		fixtureImage:   controller.fixtureImageResolver(region),
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
	status := c.firstCraft.statusOf(fixtureID)
	c.firstCraft.count(status)
	obtained := !c.options.showObtained || hasFixture(c.obtainedIDs, fixtureID)
	if c.firstCraftOnly {
		if !status.Known || !status.Available || (!status.Craftable && !c.firstCraftShowAll) {
			return
		}
		obtained = status.Craftable
	}
	c.ensureGenre(mainGenreID)
	firstCraft := status.Known && status.Available && status.Craftable
	if firstCraft {
		c.hasFirstCraft = true
	}
	characterID := fixtureBirthdayCharacterID(c.characters, item)
	var charaIconPath *string
	if characterID != nil {
		charaIconPath = drawing.StringPtr(c.controller.staticPath(fmt.Sprintf("chara_icon/%s.png", charaIconName(*characterID))))
	}
	c.grouped[mainGenreID][subGenreID] = append(c.grouped[mainGenreID][subGenreID], drawing.MysekaiFixture{
		ID:            fixtureID,
		ImagePath:     fixtureThumbnailPath(c.fixtureImage, item),
		CharacterID:   characterID,
		CharaIconPath: charaIconPath,
		Obtained:      obtained,
		FirstCraft:    firstCraft,
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
	message := i18n.T("mysekai.image.progress", i18n.Data{"Obtained": obtained, "Total": total, "Percent": i18n.Percent(percent(obtained, total))})
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

// The obtained sources of a fixture list: the fixtures the user has held
// (any colour, quantity 0 included) or the blueprints the user owns.
const (
	obtainedSourceFixture   = "fixture"
	obtainedSourceBlueprint = "blueprint"
)

// fixtureListObtainedSource resolves a query's obtained source. Without one,
// held fixtures decide when the snapshot lists them, else blueprints.
func fixtureListObtainedSource(source string, merged map[string]any) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "fixture", "fixtures", "crafted", "craft":
		return obtainedSourceFixture
	case "blueprint", "blueprints":
		return obtainedSourceBlueprint
	}
	if nestedList(merged, "userMysekaiFixtures") != nil {
		return obtainedSourceFixture
	}
	return obtainedSourceBlueprint
}

func (c *Controller) fixtureListObtainedIDs(source string, merged map[string]any, blueprints map[int]map[string]any) map[int]struct{} {
	if source == obtainedSourceBlueprint {
		return userMysekaiBlueprintFixtureIDs(merged, blueprints)
	}
	return userMysekaiHeldFixtureIDs(merged)
}

// RenderFixtureList renders the MySekai fixture list view.
func (c *Controller) RenderFixtureList(query FixtureListQuery) ([]byte, error) {
	image, err := c.RenderFixtureListImage(query)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderFixtureListImage(query FixtureListQuery) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildFixtureListRequest(query)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateMysekaiFixtureListImage(payload)
}

// BuildFixtureDetailRequests builds the requests for rendering MySekai fixture detail views.
func (c *Controller) BuildFixtureDetailRequests(query FixtureDetailQuery) ([]drawing.MysekaiFixtureDetailRequest, error) {
	c = c.withRegion(query.Region)
	if err := c.ensureMasterdata(); err != nil {
		return nil, err
	}

	fixtureIDs := parseIntTokens(query.Query)
	if len(fixtureIDs) == 0 {
		return nil, usererror.BadParam(query.Query, i18n.M("mysekai.fixture.id_invalid"))
	}

	fixtureMap := c.masterdata.loadMapByID("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	subGenreMap := c.masterdata.loadMapByID("mysekaiFixtureSubGenres.json")
	blueprints := c.masterdata.loadList("mysekaiBlueprints.json")
	blueprintCosts := c.masterdata.loadList("mysekaiBlueprintMysekaiMaterialCosts.json")
	onlyDisassemble := c.masterdata.loadList("mysekaiFixtureOnlyDisassembleMaterials.json")
	tags := c.masterdata.loadMapByID("mysekaiFixtureTags.json")
	region := c.resolveRegion(query.Region)
	now := query.NowMillis
	if now == 0 {
		now = time.Now().UnixMilli()
	}
	firstCraft, bonusExp := c.fixtureDetailFirstCraft(query, now)

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
			Title:              i18n.T("mysekai.image.fixture.title", i18n.Data{"Region": i18n.RegionLabel(region.String()), "ID": fixtureID, "Name": stringValue(fixture["name"])}),
			Images:             fixtureColorImages(c.fixtureImageResolver(region), fixture),
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
			request.BasicInfo = append(request.BasicInfo, c.fixtureBlueprintTermInfo(intNumber(blueprint["id"], 0), now)...)
			request.CostMaterials = c.fixtureCostMaterials(region, intNumber(blueprint["id"], 0), blueprintCosts)
			request.Friendcodes, request.FriendcodeSource = c.fixtureFriendcodes(
				region,
				fixtureID,
				stringValue(fixture["name"]),
				boolValue(blueprint["isEnableSketch"]),
			)
		}
		if info := fixtureFirstCraftInfo(firstCraft.statusOf(fixtureID), bonusExp); info != "" {
			request.BasicInfo = append(request.BasicInfo, info)
		}
		requests = append(requests, request)
	}

	if len(requests) == 0 {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("mysekai.fixture.not_found"))
	}
	return requests, nil
}

// fixtureDetailFirstCraft reads the user's first-craft state for the detail.
// The detail is static without it, so a missing or unreadable snapshot only
// leaves the first-craft tag out.
func (c *Controller) fixtureDetailFirstCraft(query FixtureDetailQuery, now int64) (*firstCraftTally, int) {
	if !query.WithUserData {
		return nil, 0
	}
	merged, _, err := c.prepareSnapshotOnly(query.Region)
	if err != nil {
		return nil, 0
	}
	tally := c.newFirstCraftTally(merged, c.masterdata.loadMapByID("mysekaiBlueprints.json"), time.UnixMilli(now))
	if tally == nil {
		return nil, 0
	}
	return tally, c.firstCraftBonusExp()
}

// RenderFixtureDetail renders the MySekai fixture detail view.
func (c *Controller) RenderFixtureDetail(query FixtureDetailQuery) ([]byte, error) {
	image, err := c.RenderFixtureDetailImage(query)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderFixtureDetailImage(query FixtureDetailQuery) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	requests, err := c.BuildFixtureDetailRequests(query)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	if len(requests) != 1 {
		return drawing.ImageResult{}, usererror.Misuse(i18n.M("mysekai.fixture.one_only"))
	}
	return c.drawing.GenerateMysekaiFixtureDetailImage(&requests[0])
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
