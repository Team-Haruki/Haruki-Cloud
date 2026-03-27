package music

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
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/database/sekai/limitedtimemusic"
	"haruki-cloud/database/sekai/music"
	"haruki-cloud/database/sekai/musicdifficultie"
	"haruki-cloud/database/sekai/musictag"
	"haruki-cloud/database/sekai/musicvocal"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type CloudSource struct {
	client      *sekaiDB.Client
	events      renderevent.DataSource
	region      renderregion.Value
	queryRegion string

	mu            sync.RWMutex
	musicByID     map[int]*masterdata.Music
	musicList     []*masterdata.Music
	characterByID map[int]*masterdata.Character
	localizedByID map[int][]string
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:        client,
		events:        renderevent.NewCloudSource(client, region),
		region:        region,
		queryRegion:   region.String(),
		musicByID:     make(map[int]*masterdata.Music),
		characterByID: make(map[int]*masterdata.Character),
		localizedByID: make(map[int][]string),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) SearchMusic(query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music not found: empty query")
	}

	if id, err := strconv.Atoi(query); err == nil {
		return c.GetMusicByID(id)
	}

	musics := c.GetMusics()
	if len(musics) == 0 {
		return nil, fmt.Errorf("music not found: %s", query)
	}

	queryLower := strings.ToLower(query)
	for _, musicInfo := range musics {
		if strings.EqualFold(strings.TrimSpace(musicInfo.Title), query) {
			return cloneMusic(musicInfo), nil
		}
	}

	var best *masterdata.Music
	for _, musicInfo := range musics {
		if strings.Contains(strings.ToLower(musicInfo.Title), queryLower) {
			if best == nil || len(musicInfo.Title) < len(best.Title) {
				best = musicInfo
			}
		}
	}
	if best != nil {
		return cloneMusic(best), nil
	}

	for _, musicInfo := range musics {
		titles, err := c.GetMusicLocalizedTitles(musicInfo.ID)
		if err != nil {
			continue
		}
		for _, title := range titles {
			if strings.EqualFold(strings.TrimSpace(title), query) {
				return cloneMusic(musicInfo), nil
			}
		}
	}
	for _, musicInfo := range musics {
		titles, err := c.GetMusicLocalizedTitles(musicInfo.ID)
		if err != nil {
			continue
		}
		for _, title := range titles {
			if strings.Contains(strings.ToLower(strings.TrimSpace(title)), queryLower) {
				return cloneMusic(musicInfo), nil
			}
		}
	}
	return nil, fmt.Errorf("music not found: %s", query)
}

func (c *CloudSource) GetMusicByID(id int) (*masterdata.Music, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", id)
	}

	c.mu.RLock()
	if cached, ok := c.musicByID[id]; ok {
		c.mu.RUnlock()
		return cloneMusic(cached), nil
	}
	c.mu.RUnlock()

	entity, err := c.client.Music.Query().
		Where(music.ServerRegionEQ(c.queryRegion), music.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, err
	}

	model := convertMusicEntity(entity)
	c.mu.Lock()
	c.musicByID[model.ID] = model
	c.mu.Unlock()
	return cloneMusic(model), nil
}

func (c *CloudSource) GetMusicByEventID(eventID int) (*masterdata.Music, error) {
	links, err := c.client.Eventmusic.Query().
		Where(eventmusic.ServerRegionEQ(c.queryRegion), eventmusic.EventIDEQ(int64(eventID))).
		Order(eventmusic.BySeq()).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no music found for event %d", eventID)
	}
	return c.GetMusicByID(int(links[0].MusicID))
}

