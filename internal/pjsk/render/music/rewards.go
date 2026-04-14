package music

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
	sekai "haruki-cloud/utils/sekai"
)

type userMusicAchievement struct {
	MusicID            int `json:"musicId"`
	MusicAchievementID int `json:"musicAchievementId"`
}

type musicAchievementReward struct {
	Coin  int
	Jewel int
	Shard int
}

var musicRankRewards = map[int]musicAchievementReward{
	1: {Jewel: 10},
	2: {Jewel: 20},
	3: {Jewel: 30},
	4: {Jewel: 50},
}

var musicComboRewards = map[string]map[int]musicAchievementReward{
	"easy": {
		5: {Coin: 500},
		6: {Coin: 1000},
		7: {Coin: 2000},
		8: {Coin: 5000},
	},
	"normal": {
		9:  {Coin: 1000},
		10: {Coin: 2000},
		11: {Coin: 4000},
		12: {Coin: 10000},
	},
	"hard": {
		13: {Coin: 1500},
		14: {Coin: 3000},
		15: {Coin: 6000},
		16: {Jewel: 50},
	},
	"expert": {
		17: {Coin: 2000},
		18: {Coin: 4000},
		19: {Jewel: 20},
		20: {Jewel: 50},
	},
	"master": {
		21: {Coin: 3000},
		22: {Coin: 6000},
		23: {Jewel: 20},
		24: {Jewel: 50},
	},
	"append": {
		25: {Coin: 3000},
		26: {Coin: 6000},
		27: {Shard: 5},
		28: {Shard: 10},
	},
}

func (c *Controller) BuildMusicRewardsDetailRequestFromAchievements(query RewardsDetailQuery, achievementsJSON []byte) (*drawing.DetailMusicRewardsRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}

	achievements, err := decodeUserMusicAchievements(achievementsJSON)
	if err != nil {
		return nil, fmt.Errorf("decode userMusicAchievements: %w", err)
	}

	validMusicIDs := c.validRewardMusicIDs(region, source, builder)
	if len(validMusicIDs) == 0 {
		return nil, fmt.Errorf("no reward-eligible musics found")
	}

	achievementsByMusic := make(map[int]map[int]struct{}, len(validMusicIDs))
	for _, item := range achievements {
		if _, ok := validMusicIDs[item.MusicID]; !ok {
			continue
		}
		if _, ok := achievementsByMusic[item.MusicID]; !ok {
			achievementsByMusic[item.MusicID] = make(map[int]struct{})
		}
		achievementsByMusic[item.MusicID][item.MusicAchievementID] = struct{}{}
	}

	rankRewards := 0
	comboRewards := map[string]map[int]int{
		"hard":   {},
		"expert": {},
		"master": {},
		"append": {},
	}

	for musicID := range validMusicIDs {
		for achievementID, reward := range musicRankRewards {
			if _, ok := achievementsByMusic[musicID][achievementID]; !ok {
				rankRewards += reward.Jewel
			}
		}

		diffInfo, err := builder.buildDifficultyInfo(musicID)
		if err != nil || diffInfo == nil {
			continue
		}
		for _, diff := range []string{"hard", "expert", "master", "append"} {
			level := difficultyLevelFromInfo(diffInfo, diff)
			if level == 0 {
				continue
			}
			comboRewards[diff][level] += missingComboRewardTotal(diff, achievementsByMusic[musicID])
		}
	}

	out := make(map[string][]drawing.MusicComboReward, len(comboRewards))
	for _, diff := range []string{"hard", "expert", "master", "append"} {
		levels := sortedRewardLevels(comboRewards[diff])
		items := make([]drawing.MusicComboReward, 0, len(levels))
		for _, level := range levels {
			items = append(items, drawing.MusicComboReward{
				Level:  level,
				Reward: comboRewards[diff][level],
			})
		}
		out[diff] = items
	}

	return &drawing.DetailMusicRewardsRequest{
		RankRewards:   rankRewards,
		ComboRewards:  ensureDetailComboRewards(out),
		Profile:       c.profileCardWithMessage(query.Profile, region, nil),
		JewelIconPath: c.resolveStaticIcon(query.JewelIconPath, "jewel.png"),
		ShardIconPath: c.resolveStaticIcon(query.ShardIconPath, "shard.png"),
	}, nil
}

