package vlive

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/usererror"
)

const (
	liveTypeSolo  = "solo_virtual_live"
	groupTypeSolo = "solo_virtual_live"

	totalCheerPointRewardPurpose  = "virtual_live_total_cheer_point_reward"
	totalCheerPointSurplusPurpose = "virtual_live_total_cheer_point_surplus_reward"
)

var (
	// ErrNoSoloLives reports a solo live query on a region whose data has
	// no solo virtual lives.
	ErrNoSoloLives = errors.New("region has no solo virtual lives")
	// ErrSoloLiveNotFound reports a solo live query that matched no solo
	// live or solo group of the region.
	ErrSoloLiveNotFound = errors.New("solo virtual live not found")
)

//copylint:ignore-block 解析关键字
var soloQueryKeywords = []string{"solo", "ソロ", "个人", "個人", "单人", "單人"}

// IsDetailQuery reports whether a /vlive argument asks for a solo live
// detail: a numeric id or a solo keyword. Any other argument keeps the list.
func IsDetailQuery(query string) bool {
	_, _, ok := parseDetailQuery(query)
	return ok
}

func parseDetailQuery(query string) (id int, keyword bool, ok bool) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 0, false, false
	}
	if n, err := strconv.Atoi(query); err == nil && n > 0 {
		return n, false, true
	}
	for _, word := range soloQueryKeywords {
		if strings.Contains(query, word) {
			return 0, true, true
		}
	}
	return 0, false, false
}

// isSoloLive reports whether a live belongs to a solo virtual live group:
// its group says so, or (groups unavailable) the live's own type does.
func isSoloLive(live ResolvedLive, groups map[int]*Group) bool {
	if live.GroupID <= 0 {
		return false
	}
	if group := groups[live.GroupID]; group != nil && group.Type != "" {
		return group.Type == groupTypeSolo
	}
	return live.VirtualLiveType == liveTypeSolo
}

func (c *Controller) groupsFor(source DataSource, region renderregion.Value, lives []ResolvedLive) map[int]*Group {
	hasGrouped := false
	for _, live := range lives {
		if live.GroupID > 0 && live.VirtualLiveType == liveTypeSolo {
			hasGrouped = true
			break
		}
	}
	if !hasGrouped || source == nil {
		return nil
	}
	groupSource, ok := source.(groupDataSource)
	if !ok {
		return nil
	}
	groups, _ := groupSource.GetGroups(region)
	return groups
}

// collapseSoloGroups replaces the lives of each solo virtual live group by
// one entry named after the group. Other lives are returned unchanged and
// in order; without solo lives the input is returned as is.
func collapseSoloGroups(lives []ResolvedLive, groups map[int]*Group) []ResolvedLive {
	members := map[int][]ResolvedLive{}
	for _, live := range lives {
		if isSoloLive(live, groups) {
			members[live.GroupID] = append(members[live.GroupID], live)
		}
	}
	if len(members) == 0 {
		return lives
	}
	out := make([]ResolvedLive, 0, len(lives))
	emitted := map[int]bool{}
	for _, live := range lives {
		group, ok := members[live.GroupID]
		if !ok || !isSoloLive(live, groups) {
			out = append(out, live)
			continue
		}
		if emitted[live.GroupID] {
			continue
		}
		emitted[live.GroupID] = true
		out = append(out, mergeSoloGroup(live.GroupID, groups[live.GroupID], group))
	}
	return out
}

func mergeSoloGroup(groupID int, group *Group, members []ResolvedLive) ResolvedLive {
	merged := ResolvedLive{
		ID:              groupID,
		Name:            members[0].Name,
		StartAt:         members[0].StartAt,
		EndAt:           members[0].EndAt,
		VirtualLiveType: liveTypeSolo,
		GroupID:         groupID,
		GroupName:       members[0].Name,
		Members:         members,
		Rewards:         append([]Reward(nil), members[0].Rewards...),
	}
	if group != nil && strings.TrimSpace(group.Name) != "" {
		merged.Name = strings.TrimSpace(group.Name)
		merged.GroupName = merged.Name
		merged.GroupBannerAsset = strings.TrimSpace(group.AssetBundleName)
	}
	for _, member := range members {
		if member.StartAt.Before(merged.StartAt) {
			merged.StartAt = member.StartAt
		}
		if member.EndAt.After(merged.EndAt) {
			merged.EndAt = member.EndAt
		}
		merged.RestCount += member.RestCount
		merged.Characters = append(merged.Characters, member.Characters...)
		if member.Current == nil {
			continue
		}
		if merged.Current == nil || soonerWindow(member, merged) {
			merged.Current = new(*member.Current)
			merged.Living = member.Living
		}
	}
	return merged
}

