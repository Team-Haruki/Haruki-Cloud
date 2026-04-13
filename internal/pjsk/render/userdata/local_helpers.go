package userdata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func mergeMySekaiData(userData []byte, mySekaiData []byte) ([]byte, error) {
	userData, err := normalizeSnapshotJSON(userData)
	if err != nil {
		return nil, err
	}
	mySekaiData, err = normalizeSnapshotJSON(mySekaiData)
	if err != nil {
		return nil, err
	}

	var baseMap map[string]any
	if err := json.Unmarshal(userData, &baseMap); err != nil {
		return nil, fmt.Errorf("decode user snapshot for mysekai merge: %w", err)
	}

	var mySekaiMap map[string]any
	if err := json.Unmarshal(mySekaiData, &mySekaiMap); err != nil {
		return nil, fmt.Errorf("decode mysekai snapshot: %w", err)
	}

	if updatedResources, ok := mySekaiMap["updatedResources"].(map[string]any); ok {
		for key, value := range updatedResources {
			// Don't overwrite a non-empty suite array with an empty mysekai delta.
			if existing, exists := baseMap[key]; exists {
				if existingSlice, ok := existing.([]any); ok && len(existingSlice) > 0 {
					if newSlice, ok := value.([]any); ok && len(newSlice) == 0 {
						continue
					}
				}
			}
			baseMap[key] = value
		}
	}
	for key, value := range mySekaiMap {
		if key == "updatedResources" {
			continue
		}
		baseMap[key] = value
	}

	merged, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("encode merged mysekai snapshot: %w", err)
	}
	return merged, nil
}

func resolveLeaderImagePath(ctx context.Context, sekaiClient *sekaiDB.Client, assetHelper *assets.AssetHelper, region renderregion.Value, cardID int, afterTraining bool) string {
	if cardID == 0 {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var assetBundleName string
	if sekaiClient != nil {
		entity, err := sekaiClient.Card.Query().
			Where(card.ServerRegionEQ(renderregion.WithDefault(region).String()), card.GameIDEQ(int64(cardID))).
			Only(ctx)
		if err == nil {
			assetBundleName = entity.AssetbundleName
		}
	}
	if strings.TrimSpace(assetBundleName) == "" {
		return ""
	}

	imageType := "normal"
	if afterTraining {
		imageType = "after_training"
	}

	regionStr := renderregion.WithDefault(region).String()
	return assets.ResolveRegionAssetPath(assetHelper, regionStr,
		filepath.Join("thumbnail", "chara", fmt.Sprintf("%s_%s.png", assetBundleName, imageType)),
		filepath.Join("character", "member", assetBundleName, "card_normal.png"))
}

func SelectProfileImageCardID(profileImageType string, profileImageID, leaderCardID int) int {
	mode := strings.ToLower(strings.TrimSpace(profileImageType))
	if profileImageID > 0 && mode != "" && mode != "default" {
		return profileImageID
	}
	if leaderCardID > 0 {
		return leaderCardID
	}
	if profileImageID > 0 {
		return profileImageID
	}
	return 0
}

func fallbackLeaderImagePath(assetHelper *assets.AssetHelper) string {
	const fallback = "user/leader.png"
	if assetHelper == nil {
		return fallback
	}
	if existing := assetHelper.FirstExisting(fallback); existing != "" {
		return makeRelativeAsset(assetHelper, existing)
	}
	return fallback
}

func makeRelativeAsset(assetHelper *assets.AssetHelper, target string) string {
	if assetHelper == nil {
		return filepath.ToSlash(filepath.Clean(target))
	}
	for _, root := range assetHelper.Roots() {
		relative := assets.MakeRelative(root, target)
		if relative != target {
			return relative
		}
	}
	return filepath.ToSlash(filepath.Clean(target))
}

func buildUserCardEntries(cards []RawUserCard) []any {
	seen := make(map[int]struct{}, len(cards))
	entries := make([]any, 0, len(cards))
	for _, card := range cards {
		if card.CardID == 0 {
			continue
		}
		if _, ok := seen[card.CardID]; ok {
			continue
		}
		seen[card.CardID] = struct{}{}
		entry := map[string]any{
			"cardId":                card.CardID,
			"level":                 card.Level,
			"masterRank":            card.MasterRank,
			"defaultImage":          card.DefaultImage,
			"specialTrainingStatus": card.SpecialTrainingStatus,
		}
		if card.SkillLevel > 0 {
			entry["skillLevel"] = card.SkillLevel
		}
		entries = append(entries, entry)
	}
	return entries
}

func CloneRawUserData(raw *RawUserData) (*RawUserData, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode raw user snapshot: %w", err)
	}
	var cloned RawUserData
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("decode raw user snapshot clone: %w", err)
	}
	return &cloned, nil
}