func (c *CloudSource) GetMusics() []*masterdata.Music {
	c.mu.RLock()
	if len(c.musicList) > 0 {
		cached := cloneMusicList(c.musicList)
		c.mu.RUnlock()
		return cached
	}
	c.mu.RUnlock()

	entities, err := c.client.Music.Query().
		Where(music.ServerRegionEQ(c.queryRegion)).
		Order(music.ByPublishedAt(), music.ByGameID()).
		All(context.Background())
	if err != nil {
		return nil
	}

	list := make([]*masterdata.Music, 0, len(entities))
	byID := make(map[int]*masterdata.Music, len(entities))
	for _, entity := range entities {
		model := convertMusicEntity(entity)
		list = append(list, model)
		byID[model.ID] = model
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].PublishedAt == list[j].PublishedAt {
			return list[i].ID < list[j].ID
		}
		return list[i].PublishedAt < list[j].PublishedAt
	})

	c.mu.Lock()
	c.musicList = list
	for id, item := range byID {
		c.musicByID[id] = item
	}
	c.mu.Unlock()
	return cloneMusicList(list)
}

func (c *CloudSource) GetBanEvents(charID int) []*masterdata.Event {
	if c.events == nil {
		return nil
	}
	return c.events.GetBanEvents(charID)
}

func (c *CloudSource) GetMusicLocalizedTitles(musicID int) ([]string, error) {
	if musicID <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", musicID)
	}

	c.mu.RLock()
	if titles, ok := c.localizedByID[musicID]; ok {
		c.mu.RUnlock()
		return append([]string(nil), titles...), nil
	}
	c.mu.RUnlock()

	items, err := c.client.Music.Query().
		Where(music.GameIDEQ(int64(musicID))).
		All(context.Background())
	if err != nil {
		return nil, err
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

	c.mu.Lock()
	c.localizedByID[musicID] = append([]string(nil), titles...)
	c.mu.Unlock()
	return titles, nil
}

