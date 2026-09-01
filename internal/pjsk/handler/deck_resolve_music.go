package handler

import (
	"fmt"
	"strings"

	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/music"

	"haruki-cloud/internal/pjsk/drawing"
)

const deckOmakaseMusicID = 10000

func resolveDeckMusicSelection(q *deck.AutoQuery, app *renderapp.App) error {
	if q == nil {
		return nil
	}
	if q.MusicID != nil && *q.MusicID == deckOmakaseMusicID {
		return nil
	}
	if app == nil || app.Music == nil {
		return fmt.Errorf("deck music resolve requires music controller")
	}
	if q.MusicCompare {
		return resolveAndApplyDeckMusicCompare(q, app)
	}
	result, err := resolveDeckMusicCover(q, app)
	if err != nil {
		return err
	}
	if result == nil && (q.MusicID == nil || *q.MusicID <= 0) && strings.TrimSpace(q.MusicQuery) == "" {
		return nil
	}
	if result == nil || result.Music == nil || result.Music.ID <= 0 {
		return fmt.Errorf("failed to resolve deck music selection")
	}

	q.MusicID = drawing.IntPtr(result.Music.ID)
	q.MusicTitle = result.Music.Title
	q.MusicCoverPath = result.JacketPath
	return nil
}

func resolveAndApplyDeckMusicCompare(q *deck.AutoQuery, app *renderapp.App) error {
	selections, err := resolveDeckMusicCompareSelections(q.Region, q.MusicCompareQueries, app)
	if err != nil {
		return err
	}
	q.MusicCompareSelections = selections
	return nil
}

func resolveDeckMusicCover(q *deck.AutoQuery, app *renderapp.App) (*music.CoverResult, error) {
	if q.MusicID != nil && *q.MusicID > 0 {
		return app.Music.ResolveMusicCover(music.Query{Query: fmt.Sprintf("music%d", *q.MusicID), Region: q.Region})
	}
	query := strings.TrimSpace(q.MusicQuery)
	if query == "" {
		return nil, nil
	}
	return app.Music.ResolveMusicCoverByTitleOrAlias(music.Query{Query: query, Region: q.Region})
}

func resolveDeckMusicCompareSelections(region string, queries []string, app *renderapp.App) ([]deck.MusicCompareSelection, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	if app == nil || app.Music == nil {
		return nil, fmt.Errorf("deck music resolve requires music controller")
	}

	result := make([]deck.MusicCompareSelection, 0, len(queries))
	for _, raw := range queries {
		query := strings.TrimSpace(raw)
		if query == "" {
			continue
		}
		selection, err := resolveDeckMusicCompareSelection(region, query, app)
		if err != nil {
			return nil, err
		}
		result = append(result, selection)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func resolveDeckMusicCompareSelection(region, query string, app *renderapp.App) (deck.MusicCompareSelection, error) {
	diff, cleaned := extractCompactMusicDifficulty(query)
	if diff == "" {
		diff = "master"
	}
	if cleaned == "" {
		return deck.MusicCompareSelection{}, fmt.Errorf("无法解析要比较的歌曲 %q", query)
	}
	coverResult, err := app.Music.ResolveMusicCoverByTitleOrAlias(music.Query{Query: cleaned, Region: region})
	if err != nil {
		return deck.MusicCompareSelection{}, err
	}
	if coverResult == nil || coverResult.Music == nil || coverResult.Music.ID <= 0 {
		return deck.MusicCompareSelection{}, fmt.Errorf("failed to resolve compare music selection %q", query)
	}
	return deck.MusicCompareSelection{
		MusicID: coverResult.Music.ID, MusicDiff: diff, MusicTitle: coverResult.Music.Title,
		MusicCoverPath: coverResult.JacketPath, MusicQuery: query,
	}, nil
}

func assignDeckFallbackMusicQuery(q *deck.AutoQuery, raw string) {
	if q == nil {
		return
	}

	query := strings.TrimSpace(raw)
	if query == "" {
		return
	}
	if strings.TrimSpace(q.MusicDiff) == "" {
		if diff, cleaned := extractCompactMusicDifficulty(query); diff != "" && strings.TrimSpace(cleaned) != "" {
			q.MusicDiff = diff
			query = cleaned
		}
	}
	q.MusicQuery = strings.TrimSpace(query)
}
