package provider

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localMusicProvider
// ===========================================================================

type localMusicProvider struct {
	store  *localStore
	events EventProvider

	musicOnce sync.Once
	musicAll  []*masterdata.Music
	musicByID map[int]*masterdata.Music
	musicErr  error

	diffOnce    sync.Once
	diffByMusic map[int][]*masterdata.MusicDifficulty
	diffErr     error

	vocalOnce    sync.Once
	vocalByMusic map[int][]*masterdata.MusicVocal
	vocalErr     error

	tagOnce    sync.Once
	tagByMusic map[int][]string
	tagErr     error

	outsideOnce sync.Once
	outsideByID map[int]string
	outsideErr  error

	eventMusicOnce  sync.Once
	musicIDByEvent  map[int]int
	eventIDsByMusic map[int][]int
	eventMusicErr   error

	limitedOnce    sync.Once
	limitedByMusic map[int][]*masterdata.LimitedTimeMusic
	limitedErr     error
}

func (p *localMusicProvider) ensureMusics() error {
	p.musicOnce.Do(func() {
		items, err := loadJSON[localMusicJSON](p.store, "musics.json")
		if err != nil {
			p.musicErr = err
			return
		}
		p.musicByID = make(map[int]*masterdata.Music, len(items))
		p.musicAll = make([]*masterdata.Music, 0, len(items))
		for i := range items {
			m := items[i].toModel()
			p.musicByID[m.ID] = m
			p.musicAll = append(p.musicAll, m)
		}
		sort.Slice(p.musicAll, func(i, j int) bool {
			if p.musicAll[i].PublishedAt == p.musicAll[j].PublishedAt {
				return p.musicAll[i].ID < p.musicAll[j].ID
			}
			return p.musicAll[i].PublishedAt < p.musicAll[j].PublishedAt
		})
	})
	return p.musicErr
}

func (p *localMusicProvider) ensureDifficulties() error {
	p.diffOnce.Do(func() {
		items, err := loadJSON[masterdata.MusicDifficulty](p.store, "musicDifficulties.json")
		if err != nil {
			p.diffErr = err
			return
		}
		p.diffByMusic = make(map[int][]*masterdata.MusicDifficulty)
		for i := range items {
			p.diffByMusic[items[i].MusicID] = append(p.diffByMusic[items[i].MusicID], &items[i])
		}
	})
	return p.diffErr
}

func (p *localMusicProvider) ensureVocals() error {
	p.vocalOnce.Do(func() {
		items, err := loadJSON[localMusicVocalJSON](p.store, "musicVocals.json")
		if err != nil {
			p.vocalErr = err
			return
		}
		p.vocalByMusic = make(map[int][]*masterdata.MusicVocal)
		for i := range items {
			m := items[i].toModel()
			p.vocalByMusic[m.MusicID] = append(p.vocalByMusic[m.MusicID], m)
		}
	})
	return p.vocalErr
}

func (p *localMusicProvider) ensureTags() error {
	p.tagOnce.Do(func() {
		items, err := loadJSON[localMusicTagJSON](p.store, "musicTags.json")
		if err != nil {
			p.tagErr = err
			return
		}
		p.tagByMusic = make(map[int][]string)
		for _, item := range items {
			tag := strings.TrimSpace(item.MusicTag)
			if tag != "" {
				p.tagByMusic[item.MusicID] = append(p.tagByMusic[item.MusicID], tag)
			}
		}
	})
	return p.tagErr
}

func (p *localMusicProvider) ensureOutsideCharacters() error {
	p.outsideOnce.Do(func() {
		items, err := loadJSON[localOutsideCharacterJSON](p.store, "outsideCharacters.json")
		if err != nil {
			p.outsideErr = err
			return
		}
		p.outsideByID = make(map[int]string, len(items))
		for _, item := range items {
			p.outsideByID[item.ID] = strings.TrimSpace(item.Name)
		}
	})
	return p.outsideErr
}

func (p *localMusicProvider) ensureEventMusics() error {
	p.eventMusicOnce.Do(func() {
		items, err := loadJSON[localEventMusicJSON](p.store, "eventMusics.json")
		if err != nil {
			p.eventMusicErr = err
			return
		}
		p.musicIDByEvent = make(map[int]int)
		p.eventIDsByMusic = make(map[int][]int)
		sort.Slice(items, func(i, j int) bool {
			return items[i].Seq < items[j].Seq
		})
		for _, item := range items {
			if _, ok := p.musicIDByEvent[item.EventID]; !ok {
				p.musicIDByEvent[item.EventID] = item.MusicID
			}
			p.eventIDsByMusic[item.MusicID] = append(p.eventIDsByMusic[item.MusicID], item.EventID)
		}
	})
	return p.eventMusicErr
}

func (p *localMusicProvider) ensureLimitedTimeMusics() error {
	p.limitedOnce.Do(func() {
		items, err := loadJSON[masterdata.LimitedTimeMusic](p.store, "limitedTimeMusics.json")
		if err != nil {
			p.limitedErr = err
			return
		}
		p.limitedByMusic = make(map[int][]*masterdata.LimitedTimeMusic)
		for i := range items {
			p.limitedByMusic[items[i].MusicID] = append(p.limitedByMusic[items[i].MusicID], &items[i])
		}
	})
	return p.limitedErr
}

