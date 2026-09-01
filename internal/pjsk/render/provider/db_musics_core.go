package provider

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/database/sekai/eventmusic"
	"haruki-cloud/database/sekai/music"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbMusicProvider) Search(ctx context.Context, query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music not found: empty query")
	}
	if p.local != nil {
		if musicInfo, err := p.local.Search(ctx, query); err == nil && musicInfo != nil {
			return musicInfo, nil
		}
	}

	if id, err := strconv.Atoi(query); err == nil {
		return p.GetByID(ctx, id)
	}

	// Fall back to title match across all musics.
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

func (p *dbMusicProvider) GetByID(ctx context.Context, id int) (*masterdata.Music, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", id)
	}
	p.init()
	if p.local != nil {
		if musicInfo, err := p.local.GetByID(ctx, id); err == nil && musicInfo != nil {
			p.mu.Lock()
			p.musicByID[id] = common.CloneMusic(musicInfo)
			p.mu.Unlock()
			return musicInfo, nil
		}
	}

	p.mu.RLock()
	if cached, ok := p.musicByID[id]; ok {
		p.mu.RUnlock()
		return common.CloneMusic(cached), nil
	}
	p.mu.RUnlock()

	entity, err := p.client.Music.Query().
		Where(music.ServerRegionEQ(p.region.String()), music.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query music %d: %w", id, err)
	}

	model := common.ConvertMusicEntity(entity)
	p.mu.Lock()
	p.musicByID[model.ID] = model
	p.mu.Unlock()
	return common.CloneMusic(model), nil
}

func (p *dbMusicProvider) GetByEventID(ctx context.Context, eventID int) (*masterdata.Music, error) {
	if p.local != nil {
		if musicInfo, err := p.local.GetByEventID(ctx, eventID); err == nil && musicInfo != nil {
			return musicInfo, nil
		}
	}
	links, err := p.client.Eventmusic.Query().
		Where(eventmusic.ServerRegionEQ(p.region.String()), eventmusic.EventIDEQ(int64(eventID))).
		Order(eventmusic.BySeq()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query event music for event %d: %w", eventID, err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no music found for event %d", eventID)
	}
	return p.GetByID(ctx, int(links[0].MusicID))
}

func (p *dbMusicProvider) GetAll(ctx context.Context) []*masterdata.Music {
	p.init()
	if localItems := p.localMusicList(ctx); len(localItems) > 0 {
		p.storeMusicList(localItems, true)
		return common.CloneMusicList(localItems)
	}
	if cached := p.cachedMusicList(); len(cached) > 0 {
		return cached
	}
	entities, err := p.client.Music.Query().
		Where(music.ServerRegionEQ(p.region.String())).
		Order(music.ByPublishedAt(), music.ByGameID()).
		All(ctx)
	if err != nil {
		return nil
	}

	list := make([]*masterdata.Music, 0, len(entities))
	byID := make(map[int]*masterdata.Music, len(entities))
	for _, entity := range entities {
		model := common.ConvertMusicEntity(entity)
		list = append(list, model)
		byID[model.ID] = model
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].PublishedAt == list[j].PublishedAt {
			return list[i].ID < list[j].ID
		}
		return list[i].PublishedAt < list[j].PublishedAt
	})
	p.storeMusicListWithIndex(list, byID, false)
	return common.CloneMusicList(list)
}

func (p *dbMusicProvider) localMusicList(ctx context.Context) []*masterdata.Music {
	if p.local == nil {
		return nil
	}
	return common.CloneMusicList(p.local.GetAll(ctx))
}

func (p *dbMusicProvider) cachedMusicList() []*masterdata.Music {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return common.CloneMusicList(p.musicList)
}

func (p *dbMusicProvider) storeMusicList(list []*masterdata.Music, replaceIndex bool) {
	byID := make(map[int]*masterdata.Music, len(list))
	for _, item := range list {
		if item != nil {
			byID[item.ID] = item
		}
	}
	p.storeMusicListWithIndex(list, byID, replaceIndex)
}

func (p *dbMusicProvider) storeMusicListWithIndex(list []*masterdata.Music, byID map[int]*masterdata.Music, replaceIndex bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.musicList = list
	if replaceIndex {
		p.musicByID = byID
		return
	}
	for id, item := range byID {
		p.musicByID[id] = item
	}
}

func (p *dbMusicProvider) GetLocalizedTitles(ctx context.Context, musicID int) ([]string, error) {
	if musicID <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", musicID)
	}
	p.init()
	if p.local != nil {
		if titles, err := p.local.GetLocalizedTitles(ctx, musicID); err == nil && len(titles) > 0 {
			p.mu.Lock()
			p.localizedByID[musicID] = slices.Clone(titles)
			p.mu.Unlock()
			return slices.Clone(titles), nil
		}
	}

	p.mu.RLock()
	if titles, ok := p.localizedByID[musicID]; ok {
		p.mu.RUnlock()
		return slices.Clone(titles), nil
	}
	p.mu.RUnlock()

	items, err := p.client.Music.Query().
		Where(music.GameIDEQ(int64(musicID))).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query localized titles for music %d: %w", musicID, err)
	}

	unique := make(map[string]struct{}, len(items)*2)
	titles := make([]string, 0, len(items)*2)
	appendTitle := func(raw string) {
		title := strings.TrimSpace(raw)
		if title == "" {
			return
		}
		key := strings.ToLower(title)
		if _, ok := unique[key]; ok {
			return
		}
		unique[key] = struct{}{}
		titles = append(titles, title)
	}
	for _, item := range items {
		appendTitle(item.Title)
		appendTitle(item.Pronunciation)
	}

	p.mu.Lock()
	p.localizedByID[musicID] = slices.Clone(titles)
	p.mu.Unlock()
	return titles, nil
}
