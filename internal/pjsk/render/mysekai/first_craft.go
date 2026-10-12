package mysekai

import (
	"errors"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/utils/usererror"
)

// The client (MysekaiUserDataUtility.IsFirstCraft) decides the first-craft
// bonus before it sends the craft request:
//   - mysekai_fixture: no userMysekaiFixtures row with this mysekaiFixtureId,
//     whatever its textureId or quantity (rows survive dismantling to 0);
//   - mysekai_canvas: no userMysekaiCanvases row with this mysekaiFixtureId,
//     whatever its cardId or isSpecialTraining;
//   - tools and materials never give it.
//
// So "first craft available" means "never held": a fixture received as a
// gift, exchange or reward has used it up as well.
//
// A blueprint can be crafted now when the user owns it, or when it is usable
// without owning it (isAvailableWithoutPossession). Every such blueprint in
// master is a birthday or anniversary party fixture with term windows in
// mysekaiBlueprintTerms, so outside those windows it counts as limited, not
// craftable.
const (
	craftTypeFixture = "mysekai_fixture"
	craftTypeCanvas  = "mysekai_canvas"

	firstCraftBonusExpType = "craft_mysekai_fixture_first_bonus"
	rankObtainedExpsFile   = "mysekaiRankObtainedExps.json"
)

// firstCraftBonusExp is the MySekai rank exp of one first craft, from master
// mysekaiRankObtainedExps. It is 0 when the region's master has no such row.
func (c *Controller) firstCraftBonusExp() int {
	if c == nil || c.masterdata == nil {
		return 0
	}
	for _, row := range c.masterdata.loadList(rankObtainedExpsFile) {
		if stringValue(row["mysekaiRankObtainedExpType"]) == firstCraftBonusExpType {
			return intNumber(row["quantity"], 0)
		}
	}
	return 0
}

// firstCraftHistory is what one snapshot says about first crafts.
type firstCraftHistory struct {
	fixtures    map[int]struct{}
	canvases    map[int]struct{}
	blueprints  map[int]struct{}
	hasFixtures bool
	hasCanvases bool
	terms       blueprintTermWindows
}

// blueprintTermWindows are the mysekaiBlueprintTerms windows by blueprint.
type blueprintTermWindows struct {
	windows map[int][][2]int64
	now     int64
}

func newBlueprintTermWindows(terms []map[string]any, now int64) blueprintTermWindows {
	windows := map[int][][2]int64{}
	for _, term := range terms {
		id := intNumber(term["mysekaiBlueprintId"], 0)
		if id == 0 {
			continue
		}
		windows[id] = append(windows[id], [2]int64{int64(intNumber(term["startAt"], 0)), int64(intNumber(term["endAt"], 0))})
	}
	return blueprintTermWindows{windows: windows, now: now}
}

// open reports whether a blueprint usable without possession can be used
// now: one of its windows is open, or it has none.
func (w blueprintTermWindows) open(blueprintID int) bool {
	windows := w.windows[blueprintID]
	if len(windows) == 0 {
		return true
	}
	for _, window := range windows {
		if window[0] <= w.now && w.now <= window[1] {
			return true
		}
	}
	return false
}

func newFirstCraftHistory(merged map[string]any) firstCraftHistory {
	fixtures, hasFixtures := presentList(merged, "userMysekaiFixtures")
	canvases, hasCanvases := presentList(merged, "userMysekaiCanvases")
	history := firstCraftHistory{
		fixtures:    userMysekaiFixtureIDs(fixtures),
		canvases:    userMysekaiFixtureIDs(canvases),
		blueprints:  map[int]struct{}{},
		hasFixtures: hasFixtures,
		hasCanvases: hasCanvases,
	}
	for _, raw := range nestedList(merged, "userMysekaiBlueprints") {
		if row, ok := raw.(map[string]any); ok {
			if id := intNumber(row["mysekaiBlueprintId"], 0); id != 0 {
				history.blueprints[id] = struct{}{}
			}
		}
	}
	return history
}

// known reports whether the snapshot holds any first-craft record at all.
func (h firstCraftHistory) known() bool {
	return h.hasFixtures || h.hasCanvases
}

