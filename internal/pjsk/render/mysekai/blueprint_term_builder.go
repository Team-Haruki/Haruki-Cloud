package mysekai

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/drawing"
)

// BlueprintTermQuery requests the limited-time blueprint view built from
// mysekaiBlueprintTerms (JP 7.0.0 adds tab types, term material costs and
// craft limits; EN 6.0.0 has the terms without them).
type BlueprintTermQuery struct {
	Region  string `json:"region,omitempty"`
	ShowAll *bool  `json:"show_all,omitempty"`
	// NowMillis overrides the clock (tests).
	NowMillis int64 `json:"-"`
}

var blueprintTermTabTitles = map[string]string{
	"limited_term":         "限时蓝图",
	"birthday_anniversary": "生日/周年蓝图",
}

var blueprintTermTabOrder = map[string]int{"limited_term": 0, "birthday_anniversary": 1}

// blueprintTermTabTitle names a tab; a region whose terms carry no tab type
// gets one generic tab.
func blueprintTermTabTitle(tabType string) string {
	if title, ok := blueprintTermTabTitles[tabType]; ok {
		return title
	}
	if tabType == "" {
		return "限时蓝图"
	}
	return tabType
}

// BuildBlueprintTermRequest lists the current and upcoming limited-time
// blueprints (all terms with ShowAll).
func (c *Controller) BuildBlueprintTermRequest(query BlueprintTermQuery) (*drawing.MysekaiBlueprintTermRequest, error) {
	c = c.withRegion(query.Region)
	if err := c.ensureMasterdata(); err != nil {
		return nil, err
	}
	region := c.resolveRegion(query.Region)
	terms := c.masterdata.loadList("mysekaiBlueprintTerms.json")
	if len(terms) == 0 {
		return nil, fmt.Errorf("mysekai blueprint terms are not available in region %s", region)
	}
	now := query.NowMillis
	if now == 0 {
		now = time.Now().UnixMilli()
	}
	showAll := query.ShowAll != nil && *query.ShowAll
	blueprints := c.masterdata.loadMapByID("mysekaiBlueprints.json")
	fixtures := c.masterdata.loadMapByID("mysekaiFixtures.json")
	costGroups := blueprintTermCostGroups(c.masterdata.loadList("mysekaiBlueprintTermMysekaiMaterialCosts.json"))
	materialIcons := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	resolve := func(p string) string { return c.regionPath(region, p) }

	tabs := map[string]*drawing.MysekaiBlueprintTermTab{}
	for _, term := range sortedBlueprintTerms(terms) {
		if !showAll && int64(intNumber(term["endAt"], 0)) < now {
			continue
		}
		tabType := stringValue(term["mysekaiBlueprintTermTabType"])
		tab := tabs[tabType]
		if tab == nil {
			tab = &drawing.MysekaiBlueprintTermTab{TabType: tabType, Title: blueprintTermTabTitle(tabType)}
			tabs[tabType] = tab
		}
		entry := drawing.MysekaiBlueprintTermBlueprint{
			ID:      intNumber(term["mysekaiBlueprintId"], 0),
			StartAt: int64(intNumber(term["startAt"], 0)),
			EndAt:   int64(intNumber(term["endAt"], 0)),
		}
		if fixture := fixtures[intNumber(blueprints[entry.ID]["craftTargetId"], 0)]; fixture != nil {
			entry.Name = stringValue(fixture["name"])
			entry.ImagePath = drawing.AssetPath(fixtureThumbnailPath(resolve, fixture))
		}
		if limit := intNumber(term["craftLimit"], 0); limit > 0 {
			entry.CraftLimit = &limit
		}
		for _, cost := range costGroups[intNumber(term["mysekaiBlueprintTermMysekaiMaterialCostGroupId"], 0)] {
			icon := materialIcons[intNumber(cost["mysekaiMaterialId"], 0)]
			if icon == "" {
				continue
			}
			entry.CostMaterials = append(entry.CostMaterials, drawing.MysekaiBlueprintTermMaterial{
				ImagePath: drawing.AssetPath(resolve(fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon))),
				Quantity:  intNumber(cost["quantity"], 0),
			})
		}
		tab.Blueprints = append(tab.Blueprints, entry)
	}
	if len(tabs) == 0 {
		return nil, fmt.Errorf("mysekai blueprint terms have no current term in region %s", region)
	}
	request := &drawing.MysekaiBlueprintTermRequest{Tabs: make([]drawing.MysekaiBlueprintTermTab, 0, len(tabs))}
	for _, tab := range tabs {
		request.Tabs = append(request.Tabs, *tab)
	}
	sort.SliceStable(request.Tabs, func(i, j int) bool {
		return blueprintTermTabRank(request.Tabs[i].TabType) < blueprintTermTabRank(request.Tabs[j].TabType) ||
			blueprintTermTabRank(request.Tabs[i].TabType) == blueprintTermTabRank(request.Tabs[j].TabType) && request.Tabs[i].TabType < request.Tabs[j].TabType
	})
	return request, nil
}

