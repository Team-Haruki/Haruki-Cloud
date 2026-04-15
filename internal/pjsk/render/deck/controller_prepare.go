package deck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/logger"
)

var deckPrepareDebugLogger = logger.NewLoggerFromGlobal("DeckPrepare")

type allCardSource interface {
	GetAllCards() ([]*masterdata.Card, error)
}

type areaItemLevelCapSource interface {
	AreaItemLevelCaps(limit int) map[int]int
}

type cardUnitSource interface {
	GetUnitByCardID(cardID int) (string, error)
}

type characterSource interface {
	GetCharacterByID(id int) (*masterdata.Character, error)
}

func (c *Controller) prepareRecommendUserData(region renderregion.Value, recType string, query AutoQuery, option map[string]any) (*userdata.RawUserData, []byte, error) {
	original := c.snapshot.RawData()
	if original == nil {
		return nil, nil, fmt.Errorf("raw user snapshot is unavailable")
	}
	originalBytes, err := c.snapshot.RawBytes()
	if err != nil {
		return nil, nil, err
	}

	if !shouldPrepareRecommendUserData(query) {
		return original, originalBytes, nil
	}

	raw, err := userdata.CloneRawUserData(original)
	if err != nil {
		return nil, nil, err
	}
	if raw == nil {
		return nil, nil, fmt.Errorf("raw user snapshot is unavailable")
	}

	if err := c.applyProfilePreset(region, raw, query); err != nil {
		return nil, nil, err
	}
	if err := c.applyUserCardFilters(region, raw, query); err != nil {
		return nil, nil, err
	}
	if err := c.applyCurrentDeckOption(raw, original, recType, query, option); err != nil {
		return nil, nil, err
	}
	if err := c.restoreFixedCards(raw, original, option, query.UseCurrentDeck); err != nil {
		return nil, nil, err
	}
	if err := c.applyAreaItemLevel(region, raw, optionInt(option, "area_item_level")); err != nil {
		return nil, nil, err
	}

	userBytes, err := encodePreparedRecommendUserData(originalBytes, original, raw)
	if err != nil {
		return nil, nil, err
	}
	logPreparedRecommendUserData(region, recType, query, raw, originalBytes, userBytes)
	return raw, userBytes, nil
}

func encodePreparedRecommendUserData(originalBytes []byte, original, raw *userdata.RawUserData) ([]byte, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw user snapshot is unavailable")
	}
	if len(bytes.TrimSpace(originalBytes)) == 0 {
		return userdata.EncodeRawUserData(raw)
	}

	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(originalBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode original user snapshot: %w", err)
	}
	if payload == nil {
		return userdata.EncodeRawUserData(raw)
	}

	var originalCards []userdata.RawUserCard
	if original != nil {
		originalCards = original.UserCards
	}
	userCards, err := mergePreparedUserCards(payload["userCards"], originalCards, raw.UserCards)
	if err != nil {
		return nil, err
	}
	userAreas, err := mergePreparedUserAreas(payload["userAreas"], raw.UserAreas)
	if err != nil {
		return nil, err
	}

	payload["userCards"] = userCards
	payload["userAreas"] = userAreas

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode prepared user snapshot: %w", err)
	}
	return encoded, nil
}

func mergePreparedUserCards(original any, originalCards, cards []userdata.RawUserCard) ([]any, error) {
	if len(cards) == 0 {
		return []any{}, nil
	}

	originalItems := jsonArrayToObjects(original)
	originalByCardID := make(map[int]map[string]any, len(originalItems))
	for _, item := range originalItems {
		cardID, _ := jsonNumberToInt(item["cardId"])
		if cardID > 0 {
			originalByCardID[cardID] = item
		}
	}

	merged := make([]any, 0, len(cards))
	for _, card := range cards {
		if existing := originalByCardID[card.CardID]; existing != nil {
			if samePreparedUserCard(userdata.FindUserCard(originalCards, card.CardID), &card) {
				cardMap := copyJSONObject(existing)
				normalizePreparedUserCardJSON(cardMap)
				merged = append(merged, cardMap)
				continue
			}
		}

		cardMap, err := structToJSONObject(card)
		if err != nil {
			return nil, fmt.Errorf("encode prepared user card %d: %w", card.CardID, err)
		}
		if existing := originalByCardID[card.CardID]; existing != nil {
			cardMap = mergeJSONObjects(existing, cardMap)
		}
		normalizePreparedUserCardJSON(cardMap)
		merged = append(merged, cardMap)
	}
	return merged, nil
}

