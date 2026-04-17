package deck

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
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

	result := make([]snapshot.RawUserCard, 0, len(allCards))
	for _, card := range allCards {
		if card == nil || card.ID <= 0 {
			continue
		}
		if card.ReleaseAt > 0 && card.ReleaseAt > now {
			continue
		}
		result = append(result, snapshot.RawUserCard{
			CardID:                card.ID,
			Level:                 maxProfileCardLevel(card.CardRarityType),
			SkillLevel:            4,
			MasterRank:            5,
			SpecialTrainingStatus: maxProfileTrainingStatus(card),
			DefaultImage:          maxProfileDefaultImage(card),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CardID < result[j].CardID
	})
	return result, nil
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

func (c *Controller) applyCurrentDeckOption(_ *snapshot.RawUserData, original *snapshot.RawUserData, recType string, query AutoQuery, option map[string]any) error {
	if !query.UseCurrentDeck || recType == "challenge" {
		return nil
	}
	if original == nil {
		return fmt.Errorf("raw user snapshot is unavailable")
	}

	deckInfo := snapshot.FindActiveDeck(original.UserDecks, original.UserGamedata.Deck)
	if deckInfo.DeckID == 0 {
		return fmt.Errorf("找不到你的当前主队配置（更新当前主队需要抓包）")
	}

	cards, ok := snapshot.UserDeckCardIDs(&deckInfo)
	if !ok {
		return fmt.Errorf("你的当前主队不足5张，无法使用\"当前\"参数（更新当前主队需要抓包）")
	}

	option["fixed_cards"] = append([]int(nil), cards...)
	delete(option, "fixed_characters")
	option["best_skill_as_leader"] = false
	return nil
}

func (c *Controller) restoreFixedCards(raw *snapshot.RawUserData, original *snapshot.RawUserData, option map[string]any, preferOriginal bool) error {
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
			return fmt.Errorf("当前卡组中的卡牌 %d 不在抓包数据中，请更新抓包数据", cardID)
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
