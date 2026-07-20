package deck

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
)

func (c *Controller) applyProfilePreset(region renderregion.Value, raw *snapshot.RawUserData, query AutoQuery) error {
	if raw == nil || (!query.MaxProfile && !query.SubMaxProfile) {
		return nil
	}

	userCards, err := c.buildMaxProfileCards(region, raw.Now)
	if err != nil {
		return err
	}
	raw.UserCards = userCards
	raw.UserMysekaiCanvases = buildMaxProfileCanvases(userCards)

	if _, source, err := c.resolveCardSource(region); err == nil {
		if mysekaiSource, ok := source.(maxProfileMySekaiSource); ok {
			raw.UserMysekaiGates = slices.Clone(mysekaiSource.GetMaxProfileMysekaiGates())
			raw.UserMysekaiFixtureGameCharacterPerformanceBonuses = slices.Clone(mysekaiSource.GetMaxProfileMysekaiFixtureBonuses())
		}
	}
	c.applyMaxProfileEventHonors(region, raw, query)

	switch {
	case query.MaxProfile:
		c.applyAreaItemCaps(region, raw, 0)
	case query.SubMaxProfile:
		c.applyAreaItemCaps(region, raw, 15)
	}
	return nil
}

func (c *Controller) buildMaxProfileCards(region renderregion.Value, rawNow int64) ([]snapshot.RawUserCard, error) {
	_, source, err := c.resolveCardSource(region)
	if err != nil {
		return nil, err
	}

	allSource, ok := source.(allCardSource)
	if !ok {
		return nil, fmt.Errorf("deck card source does not support max profile construction")
	}

	allCards, err := allSource.GetAllCards()
	if err != nil {
		return nil, err
	}

	now := rawNow
	if now <= 0 {
		now = time.Now().UnixMilli()
	}

	eligibleCards := make([]*masterdata.Card, 0, len(allCards))
	for _, card := range allCards {
		if card == nil || card.ID <= 0 {
			continue
		}
		if card.ReleaseAt > 0 && card.ReleaseAt > now {
			continue
		}
		eligibleCards = append(eligibleCards, card)
	}

	episodeSource, _ := source.(cardEpisodeSource)
	episodesByCard, err := func() (map[int][]snapshot.RawUserCardEpisode, error) {
		finish := commandtrace.MeasureOperation(c.contextOrBackground(), "deck.max_profile.episodes")
		defer finish()
		result := make(map[int][]snapshot.RawUserCardEpisode, len(eligibleCards))
		for _, card := range eligibleCards {
			episodes, err := maxProfileCardEpisodes(episodeSource, card.ID)
			if err != nil {
				return nil, err
			}
			result[card.ID] = episodes
		}
		return result, nil
	}()
	if err != nil {
		return nil, err
	}

	result := make([]snapshot.RawUserCard, 0, len(eligibleCards))
	for _, card := range eligibleCards {
		result = append(result, snapshot.RawUserCard{
			CardID:                card.ID,
			Level:                 maxProfileCardLevel(card.CardRarityType),
			SkillLevel:            4,
			MasterRank:            5,
			SpecialTrainingStatus: maxProfileTrainingStatus(card),
			DefaultImage:          maxProfileDefaultImage(card),
			Episodes:              episodesByCard[card.ID],
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CardID < result[j].CardID
	})
	return result, nil
}

func buildMaxProfileCanvases(cards []snapshot.RawUserCard) []snapshot.RawUserMysekaiCanvas {
	if len(cards) == 0 {
		return nil
	}
	canvases := make([]snapshot.RawUserMysekaiCanvas, 0, len(cards))
	for _, card := range cards {
		if card.CardID <= 0 {
			continue
		}
		canvases = append(canvases, snapshot.RawUserMysekaiCanvas{
			CardID:   card.CardID,
			Quantity: 1,
		})
	}
	return canvases
}

func maxProfileCardEpisodes(source cardEpisodeSource, cardID int) ([]snapshot.RawUserCardEpisode, error) {
	if source == nil || cardID == 0 {
		return nil, nil
	}
	episodes, err := source.GetCardEpisodes(cardID)
	if err != nil {
		return nil, err
	}
	return slices.Clone(episodes), nil
}

func maxProfileCardLevel(rarity string) int {
	switch strings.ToLower(strings.TrimSpace(rarity)) {
	case "rarity_1":
		return 20
	case "rarity_2":
		return 30
	case "rarity_3":
		return 50
	default:
		return 60
	}
}

func maxProfileTrainingStatus(card *masterdata.Card) string {
	if cardHasAfterTraining(card) {
		return "done"
	}
	return "none"
}

func maxProfileDefaultImage(card *masterdata.Card) string {
	if cardHasAfterTraining(card) {
		return "special_training"
	}
	return "normal"
}

func cardHasAfterTraining(card *masterdata.Card) bool {
	if card == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(card.CardRarityType)) {
	case "rarity_3", "rarity_4":
		return true
	default:
		return false
	}
}