// firstCraftStatus is the first-craft state of one blueprint for one user.
type firstCraftStatus struct {
	// Known is false for tools and materials, and when the snapshot lacks the
	// list the blueprint's craft type is checked against.
	Known bool
	// Available is true while crafting would still give the bonus.
	Available bool
	// Craftable is true when the user can craft it now: they own the
	// blueprint, or it is usable without owning it and its term is open.
	Craftable bool
	// Limited is true for a blueprint usable without owning it whose terms
	// are all closed now.
	Limited bool
}

func (h firstCraftHistory) status(blueprint map[string]any) firstCraftStatus {
	targetID := intNumber(blueprint["craftTargetId"], 0)
	if targetID == 0 {
		return firstCraftStatus{}
	}
	var held map[int]struct{}
	switch stringValue(blueprint["mysekaiCraftType"]) {
	case craftTypeFixture:
		if !h.hasFixtures {
			return firstCraftStatus{}
		}
		held = h.fixtures
	case craftTypeCanvas:
		if !h.hasCanvases {
			return firstCraftStatus{}
		}
		held = h.canvases
	default:
		return firstCraftStatus{}
	}
	blueprintID := intNumber(blueprint["id"], 0)
	status := firstCraftStatus{Known: true, Available: !hasFixture(held, targetID)}
	if _, owned := h.blueprints[blueprintID]; owned {
		status.Craftable = true
	} else if boolValue(blueprint["isAvailableWithoutPossession"]) {
		status.Craftable = h.terms.open(blueprintID)
		status.Limited = !status.Craftable
	}
	return status
}

// firstCraftBlueprintsByTarget indexes the fixture and canvas blueprints by
// the fixture they craft.
func firstCraftBlueprintsByTarget(blueprints map[int]map[string]any) map[int]map[string]any {
	result := make(map[int]map[string]any, len(blueprints))
	for _, blueprint := range blueprints {
		if !craftsFixture(blueprint) {
			continue
		}
		if targetID := intNumber(blueprint["craftTargetId"], 0); targetID != 0 {
			result[targetID] = blueprint
		}
	}
	return result
}

// firstCraftTally counts first-craft bonuses that are still available.
type firstCraftTally struct {
	history  firstCraftHistory
	byTarget map[int]map[string]any
	// Craftable counts the ones the user can craft now, NotOwned the ones
	// waiting for a blueprint and Limited the ones waiting for their term.
	Craftable int
	NotOwned  int
	Limited   int
}

// newFirstCraftTally is nil when the snapshot cannot tell first crafts.
func (c *Controller) newFirstCraftTally(merged map[string]any, blueprints map[int]map[string]any, now time.Time) *firstCraftTally {
	history := newFirstCraftHistory(merged)
	if !history.known() {
		return nil
	}
	history.terms = newBlueprintTermWindows(c.masterdata.loadList("mysekaiBlueprintTerms.json"), now.UnixMilli())
	return &firstCraftTally{history: history, byTarget: firstCraftBlueprintsByTarget(blueprints)}
}

// statusOf returns the first-craft state of a fixture; Known is false when
// no fixture or canvas blueprint crafts it.
func (t *firstCraftTally) statusOf(fixtureID int) firstCraftStatus {
	if t == nil {
		return firstCraftStatus{}
	}
	blueprint := t.byTarget[fixtureID]
	if blueprint == nil {
		return firstCraftStatus{}
	}
	return t.history.status(blueprint)
}

func (t *firstCraftTally) count(status firstCraftStatus) {
	if t == nil || !status.Known || !status.Available {
		return
	}
	switch {
	case status.Craftable:
		t.Craftable++
	case status.Limited:
		t.Limited++
	default:
		t.NotOwned++
	}
}

// FirstCraftListQuery asks for the fixtures and canvases whose first-craft
// bonus is still available.
type FirstCraftListQuery struct {
	Region string `json:"region,omitempty"`
	// ShowAll also draws the ones the user cannot craft now, greyed out.
	ShowAll       bool                        `json:"show_all,omitempty"`
	CategoryQuery string                      `json:"category_query,omitempty"`
	Profile       *drawing.ProfileCardRequest `json:"-"`
	// Now overrides the clock for the blueprint terms (tests).
	Now time.Time `json:"-"`
}

