package music

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/render/common"
)

var musicBoardTargetLabels = map[string]i18n.Message{
	"score":             i18n.M("music.image.board.target_score"),
	"pt":                i18n.M("music.image.board.target_pt"),
	pointsPerTimeMetric: i18n.M("music.image.board.target_pt_per_time"),
	"tps":               i18n.M("music.image.board.target_tps"),
	"time":              i18n.M("music.image.board.target_time"),
}

// buildMusicBoardTexts is the title and the parameter line of the song
// ranking image.
func buildMusicBoardTexts(query musicBoardResolvedQuery, totalPage int) (string, string) {
	target := i18n.Verbatim(query.Target)
	if label, ok := musicBoardTargetLabels[query.Target]; ok {
		target = label
	}
	order := i18n.M("music.image.board.order_desc")
	if query.Ascend {
		order = i18n.M("music.image.board.order_asc")
	}
	board := i18n.M("music.image.board.name")
	switch query.LiveType {
	case "solo", "auto", "multi":
		if query.Target != "tps" && query.Target != "time" {
			board = i18n.M("music.image.board.name_live", i18n.Data{"Live": i18n.LiveTypeLabel(query.LiveType)})
		}
	}
	title := i18n.T("music.image.board.title", i18n.Data{
		"Board":  board,
		"Target": target,
		"Order":  order,
		"Page":   i18n.PageLabel(query.Page, totalPage),
	})

	parts := make([]string, 0, 5)
	if query.Target == "score" || query.Target == "pt" || query.Target == pointsPerTimeMetric {
		if query.LiveType == "multi" {
			parts = append(parts, i18n.T("music.image.board.effective_skill", i18n.Data{"Percent": i18n.PercentN(query.Skills[0]*100, 0)}))
		} else {
			skills := make([]string, 0, 5)
			for _, skill := range query.Skills[:5] {
				skills = append(skills, strconv.FormatFloat(math.Round(skill*100), 'f', 0, 64))
			}
			parts = append(parts, i18n.T("music.image.board.skills", i18n.Data{"Skills": strings.Join(skills, "/")}))
			parts = append(parts, i18n.T("music.image.board.strategy", i18n.Data{"Strategy": strings.ToUpper(query.SkillStrategy)}))
		}
	}
	if query.Target == "pt" || query.Target == pointsPerTimeMetric {
		parts = append(parts, i18n.T("music.image.board.power", i18n.Data{"Power": query.Power}))
		parts = append(parts, i18n.T("music.image.board.bonus", i18n.Data{"Percent": i18n.PercentN(query.DeckBonus, 0)}))
	}
	if query.Target == pointsPerTimeMetric || query.Target == "time" {
		parts = append(parts, i18n.T("music.image.board.interval", i18n.Data{"Seconds": strconv.FormatFloat(query.PlayInterval, 'f', 1, 64)}))
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