// soonerWindow prefers a live window over an upcoming one, then the earlier
// start.
func soonerWindow(candidate, current ResolvedLive) bool {
	if candidate.Living != current.Living {
		return candidate.Living
	}
	return candidate.Current.StartAt.Before(current.Current.StartAt)
}

// BuildDetailRequest builds the solo virtual live detail: the whole group's
// per-character lives, plus the cheer-point rewards of the queried live when
// the query names one live.
func (c *Controller) BuildDetailRequest(query DetailQuery) (*drawing.VLiveDetailRequest, error) {
	if c == nil || c.sources == nil {
		return nil, usererror.Misconfigured(errors.New("vlive controller is not configured"))
	}
	region := c.resolveRegion(query.Region)
	source, ok := c.sources.SourceForRegion(region)
	if !ok {
		return nil, fmt.Errorf("no vlive data source for region %s", region)
	}
	id, _, ok := parseDetailQuery(query.Query)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSoloLiveNotFound, strings.TrimSpace(query.Query))
	}
	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}
	lives, err := source.GetLives(region)
	if err != nil {
		return nil, err
	}
	all := make([]ResolvedLive, 0, len(lives))
	byID := make(map[int]*Live, len(lives))
	for _, live := range lives {
		if live == nil {
			continue
		}
		byID[live.ID] = live
		all = append(all, resolveLiveForDetail(live, now))
	}
	groups := c.groupsFor(source, region, all)
	soloGroups := map[int][]ResolvedLive{}
	for _, live := range all {
		if isSoloLive(live, groups) {
			soloGroups[live.GroupID] = append(soloGroups[live.GroupID], live)
		}
	}
	if len(soloGroups) == 0 {
		return nil, ErrNoSoloLives
	}
	groupID, selected := selectSoloGroup(soloGroups, id, now)
	if groupID == 0 {
		return nil, fmt.Errorf("%w: %d", ErrSoloLiveNotFound, id)
	}
	members := soloGroups[groupID]
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	merged := mergeSoloGroup(groupID, groups[groupID], members)

	req := &drawing.VLiveDetailRequest{
		Region:          region.String(),
		ID:              merged.ID,
		Title:           fallbackLiveName(merged.Name, merged.ID),
		VirtualLiveType: liveTypeSolo,
		BannerPath:      c.bannerPath(source, region, merged),
		StartAt:         merged.StartAt.UnixMilli(),
		EndAt:           merged.EndAt.UnixMilli(),
		Lives:           c.buildDetailLives(source, members, byID, merged.GroupName),
	}
	cost := members[0]
	if selected != nil {
		req.ID = selected.ID
		req.Title = fallbackLiveName(selected.Name, selected.ID)
		cost = *selected
		c.applyCheerPointRewards(req, source, byID[selected.ID])
	}
	req.OverrideCost = c.buildOverrideCost(source, byID[cost.ID])
	return req, nil
}

// resolveLiveForDetail resolves a live's schedule state without the list's
// visibility window, so a finished or far-future group can still be shown.
func resolveLiveForDetail(live *Live, now time.Time) ResolvedLive {
	resolved := ResolvedLive{
		ID:              live.ID,
		Name:            strings.TrimSpace(live.Name),
		AssetBundleName: strings.TrimSpace(live.AssetBundleName),
		StartAt:         unixTime(live.StartAt),
		EndAt:           unixTime(live.EndAt),
		Rewards:         append([]Reward(nil), live.Rewards...),
		Characters:      append([]Character(nil), live.Characters...),
		VirtualLiveType: strings.TrimSpace(live.VirtualLiveType),
		GroupID:         live.GroupID,
	}
	applyLiveSchedules(&resolved, normalizeSchedules(live.Schedules), now)
	if !resolved.EndAt.IsZero() {
		applyLiveWindowFallback(&resolved, now)
	}
	return resolved
}

