package music

import (
	"math"
	"sort"
	"strings"
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
		case "real_score":
			return row.SoloRealScore
		case "pt":
			return row.SoloPt
		case "pt/time", "pt_per_hour":
			return row.SoloPtPerHour
		case "skill_account":
			return row.SoloSkillAccount
		}
	case "auto":
		switch metric {
		case "score":
			return row.AutoScore
		case "real_score":
			return row.AutoRealScore
		case "pt":
			return row.AutoPt
		case "pt/time", "pt_per_hour":
			return row.AutoPtPerHour
		case "skill_account":
			return row.AutoSkillAccount
		}
	case "multi":
		switch metric {
		case "score":
			return row.MultiScore
		case "real_score":
			return row.MultiRealScore
		case "pt":
			return row.MultiPt
		case "pt/time", "pt_per_hour":
			return row.MultiPtPerHour
		case "skill_account":
			return row.MultiSkillAccount
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