// FirstCraftSummary is the text part of the first-craft list.
type FirstCraftSummary struct {
	// BonusExp is the exp of one first craft; 0 when master lacks it.
	BonusExp int
	// Craftable counts the bonuses the user can collect now, NotOwned the
	// ones that need a blueprint first and Limited the ones whose
	// limited-time blueprint is not open now.
	Craftable int
	NotOwned  int
	Limited   int
	ShowAll   bool
}

// waiting is how many bonuses cannot be collected now.
func (s FirstCraftSummary) waiting() int {
	return s.NotOwned + s.Limited
}

// FirstCraftList is the first-craft list: the summary and, when anything is
// left to draw, the fixture grid.
type FirstCraftList struct {
	Summary FirstCraftSummary
	Request *drawing.MysekaiFixtureListRequest
}

// BuildFirstCraftList lists the fixtures and canvases whose first-craft
// bonus is still available. Bright tiles can be crafted now; ShowAll adds the
// ones that cannot, greyed out.
func (c *Controller) BuildFirstCraftList(query FirstCraftListQuery) (*FirstCraftList, error) {
	c = c.withRegion(query.Region)
	if c == nil {
		return nil, usererror.Misconfigured(errors.New("mysekai controller is not initialized"))
	}
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	subGenreMap := c.masterdata.loadMapByID("mysekaiFixtureSubGenres.json")
	categoryFilter, err := resolveFixtureCategoryFilter(query.CategoryQuery, mainGenreMap, subGenreMap)
	if err != nil {
		return nil, err
	}
	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}
	tally := c.newFirstCraftTally(merged, c.masterdata.loadMapByID("mysekaiBlueprints.json"), now)
	if tally == nil {
		return nil, usererror.Setup(i18n.M("mysekai.first_craft.no_history"))
	}

	options := fixtureListOptions{showID: true, showProfile: true, showObtained: true}
	collector := newFixtureListCollector(c, region, options, categoryFilter, nil, nil, c.masterdata.loadMapByID("gameCharacters.json"))
	collector.firstCraft = tally
	collector.firstCraftOnly = true
	collector.firstCraftShowAll = query.ShowAll
	for _, item := range c.masterdata.loadList("mysekaiFixtures.json") {
		collector.add(item)
	}

	summary := FirstCraftSummary{
		BonusExp:  c.firstCraftBonusExp(),
		Craftable: tally.Craftable,
		NotOwned:  tally.NotOwned,
		Limited:   tally.Limited,
		ShowAll:   query.ShowAll,
	}
	result := &FirstCraftList{Summary: summary}
	if summary.Craftable == 0 && (!query.ShowAll || summary.waiting() == 0) {
		return result, nil
	}
	message := firstCraftImageHeader(summary).String()
	result.Request = &drawing.MysekaiFixtureListRequest{
		Profile:         c.mysekaiProfileCard(region, merged, query.Profile, false),
		ProgressMessage: &message,
		ShowID:          true,
		MainGenres:      collector.mainGenres(mainGenreMap, subGenreMap),
	}
	return result, nil
}

// RenderFirstCraftList renders the image of a first-craft list.
func (c *Controller) RenderFirstCraftList(list *FirstCraftList) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	if list == nil || list.Request == nil {
		return drawing.ImageResult{}, usererror.Internal(errors.New("first craft list has nothing to draw"))
	}
	return c.drawing.GenerateMysekaiFixtureListImage(list.Request)
}

func firstCraftImageHeader(summary FirstCraftSummary) i18n.Message {
	exp := i18n.Wan(float64(summary.Craftable * summary.BonusExp))
	switch {
	case summary.BonusExp <= 0 && summary.ShowAll:
		return i18n.M("mysekai.image.first_craft.header_all_no_exp", i18n.Data{"Craftable": summary.Craftable, "Waiting": summary.waiting()})
	case summary.BonusExp <= 0:
		return i18n.M("mysekai.image.first_craft.header_no_exp", i18n.Data{"Craftable": summary.Craftable})
	case summary.ShowAll:
		return i18n.M("mysekai.image.first_craft.header_all", i18n.Data{"Craftable": summary.Craftable, "Exp": exp, "Waiting": summary.waiting()})
	default:
		return i18n.M("mysekai.image.first_craft.header", i18n.Data{"Craftable": summary.Craftable, "Exp": exp})
	}
}

