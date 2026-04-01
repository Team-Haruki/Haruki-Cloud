package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/profile"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
	sekaiutils "haruki-cloud/utils/sekai"
)

func executeDeck(rc *RequestContext) (message onebot11.Message, err error) {
	var data []byte
	recommendType := ""
	switch rc.Cmd.Mode {
	case "deck-event":
		recommendType = "event"
	case "deck-challenge":
		recommendType = "challenge"
	case "deck-no-event":
		recommendType = "no_event"
	case "deck-bonus":
		recommendType = "bonus"
	case "deck-mysekai":
		recommendType = "mysekai"

		// deck-mysekai sends combined params: {deck: ..., query: ...}
		var combined struct {
			Deck  json.RawMessage `json:"deck"`
			Query userQueryParams `json:"query"`
		}
		mergeParams(rc.Cmd.Params, &combined)

		regionStr := regionWithDefault(rc.Cmd.Region)

		// Resolve target binding from user query params.
		p := combined.Query
		if p.Mode == "" {
			p.Mode = "self"
			p.Platform = strings.TrimSpace(rc.Cmd.RequesterPlatform)
			p.PlatformUserID = strings.TrimSpace(rc.Cmd.RequesterUserID)
		}
		target, targetErr := resolveGameTarget(rc.Ctx, p, regionStr, rc.Cmd.RegionExplicit, rc.App)
		if targetErr != nil {
			return nil, targetErr
		}

		platform, platformUserID := platformCredentials(p)
		uid, _ := strconv.ParseInt(target.PJSKUserID, 10, 64)

		q := deck.AutoQuery{Region: rc.Cmd.Region, RecommendType: recommendType}
		mergeParams(combined.Deck, &q)
		if err := resolveDeckCharacterSelections(rc.Ctx, &q, rc.App); err != nil {
			return nil, err
		}
		if err := resolveDeckMusicSelection(&q, rc.App); err != nil {
			return nil, err
		}

		// Build detailed profile for deck rendering from the resolved target.
		if rc.App.Profiles != nil {
			if resp, apiErr := sekaiutils.GetSekaiAPIClient().GetUserProfile(regionStr, target.PJSKUserID); apiErr == nil {
				var framesJSON []byte
				if hasUsableSuiteData(target.Binding) {
					framesJSON, _ = sekaiutils.GetToolboxClient().GetPrivateDataValue(
						regionStr, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userPlayerFrames")
				}
				pq := profile.Query{Region: regionStr, Visible: target.Visible, BgSettings: target.BgSettings}
				if detail, buildErr := rc.App.Profiles.BuildDetailedProfileCardFromAPI(pq, resp, framesJSON); buildErr == nil {
					q.Profile = detail
				}
			}
		}

		deckCtrl := rc.App.Decks
		tc := sekaiutils.GetToolboxClient()
		if target.Binding != nil && hasUsableSuiteData(target.Binding) {
			suiteJSON, suiteErr := tc.GetSuiteData(regionStr, uid, platform, platformUserID)
			if suiteErr == nil && len(suiteJSON) > 0 {
				region := renderregion.Normalize(regionStr)
				if snapshot, snapErr := userdata.NewFromBytes(rc.App.Sekai, rc.App.Assets, region, suiteJSON, nil, nil); snapErr == nil {
					deckCtrl = deckCtrl.WithSnapshot(snapshot)
				}
			}
		}

		data, err = deckCtrl.RenderAutoRecommend(q)
		if err != nil {
			return nil, err
		}
		return rc.ImageMessage(data)
	case "deck-score-up":
		var msg string
		err := json.Unmarshal(rc.Cmd.Params, &msg)
		if err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Text(msg)}, nil
	default:
		return nil, unsupportedModeError("deck", rc.Cmd.Mode)
	}
	q := deck.AutoQuery{Region: rc.Cmd.Region, RecommendType: recommendType}
	mergeParams(rc.Cmd.Params, &q)
	if err := resolveDeckCharacterSelections(rc.Ctx, &q, rc.App); err != nil {
		return nil, err
	}
	if err := resolveDeckMusicSelection(&q, rc.App); err != nil {
		return nil, err
	}
	if detail := rc.GetDetailedProfile(); detail != nil {
		q.Profile = detail
	}

	// Try to inject live Toolbox snapshot so the deck controller can operate
	// on real user data even when no local snapshot file is configured.
	deckCtrl := rc.App.Decks
	if snapshot := rc.ResolveSnapshot(false); snapshot != nil {
		deckCtrl = deckCtrl.WithSnapshot(snapshot)
	}

	data, err = deckCtrl.RenderAutoRecommend(q)
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func resolveDeckMusicSelection(q *deck.AutoQuery, app *renderapp.App) error {
	if q == nil {
		return nil
	}
	if app == nil || app.Music == nil {
		return fmt.Errorf("deck music resolve requires music controller")
	}

	var (
		result *music.CoverResult
		err    error
	)
	if q.MusicID != nil && *q.MusicID > 0 {
		result, err = app.Music.ResolveMusicCover(music.Query{
			Query:  fmt.Sprintf("music%d", *q.MusicID),
			Region: q.Region,
		})
	} else {
		query := strings.TrimSpace(q.MusicQuery)
		if query == "" {
			return nil
		}
		result, err = app.Music.ResolveMusicCoverByTitleOrAlias(music.Query{
			Query:  query,
			Region: q.Region,
		})
	}
	if err != nil {
		return err
	}
	if result == nil || result.Music == nil || result.Music.ID <= 0 {
		return fmt.Errorf("failed to resolve deck music selection")
	}

	q.MusicID = drawing.IntPtr(result.Music.ID)
	q.MusicTitle = result.Music.Title
	q.MusicCoverPath = result.JacketPath
	return nil
}

func resolveDeckCharacterSelections(ctx context.Context, q *deck.AutoQuery, app *renderapp.App) error {
	if q == nil {
		return nil
	}

	region := renderregion.WithDefault(renderregion.Normalize(q.Region))

	if q.WorldBloomCharacterID == nil && strings.TrimSpace(q.WorldBloomCharacterQuery) != "" {
		charID, err := resolveGameCharacterIDByQuery(ctx, app, region, q.WorldBloomCharacterQuery, "deck")
		if err != nil {
			if q.WorldBloomEventTurn == nil && strings.TrimSpace(q.MusicQuery) == "" && isCharacterNotFoundError(err) {
				q.MusicQuery = strings.TrimSpace(q.WorldBloomCharacterQuery)
				q.WorldBloomCharacterQuery = ""
			} else {
				return err
			}
		} else {
			q.WorldBloomCharacterID = drawing.IntPtr(charID)
			q.WorldBloomCharacterQuery = ""
			if strings.TrimSpace(q.EventUnit) == "" {
				q.EventUnit = resolveDeckCharacterUnit(charID)
			}
		}
	}

	if q.ChallengeLiveCharacterID == nil && strings.TrimSpace(q.ChallengeLiveCharacterQuery) != "" {
		charID, err := resolveGameCharacterIDByQuery(ctx, app, region, q.ChallengeLiveCharacterQuery, "deck")
		if err != nil {
			if strings.TrimSpace(q.MusicQuery) == "" && isCharacterNotFoundError(err) {
				q.MusicQuery = strings.TrimSpace(q.ChallengeLiveCharacterQuery)
				q.ChallengeLiveCharacterQuery = ""
			} else {
				return err
			}
		} else {
			q.ChallengeLiveCharacterID = drawing.IntPtr(charID)
			q.ChallengeLiveCharacterQuery = ""
		}
	}

	if len(q.FixedCharacterQueries) > 0 {
		for _, raw := range q.FixedCharacterQueries {
			charID, err := resolveGameCharacterIDByQuery(ctx, app, region, raw, "deck")
			if err != nil {
				return err
			}
			q.FixedCharacters = append(q.FixedCharacters, charID)
		}
		q.FixedCharacterQueries = nil
	}

	if err := validateDeckCharacterIDs(q.FixedCharacters); err != nil {
		return err
	}
	return nil
}

func validateDeckCharacterIDs(values []int) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) > 5 {
		return fmt.Errorf("固定角色最多只能指定5个")
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return fmt.Errorf("固定角色ID必须为正整数")
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("固定角色ID不能重复")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func isCharacterNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "未找到角色")
}

func resolveDeckCharacterUnit(charID int) string {
	switch {
	case charID >= 1 && charID <= 4:
		return "light_sound"
	case charID >= 5 && charID <= 8:
		return "idol"
	case charID >= 9 && charID <= 12:
		return "street"
	case charID >= 13 && charID <= 16:
		return "theme_park"
	case charID >= 17 && charID <= 20:
		return "school_refusal"
	case charID >= 21 && charID <= 26:
		return "piapro"
	default:
		return ""
	}
}
