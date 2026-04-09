package music

import (
	"encoding/json"
	"fmt"
	"sort"
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

	var achievements []userMusicAchievement
	if err := json.Unmarshal(achievementsJSON, &achievements); err != nil {
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
	achievementsJSON, err := snapshot.RawValue("userMusicAchievements")
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