// FirstCraftSummaryLines is the chat reply that goes with the first-craft
// list.
func FirstCraftSummaryLines(summary FirstCraftSummary) []i18n.Message {
	if summary.Craftable == 0 && summary.waiting() == 0 {
		return []i18n.Message{i18n.M("mysekai.first_craft.none_left")}
	}
	lines := make([]i18n.Message, 0, 5)
	if summary.BonusExp > 0 {
		total := func(count int) i18n.Message { return i18n.Wan(float64(count * summary.BonusExp)) }
		lines = append(lines, i18n.M("mysekai.first_craft.bonus", i18n.Data{"Exp": summary.BonusExp}))
		if summary.Craftable > 0 {
			lines = append(lines, i18n.M("mysekai.first_craft.craftable", i18n.Data{"Count": summary.Craftable, "Exp": total(summary.Craftable)}))
		} else {
			lines = append(lines, i18n.M("mysekai.first_craft.craftable_no_exp", i18n.Data{"Count": 0}))
		}
		if summary.NotOwned > 0 {
			lines = append(lines, i18n.M("mysekai.first_craft.not_owned", i18n.Data{"Count": summary.NotOwned, "Exp": total(summary.NotOwned)}))
		}
		if summary.Limited > 0 {
			lines = append(lines, i18n.M("mysekai.first_craft.limited", i18n.Data{"Count": summary.Limited, "Exp": total(summary.Limited)}))
		}
	} else {
		lines = append(lines, i18n.M("mysekai.first_craft.craftable_no_exp", i18n.Data{"Count": summary.Craftable}))
		if summary.NotOwned > 0 {
			lines = append(lines, i18n.M("mysekai.first_craft.not_owned_no_exp", i18n.Data{"Count": summary.NotOwned}))
		}
		if summary.Limited > 0 {
			lines = append(lines, i18n.M("mysekai.first_craft.limited_no_exp", i18n.Data{"Count": summary.Limited}))
		}
	}
	switch {
	case summary.waiting() == 0:
	case summary.ShowAll:
		lines = append(lines, i18n.M("mysekai.first_craft.hint_grey"))
	case summary.Craftable == 0:
		lines = append(lines, i18n.M("mysekai.first_craft.hint_all_only"))
	default:
		lines = append(lines, i18n.M("mysekai.first_craft.hint_all"))
	}
	return lines
}

// fixtureFirstCraftInfo is the first-craft tag of the fixture detail, or ""
// when the fixture has no fixture or canvas blueprint or the snapshot cannot
// tell.
func fixtureFirstCraftInfo(status firstCraftStatus, bonusExp int) string {
	if !status.Known {
		return ""
	}
	if !status.Available {
		return i18n.T("mysekai.image.fixture.first_craft_used")
	}
	if bonusExp <= 0 {
		switch {
		case status.Craftable:
			return i18n.T("mysekai.image.fixture.first_craft_available_no_exp")
		case status.Limited:
			return i18n.T("mysekai.image.fixture.first_craft_available_no_exp_limited")
		default:
			return i18n.T("mysekai.image.fixture.first_craft_available_no_exp_no_blueprint")
		}
	}
	data := i18n.Data{"Exp": bonusExp}
	switch {
	case status.Craftable:
		return i18n.T("mysekai.image.fixture.first_craft_available", data)
	case status.Limited:
		return i18n.T("mysekai.image.fixture.first_craft_available_limited", data)
	default:
		return i18n.T("mysekai.image.fixture.first_craft_available_no_blueprint", data)
	}
}

// presentList is nestedList that also says whether the key exists, so an
// empty list (nothing held) is told apart from a missing one (unknown).
func presentList(root map[string]any, key string) ([]any, bool) {
	if root == nil {
		return nil, false
	}
	if items, ok := root[key].([]any); ok {
		return items, true
	}
	if updated, ok := root["updatedResources"].(map[string]any); ok {
		if items, ok := updated[key].([]any); ok {
			return items, true
		}
	}
	return nil, false
}