func mergePreparedUserAreas(original any, areas []userdata.RawUserArea) ([]any, error) {
	if len(areas) == 0 {
		return []any{}, nil
	}

	originalAreas := jsonArrayToObjects(original)
	merged := make([]any, 0, len(areas))
	for i := range areas {
		areaMap, err := structToJSONObject(areas[i])
		if err != nil {
			return nil, fmt.Errorf("encode prepared user area %d: %w", i, err)
		}
		if existing := objectAt(originalAreas, i); existing != nil {
			areaMap = mergeJSONObjects(existing, areaMap)
			areaItems, err := mergePreparedAreaItems(existing["areaItems"], areas[i].AreaItems)
			if err != nil {
				return nil, err
			}
			areaMap["areaItems"] = areaItems
		} else {
			areaItems, err := mergePreparedAreaItems(nil, areas[i].AreaItems)
			if err != nil {
				return nil, err
			}
			areaMap["areaItems"] = areaItems
		}
		normalizePreparedUserAreaJSON(areaMap)
		merged = append(merged, areaMap)
	}
	return merged, nil
}

func mergePreparedAreaItems(original any, items []userdata.RawUserAreaItem) ([]any, error) {
	if len(items) == 0 {
		return []any{}, nil
	}

	originalItems := jsonArrayToObjects(original)
	originalByItemID := make(map[int]map[string]any, len(originalItems))
	for _, item := range originalItems {
		itemID, _ := jsonNumberToInt(item["areaItemId"])
		if itemID > 0 {
			originalByItemID[itemID] = item
		}
	}

	merged := make([]any, 0, len(items))
	for _, item := range items {
		itemMap, err := structToJSONObject(item)
		if err != nil {
			return nil, fmt.Errorf("encode prepared area item %d: %w", item.AreaItemID, err)
		}
		if existing := originalByItemID[item.AreaItemID]; existing != nil {
			itemMap = mergeJSONObjects(existing, itemMap)
		}
		merged = append(merged, itemMap)
	}
	return merged, nil
}

func structToJSONObject(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return map[string]any{}, nil
	}
	return object, nil
}

func jsonArrayToObjects(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, _ := item.(map[string]any)
		if object != nil {
			result = append(result, object)
		}
	}
	return result
}

func objectAt(items []map[string]any, index int) map[string]any {
	if index < 0 || index >= len(items) {
		return nil
	}
	return items[index]
}