func blueprintTermTabRank(tabType string) int {
	if rank, ok := blueprintTermTabOrder[tabType]; ok {
		return rank
	}
	return len(blueprintTermTabOrder)
}

func sortedBlueprintTerms(terms []map[string]any) []map[string]any {
	sorted := append([]map[string]any(nil), terms...)
	sort.SliceStable(sorted, func(i, j int) bool {
		si, sj := intNumber(sorted[i]["startAt"], 0), intNumber(sorted[j]["startAt"], 0)
		if si != sj {
			return si < sj
		}
		return intNumber(sorted[i]["id"], 0) < intNumber(sorted[j]["id"], 0)
	})
	return sorted
}

func blueprintTermCostGroups(rows []map[string]any) map[int][]map[string]any {
	groups := map[int][]map[string]any{}
	for _, row := range rows {
		if groupID := intNumber(row["groupId"], 0); groupID > 0 {
			groups[groupID] = append(groups[groupID], row)
		}
	}
	for _, rows := range groups {
		sort.SliceStable(rows, func(i, j int) bool { return intNumber(rows[i]["seq"], 0) < intNumber(rows[j]["seq"], 0) })
	}
	return groups
}

// fixtureBlueprintTermInfo describes, for /msf, the limited-time term of a
// blueprint: the current or next term (else the latest one), its tab, craft
// limit and extra term materials. It returns nothing for a blueprint without
// terms, so fixtures of regions without mysekaiBlueprintTerms are unchanged.
func (c *Controller) fixtureBlueprintTermInfo(blueprintID int, now int64) []string {
	var chosen map[string]any
	for _, term := range sortedBlueprintTerms(c.masterdata.loadList("mysekaiBlueprintTerms.json")) {
		if intNumber(term["mysekaiBlueprintId"], 0) != blueprintID {
			continue
		}
		chosen = term
		if int64(intNumber(term["endAt"], 0)) >= now {
			break
		}
	}
	if chosen == nil {
		return nil
	}
	start := int64(intNumber(chosen["startAt"], 0))
	end := int64(intNumber(chosen["endAt"], 0))
	label := blueprintTermTabTitle(stringValue(chosen["mysekaiBlueprintTermTabType"]))
	period := fmt.Sprintf("%s ~ %s", formatBlueprintTermTime(start), formatBlueprintTermTime(end))
	status := ""
	switch {
	case end < now:
		status = "(已结束)"
	case start > now:
		status = "(未开始)"
	}
	info := []string{fmt.Sprintf("【⏰%s %s%s】", label, period, status)}
	if limit := intNumber(chosen["craftLimit"], 0); limit > 0 {
		info = append(info, fmt.Sprintf("【限时期间最多制作%d次】", limit))
	}
	if costs := c.blueprintTermCostText(intNumber(chosen["mysekaiBlueprintTermMysekaiMaterialCostGroupId"], 0)); costs != "" {
		info = append(info, fmt.Sprintf("【限时额外材料：%s】", costs))
	}
	return info
}

func (c *Controller) blueprintTermCostText(groupID int) string {
	if groupID <= 0 {
		return ""
	}
	rows := blueprintTermCostGroups(c.masterdata.loadList("mysekaiBlueprintTermMysekaiMaterialCosts.json"))[groupID]
	if len(rows) == 0 {
		return ""
	}
	materials := c.masterdata.loadMapByID("mysekaiMaterials.json")
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		materialID := intNumber(row["mysekaiMaterialId"], 0)
		name := stringValue(materials[materialID]["name"])
		if name == "" {
			name = fmt.Sprintf("素材#%d", materialID)
		}
		parts = append(parts, fmt.Sprintf("%s×%d", name, intNumber(row["quantity"], 0)))
	}
	return strings.Join(parts, "、")
}

func formatBlueprintTermTime(ms int64) string {
	return displaytime.FormatTime(displaytime.TimeFromUnixMillis(ms, displaytime.DefaultTimeZone), "2006-01-02 15:04")
}

// RenderBlueprintTerm renders the limited-time blueprint view.
func (c *Controller) RenderBlueprintTerm(query BlueprintTermQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	request, err := c.BuildBlueprintTermRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiBlueprintTerm(request)
}
