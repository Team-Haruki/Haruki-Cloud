package music

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"haruki-cloud/utils/drawing"
)

func weightedMusicBoardSkill(skillScores []float64, sortedSkills []float64, leaderSkill float64) float64 {
	if len(skillScores) == 0 {
		return 0
	}

	core := append([]float64(nil), skillScores...)
	extra := 0.0
	if len(core) > 5 {
		extra = core[5]
		core = core[:5]
	}
	sort.Slice(core, func(i, j int) bool { return core[i] > core[j] })

	total := 0.0
	limit := len(core)
	if len(sortedSkills) < limit {
		limit = len(sortedSkills)
	}
	for i := 0; i < limit; i++ {
		total += core[i] * sortedSkills[i]
	}
	if len(skillScores) > 5 {
		total += extra * leaderSkill
	}
	return total
}

func musicBoardSkillAccount(skillScore, totalScore float64) float64 {
	if totalScore <= 0 {
		return 0
	}
	return skillScore / totalScore
}

func populateMusicBoardLiveMetrics(row *musicBoardRow, liveType string, score, skillAccount float64, power int, deckBonus, playInterval float64) {
	if row == nil {
		return
	}

	activeBonus := 0.0
	if liveType == "multi" {
		activeBonus = 5 * 0.015 * float64(power)
	}
	realScore := math.Floor(score*float64(power)*4 + activeBonus)

	eventRate := row.EventRate / 100.0
	deckRate := deckBonus/100.0 + 1

	pt := 0.0
	switch liveType {
	case "solo", "auto":
		base := 100 + int(realScore/20000)
		pt = math.Floor(float64(base) * eventRate * deckRate)
	case "multi":
		otherScore := realScore * 4
		base := 110 + int(realScore/17000) + minInt(13, int(otherScore/340000))
		pt = math.Floor(float64(base) * eventRate * deckRate)
	}

	playCountPerHour := 0.0
	if totalTime := row.MusicTime + playInterval; totalTime > 0 {
		playCountPerHour = 3600 / totalTime
	}
	ptPerHour := pt * playCountPerHour

	switch liveType {
	case "solo":
		row.SoloScore = float64Ptr(score)
		row.SoloRealScore = float64Ptr(realScore)
		row.SoloPt = float64Ptr(pt)
		row.SoloSkillAccount = float64Ptr(skillAccount)
		row.SoloPtPerHour = float64Ptr(ptPerHour)
		row.PlayCountPerHour = float64Ptr(playCountPerHour)
	case "auto":
		row.AutoScore = float64Ptr(score)
		row.AutoRealScore = float64Ptr(realScore)
		row.AutoPt = float64Ptr(pt)
		row.AutoSkillAccount = float64Ptr(skillAccount)
		row.AutoPtPerHour = float64Ptr(ptPerHour)
	case "multi":
		row.MultiScore = float64Ptr(score)
		row.MultiRealScore = float64Ptr(realScore)
		row.MultiPt = float64Ptr(pt)
		row.MultiSkillAccount = float64Ptr(skillAccount)
		row.MultiPtPerHour = float64Ptr(ptPerHour)
	}
}

func sortMusicBoardRows(rows []musicBoardRow, target, liveType string, ascend, keepOneDiffPerMusic bool) {
	sort.Slice(rows, func(i, j int) bool {
		left := musicBoardMetric(rows[i], target, liveType)
		right := musicBoardMetric(rows[j], target, liveType)
		if left == right {
			return boardDifficultyPriority(rows[i].Difficulty) > boardDifficultyPriority(rows[j].Difficulty)
		}
		if ascend {
			return left < right
		}
		return left > right
	})

	if keepOneDiffPerMusic {
		seen := make(map[int]struct{}, len(rows))
		rank := 1
		for idx := range rows {
			if _, ok := seen[rows[idx].MusicID]; ok {
				rows[idx].Rank = 0
				continue
			}
			seen[rows[idx].MusicID] = struct{}{}
			rows[idx].Rank = rank
			rank++
		}
		return
	}

	for idx := range rows {
		rows[idx].Rank = idx + 1
	}
}

func musicBoardMetric(row musicBoardRow, target, liveType string) float64 {
	switch target {
	case "score":
		return derefMusicBoardFloat(selectMusicBoardLiveValue(row, liveType, "score"))
	case "pt":
		return derefMusicBoardFloat(selectMusicBoardLiveValue(row, liveType, "pt"))
	case "pt/time":
		return derefMusicBoardFloat(selectMusicBoardLiveValue(row, liveType, "pt/time"))
	case "tps":
		return row.Tps
	case "time":
		return row.MusicTime
	default:
		return 0
	}
}

