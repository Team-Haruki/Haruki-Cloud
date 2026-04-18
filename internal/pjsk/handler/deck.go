package handler

import (
	"encoding/json"
	"fmt"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/profile"
	"strconv"
	"strings"
)

func buildDeckParamsWithSelfQuery(ctx HarrukiSekaiHandlerContext, mode string) (deckAutoQueryParams, UserQueryParams, error) {
	params, err := buildDeckQueryParams(ctx, mode)
	if err != nil {
		return deckAutoQueryParams{}, UserQueryParams{}, err
	}
	query, err := resolveSelfOnlyQueryParams(ctx)
	if err != nil {
		return deckAutoQueryParams{}, UserQueryParams{}, err
	}
	params.Selector = query.Selector
	return params, query, nil
}

func (sekaiHandlers) EventDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "deck/event",
			Commands: []string{
				"/pjsk event card", "/pjsk event deck", "/pjsk deck",
				"/活动组卡", "/活动组队", "/活动卡组", "/活动配队",
				"/组卡", "/组队", "/配队",
				"/指定属性组卡", "/指定属性组队", "/指定属性卡组", "/指定属性配队",
				"/模拟组卡", "/模拟配队", "/模拟组队", "/模拟卡组",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, "deck-event")
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, "deck-event", params), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) ChallengeDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "deck/challenge",
			Commands: []string{
				"/pjsk challenge card", "/pjsk challenge deck",
				"/挑战组卡", "/挑战组队", "/挑战卡组", "/挑战配队",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, "deck-challenge")
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, "deck-challenge", params), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) NoEventDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "deck/no-event",
			Commands: []string{
				"/pjsk no event deck", "/pjsk best deck",
				"/长草组卡", "/长草组队", "/长草卡组", "/长草配队",
				"/最强卡组", "/最强组卡", "/最强组队", "/最强配队",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, "deck-no-event")
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, "deck-no-event", params), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) BonusDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "deck/bonus",
			Commands: []string{
				"/pjsk bonus deck", "/pjsk bonus card",
				"/加成组卡", "/加成组队", "/加成卡组", "/加成配队",
				"/控分组卡", "/控分组队", "/控分卡组", "/控分配队",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, "deck-bonus")
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, "deck-bonus", params), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) MysekaiDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "deck/mysekai",
			Commands: []string{
				"/mysekai deck", "/pjsk mysekai deck",
				"/烤森组卡", "/烤森组队", "/烤森卡组", "/烤森配队",
				"/ms组卡", "/ms组队", "/ms卡组", "/ms配队",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, p, err := buildDeckParamsWithSelfQuery(ctx, "deck-mysekai")
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, "deck-mysekai", mysekaiDeckCombinedParams{
				Deck:  params,
				Query: p,
			}), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) ScoreUpHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "deck/score-up",
			Commands: []string{
				"/实效", "/倍率", "/时效", "/pjsk score up",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			parts := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
			if len(parts) != 5 {
				return nil, onebot11.NewReplayError("使用方式: %s 队长技能 技能2 技能3 技能4 技能5\n例: %s 160 160 150 150 150", ctx.GetTriggerCmd(), ctx.GetTriggerCmd())
			}

			values := make([]float64, 0, 5)
			for _, p := range parts {
				v, err := strconv.ParseFloat(p, 64)
				if err != nil || v < 0 {
					return nil, onebot11.NewReplayError("使用方式: %s 队长技能 技能2 技能3 技能4 技能5\n例: %s 160 160 150 150 150", ctx.GetTriggerCmd(), ctx.GetTriggerCmd())
				}
				values = append(values, v)
			}

			leader := values[0]
			others := values[1] + values[2] + values[3] + values[4]
			internalValue := values[0] + values[1] + values[2] + values[3] + values[4]
			scoreUp := leader + others*0.2
			multiplier := scoreUp/100.0 + 1.0
			return makeCommandRequestWithParams(
				ctx,
				parser.ModuleDeck,
				"deck-score-up",
				fmt.Sprintf(
					"队长技能加成: %.4g%%\n内部值: %.4g\n实效: %.4g%%\n倍率: %.4g",
					leader, internalValue, scoreUp, multiplier,
				)), nil
		},
	}, executeDeck)
}

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

		var combined struct {
			Deck  json.RawMessage `json:"deck"`
			Query userQueryParams `json:"query"`
		}
		mergeParams(rc.Cmd.Params, &combined)

		regionStr := regionWithDefault(rc.Cmd.Region)
		if !isMySekaiRegionAllowed(rc.Cmd, regionStr) {
			return mySekaiRegionUnavailableMessage(), nil
		}

		q := deck.AutoQuery{Region: regionStr, RecommendType: recommendType}
		mergeParams(combined.Deck, &q)

		if isTheoreticalDeckQuery(q) {
			explicitMysekaiEventSelection := q.EventID != nil ||
				strings.TrimSpace(q.EventUnit) != "" ||
				strings.TrimSpace(q.EventAttr) != "" ||
				q.WorldBloomEventTurn != nil ||
				q.WorldBloomCharacterID != nil ||
				strings.TrimSpace(q.WorldBloomCharacterQuery) != ""
			if err := resolveDeckCharacterSelections(rc.Ctx, &q, rc.App); err != nil {
				return nil, err
			}
			if !explicitMysekaiEventSelection {
				preserveImplicitMysekaiWorldBloomMetadata(&q)
			}
			if err := resolveDeckMusicSelection(&q, rc.App); err != nil {
				return nil, err
			}

			data, err = rc.App.Decks.RenderAutoRecommend(q)
			if err != nil {
				return nil, err
			}
			image, imageErr := rc.ImageMessage(data)
			if imageErr != nil {
				return nil, imageErr
			}
			return append(onebot11.Message{onebot11.Text(buildDoneText(q))}, image...), nil
		}

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
		if !isMySekaiRegionAllowed(rc.Cmd, regionStr) {
			return mySekaiRegionUnavailableMessage(), nil
		}
		platform, platformUserID := platformCredentials(p)
		targetSnapshot := resolveTargetSnapshot(rc.Ctx, rc.App, regionStr, platform, platformUserID, target.PJSKUserID, false)

		q.Region = regionStr
		explicitMysekaiEventSelection := q.EventID != nil ||
			strings.TrimSpace(q.EventUnit) != "" ||
			strings.TrimSpace(q.EventAttr) != "" ||
			q.WorldBloomEventTurn != nil ||
			q.WorldBloomCharacterID != nil ||
			strings.TrimSpace(q.WorldBloomCharacterQuery) != ""
		if err := resolveDeckCharacterSelections(rc.Ctx, &q, rc.App); err != nil {
			return nil, err
		}
		if !explicitMysekaiEventSelection {
			preserveImplicitMysekaiWorldBloomMetadata(&q)
		}
		if err := resolveDeckMusicSelection(&q, rc.App); err != nil {
			return nil, err
		}

		if rc.App.Profiles != nil {
			if resp, apiErr := rc.App.SekaiAPI.GetUserProfile(regionStr, target.PJSKUserID); apiErr == nil {
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
	detail, snapshot, region, err := resolveDeckRenderProfileAndSnapshot(rc, targetParams.Selector)
	if err != nil {
		return nil, err
	}
	q.Region = region
	if detail != nil {
		q.Profile = detail
	}
	if err := resolveDeckCharacterSelections(rc.Ctx, &q, rc.App); err != nil {
		return nil, err
	}
	if err := resolveDeckMusicSelection(&q, rc.App); err != nil {
		return nil, err
	}

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

func preserveImplicitMysekaiWorldBloomMetadata(q *deck.AutoQuery) {
	if q == nil {
		return
	}
	if q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0 {
		q.MetadataWorldBloomCharacterID = drawing.IntPtr(*q.WorldBloomCharacterID)
	}
	q.EventID = nil
	q.EventUnit = ""
	q.EventAttr = ""
	q.WorldBloomEventTurn = nil
	q.WorldBloomCharacterID = nil
	q.WorldBloomCharacterQuery = ""
}

func isTheoreticalDeckQuery(q deck.AutoQuery) bool {
	return !q.UseCurrentDeck && (q.MaxProfile || q.SubMaxProfile)
}
