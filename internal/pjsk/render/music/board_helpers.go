package music

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
)

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
	if common.ContainsString(values, item) {
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