func selectMusicBoardLiveValue(row musicBoardRow, liveType, metric string) *float64 {
	switch liveType {
	case "solo":
		switch metric {
		case "score":
			return row.SoloScore
		case "pt":
			return row.SoloPt
		case "pt/time":
			return row.SoloPtPerHour
		}
	case "auto":
		switch metric {
		case "score":
			return row.AutoScore
		case "pt":
			return row.AutoPt
		case "pt/time":
			return row.AutoPtPerHour
		}
	case "multi":
		switch metric {
		case "score":
			return row.MultiScore
		case "pt":
			return row.MultiPt
		case "pt/time":
			return row.MultiPtPerHour
		}
	}
	return nil
}

func derefMusicBoardFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func matchesMusicBoardLevelFilter(level int, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}

	var op, value string
	switch {
	case strings.HasPrefix(filter, "<="), strings.HasPrefix(filter, ">="), strings.HasPrefix(filter, "=="):
		op, value = filter[:2], filter[2:]
	case strings.HasPrefix(filter, "<"), strings.HasPrefix(filter, ">"), strings.HasPrefix(filter, "="):
		op, value = filter[:1], filter[1:]
	default:
		return true
	}

	target := 0
	for _, ch := range strings.TrimSpace(value) {
		if ch < '0' || ch > '9' {
			return true
		}
		target = target*10 + int(ch-'0')
	}

	switch op {
	case "<":
		return level < target
	case ">":
		return level > target
	case "<=":
		return level <= target
	case ">=":
		return level >= target
	case "=", "==":
		return level == target
	default:
		return true
	}
}

func buildMusicBoardTexts(query musicBoardResolvedQuery, totalPage int) (string, string) {
	targetText := map[string]string{
		"score":   "LIVE分数",
		"pt":      "活动PT/体力",
		"pt/time": "活动PT/时间",
		"tps":     "每秒点击",
		"time":    "歌曲时长",
	}[query.Target]
	orderText := "降序"
	if query.Ascend {
		orderText = "升序"
	}

	liveText := map[string]string{
		"solo":  "单人LIVE",
		"auto":  "自动LIVE",
		"multi": "多人LIVE",
	}[query.LiveType]
	if query.Target == "tps" || query.Target == "time" {
		liveText = ""
	}

	title := "歌曲排行"
	if liveText != "" {
		title = liveText + "歌曲排行"
	}
	title = fmt.Sprintf("%s - %s %s - 第%d页/共%d页", title, targetText, orderText, query.Page, totalPage)

	parts := make([]string, 0, 5)
	if query.Target == "score" || query.Target == "pt" || query.Target == "pt/time" {
		if query.LiveType == "multi" {
			parts = append(parts, fmt.Sprintf("实效 %.0f%%", query.Skills[0]*100))
		} else {
			parts = append(parts, fmt.Sprintf("技能 %.0f/%.0f/%.0f/%.0f/%.0f", query.Skills[0]*100, query.Skills[1]*100, query.Skills[2]*100, query.Skills[3]*100, query.Skills[4]*100))
			parts = append(parts, "策略 "+strings.ToUpper(query.SkillStrategy))
		}
	}
	if query.Target == "pt" || query.Target == "pt/time" {
		parts = append(parts, fmt.Sprintf("综合 %d", query.Power))
		parts = append(parts, fmt.Sprintf("加成 %.0f%%", query.DeckBonus))
	}
	if query.Target == "pt/time" || query.Target == "time" {
		parts = append(parts, fmt.Sprintf("间隔 %.1fs", query.PlayInterval))
	}

	return title, strings.Join(parts, "  |  ")
}

