package deck

import (
	"context"
	"fmt"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func (c *Controller) buildAutoRecommendWithEngine(ctx context.Context, query AutoQuery) (*drawing.DeckRequest, error) {
	ctx = normalizeRecommendContext(ctx)
	finishPrepare := commandtrace.MeasureOperation(ctx, "deck.prepare_request")
	defer finishPrepare()
	prepared, err := c.prepareAutoRecommendExecution(ctx, query)
	if err != nil {
		return nil, err
	}
	finishPrepare()

	result, selections, err := c.runAutoRecommendExecution(ctx, &prepared)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	finishBuild := commandtrace.MeasureOperation(ctx, "payload.build")
	payload, buildErr := c.buildDrawingRequestFromRecommendResult(
		prepared.region,
		prepared.recType,
		prepared.query,
		prepared.option,
		prepared.userData,
		result,
		selections,
	)
	finishBuild()
	return payload, buildErr
}

type autoRecommendExecution struct {
	region          renderregion.Value
	recType         string
	query           AutoQuery
	option          map[string]any
	userData        *snapshot.RawUserData
	recommender     PjskDeckRecommender
	request         RecommendRequest
	musicSelections []MusicCompareSelection
	musicShowNum    int
}

func (c *Controller) prepareAutoRecommendExecution(ctx context.Context, query AutoQuery) (autoRecommendExecution, error) {
	resources, err := c.resolveAutoRecommendResources(ctx, query)
	if err != nil {
		return autoRecommendExecution{}, err
	}
	option, err := c.buildRecommendOption(resources.region, resources.recType, query)
	if err != nil {
		return autoRecommendExecution{}, err
	}
	applyRecommendChallengeAllDefaults(option, resources.recType, query)
	query, option = c.applyWorldBloomSimulationFallbackIfMasterdataMissing(resources.region, resources.recType, query, option)
	if resources.recType == "challenge" {
		if err := c.prepareChallengeRecommend(query, option); err != nil {
			return autoRecommendExecution{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return autoRecommendExecution{}, err
	}

	preparedRaw, userBytes, err := c.prepareRecommendUserData(resources.region, resources.recType, query, option)
	if err != nil {
		return autoRecommendExecution{}, err
	}
	if err := ctx.Err(); err != nil {
		return autoRecommendExecution{}, err
	}

	musicCompareSelections, musicCompareShowNum, err := c.prepareMusicCompareSelections(
		resources.region,
		resources.recType,
		query,
		option,
		resources.musicMeta,
		resources.musicMetaPath,
	)
	if err != nil {
		return autoRecommendExecution{}, err
	}

	return autoRecommendExecution{
		region:          resources.region,
		recType:         resources.recType,
		query:           query,
		option:          option,
		userData:        preparedRaw,
		recommender:     resources.recommender,
		musicSelections: musicCompareSelections,
		musicShowNum:    musicCompareShowNum,
		request: RecommendRequest{
			Region:            resources.region.String(),
			RecommendType:     resources.recType,
			UserData:          userBytes,
			UserDataFilePath:  c.resolveUserDataFilePath(),
			MusicMeta:         resources.musicMeta,
			MusicMetaFilePath: resources.musicMetaPath,
		},
	}, nil
}

type autoRecommendResources struct {
	region        renderregion.Value
	recType       string
	recommender   PjskDeckRecommender
	musicMeta     []byte
	musicMetaPath string
}

func (c *Controller) resolveAutoRecommendResources(ctx context.Context, query AutoQuery) (autoRecommendResources, error) {
	if err := ctx.Err(); err != nil {
		return autoRecommendResources{}, err
	}
	if c.engine == nil {
		return autoRecommendResources{}, fmt.Errorf("deck recommend engine is not configured")
	}

	region, recType, err := c.normalizeAutoQuery(query)
	if err != nil {
		return autoRecommendResources{}, err
	}
	if err := ctx.Err(); err != nil {
		return autoRecommendResources{}, err
	}
	region, _, err = c.resolveCardSource(region)
	if err != nil {
		return autoRecommendResources{}, err
	}

	recommender, err := c.engine.Get(region.String())
	if err != nil {
		return autoRecommendResources{}, err
	}

	musicMeta := c.resolveMusicMeta(region)
	musicMetaPath := c.resolveMusicMetaFilePath()
	if len(musicMeta) == 0 && musicMetaPath == "" {
		return autoRecommendResources{}, fmt.Errorf("deck recommend requires music meta data")
	}
	if err := ctx.Err(); err != nil {
		return autoRecommendResources{}, err
	}
	return autoRecommendResources{
		region:        region,
		recType:       recType,
		recommender:   recommender,
		musicMeta:     musicMeta,
		musicMetaPath: musicMetaPath,
	}, nil
}

func (c *Controller) runAutoRecommendExecution(ctx context.Context, prepared *autoRecommendExecution) (*RecommendResult, []MusicCompareSelection, error) {
	if prepared.recType == "challenge" {
		return c.runChallengeRecommendExecution(ctx, prepared)
	}
	if prepared.query.MusicCompare {
		result, selections, err := c.recommendMusicCompare(
			ctx,
			prepared.recommender,
			prepared.request,
			prepared.option,
			prepared.musicSelections,
			prepared.musicShowNum,
			prepared.recType,
		)
		return result, selections, err
	}
	return c.runStandardRecommendExecution(ctx, prepared)
}

func (c *Controller) runChallengeRecommendExecution(ctx context.Context, prepared *autoRecommendExecution) (*RecommendResult, []MusicCompareSelection, error) {
	if prepared.query.MusicCompare {
		return c.recommendMusicCompare(
			ctx,
			prepared.recommender,
			prepared.request,
			prepared.option,
			prepared.musicSelections,
			prepared.musicShowNum,
			prepared.recType,
		)
	}
	if shouldRunChallengeAll(prepared.option) {
		result, err := c.recommendChallengeAll(ctx, prepared.recommender, prepared.request, prepared.option)
		return result, prepared.musicSelections, err
	}
	request := prepared.request
	request.BatchOption = expandRecommendBatchOptions(prepared.recommender, prepared.recType, prepared.option)
	result, err := recommendWithContext(ctx, prepared.recommender, request)
	if err == nil {
		applyChallengeScoreDelta(result, optionInt(prepared.option, "challenge_live_character_id"), c.snapshot.RawData())
	}
	return result, prepared.musicSelections, err
}

func (c *Controller) runStandardRecommendExecution(ctx context.Context, prepared *autoRecommendExecution) (*RecommendResult, []MusicCompareSelection, error) {
	request := prepared.request
	request.BatchOption = expandRecommendBatchOptions(prepared.recommender, prepared.recType, prepared.option)
	result, err := recommendWithContext(ctx, prepared.recommender, request)
	if err == nil {
		return result, prepared.musicSelections, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, prepared.musicSelections, ctxErr
	}
	fallbackQuery, fallbackOption, ok := buildWorldBloomSimulationFallbackOnError(prepared.query, prepared.option, prepared.recType, err)
	if !ok {
		return nil, prepared.musicSelections, err
	}
	fallbackRequest := request
	fallbackRequest.BatchOption = expandRecommendBatchOptions(prepared.recommender, prepared.recType, fallbackOption)
	fallbackResult, fallbackErr := recommendWithContext(ctx, prepared.recommender, fallbackRequest)
	if fallbackErr != nil {
		return nil, prepared.musicSelections, fallbackErr
	}
	prepared.query = fallbackQuery
	prepared.option = fallbackOption
	return fallbackResult, prepared.musicSelections, nil
}

func (c *Controller) buildRecommendOption(region renderregion.Value, recType string, query AutoQuery) (map[string]any, error) {
	eventID := 0
	if query.EventID != nil && *query.EventID > 0 {
		eventID = *query.EventID
	}
	if eventID == 0 && recType != "no_event" && recType != "challenge" {
		if id := c.pickCurrentOrNextEventID(region); id > 0 {
			eventID = id
		}
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 6
	}

	option := map[string]any{
		"region":                    region.String(),
		"algorithm":                 "all",
		"timeout_ms":                c.recommendTimeoutMs(),
		"limit":                     limit,
		"target":                    "score",
		"live_type":                 "multi",
		"music_id":                  10000,
		"music_diff":                "master",
		"member":                    5,
		"rarity_1_config":           defaultDeckConfig12(),
		"rarity_2_config":           defaultDeckConfig12(),
		"rarity_3_config":           defaultDeckConfig34bd(),
		"rarity_4_config":           defaultDeckConfig34bd(),
		"rarity_birthday_config":    defaultDeckConfig34bd(),
		"single_card_configs":       []any{},
		"best_skill_as_leader":      true,
		"keep_after_training_state": false,
	}

	switch recType {
	case "challenge":
		option["live_type"] = "challenge"
		option["event_id"] = nil
	case "no_event":
		option["algorithm"] = "all"
		option["live_type"] = "multi"
		option["event_id"] = nil
	case "bonus":
		option["algorithm"] = "all"
		option["live_type"] = "solo"
		option["target"] = "bonus"
		option["target_bonus_list"] = pickBonusTargets(query.TargetBonuses, query.Args)
		option["rarity_1_config"] = noChangeDeckConfig()
		option["rarity_2_config"] = noChangeDeckConfig()
		option["rarity_3_config"] = noChangeDeckConfig()
		option["rarity_4_config"] = noChangeDeckConfig()
		option["rarity_birthday_config"] = noChangeDeckConfig()
		if eventID > 0 {
			option["event_id"] = eventID
		}
	case "mysekai":
		option["algorithm"] = "all"
		option["live_type"] = "mysekai"
		if eventID > 0 {
			option["event_id"] = eventID
		}
		option["rarity_1_config"] = noChangeDeckConfig()
		option["rarity_2_config"] = noChangeDeckConfig()
		option["rarity_3_config"] = noChangeDeckConfig()
		option["rarity_4_config"] = noChangeDeckConfig()
		option["rarity_birthday_config"] = noChangeDeckConfig()
	default:
		if eventID > 0 {
			option["event_id"] = eventID
		}
	}

	applyRecommendOptionOverrides(option, recType, query)
	normalizeRecommendLiveOptions(option)
	applyRecommendStrategyDefaults(option, recType)
	applyEventRecommendRLDowngrade(option, recType)
	return option, nil
}