func (c *Controller) BuildMusicRewardsDetailRequestFromSnapshot(query RewardsDetailQuery, snapshot userdata.Snapshot) (*drawing.DetailMusicRewardsRequest, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("user snapshot is required for music rewards detail")
	}
	if err := snapshot.Require(); err != nil {
		return nil, err
	}
	achievementsJSON, err := resolveSnapshotAchievementsJSON(snapshot)
	if err != nil {
		return nil, err
	}
	return c.BuildMusicRewardsDetailRequestFromAchievements(query, achievementsJSON)
}

func (c *Controller) BuildMusicRewardsBasicEstimateRequest(query RewardsBasicQuery, clearCounts []sekai.AnotherUserMusicDifficultyClearCount, reason string) (*drawing.BasicMusicRewardsRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}

	validMusicIDs := c.validRewardMusicIDs(region, source, builder)
	if len(validMusicIDs) == 0 {
		return nil, fmt.Errorf("no reward-eligible musics found")
	}

	clearByDiff := make(map[string]int)
	fcByDiff := make(map[string]int)
	for _, item := range clearCounts {
		diff := strings.ToLower(strings.TrimSpace(string(item.MusicDifficultyType)))
		if diff == "" {
			continue
		}
		clearByDiff[diff] = item.LiveClear
		fcByDiff[diff] = item.FullCombo
	}

	musicNum := len(validMusicIDs)
	appendMusicNum := 0
	for musicID := range validMusicIDs {
		diffInfo, err := builder.buildDifficultyInfo(musicID)
		if err != nil || diffInfo == nil {
			continue
		}
		if difficultyLevelFromInfo(diffInfo, "append") > 0 {
			appendMusicNum++
		}
	}

	rankSNum := 0
	for _, count := range clearByDiff {
		if count > rankSNum {
			rankSNum = count
		}
	}
	if rankSNum > musicNum {
		rankSNum = musicNum
	}

	totalRankReward := 0
	for _, reward := range musicRankRewards {
		totalRankReward += reward.Jewel
	}

	comboRewards := map[string]string{}
	for _, diff := range []string{"hard", "expert", "master", "append"} {
		totalPerMusic := 0
		for _, reward := range musicComboRewards[diff] {
			if diff == "append" {
				totalPerMusic += reward.Shard
			} else {
				totalPerMusic += reward.Jewel
			}
		}
		targetMusicCount := musicNum
		if diff == "append" {
			targetMusicCount = appendMusicNum
		}
		missingCount := targetMusicCount - fcByDiff[diff]
		if missingCount < 0 {
			missingCount = 0
		}
		comboRewards[diff] = formatEstimatedReward(totalPerMusic, missingCount)
	}

	message := reason
	if message == "" {
		message = "当前未使用 Suite 抓包数据，以下为基于公开信息的估算结果。"
	} else {
		message += "\n以下为基于公开信息的估算结果。"
	}

	return &drawing.BasicMusicRewardsRequest{
		RankRewards:   formatEstimatedReward(totalRankReward, musicNum-rankSNum),
		ComboRewards:  comboRewards,
		Profile:       c.profileCardWithMessage(query.Profile, region, &message),
		JewelIconPath: c.resolveStaticIcon(query.JewelIconPath, "jewel.png"),
		ShardIconPath: c.resolveStaticIcon(query.ShardIconPath, "shard.png"),
	}, nil
}

