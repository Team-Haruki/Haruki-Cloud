package sekai

import (
	"fmt"
	"strconv"
	"strings"

	rendermusic "haruki-cloud/internal/pjsk/render/music"
)

func extractMusicBoardPageArg(args string) (int, string, bool) {
	for _, token := range strings.Fields(args) {
		page, ok := parseMusicBoardPage(strings.ToLower(strings.TrimSpace(token)))
		if !ok {
			continue
		}
		return page, removeMusicBoardToken(args, token), true
	}
	return 0, args, false
}

func extractMusicBoardSkills(args, liveType string) ([]float64, string, error) {
	hadKeyword := strings.Contains(args, "技能") || strings.Contains(args, "实效")
	cleaned := strings.ReplaceAll(args, "技能", "")
	cleaned = strings.ReplaceAll(cleaned, "实效", "")
	cleaned = strings.TrimSpace(cleaned)

	required := 5
	if liveType == "multi" {
		required = 1
	}

	fields := strings.Fields(cleaned)
	numbers := make([]float64, 0, required)
	numberTokens := make([]string, 0, required)
	for _, field := range fields {
		value, ok := parseMusicBoardSkillNumber(field)
		if !ok {
			break
		}
		numbers = append(numbers, value/100.0)
		numberTokens = append(numberTokens, field)
		if len(numbers) >= required {
			break
		}
	}

	shouldTreatAsSkills := hadKeyword || (required > 1 && len(numbers) == required)
	if !shouldTreatAsSkills || len(numbers) == 0 {
		return nil, cleaned, nil
	}
	if len(numbers) != required {
		return nil, "", fmt.Errorf("解析技能加分失败")
	}

	remaining := cleaned
	for _, token := range numberTokens {
		remaining = removeMusicBoardToken(remaining, token)
	}

	if liveType == "multi" {
		return []float64{numbers[0], numbers[0], numbers[0], numbers[0], numbers[0]}, remaining, nil
	}
	return numbers, remaining, nil
}

func parseMusicBoardSkillNumber(token string) (float64, bool) {
	raw := strings.TrimSpace(strings.TrimSuffix(token, "%"))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && value > 0
}

func extractMusicBoardLevelFilter(args string) (string, string) {
	remaining := strings.TrimSpace(args)
	for _, token := range strings.Fields(args) {
		lower := strings.ToLower(strings.TrimSpace(token))
		if !isMusicBoardLevelFilter(lower) {
			continue
		}
		return lower, removeMusicBoardToken(remaining, token)
	}
	return "", remaining
}

func extractMusicBoardDiffFilters(args string) ([]string, string) {
	remaining := strings.TrimSpace(args)
	diffFilter := make([]string, 0, 2)
	for _, token := range strings.Fields(args) {
		diff, rest := rendermusic.ExtractMusicDifficulty(token)
		if diff == "" || strings.TrimSpace(rest) != "" {
			continue
		}
		if !containsMusicBoardString(diffFilter, diff) {
			diffFilter = append(diffFilter, diff)
		}
		remaining = removeMusicBoardToken(remaining, token)
	}
	return diffFilter, strings.TrimSpace(remaining)
}

func removeMusicBoardToken(args, token string) string {
	index := strings.Index(args, token)
	if index < 0 {
		return strings.TrimSpace(args)
	}
	return removeMusicBoardSpan(args, index, len(token))
}

func removeMusicBoardSpan(args string, start, length int) string {
	if start < 0 || length <= 0 || start+length > len(args) {
		return strings.TrimSpace(args)
	}
	return strings.TrimSpace(args[:start] + args[start+length:])
}

func containsMusicBoardString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func parseMusicBoardPage(token string) (int, bool) {
	if strings.Contains(token, "页") || strings.Contains(token, "p") {
		value := strings.Replace(token, "页", "", 1)
		value = strings.Replace(value, "p", "", 1)
		page, err := strconv.Atoi(value)
		return page, err == nil && page > 0
	}
	return 0, false
}

func parseMusicBoardLargeNumber(raw string) (int, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return 0, fmt.Errorf("empty power")
	}
	multiplier := 1.0
	switch {
	case strings.HasSuffix(raw, "万"):
		raw = strings.TrimSuffix(raw, "万")
		multiplier = 10000
	case strings.HasSuffix(raw, "w"):
		raw = strings.TrimSuffix(raw, "w")
		multiplier = 10000
	case strings.HasSuffix(raw, "k"):
		raw = strings.TrimSuffix(raw, "k")
		multiplier = 1000
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return int(value * multiplier), nil
}

func isMusicBoardLevelFilter(token string) bool {
	if token == "" {
		return false
	}
	switch {
	case strings.HasPrefix(token, "<="), strings.HasPrefix(token, ">="), strings.HasPrefix(token, "=="):
		token = token[2:]
	case strings.HasPrefix(token, "<"), strings.HasPrefix(token, ">"), strings.HasPrefix(token, "="):
		token = token[1:]
	default:
		return false
	}
	for _, ch := range token {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return token != ""
}
