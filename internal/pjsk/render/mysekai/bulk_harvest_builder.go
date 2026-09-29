package mysekai

import (
	"fmt"
	"sort"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
)

// BulkHarvestQuery requests the MySekai bulk-harvest target view (JP 7.0.0).
type BulkHarvestQuery struct {
	Region string `json:"region,omitempty"`
}

// BuildBulkHarvestRequest lists, per harvest site, the bulk-harvest target
// groups (with their required tool) and targets from the region's
// mysekaiSiteBulkHarvests / Targets / TargetGroups rows. Regions without
// those rows report the feature as unavailable.
func (c *Controller) BuildBulkHarvestRequest(query BulkHarvestQuery) (*drawing.MysekaiBulkHarvestRequest, error) {
	c = c.withRegion(query.Region)
	if err := c.ensureMasterdata(); err != nil {
		return nil, err
	}
	region := c.resolveRegion(query.Region)
	siteTargets := c.masterdata.loadList("mysekaiSiteBulkHarvests.json")
	targets := c.masterdata.loadMapByID("mysekaiSiteBulkHarvestTargets.json")
	if len(siteTargets) == 0 || len(targets) == 0 {
		return nil, fmt.Errorf("mysekai bulk harvest is not available in region %s", region)
	}
	groups := c.masterdata.loadMapByID("mysekaiSiteBulkHarvestTargetGroups.json")
	fixtureCounts := bulkHarvestFixtureCounts(c.masterdata.loadList("mysekaiSiteHarvestFixtures.json"))
	resolver := c.newMysekaiResourceResolver(region)

	targetsBySite := map[int][]int{}
	for _, row := range siteTargets {
		siteID, targetID := intNumber(row["mysekaiSiteId"], 0), intNumber(row["mysekaiSiteBulkHarvestTargetId"], 0)
		if siteID > 0 && targets[targetID] != nil {
			targetsBySite[siteID] = append(targetsBySite[siteID], targetID)
		}
	}
	request := &drawing.MysekaiBulkHarvestRequest{Sites: make([]drawing.MysekaiBulkHarvestSite, 0, len(targetsBySite))}
	for _, siteID := range bulkHarvestSiteOrder(targetsBySite) {
		request.Sites = append(request.Sites, drawing.MysekaiBulkHarvestSite{
			SiteID:    siteID,
			ImagePath: drawing.AssetPath(c.regionPath(region, fmt.Sprintf("mysekai/site/sitemap/texture/img_harvest_site_%d.png", siteID))),
			Groups:    buildBulkHarvestGroups(targetsBySite[siteID], targets, groups, fixtureCounts, resolver),
		})
	}
	return request, nil
}

// bulkHarvestSiteOrder keeps the site order the resource views use and
// appends any other site the data names.
func bulkHarvestSiteOrder(targetsBySite map[int][]int) []int {
	order := make([]int, 0, len(targetsBySite))
	seen := map[int]bool{}
	for _, siteID := range mysekaiMapSiteOrder {
		if _, ok := targetsBySite[siteID]; ok {
			order = append(order, siteID)
			seen[siteID] = true
		}
	}
	extra := make([]int, 0)
	for siteID := range targetsBySite {
		if !seen[siteID] {
			extra = append(extra, siteID)
		}
	}
	sort.Ints(extra)
	return append(order, extra...)
}

// bulkHarvestFixtureCounts counts the harvest fixture kinds a bulk-harvest
// target covers (mysekaiSiteHarvestFixtures.mysekaiSiteBulkHarvestTargetId).
func bulkHarvestFixtureCounts(fixtures []map[string]any) map[int]int {
	counts := map[int]int{}
	for _, fixture := range fixtures {
		if targetID := intNumber(fixture["mysekaiSiteBulkHarvestTargetId"], 0); targetID > 0 {
			counts[targetID]++
		}
	}
	return counts
}

func buildBulkHarvestGroups(targetIDs []int, targets, groups map[int]map[string]any, fixtureCounts map[int]int, resolver mysekaiResourceResolver) []drawing.MysekaiBulkHarvestGroup {
	byGroup := map[int][]map[string]any{}
	for _, targetID := range targetIDs {
		target := targets[targetID]
		groupID := intNumber(target["mysekaiSiteBulkHarvestTargetGroupId"], 0)
		byGroup[groupID] = append(byGroup[groupID], target)
	}
	groupIDs := make([]int, 0, len(byGroup))
	for groupID := range byGroup {
		groupIDs = append(groupIDs, groupID)
	}
	sort.SliceStable(groupIDs, func(i, j int) bool {
		si, sj := intNumber(groups[groupIDs[i]]["seq"], groupIDs[i]), intNumber(groups[groupIDs[j]]["seq"], groupIDs[j])
		return si < sj || si == sj && groupIDs[i] < groupIDs[j]
	})
	result := make([]drawing.MysekaiBulkHarvestGroup, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		group := groups[groupID]
		out := drawing.MysekaiBulkHarvestGroup{ID: groupID, Name: stringValue(group["name"])}
		if toolID := intNumber(group["requiredToolId"], 0); toolID > 0 {
			name, image := resolver.tool(toolID)
			if name == "" {
				name = fmt.Sprintf("工具 #%d", toolID)
			}
			out.RequiredToolName = &name
			out.RequiredToolImagePath = drawing.AssetPath(image)
		}
		members := byGroup[groupID]
		sort.SliceStable(members, func(i, j int) bool {
			return intNumber(members[i]["seq"], 0) < intNumber(members[j]["seq"], 0)
		})
		for _, target := range members {
			entry := drawing.MysekaiBulkHarvestTarget{ID: intNumber(target["id"], 0), Name: stringValue(target["name"])}
			if count := fixtureCounts[entry.ID]; count > 0 {
				entry.FixtureCount = &count
			}
			out.Targets = append(out.Targets, entry)
		}
		result = append(result, out)
	}
	return result
}

// RenderBulkHarvest renders the MySekai bulk-harvest target view.
func (c *Controller) RenderBulkHarvest(query BulkHarvestQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	request, err := c.BuildBulkHarvestRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiBulkHarvest(request)
}