func (c *Controller) validRewardMusicIDs(region renderregion.Value, source DataSource, builder *Builder) map[int]struct{} {
	now := time.Now().UnixMilli()
	result := make(map[int]struct{})
	for _, musicInfo := range source.GetMusics() {
		if musicInfo == nil {
			continue
		}
		if _, blocked := hiddenMusicIDs[musicInfo.ID]; blocked {
			continue
		}
		if musicInfo.PublishedAt > now {
			continue
		}
		if !musicRewardAvailableNow(source, musicInfo.ID, now, region) {
			continue
		}
		diffInfo, err := builder.buildDifficultyInfo(musicInfo.ID)
		if err != nil || diffInfo == nil {
			continue
		}
		if difficultyLevelFromInfo(diffInfo, "easy") == 0 &&
			difficultyLevelFromInfo(diffInfo, "normal") == 0 &&
			difficultyLevelFromInfo(diffInfo, "hard") == 0 &&
			difficultyLevelFromInfo(diffInfo, "expert") == 0 &&
			difficultyLevelFromInfo(diffInfo, "master") == 0 &&
			difficultyLevelFromInfo(diffInfo, "append") == 0 {
			continue
		}
		result[musicInfo.ID] = struct{}{}
	}
	return result
}

func musicRewardAvailableNow(source DataSource, musicID int, now int64, region renderregion.Value) bool {
	limited := source.GetLimitedTimeMusics(musicID)
	if len(limited) == 0 {
		return true
	}
	for _, item := range limited {
		if item == nil {
			continue
		}
		if item.StartAt <= now && now < item.EndAt {
			return true
		}
	}
	return false
}

func difficultyLevelFromInfo(info *drawing.DifficultyInfo, diff string) int {
	if info == nil {
		return 0
	}
	for idx, name := range info.Order {
		if strings.EqualFold(strings.TrimSpace(name), diff) && idx < len(info.Level) {
			return info.Level[idx]
		}
	}
	return 0
}

func missingComboRewardTotal(diff string, achievements map[int]struct{}) int {
	total := 0
	for achievementID, reward := range musicComboRewards[diff] {
		if _, ok := achievements[achievementID]; ok {
			continue
		}
		if diff == "append" {
			total += reward.Shard
		} else {
			total += reward.Jewel
		}
	}
	return total
}

func sortedRewardLevels(levelRewards map[int]int) []int {
	levels := make([]int, 0, len(levelRewards))
	for level, reward := range levelRewards {
		if reward <= 0 {
			continue
		}
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels
}

func formatEstimatedReward(single int, count int) string {
	if count < 0 {
		count = 0
	}
	total := single * count
	return fmt.Sprintf("%d (%d×%d)", total, single, count)
}

func decodeUserMusicAchievements(raw []byte) ([]userMusicAchievement, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var direct []userMusicAchievement
	if err := json.Unmarshal(trimmed, &direct); err == nil {
		return direct, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()

	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	items := collectUserMusicAchievements(payload)
	if len(items) == 0 {
		return nil, fmt.Errorf("unsupported achievements payload shape")
	}
	return items, nil
}

var snapshotAchievementKeys = []string{
	"userMusicAchievements",
	"compactUserMusicAchievements",
}

func resolveSnapshotAchievementsJSON(snapshot userdata.Snapshot) ([]byte, error) {
	for _, key := range snapshotAchievementKeys {
		value, err := snapshot.RawValue(key)
		if err == nil {
			return value, nil
		}
	}
	return extractNestedAchievementsJSON(snapshot)
}

func extractNestedAchievementsJSON(snapshot userdata.Snapshot) ([]byte, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("user snapshot is required for music rewards detail")
	}
	raw, err := snapshot.RawBytes()
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	for _, key := range snapshotAchievementKeys {
		want := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "_", ""), "-", ""))
		if value, ok := findNestedJSONValue(payload, want); ok {
			data, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("raw user snapshot keys %q are unavailable", strings.Join(snapshotAchievementKeys, ", "))
}