func (c *CloudSource) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	items, err := c.client.Musicdifficultie.Query().
		Where(
			musicdifficultie.ServerRegionEQ(c.queryRegion),
			musicdifficultie.MusicIDEQ(int64(musicID)),
		).
		Order(musicdifficultie.ByID()).
		All(context.Background())
	if err != nil {
		return nil, err
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

func (c *CloudSource) FindMusicDifficultiesByNoteCount(noteCount int) ([]*masterdata.MusicDifficulty, error) {
	if noteCount <= 0 {
		return nil, fmt.Errorf("invalid note count: %d", noteCount)
	}

	items, err := c.client.Musicdifficultie.Query().
		Where(
			musicdifficultie.ServerRegionEQ(c.queryRegion),
			musicdifficultie.TotalNoteCountEQ(int64(noteCount)),
		).
		Order(musicdifficultie.ByMusicID(), musicdifficultie.ByPlayLevel(), musicdifficultie.ByID()).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no difficulties found for note count %d", noteCount)
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

func (c *CloudSource) GetMusicVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	items, err := c.client.Musicvocal.Query().
		Where(
			musicvocal.ServerRegionEQ(c.queryRegion),
			musicvocal.MusicIDEQ(int64(musicID)),
		).
		Order(musicvocal.BySeq()).
		All(context.Background())
	if err != nil {
		return nil, err
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

func (c *CloudSource) GetMusicTags(musicID int) ([]string, error) {
	items, err := c.client.Musictag.Query().
		Where(
			musictag.ServerRegionEQ(c.queryRegion),
			musictag.MusicIDEQ(int64(musicID)),
		).
		Order(musictag.BySeq()).
		All(context.Background())
	if err != nil {
		return nil, err
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

func (c *CloudSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid character id: %d", id)
	}

	c.mu.RLock()
	if cached, ok := c.characterByID[id]; ok {
		c.mu.RUnlock()
		return cloneCharacter(cached), nil
	}
	c.mu.RUnlock()

	entity, err := c.client.Gamecharacter.Query().
		Where(gamecharacter.ServerRegionEQ(c.queryRegion), gamecharacter.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		entity, err = c.client.Gamecharacter.Query().
			Where(gamecharacter.ServerRegionEQ(c.queryRegion), gamecharacter.IDEQ(id)).
			Only(context.Background())
		if err != nil {
			return nil, err
		}
	}

	model := &masterdata.Character{
		ID:        int(entity.GameID),
		FirstName: entity.FirstName,
		GivenName: entity.GivenName,
		Unit:      entity.Unit,
	}
	c.mu.Lock()
	c.characterByID[id] = model
	c.mu.Unlock()
	return cloneCharacter(model), nil
}

func (c *CloudSource) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	links, err := c.client.Eventmusic.Query().
		Where(
			eventmusic.ServerRegionEQ(c.queryRegion),
			eventmusic.MusicIDEQ(int64(musicID)),
		).
		Order(eventmusic.BySeq()).
		All(context.Background())
	if err != nil {
		return nil, err
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

	items, err := c.client.Event.Query().
		Where(dbevent.ServerRegionEQ(c.queryRegion), dbevent.GameIDIn(eventIDs...)).
		Order(dbevent.ByStartAt()).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no events found for music %d", musicID)
	}
	return cloneEvent(convertEventEntity(items[0])), nil
}

func (c *CloudSource) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	items, err := c.client.Limitedtimemusic.Query().
		Where(
			limitedtimemusic.ServerRegionEQ(c.queryRegion),
			limitedtimemusic.MusicIDEQ(int64(musicID)),
		).
		Order(limitedtimemusic.ByStartAt()).
		All(context.Background())
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

func convertMusicEntity(entity *sekaiDB.Music) *masterdata.Music {
	return &masterdata.Music{
		ID:                 int(entity.GameID),
		Seq:                int(entity.Seq),
		ReleaseConditionID: int(entity.ReleaseConditionID),
		Categories:         toStringSliceFromRaw(entity.Categories),
		Title:              entity.Title,
		Pronunciation:      entity.Pronunciation,
		Lyricist:           entity.Lyricist,
		Composer:           entity.Composer,
		Arranger:           entity.Arranger,
		DancerCount:        int(entity.DancerCount),
		SelfDancerCount:    int(entity.SelfDancerPosition),
		AssetBundleName:    entity.AssetbundleName,
		PublishedAt:        entity.PublishedAt,
		DigitizedAt:        entity.ReleasedAt,
		IsFullLength:       entity.IsFullLength,
	}
}

func convertEventEntity(entity *sekaiDB.Event) *masterdata.Event {
	if entity == nil {
		return nil
	}
	return &masterdata.Event{
		ID:              int(entity.GameID),
		EventType:       entity.EventType,
		Name:            entity.Name,
		AssetBundleName: entity.AssetbundleName,
		StartAt:         entity.StartAt,
		AggregateAt:     entity.AggregateAt,
		ClosedAt:        entity.ClosedAt,
	}
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

func cloneMusic(item *masterdata.Music) *masterdata.Music {
	if item == nil {
		return nil
	}
	copy := *item
	if item.Categories != nil {
		copy.Categories = append([]string(nil), item.Categories...)
	}
	return &copy
}

func cloneMusicList(items []*masterdata.Music) []*masterdata.Music {
	result := make([]*masterdata.Music, 0, len(items))
	for _, item := range items {
		result = append(result, cloneMusic(item))
	}
	return result
}

func cloneCharacter(item *masterdata.Character) *masterdata.Character {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func cloneEvent(item *masterdata.Event) *masterdata.Event {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return string(raw)
	}
	return s
}

func parseMusicVocalCharactersFromRaw(raw json.RawMessage, vocalID int, musicID int) []masterdata.MusicVocalCharacter {
	if len(raw) == 0 {
		return nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	return parseMusicVocalCharacters(items, vocalID, musicID)
}

func parseMusicVocalCharacters(raw []map[string]interface{}, vocalID int, musicID int) []masterdata.MusicVocalCharacter {
	result := make([]masterdata.MusicVocalCharacter, 0, len(raw))
	for index, entry := range raw {
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

func toStringSliceFromRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			result = append(result, item)
		}
	}
	return result
}
