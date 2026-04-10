package provider

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localMusicProvider
// ===========================================================================

type musicIndex struct {
	all  []*masterdata.Music
	byID map[int]*masterdata.Music
}

type eventMusicIndex struct {
	musicIDByEvent  map[int]int
	eventIDsByMusic map[int][]int
}

type localMusicProvider struct {
	store  *localStore
	events EventProvider

	musics      lazyValue[musicIndex]
	diffs       lazyValue[map[int][]*masterdata.MusicDifficulty]
	vocals      lazyValue[map[int][]*masterdata.MusicVocal]
	tags        lazyValue[map[int][]string]
	outside     lazyValue[map[int]string]
	eventMusics lazyValue[eventMusicIndex]
	limited     lazyValue[map[int][]*masterdata.LimitedTimeMusic]
}

func (p *localMusicProvider) ensureMusics() error {
	return p.musics.init(func() (musicIndex, error) {
		items, err := loadJSON[localMusicJSON](p.store, "musics.json")
		if err != nil {
			return musicIndex{}, err
		}
		idx := musicIndex{
			byID: make(map[int]*masterdata.Music, len(items)),
			all:  make([]*masterdata.Music, 0, len(items)),
		}
		for i := range items {
			m := items[i].toModel()
			idx.byID[m.ID] = m
			idx.all = append(idx.all, m)
		}
		sort.Slice(idx.all, func(i, j int) bool {
			if idx.all[i].PublishedAt == idx.all[j].PublishedAt {
				return idx.all[i].ID < idx.all[j].ID
			}
			return idx.all[i].PublishedAt < idx.all[j].PublishedAt
		})
		return idx, nil
	})
}

func (p *localMusicProvider) ensureDifficulties() error {
	return p.diffs.init(func() (map[int][]*masterdata.MusicDifficulty, error) {
		items, err := loadJSON[masterdata.MusicDifficulty](p.store, "musicDifficulties.json")
		if err != nil {
			return nil, err
		}
		byMusic := make(map[int][]*masterdata.MusicDifficulty)
		for i := range items {
			byMusic[items[i].MusicID] = append(byMusic[items[i].MusicID], &items[i])
		}
		return byMusic, nil
	})
}

func (p *localMusicProvider) ensureVocals() error {
	return p.vocals.init(func() (map[int][]*masterdata.MusicVocal, error) {
		items, err := loadJSON[localMusicVocalJSON](p.store, "musicVocals.json")
		if err != nil {
			return nil, err
		}
		byMusic := make(map[int][]*masterdata.MusicVocal)
		for i := range items {
			m := items[i].toModel()
			byMusic[m.MusicID] = append(byMusic[m.MusicID], m)
		}
		return byMusic, nil
	})
}

func (p *localMusicProvider) ensureTags() error {
	return p.tags.init(func() (map[int][]string, error) {
		items, err := loadJSON[localMusicTagJSON](p.store, "musicTags.json")
		if err != nil {
			return nil, err
		}
		byMusic := make(map[int][]string)
		for _, item := range items {
			tag := strings.TrimSpace(item.MusicTag)
			if tag != "" {
				byMusic[item.MusicID] = append(byMusic[item.MusicID], tag)
			}
		}
		return byMusic, nil
	})
}

func (p *localMusicProvider) ensureOutsideCharacters() error {
	return p.outside.init(func() (map[int]string, error) {
		items, err := loadJSON[localOutsideCharacterJSON](p.store, "outsideCharacters.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]string, len(items))
		for _, item := range items {
			byID[item.ID] = strings.TrimSpace(item.Name)
		}
		return byID, nil
	})
}

func (p *localMusicProvider) ensureEventMusics() error {
	return p.eventMusics.init(func() (eventMusicIndex, error) {
		items, err := loadJSON[localEventMusicJSON](p.store, "eventMusics.json")
		if err != nil {
			return eventMusicIndex{}, err
		}
		idx := eventMusicIndex{
			musicIDByEvent:  make(map[int]int),
			eventIDsByMusic: make(map[int][]int),
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].Seq < items[j].Seq
		})
		for _, item := range items {
			if _, ok := idx.musicIDByEvent[item.EventID]; !ok {
				idx.musicIDByEvent[item.EventID] = item.MusicID
			}
			idx.eventIDsByMusic[item.MusicID] = append(idx.eventIDsByMusic[item.MusicID], item.EventID)
		}
		return idx, nil
	})
}