func (c *Controller) loadMusicBoardMetaMap(region string) map[int][]drawing.MusicMetaInfo {
	payload := c.musicBoardMetaPayload(region)
	if len(payload) == 0 {
		return nil
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil
	}

	result := make(map[int][]drawing.MusicMetaInfo)
	for _, item := range items {
		musicID := musicMetaID(item)
		if musicID <= 0 {
			continue
		}
		result[musicID] = append(result[musicID], drawing.MusicMetaInfo{
			Difficulty:      normalizedDifficultyValue(item["difficulty"]),
			MusicTime:       floatValue(item["music_time"]),
			TapCount:        intValue(item["tap_count"]),
			EventRate:       floatValue(item["event_rate"]),
			BaseScore:       floatValue(item["base_score"]),
			BaseScoreAuto:   floatValue(item["base_score_auto"]),
			SkillScoreSolo:  floatSliceValue(item["skill_score_solo"]),
			SkillScoreAuto:  floatSliceValue(item["skill_score_auto"]),
			SkillScoreMulti: floatSliceValue(item["skill_score_multi"]),
			FeverScore:      floatValue(item["fever_score"]),
		})
	}

	for musicID := range result {
		sort.SliceStable(result[musicID], func(i, j int) bool {
			return boardDifficultyPriority(result[musicID][i].Difficulty) > boardDifficultyPriority(result[musicID][j].Difficulty)
		})
	}
	return result
}

func (c *Controller) musicBoardMetaPayload(region string) []byte {
	if c != nil && c.metaLoader != nil {
		if payload := c.metaLoader.Get(region); len(payload) > 0 {
			return payload
		}
	}
	if snapshot := c.currentSnapshot(); snapshot != nil {
		if payload := snapshot.MusicMetaBytes(); len(payload) > 0 {
			return payload
		}
	}
	return nil
}

func (c *Controller) resolveMusicBoardSpecs(source DataSource, rows []musicBoardRow, queries []string) ([]musicBoardSpec, error) {
	if len(queries) == 0 {
		return nil, nil
	}

	searcher := c.newSearchService(source)
	available := make(map[int][]string)
	for _, row := range rows {
		available[row.MusicID] = appendUniqueString(available[row.MusicID], row.Difficulty)
	}

	specs := make([]musicBoardSpec, 0, len(queries))
	seen := make(map[string]struct{})
	for _, rawQuery := range queries {
		query := strings.TrimSpace(rawQuery)
		if query == "" {
			continue
		}

		expandAllDiffs := strings.Contains(query, "*")
		if expandAllDiffs {
			query = strings.TrimSpace(strings.Replace(query, "*", "", 1))
		}
		if query == "" {
			return nil, fmt.Errorf("找不到歌曲或参数错误: %q", rawQuery)
		}

		info, err := searcher.parser.Parse(query)
		if err != nil {
			return nil, fmt.Errorf("找不到歌曲或参数错误: %q", rawQuery)
		}
		musicInfo, err := searcher.SearchInfo(info)
		if err != nil {
			if isMusicAmbiguousError(err) {
				return nil, err
			}
			return nil, fmt.Errorf("找不到歌曲或参数错误: %q", rawQuery)
		}
		if musicInfo == nil {
			return nil, fmt.Errorf("找不到歌曲或参数错误: %q", rawQuery)
		}

		if rawDiff := strings.TrimSpace(info.Diff); rawDiff != "" && !expandAllDiffs {
			diff := normalizeDifficulty(rawDiff)
			if !containsString(available[musicInfo.ID], diff) {
				return nil, fmt.Errorf("找不到歌曲或参数错误: %q", rawQuery)
			}
			key := musicBoardKey(musicInfo.ID, diff)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				specs = append(specs, musicBoardSpec{MusicID: musicInfo.ID, Difficulty: diff})
			}
			continue
		}

		matches := make([]musicBoardSpec, 0, len(available[musicInfo.ID]))
		for _, row := range rows {
			if row.MusicID != musicInfo.ID {
				continue
			}
			matches = append(matches, musicBoardSpec{MusicID: musicInfo.ID, Difficulty: row.Difficulty})
		}
		sort.Slice(matches, func(i, j int) bool {
			return boardDifficultyPriority(matches[i].Difficulty) > boardDifficultyPriority(matches[j].Difficulty)
		})
		for _, item := range matches {
			key := musicBoardKey(item.MusicID, item.Difficulty)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			specs = append(specs, item)
		}
	}

	return specs, nil
}

func musicBoardKey(musicID int, difficulty string) string {
	return fmt.Sprintf("%d:%s", musicID, normalizeDifficulty(difficulty))
}

func boardDifficultyPriority(difficulty string) int {
	switch normalizeDifficulty(difficulty) {
	case "master":
		return 6
	case "append":
		return 5
	case "expert":
		return 4
	case "hard":
		return 3
	case "normal":
		return 2
	case "easy":
		return 1
	default:
		return 0
	}
}

func appendUniqueString(values []string, item string) []string {
	if containsString(values, item) {
		return values
	}
	return append(values, item)
}

func float64Ptr(value float64) *float64 {
	return &value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
