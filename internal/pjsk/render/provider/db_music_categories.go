package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"haruki-cloud/database/sekai/musiccategorie"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// fillMusicCategories sets Categories from musiccategories for the musics
// whose own categories are empty (JP 6.8 dropped musics.categories). The
// table is only loaded when at least one music needs it; a load error
// leaves the musics as they are and the cache unloaded.
func (p *dbMusicProvider) fillMusicCategories(ctx context.Context, musics ...*masterdata.Music) {
	needed := false
	for _, item := range musics {
		if item != nil && len(item.Categories) == 0 {
			needed = true
			break
		}
	}
	if !needed {
		return
	}
	if err := p.ensureMusicCategoriesLoaded(ctx); err != nil {
		return
	}
	p.categoryMu.RLock()
	defer p.categoryMu.RUnlock()
	for _, item := range musics {
		if item == nil || len(item.Categories) > 0 {
			continue
		}
		if categories := p.categoriesByMusic[item.ID]; len(categories) > 0 {
			item.Categories = append([]string(nil), categories...)
		}
	}
}

func (p *dbMusicProvider) ensureMusicCategoriesLoaded(ctx context.Context) error {
	p.categoryMu.RLock()
	loaded := dbBulkIndexFresh(p.categoriesLoaded, p.categoriesLoadedAt)
	p.categoryMu.RUnlock()
	if loaded {
		return nil
	}

	callerToken := new(dbBulkIndexFlightToken)
	result := p.categoryLoads.DoChan("all", func() (any, error) {
		completed := runDBBulkIndexFlight(callerToken, func(loadCtx context.Context) error {
			finishIndex := commandtrace.MeasureOperation(loadCtx, "musics.category_index")
			defer finishIndex()
			p.categoryMu.RLock()
			alreadyLoaded := dbBulkIndexFresh(p.categoriesLoaded, p.categoriesLoadedAt)
			p.categoryMu.RUnlock()
			if alreadyLoaded {
				return nil
			}
			items, err := p.client.Musiccategorie.Query().
				Where(musiccategorie.ServerRegionEQ(p.region.String())).
				Order(musiccategorie.ByGameID()).
				All(loadCtx)
			if err != nil {
				return fmt.Errorf("query music categories for region %s: %w", p.region, err)
			}

			byMusic := make(map[int][]string)
			for _, item := range items {
				name := strings.TrimSpace(item.MusicCategoryName)
				if item.MusicID <= 0 || name == "" {
					continue
				}
				byMusic[int(item.MusicID)] = append(byMusic[int(item.MusicID)], name)
			}

			p.categoryMu.Lock()
			p.categoriesByMusic = byMusic
			p.categoriesLoaded = true
			p.categoriesLoadedAt = time.Now()
			p.categoryMu.Unlock()
			return nil
		})
		return completed, nil
	})

	return waitDBBulkIndexFlight(ctx, result, callerToken, "musics.category_index_wait", "musics.category_index_shared")
}
