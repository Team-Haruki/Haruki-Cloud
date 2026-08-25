package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	json "haruki-cloud/internal/jsonutil"

	dbevent "haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/eventmusic"
	"haruki-cloud/database/sekai/limitedtimemusic"
	"haruki-cloud/database/sekai/musicdifficultie"
	"haruki-cloud/database/sekai/musictag"
	"haruki-cloud/database/sekai/musicvocal"
	"haruki-cloud/database/sekai/outsidecharacter"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbMusicProvider) GetDifficulties(ctx context.Context, musicID int) ([]*masterdata.MusicDifficulty, error) {
	p.init()
	if p.local != nil {
		if result, err := p.local.GetDifficulties(ctx, musicID); err == nil && len(result) > 0 {
			p.mu.Lock()
			p.difficultiesByID[musicID] = common.CloneMusicDifficulties(result)
			p.mu.Unlock()
			return result, nil
		}
	}

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
	if p.local != nil {
		if result, err := p.local.GetVocals(ctx, musicID); err == nil && len(result) > 0 {
			return result, nil
		}
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

func (p *dbMusicProvider) GetTags(ctx context.Context, musicID int) ([]string, error) {
	if p.local != nil {
		if result, err := p.local.GetTags(ctx, musicID); err == nil {
			return result, nil
		}
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

func (p *dbMusicProvider) GetOutsideCharacterByID(ctx context.Context, id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid outside character id: %d", id)
	}
	p.init()
	if p.local != nil {
		if name, err := p.local.GetOutsideCharacterByID(ctx, id); err == nil && strings.TrimSpace(name) != "" {
			p.mu.Lock()
			p.outsideByID[id] = name
			p.mu.Unlock()
			return name, nil
		}
	}

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
	if p.local != nil {
		if eventInfo, err := p.local.GetPrimaryEventByMusicID(ctx, musicID); err == nil && eventInfo != nil {
			return eventInfo, nil
		}
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

func (p *dbMusicProvider) GetLimitedTimeMusics(ctx context.Context, musicID int) []*masterdata.LimitedTimeMusic {
	if p.local != nil {
		if result := p.local.GetLimitedTimeMusics(ctx, musicID); len(result) > 0 {
			return result
		}
	}
	if err := p.ensureLimitedTimeMusicsLoaded(ctx); err != nil {
		return nil
	}
	p.limitedMu.RLock()
	items := p.limitedByMusic[musicID]
	p.limitedMu.RUnlock()
	return cloneLimitedTimeMusics(items)
}

func (p *dbMusicProvider) ensureLimitedTimeMusicsLoaded(ctx context.Context) error {
	p.limitedMu.RLock()
	loaded := dbBulkIndexFresh(p.limitedLoaded, p.limitedLoadedAt)
	p.limitedMu.RUnlock()
	if loaded {
		return nil
	}

	callerToken := new(dbBulkIndexFlightToken)
	result := p.limitedLoads.DoChan("all", func() (any, error) {
		completed := runDBBulkIndexFlight(callerToken, func(loadCtx context.Context) error {
			finishIndex := commandtrace.MeasureOperation(loadCtx, "musics.limited_time_index")
			defer finishIndex()
			p.limitedMu.RLock()
			alreadyLoaded := dbBulkIndexFresh(p.limitedLoaded, p.limitedLoadedAt)
			p.limitedMu.RUnlock()
			if alreadyLoaded {
				return nil
			}
			items, err := p.client.Limitedtimemusic.Query().
				Where(limitedtimemusic.ServerRegionEQ(p.region.String())).
				Order(limitedtimemusic.ByMusicID(), limitedtimemusic.ByStartAt()).
				All(loadCtx)
			if err != nil {
				return fmt.Errorf("query limited time musics for region %s: %w", p.region, err)
			}

			byMusic := make(map[int][]*masterdata.LimitedTimeMusic)
			for _, item := range items {
				musicID := int(item.MusicID)
				byMusic[musicID] = append(byMusic[musicID], &masterdata.LimitedTimeMusic{
					ID:      int(item.GameID),
					MusicID: musicID,
					StartAt: item.StartAt,
					EndAt:   item.EndAt,
				})
			}

			p.limitedMu.Lock()
			p.limitedByMusic = byMusic
			p.limitedLoaded = true
			p.limitedLoadedAt = time.Now()
			p.limitedMu.Unlock()
			return nil
		})
		return completed, nil
	})

	return waitDBBulkIndexFlight(ctx, result, callerToken, "musics.limited_time_index_wait", "musics.limited_time_index_shared")
}

func cloneLimitedTimeMusics(items []*masterdata.LimitedTimeMusic) []*masterdata.LimitedTimeMusic {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]*masterdata.LimitedTimeMusic, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		copied := *item
		cloned = append(cloned, &copied)
	}
	return cloned
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
