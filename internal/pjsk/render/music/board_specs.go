package music

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
)

func (c *Controller) resolveMusicBoardSpecs(source DataSource, rows []musicBoardRow, queries []string) ([]musicBoardSpec, error) {
	if len(queries) == 0 {
		return nil, nil
	}

	searcher := c.newSearchService(source, false)
	available := availableMusicBoardDifficulties(rows)
	specs := make([]musicBoardSpec, 0, len(queries))
	seen := make(map[string]struct{})
	for _, rawQuery := range queries {
		matches, err := resolveMusicBoardSpec(searcher, rows, available, rawQuery)
		if err != nil {
			return nil, err
		}
		specs = appendUniqueMusicBoardSpecs(specs, seen, matches)
	}
	return specs, nil
}

func availableMusicBoardDifficulties(rows []musicBoardRow) map[int][]string {
	available := make(map[int][]string)
	for _, row := range rows {
		available[row.MusicID] = appendUniqueString(available[row.MusicID], row.Difficulty)
	}
	return available
}

func resolveMusicBoardSpec(searcher *SearchService, rows []musicBoardRow, available map[int][]string, rawQuery string) ([]musicBoardSpec, error) {
	query := strings.TrimSpace(rawQuery)
	if query == "" {
		return nil, nil
	}
	expandAllDiffs := strings.Contains(query, "*")
	if expandAllDiffs {
		query = strings.TrimSpace(strings.Replace(query, "*", "", 1))
	}
	if query == "" {
		return nil, invalidMusicBoardSpec(rawQuery)
	}

	info, err := searcher.parser.Parse(query)
	if err != nil {
		return nil, invalidMusicBoardSpec(rawQuery)
	}
	musicInfo, err := searcher.SearchInfo(info)
	if err != nil {
		if isMusicAmbiguousError(err) {
			return nil, err
		}
		return nil, invalidMusicBoardSpec(rawQuery)
	}
	if musicInfo == nil {
		return nil, invalidMusicBoardSpec(rawQuery)
	}

	if rawDiff := strings.TrimSpace(info.Diff); rawDiff != "" && !expandAllDiffs {
		diff := normalizeDifficulty(rawDiff)
		if !common.ContainsString(available[musicInfo.ID], diff) {
			return nil, invalidMusicBoardSpec(rawQuery)
		}
		return []musicBoardSpec{{MusicID: musicInfo.ID, Difficulty: diff}}, nil
	}
	return allMusicBoardSpecsForMusic(rows, musicInfo.ID), nil
}

func allMusicBoardSpecsForMusic(rows []musicBoardRow, musicID int) []musicBoardSpec {
	matches := make([]musicBoardSpec, 0)
	for _, row := range rows {
		if row.MusicID == musicID {
			matches = append(matches, musicBoardSpec{MusicID: musicID, Difficulty: row.Difficulty})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return boardDifficultyPriority(matches[i].Difficulty) > boardDifficultyPriority(matches[j].Difficulty)
	})
	return matches
}

func appendUniqueMusicBoardSpecs(specs []musicBoardSpec, seen map[string]struct{}, matches []musicBoardSpec) []musicBoardSpec {
	for _, item := range matches {
		key := musicBoardKey(item.MusicID, item.Difficulty)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		specs = append(specs, item)
	}
	return specs
}

func invalidMusicBoardSpec(rawQuery string) error {
	return fmt.Errorf("找不到歌曲或参数错误: %q", rawQuery)
}