func EncodeRawUserData(raw *RawUserData) ([]byte, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw user snapshot is unavailable")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode raw user snapshot: %w", err)
	}
	return data, nil
}

// FindActiveDeck returns the deck with the given activeID, or the first deck if not found.
func FindActiveDeck(decks []RawUserDeck, activeID int) RawUserDeck {
	for _, deck := range decks {
		if deck.DeckID == activeID {
			return deck
		}
	}
	if len(decks) > 0 {
		return decks[0]
	}
	return RawUserDeck{}
}

func UserDeckCardIDs(deck *RawUserDeck) ([]int, bool) {
	if deck == nil {
		return nil, false
	}
	cards := []int{deck.Member1, deck.Member2, deck.Member3, deck.Member4, deck.Member5}
	for _, cardID := range cards {
		if cardID <= 0 {
			return nil, false
		}
	}
	return cards, true
}

// FindUserCard returns the card matching cardID, or nil if not found.
func FindUserCard(cards []RawUserCard, cardID int) *RawUserCard {
	for i := range cards {
		if cards[i].CardID == cardID {
			return &cards[i]
		}
	}
	return nil
}

func FindChallengeLiveDeck(decks []RawChallengeLiveDeck, characterID int) *RawChallengeLiveDeck {
	for i := range decks {
		if decks[i].CharacterID == characterID {
			return &decks[i]
		}
	}
	return nil
}

func ChallengeLiveDeckCardIDs(deck *RawChallengeLiveDeck) ([]int, bool) {
	if deck == nil {
		return nil, false
	}
	cards := []int{deck.Leader, deck.Support1, deck.Support2, deck.Support3, deck.Support4}
	for _, cardID := range cards {
		if cardID <= 0 {
			return nil, false
		}
	}
	return cards, true
}

func isAfterTraining(cardInfo *RawUserCard) bool {
	if cardInfo == nil {
		return false
	}
	return strings.EqualFold(cardInfo.SpecialTrainingStatus, "done")
}

func resolveMusicResultMap(raw RawUserData) map[string]map[int]string {
	result := buildMusicResultMap(raw.UserMusicStats)
	mergeMusicResultMaps(result, buildMusicResultMapFromUserMusics(raw.UserMusics))
	mergeMusicResultMaps(result, buildMusicResultMapFromCompact(raw.CompactUserMusicResults))
	return result
}

func buildMusicResultMap(rawResults []RawMusicResult) map[string]map[int]string {
	result := make(map[string]map[int]string)
	for _, item := range rawResults {
		diff := strings.ToLower(strings.TrimSpace(item.MusicDifficultyType))
		if diff == "" {
			diff = strings.ToLower(strings.TrimSpace(item.MusicDifficulty))
		}
		if diff == "" {
			continue
		}
		status := normalizePlayResult(item)
		if _, ok := result[diff]; !ok {
			result[diff] = make(map[int]string)
		}
		if prioritizePlayResult(status) >= prioritizePlayResult(result[diff][item.MusicID]) {
			result[diff][item.MusicID] = status
		}
	}
	return result
}

func mergeMusicResultMaps(dst map[string]map[int]string, src map[string]map[int]string) {
	for diff, items := range src {
		if len(items) == 0 {
			continue
		}
		if _, ok := dst[diff]; !ok {
			dst[diff] = make(map[int]string, len(items))
		}
		for musicID, status := range items {
			if prioritizePlayResult(status) >= prioritizePlayResult(dst[diff][musicID]) {
				dst[diff][musicID] = status
			}
		}
	}
}

func buildMusicResultMapFromUserMusics(userMusics []RawUserMusic) map[string]map[int]string {
	if len(userMusics) == 0 {
		return nil
	}

	results := make([]RawMusicResult, 0, len(userMusics))
	for _, music := range userMusics {
		for _, status := range music.UserMusicDifficultyStatuses {
			for _, item := range status.UserMusicResults {
				normalized := item
				if normalized.MusicID == 0 {
					normalized.MusicID = music.MusicID
				}
				if strings.TrimSpace(normalized.MusicDifficultyType) == "" {
					normalized.MusicDifficultyType = strings.TrimSpace(status.MusicDifficultyType)
				}
				if strings.TrimSpace(normalized.MusicDifficulty) == "" {
					normalized.MusicDifficulty = strings.TrimSpace(status.MusicDifficulty)
				}
				if strings.TrimSpace(normalized.MusicDifficultyType) == "" {
					normalized.MusicDifficultyType = strings.TrimSpace(normalized.MusicDifficulty)
				}
				if strings.TrimSpace(normalized.MusicDifficulty) == "" {
					normalized.MusicDifficulty = strings.TrimSpace(normalized.MusicDifficultyType)
				}
				results = append(results, normalized)
			}
		}
	}
	return buildMusicResultMap(results)
}

