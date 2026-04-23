package music

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

type musicFuzzyScore struct {
	matchType int
	distance  int
	lengthGap int
	textLen   int
}

func resolveFuzzyMusicQuery(source DataSource, query string, allowUnreleased bool) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music not found: empty query")
	}
	normalizedQuery := normalizeMusicFuzzyText(query)
	if normalizedQuery == "" {
		return nil, fmt.Errorf("music not found: %s", query)
	}

	now := currentMusicVisibilityTime()
	matches := make([]*masterdata.Music, 0)
	bestScores := make(map[int]musicFuzzyScore)
	for _, musicInfo := range source.GetMusics() {
		if !isMusicAccessibleAt(musicInfo, now, allowUnreleased) {
			continue
		}
		score, ok := scoreMusicFuzzyMatch(source, musicInfo, normalizedQuery)
		if !ok {
			continue
		}
		matches = append(matches, musicInfo)
		bestScores[musicInfo.ID] = score
	}
	if len(matches) == 0 {
		if allowUnreleased {
			return nil, fmt.Errorf("music not found: %s", query)
		}
		for _, musicInfo := range source.GetMusics() {
			if musicInfo == nil || isMusicVisibleAt(musicInfo, now) {
				continue
			}
			if _, ok := scoreMusicFuzzyMatch(source, musicInfo, normalizedQuery); ok {
				return nil, releasecheck.New(releasecheck.KindMusic, query, 0)
			}
		}
		return nil, fmt.Errorf("music not found: %s", query)
	}

	slices.SortFunc(matches, func(a, b *masterdata.Music) int {
		scoreA := bestScores[a.ID]
		scoreB := bestScores[b.ID]
		switch {
		case scoreA.matchType != scoreB.matchType:
			return scoreA.matchType - scoreB.matchType
		case scoreA.distance != scoreB.distance:
			return scoreA.distance - scoreB.distance
		case scoreA.lengthGap != scoreB.lengthGap:
			return scoreA.lengthGap - scoreB.lengthGap
		case scoreA.textLen != scoreB.textLen:
			return scoreA.textLen - scoreB.textLen
		default:
			return a.ID - b.ID
		}
	})

	best := bestScores[matches[0].ID]
	topMatches := make([]*masterdata.Music, 0, len(matches))
	for _, match := range matches {
		score := bestScores[match.ID]
		if score != best {
			break
		}
		topMatches = append(topMatches, match)
	}
	return selectUniqueMusicMatch("模糊匹配", topMatches)
}

func scoreMusicFuzzyMatch(source DataSource, musicInfo *masterdata.Music, normalizedQuery string) (musicFuzzyScore, bool) {
	best := musicFuzzyScore{}
	found := false
	for _, candidate := range musicFuzzyCandidates(source, musicInfo) {
		score, ok := scoreMusicFuzzyCandidate(normalizedQuery, candidate)
		if !ok {
			continue
		}
		if !found || compareMusicFuzzyScore(score, best) < 0 {
			best = score
			found = true
		}
	}
	return best, found
}

func musicFuzzyCandidates(source DataSource, musicInfo *masterdata.Music) []string {
	if musicInfo == nil {
		return nil
	}
	result := []string{musicInfo.Title, musicInfo.Pronunciation}
	if titles, err := source.GetMusicLocalizedTitles(musicInfo.ID); err == nil {
		result = append(result, titles...)
	}
	return result
}

func scoreMusicFuzzyCandidate(normalizedQuery string, candidate string) (musicFuzzyScore, bool) {
	normalizedCandidate := normalizeMusicFuzzyText(candidate)
	if normalizedCandidate == "" || normalizedQuery == "" {
		return musicFuzzyScore{}, false
	}
	queryRunes := []rune(normalizedQuery)
	candidateRunes := []rune(normalizedCandidate)
	queryLen := len(queryRunes)
	candidateLen := len(candidateRunes)
	switch {
	case normalizedCandidate == normalizedQuery:
		return musicFuzzyScore{
			matchType: 0,
			textLen:   candidateLen,
		}, true
	case queryLen >= 3 && strings.Contains(normalizedCandidate, normalizedQuery):
		return musicFuzzyScore{
			matchType: 1,
			lengthGap: absInt(candidateLen - queryLen),
			textLen:   candidateLen,
		}, true
	}

	if score, ok := scoreMusicFuzzySubstring(queryRunes, candidateRunes); ok {
		score.textLen = candidateLen
		return score, true
	}

	distance := levenshteinDistance(queryRunes, candidateRunes)
	if distance > fuzzyDistanceLimit(queryLen) {
		return musicFuzzyScore{}, false
	}
	return musicFuzzyScore{
		matchType: 3,
		distance:  distance,
		lengthGap: absInt(candidateLen - queryLen),
		textLen:   candidateLen,
	}, true
}

func compareMusicFuzzyScore(a, b musicFuzzyScore) int {
	switch {
	case a.matchType != b.matchType:
		return a.matchType - b.matchType
	case a.distance != b.distance:
		return a.distance - b.distance
	case a.lengthGap != b.lengthGap:
		return a.lengthGap - b.lengthGap
	default:
		return a.textLen - b.textLen
	}
}

func normalizeMusicFuzzyText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func fuzzyDistanceLimit(length int) int {
	switch {
	case length <= 2:
		return 0
	case length <= 5:
		return 1
	case length <= 10:
		return 2
	default:
		return 3
	}
}

func scoreMusicFuzzySubstring(queryRunes []rune, candidateRunes []rune) (musicFuzzyScore, bool) {
	queryLen := len(queryRunes)
	candidateLen := len(candidateRunes)
	if queryLen < 4 || candidateLen <= queryLen {
		return musicFuzzyScore{}, false
	}

	limit := fuzzyDistanceLimit(queryLen)
	minWindowLen := maxFuzzyInt(1, queryLen-limit)
	maxWindowLen := minFuzzyInt(candidateLen, queryLen+limit)
	best := musicFuzzyScore{}
	found := false
	for windowLen := minWindowLen; windowLen <= maxWindowLen; windowLen++ {
		for start := 0; start+windowLen <= candidateLen; start++ {
			distance := levenshteinDistance(queryRunes, candidateRunes[start:start+windowLen])
			if distance > limit {
				continue
			}
			score := musicFuzzyScore{
				matchType: 2,
				distance:  distance,
				lengthGap: absInt(windowLen - queryLen),
			}
			if !found || compareMusicFuzzyScore(score, best) < 0 {
				best = score
				found = true
			}
		}
	}
	return best, found
}

func levenshteinDistance(left []rune, right []rune) int {
	if len(left) == 0 {
		return len(right)
	}
	if len(right) == 0 {
		return len(left)
	}

	prev := make([]int, len(right)+1)
	curr := make([]int, len(right)+1)
	for j := range prev {
		prev[j] = j
	}

	for i, leftRune := range left {
		curr[0] = i + 1
		for j, rightRune := range right {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			curr[j+1] = min3Int(
				curr[j]+1,
				prev[j+1]+1,
				prev[j]+cost,
			)
		}
		copy(prev, curr)
	}
	return prev[len(right)]
}

func min3Int(left, middle, right int) int {
	best := left
	if middle < best {
		best = middle
	}
	if right < best {
		best = right
	}
	return best
}

func minFuzzyInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxFuzzyInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
