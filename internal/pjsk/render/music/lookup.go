package music

import (
	"fmt"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"regexp"
	"sort"
	"strings"
)

type NoteCountMatch struct {
	Music          *masterdata.Music
	Difficulty     string
	PlayLevel      int
	TotalNoteCount int
}

type CoverResult struct {
	Music      *masterdata.Music
	JacketPath string
}

type BPMEvent struct {
	Bar      float64
	BPM      float64
	Duration float64
}

type BPMResult struct {
	Music      *masterdata.Music
	JacketPath string
	Difficulty string
	MainBPM    float64
	Events     []BPMEvent
	BarCount   int
	Duration   float64
}

type BPMMatch struct {
	Music      *masterdata.Music
	Difficulty string
	MainBPM    float64
	Events     []BPMEvent
}

type noteCountFinder interface {
	FindMusicDifficultiesByNoteCount(noteCount int) ([]*masterdata.MusicDifficulty, error)
}

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

	now := currentMusicVisibilityTime()
	matches := make([]NoteCountMatch, 0)
	if finder, ok := source.(noteCountFinder); ok {
		items, err := finder.FindMusicDifficultiesByNoteCount(query.NoteCount)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			musicInfo, err := source.GetMusicByID(item.MusicID)
			if err != nil || !isMusicVisibleAt(musicInfo, now) {
				continue
			}
			diff := normalizeDifficulty(item.MusicDifficulty)
			if targetDifficulty != "" && diff != targetDifficulty {
				continue
			}
			matches = append(matches, NoteCountMatch{
				Music:          musicInfo,
				Difficulty:     diff,
				PlayLevel:      item.PlayLevel,
				TotalNoteCount: item.TotalNoteCount,
			})
		}
	} else {
		for _, musicInfo := range source.GetMusics() {
			if !isMusicVisibleAt(musicInfo, now) {
				continue
			}
			difficulties, err := source.GetMusicDifficulties(musicInfo.ID)
			if err != nil {
				continue
			}
			for _, item := range difficulties {
				if item == nil || item.TotalNoteCount != query.NoteCount {
					continue
				}
				diff := normalizeDifficulty(item.MusicDifficulty)
				if targetDifficulty != "" && diff != targetDifficulty {
					continue
				}
				matches = append(matches, NoteCountMatch{
					Music:          musicInfo,
					Difficulty:     diff,
					PlayLevel:      item.PlayLevel,
					TotalNoteCount: item.TotalNoteCount,
				})
			}
		}
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
