package deck

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func (c *Controller) buildDrawingRequestFromRecommendResult(region renderregion.Value, recType string, query AutoQuery, option map[string]any, preparedRaw *snapshot.RawUserData, result *RecommendResult, musicCompareSelections []MusicCompareSelection) (*drawing.DeckRequest, error) {
	if result == nil || len(result.Decks) == 0 {
		return nil, fmt.Errorf("deck recommend service returned no deck results")
	}
	region, cardSource, err := c.resolveCardSource(region)
	if err != nil {
		return nil, err
	}

	profile := c.resolveProfile(region, query.Profile, "deck_remote_service")
	userCardMap := make(map[int]snapshot.RawUserCard)
	if preparedRaw != nil {
		for _, userCard := range preparedRaw.UserCards {
			userCardMap[userCard.CardID] = userCard
		}
	} else if raw := c.snapshot.RawData(); raw != nil {
		for _, userCard := range raw.UserCards {
			userCardMap[userCard.CardID] = userCard
		}
	}

	deckData := make([]drawing.DeckData, 0, len(result.Decks))
	for index, deckInfo := range result.Decks {
		cardData := make([]drawing.DeckCardData, 0, len(deckInfo.Cards))
		for _, deckCard := range deckInfo.Cards {
			card, err := c.cardByIDWithFallback(cardSource, region, deckCard.CardID)
			if err != nil || card == nil {
				continue
			}

			userCard, hasUserCard := userCardMap[deckCard.CardID]
			displayAfterTraining, trainedArt := resolveRecommendCardDisplayState(card, deckCard)

			level := deckCard.Level
			if level <= 0 {
				level = 60
			}
			masterRank := deckCard.MasterRank
			rareImgPath := ""
			if hasUserCard && strings.EqualFold(userCard.SpecialTrainingStatus, "done") {
				rareImgPath = common.ResolveCardRareImagePath(c.assets, card, true)
			}

			cardData = append(cardData, drawing.DeckCardData{
				CardThumbnail: common.BuildCardThumbnail(c.assets, card, region, common.ThumbnailOptions{
					AfterTraining: displayAfterTraining,
					TrainedArt:    trainedArt,
					RareImgPath:   rareImgPath,
					TrainRank:     drawing.IntPtr(masterRank),
					Level:         drawing.IntPtr(level),
					IsPcard:       true,
				}),
				CharaID:         card.CharacterID,
				SkillLevel:      fmt.Sprintf("%d", deckCard.SkillLevel),
				IsAfterTraining: displayAfterTraining,
				SkillRate:       normalizeDeckDisplayRate(deckCard.SkillRate),
				EventBonusRate:  normalizeDeckDisplayRate(deckCard.EventBonusRate),
				IsBeforeStory:   deckCard.IsBeforeStory,
				IsAfterStory:    deckCard.IsAfterStory,
				HasCanvasBonus:  deckCard.HasCanvasBonus,
			})
		}

		if len(cardData) > 1 {
			teammates := cardData[1:]
			sort.SliceStable(teammates, func(i, j int) bool {
				left := deckInfo.Cards[i+1]
				right := deckInfo.Cards[j+1]
				if left.EventBonusRate != right.EventBonusRate {
					return left.EventBonusRate > right.EventBonusRate
				}
				if left.MasterRank != right.MasterRank {
					return left.MasterRank > right.MasterRank
				}
				if left.Level != right.Level {
					return left.Level > right.Level
				}
				return left.CardID > right.CardID
			})
		}

		deckItem := drawing.DeckData{
			CardData:             cardData,
			Score:                drawing.IntPtr(deckInfo.Score),
			LiveScore:            drawing.IntPtr(deckInfo.LiveScore),
			MySekaiEventPoint:    drawing.IntPtr(deckInfo.MysekaiEventPoint),
			EventBonusRate:       float64Ptr(normalizeDeckDisplayRate(deckInfo.EventBonusRate)),
			SupportDeckBonusRate: float64Ptr(normalizeDeckDisplayRate(deckInfo.SupportDeckBonusRate)),
			MultiLiveScoreUp:     float64Ptr(normalizeDeckDisplayRate(deckInfo.MultiLiveScoreUp)),
			TotalPower:           drawing.IntPtr(deckInfo.TotalPower),
			ChallengeScoreDelta:  drawing.IntPtr(deckInfo.ChallengeScoreDelta),
		}
		if query.MusicCompare && index < len(musicCompareSelections) {
			selection := musicCompareSelections[index]
			deckItem.MusicID = drawing.IntPtr(selection.MusicID)
			if strings.TrimSpace(selection.MusicDiff) != "" {
				deckItem.MusicDiff = drawing.StringPtr(selection.MusicDiff)
			}
			if strings.TrimSpace(selection.MusicTitle) != "" {
				deckItem.MusicTitle = drawing.StringPtr(selection.MusicTitle)
			}
			if strings.TrimSpace(selection.MusicCoverPath) != "" {
				deckItem.MusicCoverPath = drawing.StringPtr(selection.MusicCoverPath)
			}
		}
		deckData = append(deckData, deckItem)
	}

	target := "score"
	if value, ok := option["target"].(string); ok && strings.TrimSpace(value) != "" {
		target = value
	}
	request := &drawing.DeckRequest{
		Region:              region.String(),
		Profile:             *profile,
		DeckData:            deckData,
		MusicCompare:        query.MusicCompare,
		RecommendType:       recType,
		Target:              drawing.StringPtr(target),
		ModelName:           toInterfaceSlice(result.DeckAlgs),
		CostTimes:           toInterfaceMap(result.CostTimes),
		WaitTimes:           toInterfaceMap(result.WaitTimes),
		CanvasThumbnailPath: drawing.StringPtr(assets.ResolveRegionAssetPath(c.assets, region.String(), "mysekai/icon/category_icon/icon_canvas.png")),
	}

	c.applyOptionRequestFields(request, option, query)
	c.applyCommonRecommendMetadata(request, region, recType, metadataOption(option, recType, query), query)
	return request, nil
}

