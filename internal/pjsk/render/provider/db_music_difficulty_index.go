package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/musicdifficultie"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbMusicProvider) PreloadDifficulties(ctx context.Context) error {
	if p.local != nil {
		return nil
	}
	p.difficultyMu.RLock()
	fresh := dbBulkIndexFresh(p.difficultyIndex != nil, p.difficultyLoadedAt)
	generation := p.difficultyGeneration
	p.difficultyMu.RUnlock()
	if fresh {
		return nil
	}
	caller := new(dbBulkIndexFlightToken)
	result := p.difficultyLoads.DoChan(strconv.FormatUint(generation, 10), func() (any, error) {
		completed := runDBBulkIndexFlight(caller, func(loadCtx context.Context) error {
			finish := commandtrace.MeasureOperation(loadCtx, "musics.difficulty_index")
			defer finish()
			p.difficultyMu.RLock()
			fresh := dbBulkIndexFresh(p.difficultyIndex != nil, p.difficultyLoadedAt)
			current := p.difficultyGeneration
			p.difficultyMu.RUnlock()
			if fresh || current != generation {
				return nil
			}
			items, err := p.client.Musicdifficultie.Query().Where(musicdifficultie.ServerRegionEQ(p.region.String())).Order(musicdifficultie.ByID()).All(loadCtx)
			if err != nil {
				return fmt.Errorf("query difficulty index for region %s: %w", p.region, err)
			}
			index := make(map[int][]*masterdata.MusicDifficulty)
			for _, item := range items {
				model := convertMusicDifficulty(item)
				index[model.MusicID] = append(index[model.MusicID], model)
			}
			p.difficultyMu.Lock()
			if p.difficultyGeneration == generation {
				p.difficultyIndex = index
				p.difficultyLoadedAt = time.Now()
			}
			p.difficultyMu.Unlock()
			return nil
		})
		return completed, nil
	})
	return waitDBBulkIndexFlight(ctx, result, caller, "musics.difficulty_index_wait", "musics.difficulty_index_shared")
}

func convertMusicDifficulty(item *sekaiDB.Musicdifficultie) *masterdata.MusicDifficulty {
	return &masterdata.MusicDifficulty{
		ID: int(item.GameID), MusicID: int(item.MusicID), MusicDifficulty: item.MusicDifficulty,
		PlayLevel: int(item.PlayLevel), TotalNoteCount: int(item.TotalNoteCount),
	}
}
