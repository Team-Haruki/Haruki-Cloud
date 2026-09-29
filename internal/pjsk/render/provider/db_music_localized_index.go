package provider

import (
	"context"
	"strings"

	"haruki-cloud/database/sekai/music"
)

func (p *dbMusicProvider) localizedTitleIndex(ctx context.Context) (map[int][]string, error) {
	return p.localizedTitles.get(ctx, "musics.localized_title_index", func(loadCtx context.Context) (map[int][]string, error) {
		var rows []struct {
			GameID        int64  `json:"game_id"`
			Title         string `json:"title"`
			Pronunciation string `json:"pronunciation"`
		}
		// Match the existing (game_id, server_region) unique-index traversal
		// when selecting the first spelling, without fetching full entities.
		err := p.client.Music.Query().Order(music.ByGameID(), music.ByServerRegion()).Select(music.FieldGameID, music.FieldTitle, music.FieldPronunciation).Scan(loadCtx, &rows)
		if err != nil {
			return nil, err
		}
		index := make(map[int][]string)
		seen := make(map[int]map[string]struct{})
		for _, row := range rows {
			id := int(row.GameID)
			if seen[id] == nil {
				seen[id] = make(map[string]struct{})
			}
			for _, raw := range []string{row.Title, row.Pronunciation} {
				title := strings.TrimSpace(raw)
				if title == "" {
					continue
				}
				key := strings.ToLower(title)
				if _, exists := seen[id][key]; exists {
					continue
				}
				seen[id][key] = struct{}{}
				index[id] = append(index[id], title)
			}
		}
		return index, nil
	})
}