func metadataOption(option map[string]any, recType string, query AutoQuery) map[string]any {
	if recType != "mysekai" || option == nil {
		return option
	}
	if query.EventID != nil || optionHasSimulatedEvent(option) || optionInt(option, "event_id") <= 0 {
		return option
	}
	cloned := make(map[string]any, len(option))
	for key, value := range option {
		cloned[key] = value
	}
	delete(cloned, "event_id")
	return cloned
}

func normalizeDeckDisplayRate(value float64) float64 {
	rounded := math.Round(value*10) / 10
	if math.Abs(rounded-math.Round(rounded)) < 1e-9 {
		return math.Round(rounded)
	}
	return rounded
}

func resolveRecommendCardDisplayState(card *masterdata.Card, deckCard RecommendCard) (displayAfterTraining bool, trainedArt bool) {
	// For BFes/double-skill cards, deck-service uses default_image to report the
	// actual selected skill/art state. after_training only means the owned card is
	// trained; using it here would turn flower-before skills into flower-after art.
	trainedArt, hasDefaultImage := normalizeRecommendCardDefaultImage(deckCard.DefaultImage)
	displayAfterTraining = deckCard.IsAfterTraining
	if hasDefaultImage {
		displayAfterTraining = trainedArt
	} else {
		trainedArt = displayAfterTraining
	}
	return displayAfterTraining, trainedArt
}

func normalizeRecommendCardDefaultImage(raw string) (trainedArt bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "special_training", "after_training", "card_after_training", "trained":
		return true, true
	case "normal", "original", "before_training", "card_normal":
		return false, true
	default:
		return false, false
	}
}

