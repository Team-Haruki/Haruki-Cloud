package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	dbevent "haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/eventmusic"
	"haruki-cloud/database/sekai/limitedtimemusic"
	"haruki-cloud/database/sekai/music"
	"haruki-cloud/database/sekai/musicdifficultie"
	"haruki-cloud/database/sekai/musictag"
	"haruki-cloud/database/sekai/musicvocal"
	"haruki-cloud/database/sekai/outsidecharacter"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbMusicProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	events EventProvider

	mu            sync.RWMutex
	musicByID     map[int]*masterdata.Music
	musicList     []*masterdata.Music
	outsideByID   map[int]string
	localizedByID map[int][]string
}

func (p *dbMusicProvider) init() {
	if p.musicByID == nil {
		p.musicByID = make(map[int]*masterdata.Music)
	}
	if p.outsideByID == nil {
		p.outsideByID = make(map[int]string)
	}
	if p.localizedByID == nil {
		p.localizedByID = make(map[int][]string)
	}
}

func (p *dbMusicProvider) Search(query string) (*masterdata.Music, error) {
	return p.search(nil, query)
}

func (p *dbMusicProvider) search(ctx context.Context, query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music not found: empty query")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if id, err := strconv.Atoi(query); err == nil {
		return p.getByID(ctx, id)
	}

	// Fall back to title match across all musics
	all := p.getAll(ctx)
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

func (p *dbMusicProvider) GetByID(id int) (*masterdata.Music, error) {
	return p.getByID(nil, id)
}

func (p *dbMusicProvider) getByID(ctx context.Context, id int) (*masterdata.Music, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", id)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

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

func (p *dbMusicProvider) GetByEventID(eventID int) (*masterdata.Music, error) {
	return p.getByEventID(nil, eventID)
}

func (p *dbMusicProvider) getByEventID(ctx context.Context, eventID int) (*masterdata.Music, error) {
	if ctx == nil {
		ctx = context.Background()
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
	return p.getByID(ctx, int(links[0].MusicID))
}

func (p *dbMusicProvider) GetAll() []*masterdata.Music {
	return p.getAll(nil)
}

func (p *dbMusicProvider) getAll(ctx context.Context) []*masterdata.Music {
	p.init()
	if ctx == nil {
		ctx = context.Background()
	}

	p.mu.RLock()
	if len(p.musicList) > 0 {
		cached := common.CloneMusicList(p.musicList)
		p.mu.RUnlock()
		return cached
	}
	p.mu.RUnlock()

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

	p.mu.Lock()
	p.musicList = list
	for id, item := range byID {
		p.musicByID[id] = item
	}
	p.mu.Unlock()
	return common.CloneMusicList(list)
}

func (p *dbMusicProvider) GetLocalizedTitles(musicID int) ([]string, error) {
	return p.getLocalizedTitles(nil, musicID)
}

func (p *dbMusicProvider) getLocalizedTitles(ctx context.Context, musicID int) ([]string, error) {
	if musicID <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", musicID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	p.mu.RLock()
	if titles, ok := p.localizedByID[musicID]; ok {
		p.mu.RUnlock()
		return append([]string(nil), titles...), nil
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
	p.localizedByID[musicID] = append([]string(nil), titles...)
	p.mu.Unlock()
	return titles, nil
}

func (p *dbMusicProvider) GetDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	return p.getDifficulties(nil, musicID)
}

func (p *dbMusicProvider) getDifficulties(ctx context.Context, musicID int) ([]*masterdata.MusicDifficulty, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := p.client.Musicdifficultie.Query().
		Where(
			musicdifficultie.ServerRegionEQ(p.region.String()),
			musicdifficultie.MusicIDEQ(int64(musicID)),
		).
		Order(musicdifficultie.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query difficulties for music %d: %w", musicID, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no difficulties found for music %d", musicID)
	}

	result := make([]*masterdata.MusicDifficulty, 0, len(items))
	for _, item := range items {
		result = append(result, &masterdata.MusicDifficulty{
			ID:              int(item.GameID),
			MusicID:         int(item.MusicID),
			MusicDifficulty: item.MusicDifficulty,
			PlayLevel:       int(item.PlayLevel),
			TotalNoteCount:  int(item.TotalNoteCount),
		})
	}
	return result, nil
}

func (p *dbMusicProvider) GetVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	return p.getVocals(nil, musicID)
}

