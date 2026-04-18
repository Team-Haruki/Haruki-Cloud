package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	dbevent "haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/eventmusic"
	"haruki-cloud/database/sekai/limitedtimemusic"
	"haruki-cloud/database/sekai/musicdifficultie"
	"haruki-cloud/database/sekai/musictag"
	"haruki-cloud/database/sekai/musicvocal"
	"haruki-cloud/database/sekai/outsidecharacter"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbMusicProvider) GetDifficulties(ctx context.Context, musicID int) ([]*masterdata.MusicDifficulty, error) {
	p.init()

	p.mu.RLock()
	if cached, ok := p.difficultiesByID[musicID]; ok {
		p.mu.RUnlock()
		return common.CloneMusicDifficulties(cached), nil
	}
	p.mu.RUnlock()

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

	p.mu.Lock()
	p.difficultiesByID[musicID] = result
	p.mu.Unlock()
	return common.CloneMusicDifficulties(result), nil
}

func (p *dbMusicProvider) GetVocals(ctx context.Context, musicID int) ([]*masterdata.MusicVocal, error) {
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

func (p *dbMusicProvider) GetTags(ctx context.Context, musicID int) ([]string, error) {
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

func (p *dbMusicProvider) GetOutsideCharacterByID(ctx context.Context, id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid outside character id: %d", id)
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

func (p *dbMusicProvider) GetPrimaryEventByMusicID(ctx context.Context, musicID int) (*masterdata.Event, error) {
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

func (p *dbMusicProvider) GetLimitedTimeMusics(ctx context.Context, musicID int) []*masterdata.LimitedTimeMusic {
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
	var items []map[string]any
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

func interfaceToInt(value any) (int, bool) {
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