func collectUserMusicAchievements(value any) []userMusicAchievement {
	switch typed := value.(type) {
	case []any:
		out := make([]userMusicAchievement, 0, len(typed))
		for _, item := range typed {
			out = append(out, collectUserMusicAchievements(item)...)
		}
		return out
	case map[string]any:
		if item, ok := parseAchievementItemMap(typed); ok {
			return []userMusicAchievement{item}
		}
		if items, ok := parseAchievementColumnsMap(typed); ok {
			return items
		}
		if items, ok := parseAchievementGroupedMap(typed); ok {
			return items
		}

		out := make([]userMusicAchievement, 0)
		for _, item := range typed {
			out = append(out, collectUserMusicAchievements(item)...)
		}
		return out
	default:
		return nil
	}
}

func findNestedJSONValue(value any, want string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "_", ""), "-", ""))
			if normalized == want {
				return item, true
			}
		}
		for _, item := range typed {
			if found, ok := findNestedJSONValue(item, want); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range typed {
			if found, ok := findNestedJSONValue(item, want); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func parseAchievementItemMap(value map[string]any) (userMusicAchievement, bool) {
	musicID, okMusic := findAchievementInt(value, "musicid")
	achievementID, okAchievement := findAchievementInt(value, "musicachievementid")
	if !okMusic || !okAchievement || musicID <= 0 || achievementID <= 0 {
		return userMusicAchievement{}, false
	}
	return userMusicAchievement{
		MusicID:            musicID,
		MusicAchievementID: achievementID,
	}, true
}

func parseAchievementColumnsMap(value map[string]any) ([]userMusicAchievement, bool) {
	musicIDsRaw, okMusic := findAchievementValue(value, "musicid")
	achievementIDsRaw, okAchievement := findAchievementValue(value, "musicachievementid")
	if !okMusic || !okAchievement {
		return nil, false
	}

	musicIDs, okMusic := toAchievementIntSlice(musicIDsRaw)
	achievementIDs, okAchievement := toAchievementIntSlice(achievementIDsRaw)
	if !okMusic || !okAchievement || len(musicIDs) == 0 || len(musicIDs) != len(achievementIDs) {
		return nil, false
	}

	items := make([]userMusicAchievement, 0, len(musicIDs))
	for idx := range musicIDs {
		if musicIDs[idx] <= 0 || achievementIDs[idx] <= 0 {
			continue
		}
		items = append(items, userMusicAchievement{
			MusicID:            musicIDs[idx],
			MusicAchievementID: achievementIDs[idx],
		})
	}
	return items, len(items) > 0
}

func parseAchievementGroupedMap(value map[string]any) ([]userMusicAchievement, bool) {
	items := make([]userMusicAchievement, 0)
	for key, raw := range value {
		musicID, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil || musicID <= 0 {
			continue
		}

		achievementIDs, ok := toAchievementIntSlice(raw)
		if !ok {
			continue
		}
		for _, achievementID := range achievementIDs {
			if achievementID <= 0 {
				continue
			}
			items = append(items, userMusicAchievement{
				MusicID:            musicID,
				MusicAchievementID: achievementID,
			})
		}
	}
	return items, len(items) > 0
}

func findAchievementValue(value map[string]any, want string) (any, bool) {
	for key, item := range value {
		normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "_", ""), "-", ""))
		if normalized == want {
			return item, true
		}
	}
	return nil, false
}

func findAchievementInt(value map[string]any, want string) (int, bool) {
	raw, ok := findAchievementValue(value, want)
	if !ok {
		return 0, false
	}
	return toAchievementInt(raw)
}

func toAchievementIntSlice(value any) ([]int, bool) {
	switch typed := value.(type) {
	case []any:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			intValue, ok := toAchievementInt(item)
			if !ok {
				return nil, false
			}
			out = append(out, intValue)
		}
		return out, true
	default:
		intValue, ok := toAchievementInt(value)
		if !ok {
			return nil, false
		}
		return []int{intValue}, true
	}
}

func toAchievementInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