func (p *localMusicProvider) ensureLimitedTimeMusics() error {
	return p.limited.init(func() (map[int][]*masterdata.LimitedTimeMusic, error) {
		items, err := loadJSON[masterdata.LimitedTimeMusic](p.store, "limitedTimeMusics.json")
		if err != nil {
			return nil, err
		}
		byMusic := make(map[int][]*masterdata.LimitedTimeMusic)
		for i := range items {
			byMusic[items[i].MusicID] = append(byMusic[items[i].MusicID], &items[i])
		}
		return byMusic, nil
	})
}

func (p *localMusicProvider) Search(ctx context.Context, query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music not found: empty query")
	}
	if id, err := strconv.Atoi(query); err == nil {
		return p.GetByID(ctx, id)
	}
	all := p.GetAll(ctx)
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

func (p *localMusicProvider) GetByID(_ context.Context, id int) (*masterdata.Music, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", id)
	}
	if err := p.ensureMusics(); err != nil {
		return nil, err
	}
	m, ok := p.musics.v().byID[id]
	if !ok {
		return nil, fmt.Errorf("music %d not found", id)
	}
	return common.CloneMusic(m), nil
}

func (p *localMusicProvider) GetByEventID(ctx context.Context, eventID int) (*masterdata.Music, error) {
	if err := p.ensureEventMusics(); err != nil {
		return nil, err
	}
	musicID, ok := p.eventMusics.v().musicIDByEvent[eventID]
	if !ok {
		return nil, fmt.Errorf("no music found for event %d", eventID)
	}
	return p.GetByID(ctx, musicID)
}

func (p *localMusicProvider) GetAll(_ context.Context) []*masterdata.Music {
	if err := p.ensureMusics(); err != nil {
		return nil
	}
	return common.CloneMusicList(p.musics.v().all)
}

func (p *localMusicProvider) GetLocalizedTitles(_ context.Context, musicID int) ([]string, error) {
	if musicID <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", musicID)
	}
	if err := p.ensureMusics(); err != nil {
		return nil, err
	}
	m, ok := p.musics.v().byID[musicID]
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

func (p *localMusicProvider) GetDifficulties(_ context.Context, musicID int) ([]*masterdata.MusicDifficulty, error) {
	if err := p.ensureDifficulties(); err != nil {
		return nil, err
	}
	diffs, ok := p.diffs.v()[musicID]
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

func (p *localMusicProvider) GetVocals(_ context.Context, musicID int) ([]*masterdata.MusicVocal, error) {
	if err := p.ensureVocals(); err != nil {
		return nil, err
	}
	vocals, ok := p.vocals.v()[musicID]
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

func (p *localMusicProvider) GetTags(_ context.Context, musicID int) ([]string, error) {
	if err := p.ensureTags(); err != nil {
		return nil, err
	}
	tags := p.tags.v()[musicID]
	return append([]string(nil), tags...), nil
}

func (p *localMusicProvider) GetOutsideCharacterByID(_ context.Context, id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid outside character id: %d", id)
	}
	if err := p.ensureOutsideCharacters(); err != nil {
		return "", err
	}
	name, ok := p.outside.v()[id]
	if !ok {
		return "", fmt.Errorf("outside character %d not found", id)
	}
	return name, nil
}

func (p *localMusicProvider) GetPrimaryEventByMusicID(_ context.Context, musicID int) (*masterdata.Event, error) {
	if err := p.ensureEventMusics(); err != nil {
		return nil, err
	}
	eventIDs, ok := p.eventMusics.v().eventIDsByMusic[musicID]
	if !ok || len(eventIDs) == 0 {
		return nil, fmt.Errorf("no events found for music %d", musicID)
	}
	if p.events == nil {
		return nil, fmt.Errorf("event provider not configured")
	}
	var earliest *masterdata.Event
	for _, eid := range eventIDs {
		ev, err := p.events.GetByID(context.Background(), eid)
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

func (p *localMusicProvider) GetLimitedTimeMusics(_ context.Context, musicID int) []*masterdata.LimitedTimeMusic {
	if err := p.ensureLimitedTimeMusics(); err != nil {
		return nil
	}
	items, ok := p.limited.v()[musicID]
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