func (c *Controller) applyUserCardFilters(region renderregion.Value, raw *snapshot.RawUserData, query AutoQuery) error {
	if raw == nil {
		return nil
	}
	unitFilter := normalizeRecommendUnit(query.UnitFilter)
	attrFilter := normalizeRecommendAttr(query.AttrFilter)
	if unitFilter == "" && attrFilter == "" && len(query.ExcludedCards) == 0 {
		return nil
	}

	_, source, err := c.resolveCardSource(region)
	if err != nil {
		return err
	}

	excluded := make(map[int]struct{}, len(query.ExcludedCards))
	for _, cardID := range query.ExcludedCards {
		if cardID > 0 {
			excluded[cardID] = struct{}{}
		}
	}

	filtered := make([]snapshot.RawUserCard, 0, len(raw.UserCards))
	for _, userCard := range raw.UserCards {
		if _, ok := excluded[userCard.CardID]; ok {
			continue
		}

		cardInfo, err := source.GetCardByID(userCard.CardID)
		if err != nil || cardInfo == nil {
			if unitFilter == "" && attrFilter == "" {
				filtered = append(filtered, userCard)
			}
			continue
		}

		if unitFilter != "" {
			unit := c.resolveCardUnit(source, cardInfo, unitFilter)
			if unit != unitFilter {
				continue
			}
		}
		if attrFilter != "" && normalizeRecommendAttr(cardInfo.Attr) != attrFilter {
			continue
		}
		filtered = append(filtered, userCard)
	}

	raw.UserCards = filtered
	return nil
}

func (c *Controller) resolveCardUnit(source CardSource, card *masterdata.Card, unitFilter string) string {
	if source == nil || card == nil {
		return ""
	}

	if unitFilter != "piapro" {
		if resolver, ok := source.(cardUnitSource); ok {
			if unit, err := resolver.GetUnitByCardID(card.ID); err == nil {
				return normalizeRecommendUnit(unit)
			}
		}
		if unit := normalizeRecommendUnit(card.SupportUnit); unit != "" && unit != "piapro" {
			return unit
		}
	}

	if resolver, ok := source.(characterSource); ok {
		if character, err := resolver.GetCharacterByID(card.CharacterID); err == nil && character != nil {
			return normalizeRecommendUnit(character.Unit)
		}
	}
	return ""
}

func (c *Controller) applyMaxProfileEventHonors(region renderregion.Value, raw *snapshot.RawUserData, query AutoQuery) {
	if raw == nil || query.EventID == nil || *query.EventID != 180 {
		return
	}

	_, source, ok := c.resolveEventSource(region)
	if !ok || source == nil {
		return
	}

	rewardSource, ok := source.(eventRankingHonorSource)
	if !ok {
		return
	}

	rewards, err := rewardSource.GetEventRankingHonorRewards(*query.EventID)
	if err != nil {
		return
	}

	reward, ok := pickMaxProfileRankingHonorReward(rewards, 1000)
	if !ok || reward.HonorID <= 0 {
		return
	}

	raw.UserHonors = upsertMaxProfileUserHonor(raw.UserHonors, reward.HonorID)
	raw.UserProfileHonors = upsertMaxProfileProfileHonor(raw.UserProfileHonors, reward.HonorID)
}