func (c *Controller) cardByIDWithFallback(source CardSource, region renderregion.Value, cardID int) (*masterdata.Card, error) {
	if cardID == 0 || source == nil {
		return nil, fmt.Errorf("card not found: %d", cardID)
	}
	if card, err := source.GetCardByID(cardID); err == nil && card != nil && strings.TrimSpace(card.AssetBundleName) != "" {
		return card, nil
	}
	if c == nil || c.cardSources == nil || renderregion.WithDefault(region) == renderregion.JP {
		return nil, fmt.Errorf("card not found: %d", cardID)
	}
	jpSource, ok := c.cardSources.SourceForRegion(renderregion.JP)
	if !ok || jpSource == nil || jpSource == source {
		return nil, fmt.Errorf("card not found: %d", cardID)
	}
	card, err := jpSource.GetCardByID(cardID)
	if err != nil || card == nil || strings.TrimSpace(card.AssetBundleName) == "" {
		return nil, fmt.Errorf("card not found: %d", cardID)
	}
	return card, nil
}

func (c *Controller) applyOptionRequestFields(request *drawing.DeckRequest, option map[string]any, query AutoQuery) {
	if request == nil {
		return
	}
	if target := optionString(option, "target"); target != "" {
		request.Target = drawing.StringPtr(target)
	}
	if !query.MusicCompare {
		if musicID := optionInt(option, "music_id"); musicID > 0 {
			request.MusicID = drawing.IntPtr(musicID)
			if musicID == 10000 {
				request.MusicTitle = drawing.StringPtr("おまかせ (所有歌曲平均)")
				request.MusicCoverPath = drawing.StringPtr("static_images/omakase.png")
			}
		}
		if strings.TrimSpace(query.MusicTitle) != "" && request.MusicTitle == nil {
			request.MusicTitle = drawing.StringPtr(query.MusicTitle)
		}
		if strings.TrimSpace(query.MusicCoverPath) != "" && request.MusicCoverPath == nil {
			request.MusicCoverPath = drawing.StringPtr(query.MusicCoverPath)
		}
		if diff := optionString(option, "music_diff"); diff != "" {
			request.MusicDiff = drawing.StringPtr(diff)
		}
	}
	if option == nil {
		return
	}
	if teammatePower := optionInt(option, "multi_live_teammate_power"); teammatePower > 0 {
		request.MultiLiveTeammatePower = drawing.IntPtr(teammatePower)
	}
	if teammateScoreUp, ok := optionFloat(option, "multi_live_teammate_score_up"); ok {
		request.MultiLiveTeammateScoreUp = float64Ptr(teammateScoreUp)
	}
	if boost, ok := optionIntValue(option, "boost"); ok && boost >= 0 {
		request.Boost = drawing.IntPtr(boost)
	}
	if lowerBound, ok := optionFloat(option, "multi_live_score_up_lower_bound"); ok {
		request.MultiLiveScoreUpLowerBound = float64Ptr(lowerBound)
	}
	if strategy := optionString(option, "skill_order_choose_strategy"); strategy != "" {
		request.SkillOrderChooseStrategy = drawing.StringPtr(strategy)
	}
	if strategy := optionString(option, "skill_reference_choose_strategy"); strategy != "" {
		request.SkillReferenceChooseStrategy = drawing.StringPtr(strategy)
	}
	if unitFilter := optionString(option, "unit_filter"); unitFilter != "" {
		request.UnitFilter = drawing.StringPtr(unitFilter)
	}
	if attrFilter := optionString(option, "attr_filter"); attrFilter != "" {
		request.AttrFilter = drawing.StringPtr(attrFilter)
	}
	if excludedCards, ok := option["excluded_cards"].([]int); ok && len(excludedCards) > 0 {
		request.ExcludedCards = slices.Clone(excludedCards)
	}
	if fixedCards, ok := option["fixed_cards"].([]int); ok {
		request.FixedCardsID = slices.Clone(fixedCards)
	}
	if fixedCharacters, ok := option["fixed_characters"].([]int); ok {
		request.FixedCharactersID = slices.Clone(fixedCharacters)
	}
	if maxProfile, ok := option["max_profile"].(bool); ok {
		request.IsMaxDeck = maxProfile
	}
	if keepAfterTrainingState, ok := option["keep_after_training_state"].(bool); ok {
		request.KeepAfterTrainingState = keepAfterTrainingState
	}
}