func mergeJSONObjects(base, overlay map[string]any) map[string]any {
	if len(base) == 0 {
		return copyJSONObject(overlay)
	}
	merged := copyJSONObject(base)
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func copyJSONObject(source map[string]any) map[string]any {
	if len(source) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func jsonNumberToInt(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	default:
		return 0, false
	}
}

func samePreparedUserCard(original, current *userdata.RawUserCard) bool {
	if original == nil || current == nil {
		return false
	}
	if original.CardID != current.CardID {
		return false
	}
	if original.Level != current.Level ||
		original.SkillLevel != current.SkillLevel ||
		original.MasterRank != current.MasterRank ||
		original.SpecialTrainingStatus != current.SpecialTrainingStatus ||
		original.DefaultImage != current.DefaultImage {
		return false
	}
	if len(original.Episodes) != len(current.Episodes) {
		return false
	}
	for i := range original.Episodes {
		if original.Episodes[i] != current.Episodes[i] {
			return false
		}
	}
	return true
}

func normalizePreparedUserCardJSON(card map[string]any) {
	if card == nil {
		return
	}
	if value, ok := card["episodes"]; ok && value == nil {
		delete(card, "episodes")
	}
}

func normalizePreparedUserAreaJSON(area map[string]any) {
	if area == nil {
		return
	}
	if value, ok := area["userAreaStatus"]; !ok || value == nil {
		area["userAreaStatus"] = map[string]any{}
	}
}

func logPreparedRecommendUserData(region renderregion.Value, recType string, query AutoQuery, raw *userdata.RawUserData, originalBytes, preparedBytes []byte) {
	if raw == nil {
		return
	}

	removedKeys, nullPaths, nestedCardRemoved, nestedAreaRemoved, summaryErr := summarizePreparedRecommendUserData(originalBytes, preparedBytes)
	areaItemLevel := 0
	if query.AreaItemLevel != nil {
		areaItemLevel = *query.AreaItemLevel
	}

	deckPrepareDebugLogger.Debugf(
		"prepared userdata summary: user_id=%d nickname=%q region=%s recommend_type=%s unit_filter=%q attr_filter=%q excluded_cards=%d use_current_deck=%t max_profile=%t sub_max_profile=%t area_item_level=%d original_bytes=%d prepared_bytes=%d user_cards=%d user_areas=%d removed_keys=%v null_paths=%v removed_userCard_keys=%v removed_areaItem_keys=%v summary_err=%v",
		raw.UserGamedata.UserID,
		strings.TrimSpace(raw.UserGamedata.Name),
		region.String(),
		strings.TrimSpace(recType),
		strings.TrimSpace(query.UnitFilter),
		strings.TrimSpace(query.AttrFilter),
		len(query.ExcludedCards),
		query.UseCurrentDeck,
		query.MaxProfile,
		query.SubMaxProfile,
		areaItemLevel,
		len(originalBytes),
		len(preparedBytes),
		len(raw.UserCards),
		len(raw.UserAreas),
		removedKeys,
		nullPaths,
		nestedCardRemoved,
		nestedAreaRemoved,
		summaryErr,
	)
}

func summarizePreparedRecommendUserData(originalBytes, preparedBytes []byte) ([]string, []string, []string, []string, error) {
	var original map[string]any
	var prepared map[string]any
	if err := json.Unmarshal(originalBytes, &original); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("decode original prepared-userdata summary: %w", err)
	}
	if err := json.Unmarshal(preparedBytes, &prepared); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("decode prepared-userdata summary: %w", err)
	}

	removed := make([]string, 0)
	for key := range original {
		if _, ok := prepared[key]; !ok {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)

	nullPaths := make([]string, 0)
	collectNullJSONPaths(prepared, "", &nullPaths, 24)
	cardRemoved := removedNestedKeys(firstObjectFromArray(original, "userCards"), firstObjectFromArray(prepared, "userCards"))
	areaRemoved := removedNestedKeys(firstAreaItemObject(original), firstAreaItemObject(prepared))
	return removed, nullPaths, cardRemoved, areaRemoved, nil
}

func collectNullJSONPaths(value any, prefix string, out *[]string, limit int) {
	if len(*out) >= limit {
		return
	}
	switch typed := value.(type) {
	case nil:
		if prefix == "" {
			prefix = "$"
		}
		*out = append(*out, prefix)
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			collectNullJSONPaths(typed[key], next, out, limit)
			if len(*out) >= limit {
				return
			}
		}
	case []any:
		for idx, item := range typed {
			next := fmt.Sprintf("[%d]", idx)
			if prefix != "" {
				next = fmt.Sprintf("%s[%d]", prefix, idx)
			}
			collectNullJSONPaths(item, next, out, limit)
			if len(*out) >= limit {
				return
			}
		}
	}
}

