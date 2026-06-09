package deck

import (
	"fmt"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) buildAutoRecommendWithEngine(query AutoQuery) (*drawing.DeckRequest, error) {
	if c.engine == nil {
		return nil, fmt.Errorf("deck recommend engine is not configured")
	}

	region, recType, err := c.normalizeAutoQuery(query)
	if err != nil {
		return nil, err
	}
	region, _, err = c.resolveCardSource(region)
	if err != nil {
		return nil, err
	}

	recommender, err := c.engine.Get(region.String())
	if err != nil {
		return nil, err
	}

	musicMeta := c.resolveMusicMeta(region)
	musicMetaPath := c.resolveMusicMetaFilePath()
	if len(musicMeta) == 0 && musicMetaPath == "" {
		return nil, fmt.Errorf("deck recommend requires music meta data")
	}

	option, err := c.buildRecommendOption(region, recType, query)
	if err != nil {
		return nil, err
	}
	applyRecommendChallengeAllDefaults(option, recType, query)
	query, option = c.applyWorldBloomSimulationFallbackIfMasterdataMissing(region, recType, query, option)
	if recType == "challenge" {
		if err := c.prepareChallengeRecommend(query, option); err != nil {
			return nil, err
		}
	}

	preparedRaw, userBytes, err := c.prepareRecommendUserData(region, recType, query, option)
	if err != nil {
		return nil, err
	}

	musicCompareSelections, musicCompareShowNum, err := c.prepareMusicCompareSelections(region, recType, query, option, musicMeta, musicMetaPath)
	if err != nil {
		return nil, err
	}

	recommendRequest := RecommendRequest{
		Region:            region.String(),
		RecommendType:     recType,
		UserData:          userBytes,
		UserDataFilePath:  c.resolveUserDataFilePath(),
		MusicMeta:         musicMeta,
		MusicMetaFilePath: musicMetaPath,
	}

	var result *RecommendResult
	if recType == "challenge" {
		if query.MusicCompare {
			result, musicCompareSelections, err = c.recommendMusicCompare(recommender, recommendRequest, option, musicCompareSelections, musicCompareShowNum, recType)
		} else if shouldRunChallengeAll(option) {
			result, err = c.recommendChallengeAll(recommender, recommendRequest, option)
		} else {
			recommendRequest.BatchOption = expandRecommendBatchOptions(recommender, recType, option)
			result, err = recommender.Recommend(recommendRequest)
			if err == nil {
				applyChallengeScoreDelta(result, optionInt(option, "challenge_live_character_id"), c.snapshot.RawData())
			}
		}
	} else if query.MusicCompare {
		result, musicCompareSelections, err = c.recommendMusicCompare(recommender, recommendRequest, option, musicCompareSelections, musicCompareShowNum, recType)
	} else {
		recommendRequest.BatchOption = expandRecommendBatchOptions(recommender, recType, option)
		result, err = recommender.Recommend(recommendRequest)
		if err != nil {
			if fallbackQuery, fallbackOption, ok := buildWorldBloomSimulationFallbackOnError(query, option, recType, err); ok {
				fallbackRequest := recommendRequest
				fallbackRequest.BatchOption = expandRecommendBatchOptions(recommender, recType, fallbackOption)
				if fallbackResult, fallbackErr := recommender.Recommend(fallbackRequest); fallbackErr == nil {
					query = fallbackQuery
					option = fallbackOption
					result = fallbackResult
					err = nil
				} else {
					err = fallbackErr
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}

	return c.buildDrawingRequestFromRecommendResult(region, recType, query, option, preparedRaw, result, musicCompareSelections)
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