// selectSoloGroup resolves the query id to a solo group: a solo live id
// selects that live's group and the live itself, a solo group id selects
// the group. id 0 (keyword query) picks the current or next group, else the
// latest one.
func selectSoloGroup(groups map[int][]ResolvedLive, id int, now time.Time) (int, *ResolvedLive) {
	if id > 0 {
		for groupID, members := range groups {
			for i := range members {
				if members[i].ID == id {
					live := members[i]
					return groupID, &live
				}
			}
		}
		if _, ok := groups[id]; ok {
			return id, nil
		}
		return 0, nil
	}
	bestID, bestStart, bestActive := 0, time.Time{}, false
	for groupID, members := range groups {
		start, end := members[0].StartAt, members[0].EndAt
		for _, member := range members[1:] {
			if member.StartAt.Before(start) {
				start = member.StartAt
			}
			if member.EndAt.After(end) {
				end = member.EndAt
			}
		}
		active := now.Before(end)
		switch {
		case bestID == 0,
			active && !bestActive,
			active == bestActive && active && start.Before(bestStart),
			active == bestActive && !active && start.After(bestStart):
			bestID, bestStart, bestActive = groupID, start, active
		}
	}
	return bestID, nil
}

// soloLiveShortName is the part of a member live's name after the group's
// name (usually the character), or the whole name when nothing is left.
func soloLiveShortName(name, groupName string) string {
	name = strings.TrimSpace(name)
	groupName = strings.TrimSpace(groupName)
	if groupName != "" {
		if rest, ok := strings.CutPrefix(name, groupName); ok && strings.TrimSpace(rest) != "" {
			return strings.TrimSpace(rest)
		}
	}
	return name
}

func (c *Controller) buildDetailLives(source DataSource, members []ResolvedLive, byID map[int]*Live, groupName string) []drawing.VLiveDetailLive {
	out := make([]drawing.VLiveDetailLive, 0, len(members))
	for _, member := range members {
		name := fallbackLiveName(member.Name, member.ID)
		item := drawing.VLiveDetailLive{
			ID:        member.ID,
			Name:      name,
			ShortName: soloLiveShortName(name, groupName),
			Living:    member.Living,
			RestCount: member.RestCount,
		}
		if icons := c.buildCharacterItems(source, member); len(icons) > 0 {
			item.CharacterIconPath = icons[0].IconPath
		}
		if member.Current != nil {
			item.CurrentStartAt = member.Current.StartAt.UnixMilli()
			item.CurrentEndAt = member.Current.EndAt.UnixMilli()
		}
		if live := byID[member.ID]; live != nil {
			item.ScheduleCount = new(len(normalizeSchedules(live.Schedules)))
		}
		out = append(out, item)
	}
	return out
}

func (c *Controller) applyCheerPointRewards(req *drawing.VLiveDetailRequest, source DataSource, live *Live) {
	if live == nil {
		return
	}
	for _, reward := range live.TotalCheerPointRewards {
		box := source.GetResourceBoxByPurpose(totalCheerPointRewardPurpose, reward.ResourceBoxID)
		req.TotalCheerPointRewards = append(req.TotalCheerPointRewards, drawing.VLiveCheerPointRewardRow{
			Threshold: reward.Threshold,
			Rewards:   nonNilRewards(c.buildRewardBoxItems(source, box)),
		})
	}
	if surplus := live.SurplusReward; surplus != nil {
		box := source.GetResourceBoxByPurpose(totalCheerPointSurplusPurpose, surplus.ResourceBoxID)
		req.SurplusReward = &drawing.VLiveSurplusReward{
			BasePoint: surplus.BasePoint,
			Rewards:   nonNilRewards(c.buildRewardBoxItems(source, box)),
		}
	}
}

func (c *Controller) buildOverrideCost(source DataSource, live *Live) *drawing.VLiveOverrideCost {
	if live == nil || live.OverrideCost == nil {
		return nil
	}
	cost := live.OverrideCost
	imagePath := c.rewardImagePath(cost.ResourceType, cost.ResourceID)
	if strings.TrimSpace(imagePath) == "" {
		return nil
	}
	out := &drawing.VLiveOverrideCost{
		ImagePath:    imagePath,
		ResourceType: cost.ResourceType,
	}
	if cost.ResourceID > 0 {
		out.ResourceID = new(cost.ResourceID)
	}
	if names, ok := source.(materialNameSource); ok && cost.ResourceType == "material" {
		out.Name = names.GetMaterialName(cost.ResourceID)
	}
	return out
}

// RenderDetail renders the solo virtual live detail view.
func (c *Controller) RenderDetail(query DetailQuery) ([]byte, error) {
	image, err := c.RenderDetailImage(query)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderDetailImage(query DetailQuery) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	req, err := c.BuildDetailRequest(query)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateVLiveDetailImage(req)
}

// nonNilRewards keeps a required reward list a JSON array.
func nonNilRewards(items []drawing.VLiveRewardItem) []drawing.VLiveRewardItem {
	if items == nil {
		return []drawing.VLiveRewardItem{}
	}
	return items
}