func firstObjectFromArray(payload map[string]any, key string) map[string]any {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	first, _ := items[0].(map[string]any)
	return first
}

func firstAreaItemObject(payload map[string]any) map[string]any {
	area := firstObjectFromArray(payload, "userAreas")
	if area == nil {
		return nil
	}
	value, ok := area["areaItems"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	first, _ := items[0].(map[string]any)
	return first
}

func removedNestedKeys(original, prepared map[string]any) []string {
	if len(original) == 0 {
		return nil
	}
	removed := make([]string, 0)
	for key := range original {
		if _, ok := prepared[key]; !ok {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)
	return removed
}

func shouldPrepareRecommendUserData(query AutoQuery) bool {
	if query.MaxProfile || query.SubMaxProfile {
		return true
	}
	if query.UnitFilter != "" || query.AttrFilter != "" || len(query.ExcludedCards) > 0 {
		return true
	}
	if query.AreaItemLevel != nil && *query.AreaItemLevel > 0 {
		return true
	}
	if query.UseCurrentDeck {
		return true
	}
	return false
}

func (c *Controller) applyProfilePreset(region renderregion.Value, raw *userdata.RawUserData, query AutoQuery) error {
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

func (c *Controller) buildMaxProfileCards(region renderregion.Value, rawNow int64) ([]userdata.RawUserCard, error) {
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

	result := make([]userdata.RawUserCard, 0, len(allCards))
	for _, card := range allCards {
		if card == nil || card.ID <= 0 {
			continue
		}
		if card.ReleaseAt > 0 && card.ReleaseAt > now {
			continue
		}
		result = append(result, userdata.RawUserCard{
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

func (c *Controller) applyUserCardFilters(region renderregion.Value, raw *userdata.RawUserData, query AutoQuery) error {
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

	filtered := make([]userdata.RawUserCard, 0, len(raw.UserCards))
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

func (c *Controller) applyCurrentDeckOption(_ *userdata.RawUserData, original *userdata.RawUserData, recType string, query AutoQuery, option map[string]any) error {
	if !query.UseCurrentDeck || recType == "challenge" {
		return nil
	}
	if original == nil {
		return fmt.Errorf("raw user snapshot is unavailable")
	}

	deckInfo := userdata.FindActiveDeck(original.UserDecks, original.UserGamedata.Deck)
	if deckInfo.DeckID == 0 {
		return fmt.Errorf("找不到你的当前主队配置（更新当前主队需要抓包）")
	}

	cards, ok := userdata.UserDeckCardIDs(&deckInfo)
	if !ok {
		return fmt.Errorf("你的当前主队不足5张，无法使用\"当前\"参数（更新当前主队需要抓包）")
	}

	option["fixed_cards"] = append([]int(nil), cards...)
	delete(option, "fixed_characters")
	option["best_skill_as_leader"] = false
	return nil
}

func (c *Controller) restoreFixedCards(raw *userdata.RawUserData, original *userdata.RawUserData, option map[string]any, preferOriginal bool) error {
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
		originalCard := userdata.FindUserCard(original.UserCards, cardID)
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

func (c *Controller) applyAreaItemCaps(region renderregion.Value, raw *userdata.RawUserData, limit int) {
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

func (c *Controller) applyAreaItemLevel(region renderregion.Value, raw *userdata.RawUserData, targetLevel int) error {
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

func collectRawAreaItemLevels(areas []userdata.RawUserArea) map[int]int {
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

func buildRawUserAreas(levels map[int]int) []userdata.RawUserArea {
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

	items := make([]userdata.RawUserAreaItem, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		items = append(items, userdata.RawUserAreaItem{
			AreaItemID: itemID,
			Level:      levels[itemID],
		})
	}
	if len(items) == 0 {
		return nil
	}
	return []userdata.RawUserArea{{AreaItems: items}}
}
