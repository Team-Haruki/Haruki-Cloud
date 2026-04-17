package mysekai

import (
	"fmt"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func extractMysekaiGateInfo(merged map[string]any) (int, int, int) {
	visit, ok := merged["userMysekaiGateCharacterVisit"].(map[string]any)
	if !ok {
		return 1, 1, 0
	}
	gate, ok := visit["userMysekaiGate"].(map[string]any)
	if !ok {
		return 1, 1, 0
	}
	gateID := intNumber(gate["mysekaiGateId"], 1)
	gateLevel := intNumber(gate["mysekaiGateLevel"], 1)
	gateSkinID := intNumber(gate["mysekaiGateSkinId"], 0)
	if gateID <= 0 {
		gateID = 1
	}
	if gateLevel <= 0 {
		gateLevel = 1
	}
	return gateID, gateLevel, gateSkinID
}

var (
	mysekaiBirthdayRefreshRegions = map[string]struct{}{
		string(renderregion.JP): {},
	}
	mysekaiCharacterBirthdays = map[int]struct {
		Month int
		Day   int
	}{
		1:  {Month: 8, Day: 11},
		2:  {Month: 5, Day: 9},
		3:  {Month: 10, Day: 27},
		4:  {Month: 1, Day: 8},
		5:  {Month: 4, Day: 14},
		6:  {Month: 10, Day: 5},
		7:  {Month: 3, Day: 19},
		8:  {Month: 12, Day: 6},
		9:  {Month: 3, Day: 2},
		10: {Month: 7, Day: 26},
		11: {Month: 11, Day: 12},
		12: {Month: 5, Day: 25},
		13: {Month: 5, Day: 17},
		14: {Month: 9, Day: 9},
		15: {Month: 7, Day: 20},
		16: {Month: 6, Day: 24},
		17: {Month: 2, Day: 10},
		18: {Month: 1, Day: 27},
		19: {Month: 4, Day: 30},
		20: {Month: 8, Day: 27},
		21: {Month: 8, Day: 31},
		22: {Month: 12, Day: 27},
		23: {Month: 12, Day: 27},
		24: {Month: 1, Day: 30},
		25: {Month: 11, Day: 5},
		26: {Month: 2, Day: 17},
	}
	mysekaiRegionUTCOffsets = map[string]int{
		string(renderregion.JP): 9,
		string(renderregion.EN): -8,
		string(renderregion.CN): 8,
		string(renderregion.TW): 8,
		string(renderregion.KR): 9,
	}
)

func extractMysekaiPhenoms(region renderregion.Value, resolve pathResolver, phenomIcons map[int]string, merged map[string]any) []drawing.MysekaiPhenomRequest {
	rawSchedules, ok := merged["mysekaiPhenomenaSchedules"].([]any)
	if !ok {
		return []drawing.MysekaiPhenomRequest{}
	}

	nowMs := resolveMysekaiSnapshotTimeMs(merged)
	now := time.Now()
	if nowMs > 0 {
		now = time.UnixMilli(nowMs)
	}
	// Use region timezone for deterministic display regardless of local TZ.
	regionOffset := mysekaiRegionUTCOffset(region.String(), now)
	regionLoc := time.FixedZone(region.String(), regionOffset*3600)
	now = now.In(regionLoc)
	firstHour, secondHour := mysekaiRefreshHoursLocal(region, now)

	phenomStart := time.Date(now.Year(), now.Month(), now.Day(), firstHour, 0, 0, 0, now.Location())
	if now.Hour() < firstHour {
		phenomStart = phenomStart.Add(-24 * time.Hour)
	}
	currentIdx := 1
	if now.Hour() >= firstHour && now.Hour() < secondHour {
		currentIdx = 0
	}

	phenoms := make([]drawing.MysekaiPhenomRequest, 0, len(rawSchedules)+1)
	for i, item := range rawSchedules {
		schedule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		phenomID := intNumber(schedule["mysekaiPhenomenaId"], 1)
		currentStart := phenomStart.Add(time.Duration(i) * 12 * time.Hour)
		current := i == currentIdx
		phenomEnd := currentStart.Add(11*time.Hour + 59*time.Minute)
		lastRefreshOfEnd, reason := mysekaiLastRefreshTimeAndReason(region, phenomEnd)
		var midCurrent *bool
		if !lastRefreshOfEnd.Equal(currentStart) {
			value := !now.Before(lastRefreshOfEnd)
			midCurrent = &value
			current = current && !value
		}
		phenoms = append(phenoms, mysekaiNaturalPhenom(resolve, phenomIcons, phenomID, currentStart, current))
		if midCurrent != nil {
			phenoms = append(phenoms, mysekaiBirthdayPhenom(resolve, reason, lastRefreshOfEnd, *midCurrent))
		}
	}
	return phenoms
}

func resolveMysekaiSnapshotTimeMs(merged map[string]any) int64 {
	if merged == nil {
		return 0
	}

	best := normalizeMySekaiTimestampMs(int64Number(merged["now"], 0))
	if updated, ok := merged["updatedResources"].(map[string]any); ok {
		if current := normalizeMySekaiTimestampMs(int64Number(updated["now"], 0)); current > best {
			best = current
		}
	}
	if upload := normalizeMySekaiTimestampMs(int64Number(merged["upload_time"], 0)); upload > best {
		best = upload
	}
	return best
}

func mysekaiNaturalPhenom(resolve pathResolver, phenomIcons map[int]string, phenomID int, start time.Time, isCurrent bool) drawing.MysekaiPhenomRequest {
	bg := []int{255, 255, 255, 75}
	fg := []int{125, 125, 125, 255}
	if isCurrent {
		bg = []int{255, 255, 255, 150}
		fg = []int{0, 0, 0, 255}
	}
	iconName := strings.TrimSpace(phenomIcons[phenomID])
	if iconName == "" {
		iconName = "env_default"
	}
	return drawing.MysekaiPhenomRequest{
		RefreshReason:  "natural",
		ImagePath:      resolve(fmt.Sprintf("mysekai/thumbnail/phenomena/%s.png", iconName)),
		BackgroundFill: bg,
		StartAt:        start.UnixMilli(),
		TextFill:       fg,
	}
}

func mysekaiBirthdayPhenom(resolve pathResolver, reason string, start time.Time, isCurrent bool) drawing.MysekaiPhenomRequest {
	bg := []int{255, 255, 200, 150}
	if isCurrent {
		bg = []int{255, 255, 200, 255}
	}
	fg := []int{125, 125, 125, 255}
	if isCurrent {
		fg = []int{0, 0, 0, 255}
	}
	characterID := 0
	if lastUnderscore := strings.LastIndex(reason, "_"); lastUnderscore >= 0 {
		characterID = intNumber(reason[lastUnderscore+1:], 0)
	}
	return drawing.MysekaiPhenomRequest{
		RefreshReason:  reason,
		ImagePath:      resolve(fmt.Sprintf("thumbnail/material/material%d.png", characterID+173)),
		BackgroundFill: bg,
		StartAt:        start.UnixMilli(),
		TextFill:       fg,
	}
}

func mysekaiRefreshHoursLocal(region renderregion.Value, now time.Time) (int, int) {
	return regionHourToLocal(region, 5, now), regionHourToLocal(region, 17, now)
}

func regionHourToLocal(region renderregion.Value, hour int, now time.Time) int {
	normalized := region.String()
	localOffset := mysekaiLocalUTCOffset(now)
	regionOffset := mysekaiRegionUTCOffset(normalized, now)
	return (hour + localOffset - regionOffset + 24) % 24
}

func mysekaiLocalUTCOffset(now time.Time) int {
	_, seconds := now.Zone()
	return seconds / 3600
}

func mysekaiRegionUTCOffset(region string, now time.Time) int {
	region = renderregion.WithDefault(renderregion.Normalize(region)).String()
	offset, ok := mysekaiRegionUTCOffsets[region]
	if !ok {
		return 8
	}
	if region == string(renderregion.EN) {
		nowUTC := now.UTC()
		dstStart := time.Date(nowUTC.Year(), 3, 8, 0, 0, 0, 0, time.UTC)
		dstEnd := time.Date(nowUTC.Year(), 11, 1, 0, 0, 0, 0, time.UTC)
		if !nowUTC.Before(dstStart) && nowUTC.Before(dstEnd) {
			offset++
		}
	}
	return offset
}

func mysekaiLastRefreshTimeAndReason(region renderregion.Value, now time.Time) (time.Time, string) {
	firstHour, secondHour := mysekaiRefreshHoursLocal(region, now)
	if firstHour > secondHour {
		firstHour, secondHour = secondHour, firstHour
	}
	var lastRefresh time.Time
	switch {
	case now.Hour() < firstHour:
		lastRefresh = time.Date(now.Year(), now.Month(), now.Day(), secondHour, 0, 0, 0, now.Location()).Add(-24 * time.Hour)
	case now.Hour() < secondHour:
		lastRefresh = time.Date(now.Year(), now.Month(), now.Day(), firstHour, 0, 0, 0, now.Location())
	default:
		lastRefresh = time.Date(now.Year(), now.Month(), now.Day(), secondHour, 0, 0, 0, now.Location())
	}

	if _, ok := mysekaiBirthdayRefreshRegions[region.String()]; !ok {
		return lastRefresh, "natural"
	}

	for characterID, birthday := range mysekaiCharacterBirthdays {
		nextBirthday := mysekaiNextBirthdayLocal(region, now.Add(-24*time.Hour), birthday.Month, birthday.Day)
		start := nextBirthday.Add(-72 * time.Hour)
		end := nextBirthday
		if lastRefresh.Before(start) && !start.After(now) {
			return start, fmt.Sprintf("bdstart_%d", characterID)
		}
		if lastRefresh.Before(end) && !end.After(now) {
			return end, fmt.Sprintf("bdend_%d", characterID)
		}
	}
	return lastRefresh, "natural"
}

func mysekaiNextBirthdayLocal(region renderregion.Value, now time.Time, month, day int) time.Time {
	regionLocation := time.FixedZone(region.String(), mysekaiRegionUTCOffset(region.String(), now)*3600)
	nextBirthday := time.Date(now.Year(), time.Month(month), day, 0, 0, 0, 0, regionLocation).In(now.Location())
	if nextBirthday.Before(now) {
		nextBirthday = time.Date(now.Year()+1, time.Month(month), day, 0, 0, 0, 0, regionLocation).In(now.Location())
	}
	return nextBirthday
}
