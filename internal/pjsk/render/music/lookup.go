package music

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

var susLinePattern = regexp.MustCompile(`^#([A-Za-z0-9]{3})([A-Za-z0-9]{2})\s*:\s*(\S+)`)

func (c *Controller) FindMusicChartsByNoteCount(query NoteCountQuery) ([]NoteCountMatch, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if query.NoteCount <= 0 {
		return nil, fmt.Errorf("物量必须大于 0")
	}
	targetDifficulty := strings.TrimSpace(query.Difficulty)
	if targetDifficulty != "" {
		targetDifficulty = normalizeDifficulty(targetDifficulty)
	}

	_, source, _, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}

	matches, err := findMusicChartsByNoteCount(source, query.NoteCount, targetDifficulty)
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		if targetDifficulty != "" {
			return nil, fmt.Errorf("没有找到物量为 %d 的 %s 谱面", query.NoteCount, targetDifficulty)
		}
		return nil, fmt.Errorf("没有找到物量为 %d 的谱面", query.NoteCount)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Music.ID == matches[j].Music.ID {
			return difficultyOrder(matches[i].Difficulty) < difficultyOrder(matches[j].Difficulty)
		}
		return matches[i].Music.ID < matches[j].Music.ID
	})
	return matches, nil
}

func findMusicChartsByNoteCount(source DataSource, noteCount int, targetDifficulty string) ([]NoteCountMatch, error) {
	if finder, ok := source.(noteCountFinder); ok {
		return findIndexedMusicChartsByNoteCount(source, finder, noteCount, targetDifficulty)
	}
	return scanMusicChartsByNoteCount(source, noteCount, targetDifficulty), nil
}

func findIndexedMusicChartsByNoteCount(source DataSource, finder noteCountFinder, noteCount int, targetDifficulty string) ([]NoteCountMatch, error) {
	items, err := finder.FindMusicDifficultiesByNoteCount(noteCount)
	if err != nil {
		return nil, err
	}
	now := currentMusicVisibilityTime()
	matches := make([]NoteCountMatch, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		musicInfo, lookupErr := source.GetMusicByID(item.MusicID)
		if lookupErr != nil || !isMusicVisibleAt(musicInfo, now) {
			continue
		}
		matches = appendNoteCountMatch(matches, musicInfo, item, targetDifficulty)
	}
	return matches, nil
}

func scanMusicChartsByNoteCount(source DataSource, noteCount int, targetDifficulty string) []NoteCountMatch {
	now := currentMusicVisibilityTime()
	matches := make([]NoteCountMatch, 0)
	for _, musicInfo := range source.GetMusics() {
		if !isMusicVisibleAt(musicInfo, now) {
			continue
		}
		difficulties, err := source.GetMusicDifficulties(musicInfo.ID)
		if err != nil {
			continue
		}
		for _, item := range difficulties {
			if item != nil && item.TotalNoteCount == noteCount {
				matches = appendNoteCountMatch(matches, musicInfo, item, targetDifficulty)
			}
		}
	}
	return matches
}

func appendNoteCountMatch(matches []NoteCountMatch, musicInfo *masterdata.Music, item *masterdata.MusicDifficulty, targetDifficulty string) []NoteCountMatch {
	difficulty := normalizeDifficulty(item.MusicDifficulty)
	if targetDifficulty != "" && difficulty != targetDifficulty {
		return matches
	}
	return append(matches, NoteCountMatch{
		Music: musicInfo, Difficulty: difficulty, PlayLevel: item.PlayLevel, TotalNoteCount: item.TotalNoteCount,
	})
}
