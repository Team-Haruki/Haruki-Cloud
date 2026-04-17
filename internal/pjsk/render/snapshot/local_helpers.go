package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
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
			if shouldPreserveSuiteSnapshotKey(key) {
				continue
			}
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
		if !shouldMergeMySekaiTopLevelKey(key) {
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

func shouldPreserveSuiteSnapshotKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if strings.HasPrefix(key, "userMysekai") || strings.HasPrefix(key, "mysekai") {
		return false
	}
	switch key {
	case "userGamedata", "userProfile", "userDecks", "userCards", "userAreas", "userCharacters", "userHonors":
		return true
	default:
		return false
	}
}

func shouldMergeMySekaiTopLevelKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	switch key {
	case "now", "upload_time", "source", "local_source":
		return true
	}
	return strings.HasPrefix(key, "userMysekai") || strings.HasPrefix(key, "mysekai")
}

func resolveLeaderImagePath(ctx context.Context, sekaiClient *sekaiDB.Client, assetHelper *assets.AssetHelper, region renderregion.Value, cardID int, afterTraining bool) string {
	if cardID == 0 {
		return ""
	}
	if ctx == nil {
		ctx = context.TODO()
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
	return common.ResolveCardThumbnailPath(assetHelper, renderregion.WithDefault(region), assetBundleName, afterTraining)
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
	return assets.ResolveProfilePlaceholderPath(assetHelper)
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
	for _, userCard := range cards {
		if userCard.CardID == 0 {
			continue
		}
		if _, ok := seen[userCard.CardID]; ok {
			continue
		}
		seen[userCard.CardID] = struct{}{}
		entry := map[string]any{
			"cardId":                userCard.CardID,
			"level":                 userCard.Level,
			"masterRank":            userCard.MasterRank,
			"defaultImage":          userCard.DefaultImage,
			"specialTrainingStatus": userCard.SpecialTrainingStatus,
		}
		if userCard.SkillLevel > 0 {
			entry["skillLevel"] = userCard.SkillLevel
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
	switch strings.ToLower(strings.TrimSpace(cardInfo.DefaultImage)) {
	case "special_training":
		return true
	case "normal":
		return false
	default:
		return strings.EqualFold(cardInfo.SpecialTrainingStatus, "done")
	}
}

func leaderCardUsesTrainedArt(cardInfo *RawUserCard) bool {
	if cardInfo == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cardInfo.DefaultImage), "special_training")
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
