package mysekai

import (
	"fmt"
	"sort"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

// BuildMusicRecordRequest builds the request for rendering MySekai music record view.
func (c *Controller) BuildMusicRecordRequest(query MusicRecordQuery) (*drawing.MysekaiMusicrecordRequest, error) {
	c = c.withRegion(query.Region)
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	showID := query.ShowID != nil && *query.ShowID
	obtainedRecords := obtainedMysekaiMusicRecords(merged)
	records := c.masterdata.loadList("mysekaiMusicRecords.json")
	musicTags := c.masterdata.loadList("musicTags.json")
	musics := c.masterdata.loadMapByID("musics.json")
	limitedTimes := c.masterdata.loadList("limitedTimeMusics.json")
	categoryMusicIDs, musicObtainedAt := collectMysekaiMusicRecordIDs(records, musics, musicTagsByID(musicTags), limitedMusicWindows(limitedTimes), obtainedRecords, time.Now().UnixMilli())
	categories, totalCount, obtainedCount := c.buildMusicRecordCategories(region, showID, categoryMusicIDs, musicObtainedAt, musics)

	profile := c.mysekaiProfileCard(region, merged, query.Profile, false)
	if profile == nil {
		return nil, fmt.Errorf("mysekai music record requires profile data")
	}
	request := &drawing.MysekaiMusicrecordRequest{
		Profile:              *profile,
		CategoryMusicrecords: categories,
	}
	if totalCount > 0 {
		request.ProgressMessage = new(fmt.Sprintf("总收集进度: %d/%d (%.1f%%)", obtainedCount, totalCount, percent(obtainedCount, totalCount)))
	}
	return request, nil
}

func obtainedMysekaiMusicRecords(merged map[string]any) map[int]int64 {
	result := make(map[int]int64)
	for _, raw := range nestedList(merged, "userMysekaiMusicRecords") {
		item, _ := raw.(map[string]any)
		result[intNumber(item["mysekaiMusicRecordId"], 0)] = int64Number(item["obtainedAt"], 0)
	}
	return result
}

func limitedMusicWindows(items []map[string]any) map[int][]map[string]any {
	result := make(map[int][]map[string]any)
	for _, item := range items {
		if musicID := intNumber(item["musicId"], 0); musicID != 0 {
			result[musicID] = append(result[musicID], item)
		}
	}
	return result
}

func musicTagsByID(items []map[string]any) map[int]string {
	result := make(map[int]string)
	for _, item := range items {
		musicID := intNumber(item["musicId"], 0)
		tag := stringValue(item["musicTag"])
		if musicID != 0 && tag != "" && tag != "all" && tag != "vocaloid" && result[musicID] == "" {
			result[musicID] = tag
		}
	}
	return result
}

func collectMysekaiMusicRecordIDs(records []map[string]any, musics map[int]map[string]any, tags map[int]string, windows map[int][]map[string]any, obtainedRecords map[int]int64, nowMillis int64) (map[string][]int, map[int]int64) {
	byCategory := make(map[string][]int)
	obtainedAt := make(map[int]int64)
	for _, record := range records {
		recordID, musicID, ok := availableMysekaiMusicRecord(record, musics, windows, nowMillis)
		if !ok {
			continue
		}
		if timestamp, obtained := obtainedRecords[recordID]; obtained {
			obtainedAt[musicID] = timestamp
		}
		tag := tags[musicID]
		if tag == "" {
			tag = "vocaloid"
		}
		byCategory[tag] = append(byCategory[tag], musicID)
	}
	return byCategory, obtainedAt
}

func availableMysekaiMusicRecord(record map[string]any, musics map[int]map[string]any, windows map[int][]map[string]any, nowMillis int64) (int, int, bool) {
	if stringValue(record["mysekaiMusicTrackType"]) != "music" {
		return 0, 0, false
	}
	recordID, musicID := intNumber(record["id"], 0), intNumber(record["externalId"], 0)
	if recordID == 0 || musicID == 0 || musicID == 241 || musicID == 290 {
		return 0, 0, false
	}
	music := musics[musicID]
	if len(music) == 0 || int64Number(music["publishedAt"], 0) > nowMillis {
		return 0, 0, false
	}
	if limited := windows[musicID]; len(limited) > 0 && !isMusicAvailableNow(limited, nowMillis) {
		return 0, 0, false
	}
	return recordID, musicID, true
}

func (c *Controller) buildMusicRecordCategories(region renderregion.Value, showID bool, byCategory map[string][]int, obtainedAt map[int]int64, musics map[int]map[string]any) ([]drawing.MysekaiCategoryMusicrecord, int, int) {
	order := []string{"light_music_club", "street", "idol", "theme_park", "school_refusal", "vocaloid", "other"}
	icons := c.musicRecordTagIcons()
	categories := make([]drawing.MysekaiCategoryMusicrecord, 0, len(order))
	totalCount, obtainedCount := 0, 0
	for _, tag := range order {
		category, categoryTotal, categoryObtained := c.buildMusicRecordCategory(region, showID, tag, icons[tag], byCategory[tag], obtainedAt, musics)
		totalCount += categoryTotal
		obtainedCount += categoryObtained
		if category != nil {
			categories = append(categories, *category)
		}
	}
	return categories, totalCount, obtainedCount
}

func (c *Controller) buildMusicRecordCategory(region renderregion.Value, showID bool, tag, icon string, musicIDs []int, obtainedAt map[int]int64, musics map[int]map[string]any) (*drawing.MysekaiCategoryMusicrecord, int, int) {
	if len(musicIDs) == 0 {
		return nil, 0, 0
	}
	sortMusicRecordIDs(musicIDs, obtainedAt)
	obtainedCount := 0
	records := make([]drawing.MysekaiMusicrecord, 0, len(musicIDs))
	for _, musicID := range musicIDs {
		obtained := obtainedAt[musicID] != 0
		if obtained {
			obtainedCount++
		}
		if record := c.musicRecordDrawingEntry(region, showID, musicID, obtained, musics[musicID]); record != nil {
			records = append(records, *record)
		}
	}
	total := len(musicIDs)
	return &drawing.MysekaiCategoryMusicrecord{
		Tag:             tag,
		TagIconPath:     icon,
		ProgressMessage: new(fmt.Sprintf("%d/%d (%.1f%%)", obtainedCount, total, percent(obtainedCount, total))),
		Musicrecords:    records,
	}, total, obtainedCount
}

func sortMusicRecordIDs(musicIDs []int, obtainedAt map[int]int64) {
	sort.Slice(musicIDs, func(i, j int) bool {
		left, leftObtained := obtainedAt[musicIDs[i]]
		right, rightObtained := obtainedAt[musicIDs[j]]
		if leftObtained && rightObtained {
			return left < right
		}
		if leftObtained != rightObtained {
			return leftObtained
		}
		return musicIDs[i] < musicIDs[j]
	})
}

func (c *Controller) musicRecordDrawingEntry(region renderregion.Value, showID bool, musicID int, obtained bool, music map[string]any) *drawing.MysekaiMusicrecord {
	assetbundleName := stringValue(music["assetbundleName"])
	if assetbundleName == "" {
		return nil
	}
	record := &drawing.MysekaiMusicrecord{
		ImagePath: c.regionPath(region, fmt.Sprintf("music/jacket/%s/%s.png", assetbundleName, assetbundleName)),
		Obtained:  obtained,
	}
	if showID {
		record.ID = new(musicID)
	}
	return record
}

func (c *Controller) musicRecordTagIcons() map[string]string {
	return map[string]string{
		"light_music_club": c.staticPath("icon_light_sound.png"),
		"idol":             c.staticPath("icon_idol.png"),
		"street":           c.staticPath("icon_street.png"),
		"theme_park":       c.staticPath("icon_theme_park.png"),
		"school_refusal":   c.staticPath("icon_school_refusal.png"),
		"vocaloid":         c.staticPath("icon_piapro.png"),
		"other":            "",
	}
}

// RenderMusicRecord renders the MySekai music record view.
func (c *Controller) RenderMusicRecord(query MusicRecordQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildMusicRecordRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiMusicRecord(payload)
}
