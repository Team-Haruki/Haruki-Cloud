package userdata

import (
"context"
"encoding/json"
"fmt"
"os"
"path/filepath"
"strings"

sekaiDB "haruki-cloud/database/sekai"
"haruki-cloud/database/sekai/card"
"haruki-cloud/internal/pjsk/render/assets"
renderregion "haruki-cloud/internal/pjsk/render/region"
)

func mergeMySekaiJSON(userData []byte, mySekaiPath string) ([]byte, error) {
	mySekaiData, err := os.ReadFile(filepath.Clean(mySekaiPath))
	if err != nil {
		return nil, fmt.Errorf("read mysekai snapshot: %w", err)
	}
	userData, err = normalizeSnapshotJSON(userData)
	if err != nil {
		return nil, err
	}
	mySekaiData, err = normalizeSnapshotJSON(mySekaiData)
	if err != nil {
		return nil, err
	}

	var baseMap map[string]interface{}
	if err := json.Unmarshal(userData, &baseMap); err != nil {
		return nil, fmt.Errorf("decode user snapshot for mysekai merge: %w", err)
	}

	var mySekaiMap map[string]interface{}
	if err := json.Unmarshal(mySekaiData, &mySekaiMap); err != nil {
		return nil, fmt.Errorf("decode mysekai snapshot: %w", err)
	}

	if updatedResources, ok := mySekaiMap["updatedResources"].(map[string]interface{}); ok {
		for key, value := range updatedResources {
			// Don't overwrite a non-empty suite array with an empty mysekai delta.
			if existing, exists := baseMap[key]; exists {
				if existingSlice, ok := existing.([]interface{}); ok && len(existingSlice) > 0 {
					if newSlice, ok := value.([]interface{}); ok && len(newSlice) == 0 {
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

func resolveLeaderImagePath(sekaiClient *sekaiDB.Client, assetHelper *assets.AssetHelper, region renderregion.Value, cardID int, afterTraining bool) string {
	if cardID == 0 {
		return ""
	}

	var assetBundleName string
	if sekaiClient != nil {
		entity, err := sekaiClient.Card.Query().
			Where(card.ServerRegionEQ(renderregion.WithDefault(region).String()), card.GameIDEQ(int64(cardID))).
			Only(context.Background())
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

func buildUserCardEntries(cards []RawUserCard) []interface{} {
	seen := make(map[int]struct{}, len(cards))
	entries := make([]interface{}, 0, len(cards))
	for _, card := range cards {
		if card.CardID == 0 {
			continue
		}
		if _, ok := seen[card.CardID]; ok {
			continue
		}
		seen[card.CardID] = struct{}{}
		entries = append(entries, map[string]interface{}{
			"cardId":                card.CardID,
			"level":                 card.Level,
			"masterRank":            card.MasterRank,
			"defaultImage":          card.DefaultImage,
			"specialTrainingStatus": card.SpecialTrainingStatus,
		})
	}
	return entries
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

// FindUserCard returns the card matching cardID, or nil if not found.
func FindUserCard(cards []RawUserCard, cardID int) *RawUserCard {
	for i := range cards {
		if cards[i].CardID == cardID {
			return &cards[i]
		}
	}
	return nil
}

func isAfterTraining(cardInfo *RawUserCard) bool {
	if cardInfo == nil {
		return false
	}
	return strings.EqualFold(cardInfo.SpecialTrainingStatus, "done")
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
		out = append(out, ChallengeLiveResult{
			CharacterID: item.CharacterID,
			HighScore:   item.HighScore,
		})
	}
	return out
}

func convertChallengeStages(source []RawChallengeLiveStage) []ChallengeLiveStage {
	out := make([]ChallengeLiveStage, 0, len(source))
	for _, item := range source {
		out = append(out, ChallengeLiveStage{
			CharacterID: item.CharacterID,
			Rank:        item.Rank,
		})
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