func (p *localMusicProvider) Search(query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music not found: empty query")
	}
	if id, err := strconv.Atoi(query); err == nil {
		return p.GetByID(id)
	}
	all := p.GetAll()
	lowerQuery := strings.ToLower(query)
	for _, m := range all {
		if strings.Contains(strings.ToLower(m.Title), lowerQuery) {
			return common.CloneMusic(m), nil
		}
		if strings.Contains(strings.ToLower(m.Pronunciation), lowerQuery) {
			return common.CloneMusic(m), nil
		}
	}
	return nil, fmt.Errorf("music not found: %s", query)
}

func (p *localMusicProvider) GetByID(id int) (*masterdata.Music, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", id)
	}
	if err := p.ensureMusics(); err != nil {
		return nil, err
	}
	m, ok := p.musicByID[id]
	if !ok {
		return nil, fmt.Errorf("music %d not found", id)
	}
	return common.CloneMusic(m), nil
}

func (p *localMusicProvider) GetByEventID(eventID int) (*masterdata.Music, error) {
	if err := p.ensureEventMusics(); err != nil {
		return nil, err
	}
	musicID, ok := p.musicIDByEvent[eventID]
	if !ok {
		return nil, fmt.Errorf("no music found for event %d", eventID)
	}
	return p.GetByID(musicID)
}

func (p *localMusicProvider) GetAll() []*masterdata.Music {
	if err := p.ensureMusics(); err != nil {
		return nil
	}
	return common.CloneMusicList(p.musicAll)
}

func (p *localMusicProvider) GetLocalizedTitles(musicID int) ([]string, error) {
	if musicID <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", musicID)
	}
	if err := p.ensureMusics(); err != nil {
		return nil, err
	}
	m, ok := p.musicByID[musicID]
	if !ok {
		return nil, fmt.Errorf("music %d not found", musicID)
	}
	unique := make(map[string]struct{}, 2)
	titles := make([]string, 0, 2)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if _, ok := unique[key]; ok {
			return
		}
		unique[key] = struct{}{}
		titles = append(titles, s)
	}
	add(m.Title)
	add(m.Pronunciation)
	return titles, nil
}

func (p *localMusicProvider) GetDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	if err := p.ensureDifficulties(); err != nil {
		return nil, err
	}
	diffs, ok := p.diffByMusic[musicID]
	if !ok || len(diffs) == 0 {
		return nil, fmt.Errorf("no difficulties found for music %d", musicID)
	}
	result := make([]*masterdata.MusicDifficulty, 0, len(diffs))
	for _, d := range diffs {
		c := *d
		result = append(result, &c)
	}
	return result, nil
}

func (p *localMusicProvider) GetVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	if err := p.ensureVocals(); err != nil {
		return nil, err
	}
	vocals, ok := p.vocalByMusic[musicID]
	if !ok || len(vocals) == 0 {
		return nil, fmt.Errorf("no vocals found for music %d", musicID)
	}
	result := make([]*masterdata.MusicVocal, 0, len(vocals))
	for _, v := range vocals {
		c := *v
		if v.Characters != nil {
			c.Characters = append([]masterdata.MusicVocalCharacter(nil), v.Characters...)
		}
		result = append(result, &c)
	}
	return result, nil
}

func (p *localMusicProvider) GetTags(musicID int) ([]string, error) {
	if err := p.ensureTags(); err != nil {
		return nil, err
	}
	tags := p.tagByMusic[musicID]
	return append([]string(nil), tags...), nil
}

func (p *localMusicProvider) GetOutsideCharacterByID(id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid outside character id: %d", id)
	}
	if err := p.ensureOutsideCharacters(); err != nil {
		return "", err
	}
	name, ok := p.outsideByID[id]
	if !ok {
		return "", fmt.Errorf("outside character %d not found", id)
	}
	return name, nil
}

func (p *localMusicProvider) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	if err := p.ensureEventMusics(); err != nil {
		return nil, err
	}
	eventIDs, ok := p.eventIDsByMusic[musicID]
	if !ok || len(eventIDs) == 0 {
		return nil, fmt.Errorf("no events found for music %d", musicID)
	}
	if p.events == nil {
		return nil, fmt.Errorf("event provider not configured")
	}
	var earliest *masterdata.Event
	for _, eid := range eventIDs {
		ev, err := p.events.GetByID(eid)
		if err != nil {
			continue
		}
		if earliest == nil || ev.StartAt < earliest.StartAt {
			earliest = ev
		}
	}
	if earliest == nil {
		return nil, fmt.Errorf("no events found for music %d", musicID)
	}
	return earliest, nil
}

func (p *localMusicProvider) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	if err := p.ensureLimitedTimeMusics(); err != nil {
		return nil
	}
	items, ok := p.limitedByMusic[musicID]
	if !ok {
		return nil
	}
	result := make([]*masterdata.LimitedTimeMusic, 0, len(items))
	for _, item := range items {
		c := *item
		result = append(result, &c)
	}
	return result
}
