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
			if !common.ContainsString(available[musicInfo.ID], diff) {
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