func pickMaxProfileRankingHonorReward(rewards []masterdata.EventRankingHonorReward, targetRank int) (masterdata.EventRankingHonorReward, bool) {
	if len(rewards) == 0 {
		return masterdata.EventRankingHonorReward{}, false
	}

	bestIndex := -1
	bestScore := -1
	for i, reward := range rewards {
		if reward.HonorID <= 0 {
			continue
		}
		score := 0
		switch {
		case reward.ToRank == targetRank:
			score = 3
		case rewardContainsRank(reward, targetRank):
			score = 2
		default:
			score = 1
		}
		if score > bestScore {
			bestIndex = i
			bestScore = score
			continue
		}
		if score == bestScore && bestIndex >= 0 && rankingHonorRewardLess(reward, rewards[bestIndex]) {
			bestIndex = i
		}
	}
	if bestIndex < 0 {
		return masterdata.EventRankingHonorReward{}, false
	}
	return rewards[bestIndex], true
}

func rewardContainsRank(reward masterdata.EventRankingHonorReward, rank int) bool {
	if rank <= 0 {
		return false
	}
	if reward.FromRank > 0 && rank < reward.FromRank {
		return false
	}
	if reward.ToRank > 0 && rank > reward.ToRank {
		return false
	}
	return true
}

func rankingHonorRewardLess(left, right masterdata.EventRankingHonorReward) bool {
	leftTo := left.ToRank
	rightTo := right.ToRank
	if leftTo <= 0 {
		leftTo = int(^uint(0) >> 1)
	}
	if rightTo <= 0 {
		rightTo = int(^uint(0) >> 1)
	}
	if leftTo != rightTo {
		return leftTo < rightTo
	}
	if left.FromRank != right.FromRank {
		return left.FromRank < right.FromRank
	}
	return left.HonorID < right.HonorID
}

func upsertMaxProfileUserHonor(items []snapshot.RawUserHonor, honorID int) []snapshot.RawUserHonor {
	if honorID <= 0 {
		return items
	}

	result := slices.Clone(items)
	maxSeq := 0
	for i := range result {
		if result[i].HonorID == honorID {
			result[i].HonorLevel = 1
			result[i].ProfilePlayer = true
			return result
		}
		if result[i].Seq > maxSeq {
			maxSeq = result[i].Seq
		}
	}

	result = append(result, snapshot.RawUserHonor{
		Seq:           maxSeq + 1,
		HonorID:       honorID,
		HonorLevel:    1,
		ProfilePlayer: true,
	})
	return result
}

