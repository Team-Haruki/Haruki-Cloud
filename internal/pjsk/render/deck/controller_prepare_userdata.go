package deck

import (
	"bytes"
	"fmt"
	json "github.com/bytedance/sonic"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func (c *Controller) prepareRecommendUserData(region renderregion.Value, recType string, query AutoQuery, option map[string]any) (*snapshot.RawUserData, []byte, error) {
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

	raw, err := snapshot.CloneRawUserData(original)
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
	if err := c.restoreFixedCards(region, raw, original, option, query.UseCurrentDeck); err != nil {
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

func encodePreparedRecommendUserData(originalBytes []byte, original, raw *snapshot.RawUserData) ([]byte, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw user snapshot is unavailable")
	}
	if len(bytes.TrimSpace(originalBytes)) == 0 {
		return snapshot.EncodeRawUserData(raw)
	}

	var payload map[string]any
	decoder := json.ConfigDefault.NewDecoder(bytes.NewReader(originalBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode original user snapshot: %w", err)
	}
	if payload == nil {
		return snapshot.EncodeRawUserData(raw)
	}

	var originalCards []snapshot.RawUserCard
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
	userHonors, err := mergePreparedUserHonors(payload["userHonors"], raw.UserHonors)
	if err != nil {
		return nil, err
	}
	userProfileHonors, err := mergePreparedUserProfileHonors(payload["userProfileHonors"], raw.UserProfileHonors)
	if err != nil {
		return nil, err
	}

	payload["userCards"] = userCards
	payload["userAreas"] = userAreas
	payload["userHonors"] = userHonors
	payload["userProfileHonors"] = userProfileHonors

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode prepared user snapshot: %w", err)
	}
	return encoded, nil
}

func mergePreparedUserCards(original any, originalCards, cards []snapshot.RawUserCard) ([]any, error) {
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
			if samePreparedUserCard(snapshot.FindUserCard(originalCards, card.CardID), &card) {
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

func mergePreparedUserAreas(original any, areas []snapshot.RawUserArea) ([]any, error) {
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

func mergePreparedAreaItems(original any, items []snapshot.RawUserAreaItem) ([]any, error) {
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

func mergePreparedUserHonors(original any, honors []snapshot.RawUserHonor) ([]any, error) {
	if len(honors) == 0 {
		return []any{}, nil
	}

	originalItems := jsonArrayToObjects(original)
	originalByHonorID := make(map[int]map[string]any, len(originalItems))
	for _, item := range originalItems {
		honorID, _ := jsonNumberToInt(item["honorId"])
		if honorID > 0 {
			originalByHonorID[honorID] = item
		}
	}

	merged := make([]any, 0, len(honors))
	for _, honor := range honors {
		honorMap, err := structToJSONObject(honor)
		if err != nil {
			return nil, fmt.Errorf("encode prepared user honor %d: %w", honor.HonorID, err)
		}
		if existing := originalByHonorID[honor.HonorID]; existing != nil {
			honorMap = mergeJSONObjects(existing, honorMap)
		}
		merged = append(merged, honorMap)
	}
	return merged, nil
}

func mergePreparedUserProfileHonors(original any, honors []snapshot.RawUserProfileHonor) ([]any, error) {
	if len(honors) == 0 {
		return []any{}, nil
	}

	originalItems := jsonArrayToObjects(original)
	originalBySeq := make(map[int]map[string]any, len(originalItems))
	for _, item := range originalItems {
		seq, _ := jsonNumberToInt(item["seq"])
		if seq > 0 {
			originalBySeq[seq] = item
		}
	}

	merged := make([]any, 0, len(honors))
	for _, honor := range honors {
		honorMap, err := structToJSONObject(honor)
		if err != nil {
			return nil, fmt.Errorf("encode prepared user profile honor seq %d: %w", honor.Seq, err)
		}
		if existing := originalBySeq[honor.Seq]; existing != nil {
			honorMap = mergeJSONObjects(existing, honorMap)
		}
		merged = append(merged, honorMap)
	}
	return merged, nil
}

func samePreparedUserCard(original, current *snapshot.RawUserCard) bool {
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

func shouldPrepareRecommendUserData(query AutoQuery) bool {
	if query.MaxProfile || query.SubMaxProfile {
		return true
	}
	if len(query.FixedCards) > 0 {
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