func (p *dbMusicProvider) getVocals(ctx context.Context, musicID int) ([]*masterdata.MusicVocal, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := p.client.Musicvocal.Query().
		Where(
			musicvocal.ServerRegionEQ(p.region.String()),
			musicvocal.MusicIDEQ(int64(musicID)),
		).
		Order(musicvocal.BySeq()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query vocals for music %d: %w", musicID, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no vocals found for music %d", musicID)
	}

	result := make([]*masterdata.MusicVocal, 0, len(items))
	for _, item := range items {
		result = append(result, &masterdata.MusicVocal{
			ID:              int(item.GameID),
			MusicID:         int(item.MusicID),
			MusicVocalType:  item.MusicVocalType,
			Caption:         item.Caption,
			Characters:      parseMusicVocalCharactersFromRaw(item.Characters, int(item.GameID), int(item.MusicID)),
			AssetBundleName: item.AssetbundleName,
		})
	}
	return result, nil
}

func (p *dbMusicProvider) GetTags(musicID int) ([]string, error) {
	return p.getTags(nil, musicID)
}

func (p *dbMusicProvider) getTags(ctx context.Context, musicID int) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := p.client.Musictag.Query().
		Where(
			musictag.ServerRegionEQ(p.region.String()),
			musictag.MusicIDEQ(int64(musicID)),
		).
		Order(musictag.BySeq()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query tags for music %d: %w", musicID, err)
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		tag := strings.TrimSpace(item.MusicTag)
		if tag != "" {
			result = append(result, tag)
		}
	}
	return result, nil
}

func (p *dbMusicProvider) GetOutsideCharacterByID(id int) (string, error) {
	return p.getOutsideCharacterByID(nil, id)
}

func (p *dbMusicProvider) getOutsideCharacterByID(ctx context.Context, id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid outside character id: %d", id)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	p.mu.RLock()
	if cached, ok := p.outsideByID[id]; ok {
		p.mu.RUnlock()
		return cached, nil
	}
	p.mu.RUnlock()

	entity, err := p.client.Outsidecharacter.Query().
		Where(outsidecharacter.ServerRegionEQ(p.region.String()), outsidecharacter.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return "", fmt.Errorf("query outside character %d: %w", id, err)
	}

	name := strings.TrimSpace(entity.Name)
	p.mu.Lock()
	p.outsideByID[id] = name
	p.mu.Unlock()
	return name, nil
}

func (p *dbMusicProvider) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	return p.getPrimaryEventByMusicID(nil, musicID)
}

func (p *dbMusicProvider) getPrimaryEventByMusicID(ctx context.Context, musicID int) (*masterdata.Event, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	links, err := p.client.Eventmusic.Query().
		Where(
			eventmusic.ServerRegionEQ(p.region.String()),
			eventmusic.MusicIDEQ(int64(musicID)),
		).
		Order(eventmusic.BySeq()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query event music for music %d: %w", musicID, err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no events found for music %d", musicID)
	}

	eventIDs := make([]int64, 0, len(links))
	seen := make(map[int64]struct{}, len(links))
	for _, link := range links {
		if _, ok := seen[link.EventID]; ok {
			continue
		}
		seen[link.EventID] = struct{}{}
		eventIDs = append(eventIDs, link.EventID)
	}

	items, err := p.client.Event.Query().
		Where(dbevent.ServerRegionEQ(p.region.String()), dbevent.GameIDIn(eventIDs...)).
		Order(dbevent.ByStartAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query events for music %d: %w", musicID, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no events found for music %d", musicID)
	}
	return common.CloneEvent(common.ConvertEventEntity(items[0])), nil
}

func (p *dbMusicProvider) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	return p.getLimitedTimeMusics(nil, musicID)
}

func (p *dbMusicProvider) getLimitedTimeMusics(ctx context.Context, musicID int) []*masterdata.LimitedTimeMusic {
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := p.client.Limitedtimemusic.Query().
		Where(
			limitedtimemusic.ServerRegionEQ(p.region.String()),
			limitedtimemusic.MusicIDEQ(int64(musicID)),
		).
		Order(limitedtimemusic.ByStartAt()).
		All(ctx)
	if err != nil || len(items) == 0 {
		return nil
	}

	result := make([]*masterdata.LimitedTimeMusic, 0, len(items))
	for _, item := range items {
		result = append(result, &masterdata.LimitedTimeMusic{
			ID:      int(item.GameID),
			MusicID: int(item.MusicID),
			StartAt: item.StartAt,
			EndAt:   item.EndAt,
		})
	}
	return result
}

func parseMusicVocalCharactersFromRaw(raw json.RawMessage, vocalID int, musicID int) []masterdata.MusicVocalCharacter {
	if len(raw) == 0 {
		return nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}

	result := make([]masterdata.MusicVocalCharacter, 0, len(items))
	for index, entry := range items {
		characterType, _ := entry["characterType"].(string)
		characterID, ok := interfaceToInt(entry["characterId"])
		if !ok {
			continue
		}
		result = append(result, masterdata.MusicVocalCharacter{
			ID:            index + 1,
			MusicID:       musicID,
			MusicVocalID:  vocalID,
			CharacterType: characterType,
			CharacterID:   characterID,
		})
	}
	return result
}

func interfaceToInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}
