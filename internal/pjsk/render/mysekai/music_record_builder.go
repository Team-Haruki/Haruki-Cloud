package mysekai

import (
	"fmt"
	"sort"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
)

// BuildMusicRecordRequest builds the request for rendering MySekai music record view.
func (c *Controller) BuildMusicRecordRequest(query MusicRecordQuery) (*drawing.MysekaiMusicrecordRequest, error) {
	c = c.withRegion(query.Region)
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
		item, _ := raw.(map[string]any)
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

	limitedByMusic := map[int][]map[string]any{}
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
		"light_music_club": c.staticPath("icon_light_sound.png"),
		"idol":             c.staticPath("icon_idol.png"),
		"street":           c.staticPath("icon_street.png"),
		"theme_park":       c.staticPath("icon_theme_park.png"),
		"school_refusal":   c.staticPath("icon_school_refusal.png"),
		"vocaloid":         c.staticPath("icon_piapro.png"),
		"other":            "",
	}
	order := []string{"other", "light_music_club", "street", "idol", "theme_park", "school_refusal", "vocaloid"}

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
				ImagePath: c.regionPath(region, fmt.Sprintf("music/jacket/%s/%s.png", assetbundleName, assetbundleName)),
				Obtained:  musicObtainedAt[musicID] != 0,
			}
			if showID {
				record.ID = new(musicID)
			}
			records = append(records, record)
		}
		if categoryTotal == 0 {
			continue
		}
		categories = append(categories, drawing.MysekaiCategoryMusicrecord{
			Tag:             tag,
			TagIconPath:     tagIcons[tag],
			ProgressMessage: new(fmt.Sprintf("%d/%d (%.1f%%)", categoryObtained, categoryTotal, percent(categoryObtained, categoryTotal))),
			Musicrecords:    records,
		})
	}

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

// RenderMusicRecord renders the MySekai music record view.
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
