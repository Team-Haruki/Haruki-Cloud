package requestbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/drawing"
)

const (
	scoreControlDefaultMusicID    = 74
	scoreControlMaxScore          = 2840000
	scoreControlMaxEventBonus     = 435
	scoreControlMaxWLEventBonus   = 600
	scoreControlMaxShownSolutions = 150
)

var scoreControlBoostBonus = map[int]int{
	0:  1,
	1:  5,
	2:  10,
	3:  15,
	4:  20,
	5:  25,
	6:  27,
	7:  29,
	8:  31,
	9:  33,
	10: 35,
}

type scoreControlSelection struct {
	TargetPoint int    `json:"target_point"`
	Query       string `json:"query,omitempty"`
	WL          bool   `json:"wl,omitempty"`
}

func BuildScoreControlRequest(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (*drawing.ScoreControlRequest, error) {
	if app == nil || app.Music == nil {
		return nil, fmt.Errorf("score music service unavailable: music controller is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	musicCtrl := app.Music.WithContext(ctx)
	if app.Aliases != nil {
		musicCtrl.SetAliasResolver(app.Aliases)
	}

	params, err := resolveScoreControlSelection(r)
	if err != nil {
		return nil, err
	}
	if params.TargetPoint <= 0 {
		return nil, fmt.Errorf("invalid score control request")
	}

	query := strings.TrimSpace(params.Query)
	if query == "" {
		query = strconv.Itoa(scoreControlDefaultMusicID)
	}

	requests, err := musicCtrl.ResolveMusicMetaRequests(r.Region, []string{query})
	if err != nil {
		return nil, err
	}
	if len(requests) == 0 {
		return nil, fmt.Errorf("invalid score control request")
	}

	metaReq := requests[0]
	basicPoint := selectScoreControlBasicPoint(metaReq.Metas)
	if basicPoint <= 0 {
		return nil, fmt.Errorf("music %d has no basic point data", metaReq.MusicID)
	}

	validScores := findValidScoreRanges(params.TargetPoint, basicPoint, params.WL, scoreControlMaxShownSolutions)
	if len(validScores) == 0 {
		return nil, fmt.Errorf("找不到符合条件的分数范围")
	}

	return &drawing.ScoreControlRequest{
		MusicCoverPath:  metaReq.MusicCoverPath,
		MusicID:         metaReq.MusicID,
		MusicTitle:      metaReq.MusicTitle,
		MusicBasicPoint: basicPoint,
		TargetPoint:     params.TargetPoint,
		ValidScores:     validScores,
	}, nil
}

func resolveScoreControlSelection(r *parser.ResolvedCommand) (scoreControlSelection, error) {
	params := scoreControlSelection{}
	if r != nil && r.Params != nil {
		if err := json.Unmarshal(r.Params, &params); err != nil {
			return scoreControlSelection{}, fmt.Errorf("bridge: unmarshal score-control params: %w", err)
		}
	}
	if params.TargetPoint > 0 {
		return params, nil
	}

	if r == nil {
		return scoreControlSelection{}, fmt.Errorf("invalid score control request")
	}

	parts := strings.SplitN(strings.TrimSpace(r.Query), " ", 2)
	if len(parts) == 0 {
		return scoreControlSelection{}, fmt.Errorf("invalid score control request")
	}

	target, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || target <= 0 {
		return scoreControlSelection{}, fmt.Errorf("invalid score control request")
	}
	params.TargetPoint = target
	if len(parts) > 1 {
		params.Query = strings.TrimSpace(parts[1])
	}
	return params, nil
}

func selectScoreControlBasicPoint(metas []drawing.MusicMetaInfo) int {
	if len(metas) == 0 {
		return 0
	}

	best := metas[0]
	for _, item := range metas[1:] {
		if item.Difficulty == "master" && best.Difficulty != "master" {
			best = item
			continue
		}
		if best.Difficulty != "master" && item.EventRate > best.EventRate {
			best = item
		}
	}
	return int(best.EventRate)
}

func findValidScoreRanges(targetPoint, basicPoint int, wl bool, limit int) []drawing.ScoreData {
	maxEventBonus := scoreControlMaxEventBonus
	if wl {
		maxEventBonus = scoreControlMaxWLEventBonus
	}

	result := make([]drawing.ScoreData, 0, 32)
	for eventBonus := 0; eventBonus <= maxEventBonus; eventBonus++ {
		for boost := 0; boost <= 10; boost++ {
			boostBonus := scoreControlBoostBonus[boost]
			if boostBonus == 0 || targetPoint%boostBonus != 0 {
				continue
			}

			left, right := 0, scoreControlMaxScore
			found := false
			for left <= right {
				mid := (left + right) / 2
				points := calcScoreControlPoints(mid, eventBonus, basicPoint, boost)
				if points <= targetPoint {
					left = mid + 1
					if points == targetPoint {
						found = true
					}
					continue
				}
				right = mid - 1
			}
			if !found {
				continue
			}
			scoreMax := right

			left, right = 0, scoreControlMaxScore
			for left <= right {
				mid := (left + right) / 2
				points := calcScoreControlPoints(mid, eventBonus, basicPoint, boost)
				if points >= targetPoint {
					right = mid - 1
					continue
				}
				left = mid + 1
			}

			result = append(result, drawing.ScoreData{
				EventBonus: eventBonus,
				Boost:      boost,
				ScoreMin:   left,
				ScoreMax:   scoreMax,
			})
			if limit > 0 && len(result) >= limit {
				return result
			}
		}
	}
	return result
}

func calcScoreControlPoints(score, eventBonus, basicPoint, boost int) int {
	scoreBonus := score / 20000
	base := (100 + scoreBonus) * (100 + eventBonus) * basicPoint / 10000
	return base * scoreControlBoostBonus[boost]
}
