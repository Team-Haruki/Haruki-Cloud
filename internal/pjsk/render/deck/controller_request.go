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
	userCardMap := c.recommendUserCardMap(preparedRaw)
	deckData := c.buildRecommendDeckData(cardSource, region, result.Decks, userCardMap, query, musicCompareSelections)

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

func (c *Controller) recommendUserCardMap(preparedRaw *snapshot.RawUserData) map[int]snapshot.RawUserCard {
	userCards := []snapshot.RawUserCard(nil)
	if preparedRaw != nil {
		userCards = preparedRaw.UserCards
	} else if raw := c.snapshot.RawData(); raw != nil {
		userCards = raw.UserCards
	}
	result := make(map[int]snapshot.RawUserCard, len(userCards))
	for _, userCard := range userCards {
		result[userCard.CardID] = userCard
	}
	return result
}

func (c *Controller) buildRecommendDeckData(
	cardSource CardSource,
	region renderregion.Value,
	decks []RecommendDeck,
	userCards map[int]snapshot.RawUserCard,
	query AutoQuery,
	musicCompareSelections []MusicCompareSelection,
) []drawing.DeckData {
	result := make([]drawing.DeckData, 0, len(decks))
	for index, deckInfo := range decks {
		cardData := c.buildRecommendDeckCards(cardSource, region, deckInfo.Cards, userCards)
		sortRecommendDeckTeammates(cardData, deckInfo.Cards)
		deckItem := recommendDeckDrawingData(deckInfo, cardData)
		applyMusicCompareDrawingData(&deckItem, query, index, musicCompareSelections)
		result = append(result, deckItem)
	}
	return result
}

func (c *Controller) buildRecommendDeckCards(cardSource CardSource, region renderregion.Value, cards []RecommendCard, userCards map[int]snapshot.RawUserCard) []drawing.DeckCardData {
	result := make([]drawing.DeckCardData, 0, len(cards))
	for _, deckCard := range cards {
		card, err := c.cardByIDWithFallback(cardSource, region, deckCard.CardID)
		if err != nil || card == nil {
			continue
		}
		result = append(result, c.recommendDeckCardDrawingData(region, card, deckCard, userCards[deckCard.CardID]))
	}
	return result
}

func (c *Controller) recommendDeckCardDrawingData(region renderregion.Value, card *masterdata.Card, deckCard RecommendCard, userCard snapshot.RawUserCard) drawing.DeckCardData {
	displayAfterTraining, trainedArt := resolveRecommendCardDisplayState(card, deckCard)
	level := deckCard.Level
	if level <= 0 {
		level = 60
	}
	rareImgPath := ""
	if strings.EqualFold(userCard.SpecialTrainingStatus, "done") {
		rareImgPath = common.ResolveCardRareImagePath(c.assets, card, true)
	}
	return drawing.DeckCardData{
		CardThumbnail: common.BuildCardThumbnail(c.assets, card, region, common.ThumbnailOptions{
			AfterTraining: displayAfterTraining,
			TrainedArt:    trainedArt,
			RareImgPath:   rareImgPath,
			TrainRank:     drawing.IntPtr(deckCard.MasterRank),
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
	}
}

func sortRecommendDeckTeammates(cardData []drawing.DeckCardData, cards []RecommendCard) {
	if len(cardData) <= 1 {
		return
	}
	teammates := cardData[1:]
	sort.SliceStable(teammates, func(i, j int) bool {
		left := cards[i+1]
		right := cards[j+1]
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

func recommendDeckDrawingData(deck RecommendDeck, cards []drawing.DeckCardData) drawing.DeckData {
	return drawing.DeckData{
		CardData:             cards,
		Score:                drawing.IntPtr(deck.Score),
		LiveScore:            drawing.IntPtr(deck.LiveScore),
		MySekaiEventPoint:    drawing.IntPtr(deck.MysekaiEventPoint),
		EventBonusRate:       float64Ptr(normalizeDeckDisplayRate(deck.EventBonusRate)),
		SupportDeckBonusRate: float64Ptr(normalizeDeckDisplayRate(deck.SupportDeckBonusRate)),
		MultiLiveScoreUp:     float64Ptr(normalizeDeckDisplayRate(deck.MultiLiveScoreUp)),
		TotalPower:           drawing.IntPtr(deck.TotalPower),
		ChallengeScoreDelta:  drawing.IntPtr(deck.ChallengeScoreDelta),
	}
}

func applyMusicCompareDrawingData(deck *drawing.DeckData, query AutoQuery, index int, selections []MusicCompareSelection) {
	if !query.MusicCompare || index >= len(selections) {
		return
	}
	selection := selections[index]
	deck.MusicID = drawing.IntPtr(selection.MusicID)
	if strings.TrimSpace(selection.MusicDiff) != "" {
		deck.MusicDiff = drawing.StringPtr(selection.MusicDiff)
	}
	if strings.TrimSpace(selection.MusicTitle) != "" {
		deck.MusicTitle = drawing.StringPtr(selection.MusicTitle)
	}
	if strings.TrimSpace(selection.MusicCoverPath) != "" {
		deck.MusicCoverPath = drawing.StringPtr(selection.MusicCoverPath)
	}
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
	applyRecommendMusicRequestFields(request, option, query)
	if option == nil {
		return
	}
	applyRecommendScoringRequestFields(request, option)
	applyRecommendStrategyRequestFields(request, option)
	applyRecommendDeckSelectionRequestFields(request, option)
}

func applyRecommendMusicRequestFields(request *drawing.DeckRequest, option map[string]any, query AutoQuery) {
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
}

func applyRecommendScoringRequestFields(request *drawing.DeckRequest, option map[string]any) {
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
}

func applyRecommendStrategyRequestFields(request *drawing.DeckRequest, option map[string]any) {
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
}

func applyRecommendDeckSelectionRequestFields(request *drawing.DeckRequest, option map[string]any) {
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
