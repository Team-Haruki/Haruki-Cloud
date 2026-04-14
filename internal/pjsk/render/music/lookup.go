package music

import (
	"fmt"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"regexp"
	"sort"
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
			matches = append(matches, NoteCountMatch{
				Music:          musicInfo,
				Difficulty:     normalizeDifficulty(item.MusicDifficulty),
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
				matches = append(matches, NoteCountMatch{
					Music:          musicInfo,
					Difficulty:     normalizeDifficulty(item.MusicDifficulty),
					PlayLevel:      item.PlayLevel,
					TotalNoteCount: item.TotalNoteCount,
				})
			}
		}
	}

	if len(matches) == 0 {
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