func buildMusicResultMapFromCompact(raw json.RawMessage) map[string]map[int]string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	var payload map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}

	enumValues := decodeCompactEnumValues(payload["__ENUM__"])
	columns := make(map[string][]any, len(payload))
	rowCount := 0
	for key, fieldRaw := range payload {
		if key == "__ENUM__" {
			continue
		}

		var values []any
		decoder := json.NewDecoder(bytes.NewReader(fieldRaw))
		decoder.UseNumber()
		if err := decoder.Decode(&values); err != nil {
			continue
		}
		if len(values) == 0 {
			continue
		}
		columns[key] = values
		if len(values) > rowCount {
			rowCount = len(values)
		}
	}

	if rowCount == 0 {
		return nil
	}

	results := make([]RawMusicResult, 0, rowCount)
	for index := 0; index < rowCount; index++ {
		musicID, ok := compactIntFromColumns(columns, "musicId", index)
		if !ok || musicID <= 0 {
			continue
		}

		diff := compactStringFromColumns(columns, enumValues, []string{"musicDifficultyType", "musicDifficulty"}, index)
		if diff == "" {
			continue
		}

		playResult := compactStringFromColumns(columns, enumValues, []string{"playResult"}, index)
		fullCombo, _ := compactBoolFromColumns(columns, "fullComboFlg", index)
		fullPerfect, _ := compactBoolFromColumns(columns, "fullPerfectFlg", index)

		results = append(results, RawMusicResult{
			MusicID:             musicID,
			MusicDifficulty:     diff,
			MusicDifficultyType: diff,
			PlayResult:          playResult,
			FullComboFlg:        fullCombo,
			FullPerfectFlg:      fullPerfect,
		})
	}

	return buildMusicResultMap(results)
}

func decodeCompactEnumValues(raw json.RawMessage) map[string][]string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	var values map[string][]string
	if err := json.Unmarshal(trimmed, &values); err != nil {
		return nil
	}
	return values
}

func compactStringFromColumns(columns map[string][]any, enumValues map[string][]string, keys []string, index int) string {
	for _, key := range keys {
		values, ok := columns[key]
		if !ok || index >= len(values) {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(compactEnumString(values[index], enumValues[key])))
		if value != "" {
			return value
		}
	}
	return ""
}

func compactIntFromColumns(columns map[string][]any, key string, index int) (int, bool) {
	values, ok := columns[key]
	if !ok || index >= len(values) {
		return 0, false
	}
	return compactIntValue(values[index])
}

func compactBoolFromColumns(columns map[string][]any, key string, index int) (bool, bool) {
	values, ok := columns[key]
	if !ok || index >= len(values) {
		return false, false
	}
	return compactBoolValue(values[index])
}

func compactEnumString(value any, enumValues []string) string {
	if idx, ok := compactIntValue(value); ok {
		if idx >= 0 && idx < len(enumValues) {
			return enumValues[idx]
		}
	}

	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func compactIntValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func compactBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err != nil {
			return false, false
		}
		return parsed, true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return false, false
		}
		return parsed != 0, true
	case float64:
		return typed != 0, true
	default:
		return false, false
	}
}

func normalizePlayResult(item RawMusicResult) string {
	switch {
	case item.FullPerfectFlg:
		return "ap"
	case item.FullComboFlg:
		return "fc"
	case strings.EqualFold(item.PlayResult, "not_clear") || item.PlayResult == "":
		return "not_clear"
	default:
		return "clear"
	}
}

func prioritizePlayResult(result string) int {
	switch result {
	case "ap":
		return 3
	case "fc":
		return 2
	case "clear":
		return 1
	default:
		return 0
	}
}

func convertChallengeResults(source []RawChallengeLiveResult) []ChallengeLiveResult {
	out := make([]ChallengeLiveResult, 0, len(source))
	for _, item := range source {
		out = append(out, ChallengeLiveResult(item))
	}
	return out
}

func convertChallengeStages(source []RawChallengeLiveStage) []ChallengeLiveStage {
	out := make([]ChallengeLiveStage, 0, len(source))
	for _, item := range source {
		out = append(out, ChallengeLiveStage(item))
	}
	return out
}

func convertChallengeRewards(source []RawChallengeLiveReward) []ChallengeLiveReward {
	out := make([]ChallengeLiveReward, 0, len(source))
	for _, item := range source {
		out = append(out, ChallengeLiveReward{
			RewardID:    item.ChallengeLiveHighScoreRewardID,
			CharacterID: item.CharacterID,
		})
	}
	return out
}