func upsertMaxProfileProfileHonor(items []snapshot.RawUserProfileHonor, honorID int) []snapshot.RawUserProfileHonor {
	if honorID <= 0 {
		return items
	}

	result := make([]snapshot.RawUserProfileHonor, 0, len(items)+1)
	replaced := false
	for _, item := range items {
		if item.Seq == 1 {
			result = append(result, snapshot.RawUserProfileHonor{
				Seq:              1,
				ProfileHonorType: "normal",
				HonorID:          honorID,
				HonorLevel:       1,
			})
			replaced = true
			continue
		}
		result = append(result, item)
	}
	if !replaced {
		result = append(result, snapshot.RawUserProfileHonor{
			Seq:              1,
			ProfileHonorType: "normal",
			HonorID:          honorID,
			HonorLevel:       1,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Seq < result[j].Seq
	})
	return result
}

func publicProfileCurrentDeck(resp *sekai.GetAnotherProfileResponse) *snapshot.RawUserDeck {
	if resp == nil {
		return nil
	}
	deck := resp.UserDeck
	if deck.DeckID <= 0 && deck.Member1 <= 0 && deck.Member2 <= 0 && deck.Member3 <= 0 && deck.Member4 <= 0 && deck.Member5 <= 0 {
		return nil
	}
	return &snapshot.RawUserDeck{
		DeckID:    deck.DeckID,
		Leader:    deck.Leader,
		SubLeader: deck.SubLeader,
		Member1:   deck.Member1,
		Member2:   deck.Member2,
		Member3:   deck.Member3,
		Member4:   deck.Member4,
		Member5:   deck.Member5,
	}
}

func (c *Controller) applyCurrentDeckOption(_ *snapshot.RawUserData, original *snapshot.RawUserData, recType string, query AutoQuery, option map[string]any) error {
	if !query.UseCurrentDeck || recType == "challenge" {
		return nil
	}

	if deckInfo := publicProfileCurrentDeck(query.PublicProfileResp); deckInfo != nil {
		cards, ok := currentDeckFixedCardIDs(deckInfo)
		if !ok {
			return fmt.Errorf("你的当前主队不足5张，无法使用\"当前\"参数")
		}

		option["fixed_cards"] = slices.Clone(cards)
		delete(option, "fixed_characters")
		option["best_skill_as_leader"] = false
		return nil
	}
	if original == nil {
		return fmt.Errorf("raw user snapshot is unavailable")
	}

	deckInfo := snapshot.FindActiveDeck(original.UserDecks, original.UserGamedata.Deck)
	if deckInfo.DeckID == 0 {
		return fmt.Errorf("找不到你的当前主队配置")
	}

	cards, ok := currentDeckFixedCardIDs(&deckInfo)
	if !ok {
		return fmt.Errorf("你的当前主队不足5张，无法使用\"当前\"参数")
	}

	option["fixed_cards"] = slices.Clone(cards)
	delete(option, "fixed_characters")
	option["best_skill_as_leader"] = false
	return nil
}

func currentDeckFixedCardIDs(deck *snapshot.RawUserDeck) ([]int, bool) {
	cards, ok := snapshot.UserDeckCardIDs(deck)
	if !ok || deck == nil || deck.Leader <= 0 {
		return cards, ok
	}
	leaderIndex := slices.Index(cards, deck.Leader)
	if leaderIndex <= 0 {
		return cards, ok
	}
	ordered := make([]int, 0, len(cards))
	ordered = append(ordered, deck.Leader)
	ordered = append(ordered, cards[:leaderIndex]...)
	ordered = append(ordered, cards[leaderIndex+1:]...)
	return ordered, true
}

func (c *Controller) restoreFixedCards(region renderregion.Value, raw *snapshot.RawUserData, original *snapshot.RawUserData, option map[string]any, preferOriginal bool) error {
	if raw == nil || original == nil || option == nil {
		return nil
	}

	fixedCards, ok := option["fixed_cards"].([]int)
	if !ok || len(fixedCards) == 0 {
		return nil
	}

	indexByCardID := make(map[int]int, len(raw.UserCards))
	for index := range raw.UserCards {
		indexByCardID[raw.UserCards[index].CardID] = index
	}

	for _, cardID := range fixedCards {
		if cardID <= 0 {
			continue
		}
		originalCard := snapshot.FindUserCard(original.UserCards, cardID)
		if originalCard == nil {
			if _, ok := indexByCardID[cardID]; ok {
				continue
			}
			fallbackCard, err := c.buildFallbackRecommendUserCard(region, cardID, raw.Now, preferOriginal)
			if err != nil {
				return err
			}
			if fallbackCard == nil {
				return fmt.Errorf("当前卡组中的卡牌 %d 不在抓包数据中，请更新抓包数据", cardID)
			}
			raw.UserCards = append(raw.UserCards, *fallbackCard)
			indexByCardID[cardID] = len(raw.UserCards) - 1
			continue
		}

		if index, ok := indexByCardID[cardID]; ok {
			if preferOriginal {
				raw.UserCards[index] = *originalCard
			}
			continue
		}

		raw.UserCards = append(raw.UserCards, *originalCard)
		indexByCardID[cardID] = len(raw.UserCards) - 1
	}

	return nil
}

func (c *Controller) buildFallbackRecommendUserCard(region renderregion.Value, cardID int, rawNow int64, preferOwnedState bool) (*snapshot.RawUserCard, error) {
	if cardID <= 0 {
		return nil, nil
	}

	_, source, err := c.resolveCardSource(region)
	if err != nil {
		return nil, err
	}

	cardInfo, err := source.GetCardByID(cardID)
	if err != nil {
		return nil, err
	}
	if cardInfo == nil {
		return nil, nil
	}

	episodeSource, _ := source.(cardEpisodeSource)
	episodes, err := maxProfileCardEpisodes(episodeSource, cardID)
	if err != nil {
		return nil, err
	}

	result := &snapshot.RawUserCard{
		CardID:                cardInfo.ID,
		Level:                 maxProfileCardLevel(cardInfo.CardRarityType),
		SpecialTrainingStatus: maxProfileTrainingStatus(cardInfo),
		DefaultImage:          maxProfileDefaultImage(cardInfo),
		Episodes:              episodes,
	}
	if preferOwnedState {
		result.SkillLevel = 4
		result.MasterRank = 5
	} else {
		result.SkillLevel = 1
		result.MasterRank = 0
	}
	return result, nil
}

func (c *Controller) applyAreaItemCaps(region renderregion.Value, raw *snapshot.RawUserData, limit int) {
	if raw == nil {
		return
	}
	caps := c.areaItemLevelCaps(region, limit)
	if len(caps) == 0 {
		return
	}

	levels := make(map[int]int, len(caps))
	for itemID, level := range caps {
		if level > 0 {
			levels[itemID] = level
		}
	}
	raw.UserAreas = buildRawUserAreas(levels)
}

func (c *Controller) applyAreaItemLevel(region renderregion.Value, raw *snapshot.RawUserData, targetLevel int) error {
	if raw == nil || targetLevel <= 0 {
		return nil
	}

	caps := c.areaItemLevelCaps(region, targetLevel)
	if len(caps) == 0 {
		if len(raw.UserAreas) == 0 {
			return nil
		}
		levels := collectRawAreaItemLevels(raw.UserAreas)
		for itemID, level := range levels {
			if level < targetLevel {
				levels[itemID] = targetLevel
			}
		}
		raw.UserAreas = buildRawUserAreas(levels)
		return nil
	}

	for _, level := range caps {
		if level > 0 && level < targetLevel {
			return fmt.Errorf("该区服区域道具等级最多为%d", level)
		}
	}

	levels := collectRawAreaItemLevels(raw.UserAreas)
	for itemID, level := range caps {
		if level > levels[itemID] {
			levels[itemID] = level
		}
	}
	raw.UserAreas = buildRawUserAreas(levels)
	return nil
}

func (c *Controller) areaItemLevelCaps(region renderregion.Value, limit int) map[int]int {
	_, source, err := c.resolveCardSource(region)
	if err != nil {
		return nil
	}

	resolver, ok := source.(areaItemLevelCapSource)
	if !ok {
		return nil
	}

	caps := resolver.AreaItemLevelCaps(limit)
	if len(caps) == 0 {
		return nil
	}
	return caps
}

func collectRawAreaItemLevels(areas []snapshot.RawUserArea) map[int]int {
	levels := make(map[int]int)
	for _, area := range areas {
		for _, item := range area.AreaItems {
			if item.AreaItemID <= 0 {
				continue
			}
			if item.Level > levels[item.AreaItemID] {
				levels[item.AreaItemID] = item.Level
			}
		}
	}
	return levels
}

func buildRawUserAreas(levels map[int]int) []snapshot.RawUserArea {
	if len(levels) == 0 {
		return nil
	}

	itemIDs := make([]int, 0, len(levels))
	for itemID, level := range levels {
		if itemID > 0 && level > 0 {
			itemIDs = append(itemIDs, itemID)
		}
	}
	sort.Ints(itemIDs)

	items := make([]snapshot.RawUserAreaItem, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		items = append(items, snapshot.RawUserAreaItem{
			AreaItemID: itemID,
			Level:      levels[itemID],
		})
	}
	if len(items) == 0 {
		return nil
	}
	return []snapshot.RawUserArea{{AreaItems: items}}
}
