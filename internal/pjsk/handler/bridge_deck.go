package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/profile"
	sekaiutils "haruki-cloud/internal/pjsk/sekai"
)

type deckUserTargetParams struct {
	Selector string `json:"selector,omitempty"`
}

func executeDeck(rc *RequestContext) (message onebot11.Message, err error) {
	defer func() {
		err = normalizeDeckUserFacingError(err)
	}()

	var data []byte
	recommendType := ""
	buildDoneText := func(q deck.AutoQuery) string {
		return fmt.Sprintf("已处理%s。", formatDeckQuerySummary(q))
	}
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

		regionStr = resolvedTargetRegion(regionStr, target)
		platform, platformUserID := platformCredentials(p)
		targetSnapshot := resolveTargetSnapshot(rc.Ctx, rc.App, regionStr, platform, platformUserID, target.PJSKUserID, false)

		q := deck.AutoQuery{Region: regionStr, RecommendType: recommendType}
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
				pq := profile.Query{Region: regionStr, Visible: target.Visible, BgSettings: target.BgSettings}
				if detail, buildErr := rc.App.Profiles.BuildDetailedProfileCardFromAPIWithSnapshot(pq, resp, targetSnapshot); buildErr == nil {
					q.Profile = detail
				}
			}
		}

		deckCtrl := rc.App.Decks
		if targetSnapshot != nil {
			deckCtrl = deckCtrl.WithSnapshot(targetSnapshot)
		}

		data, err = deckCtrl.RenderAutoRecommend(q)
		if err != nil {
			return nil, err
		}
		image, imageErr := rc.ImageMessage(data)
		if imageErr != nil {
			return nil, imageErr
		}
		return append(onebot11.Message{onebot11.Text(buildDoneText(q))}, image...), nil
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
	var targetParams deckUserTargetParams
	mergeParams(rc.Cmd.Params, &targetParams)
	if err := resolveDeckCharacterSelections(rc.Ctx, &q, rc.App); err != nil {
		return nil, err
	}
	if err := resolveDeckMusicSelection(&q, rc.App); err != nil {
		return nil, err
	}
	detail, snapshot, region, err := resolveDeckRenderProfileAndSnapshot(rc, targetParams.Selector)
	if err != nil {
		return nil, err
	}
	q.Region = region
	if detail != nil {
		q.Profile = detail
	}

	// Try to inject live Toolbox snapshot so the deck controller can operate
	// on real user data even when no local snapshot file is configured.
	deckCtrl := rc.App.Decks
	if snapshot != nil {
		deckCtrl = deckCtrl.WithSnapshot(snapshot)
	}

	data, err = deckCtrl.RenderAutoRecommend(q)
	if err != nil {
		return nil, err
	}
	image, imageErr := rc.ImageMessage(data)
	if imageErr != nil {
		return nil, imageErr
	}
	return append(onebot11.Message{onebot11.Text(buildDoneText(q))}, image...), nil
}

