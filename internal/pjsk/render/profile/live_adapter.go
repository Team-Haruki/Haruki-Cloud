package profile

import (
	"slices"
	"strings"

	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"

	"github.com/bytedance/sonic"
)

// adaptAPICards converts GetAnotherProfileResponse card list to the raw type used by the
// existing buildPCards helper.
func adaptAPICards(cards []sekai.AnotherUserCard) []snapshot.RawUserCard {
	result := make([]snapshot.RawUserCard, len(cards))
	for i, c := range cards {
		result[i] = snapshot.RawUserCard{
			CardID:                c.CardID,
			Level:                 c.Level,
			MasterRank:            c.MasterRank,
			SpecialTrainingStatus: c.SpecialTrainingStatus,
			DefaultImage:          c.DefaultImage,
		}
	}
	return result
}

// adaptAPIDeckAsList wraps the singular UserDeck from the API into the slice form
// expected by buildPCards / findActiveDeck.
func adaptAPIDeckAsList(deck sekai.UserDeck) []snapshot.RawUserDeck {
	return []snapshot.RawUserDeck{{
		DeckID:  deck.DeckID,
		Leader:  deck.Leader,
		Member1: deck.Member1,
		Member2: deck.Member2,
		Member3: deck.Member3,
		Member4: deck.Member4,
		Member5: deck.Member5,
	}}
}

// adaptAPIUserHonors converts the public UserHonor list to the raw type.
func adaptAPIUserHonors(honors []sekai.UserHonor) []snapshot.RawUserHonor {
	result := make([]snapshot.RawUserHonor, len(honors))
	for i, h := range honors {
		result[i] = snapshot.RawUserHonor{
			HonorID:    h.HonorID,
			HonorLevel: h.Level,
		}
	}
	return result
}

// adaptAPIProfileHonors converts the profile-slot honors to the raw type.
// HonorId2 (dual bonds-honor fallback) is not present in the API response and defaults to 0.
func adaptAPIProfileHonors(honors []sekai.UserProfileHonor) []snapshot.RawUserProfileHonor {
	result := make([]snapshot.RawUserProfileHonor, len(honors))
	for i, h := range honors {
		result[i] = snapshot.RawUserProfileHonor{
			Seq:                h.Seq,
			ProfileHonorType:   h.ProfileHonorType,
			HonorID:            h.HonorID,
			HonorLevel:         h.HonorLevel,
			BondsHonorViewType: h.BondsHonorViewType,
			BondsHonorWordId:   h.BondsHonorWordID,
		}
	}
	return result
}

// adaptAPICharacters converts the public character rank list to the raw type.
func adaptAPICharacters(chars []sekai.AnotherUserCharacter) []snapshot.RawUserCharacter {
	result := make([]snapshot.RawUserCharacter, len(chars))
	for i, c := range chars {
		result[i] = snapshot.RawUserCharacter{
			CharacterID:   c.CharacterID,
			CharacterRank: c.CharacterRank,
		}
	}
	return result
}

// adaptAPIMusicClearCount converts the aggregate clear counts to the raw type used by
// buildMusicCounts.
func adaptAPIMusicClearCount(counts []sekai.AnotherUserMusicDifficultyClearCount) []snapshot.RawMusicClear {
	result := make([]snapshot.RawMusicClear, len(counts))
	for i, c := range counts {
		result[i] = snapshot.RawMusicClear{
			MusicDifficultyType: string(c.MusicDifficultyType),
			LiveClear:           c.LiveClear,
			FullCombo:           c.FullCombo,
			AllPerfect:          c.AllPerfect,
		}
	}
	return result
}

// adaptAPIChallengeLiveResult wraps the singular best-character result into the slice
// form expected by buildSoloLive.  Returns nil when CharacterID is 0 (no data).
func adaptAPIChallengeLiveResult(result sekai.UserChallengeLiveSoloResult) []snapshot.RawChallengeLiveResult {
	if result.CharacterID == 0 {
		return nil
	}
	return []snapshot.RawChallengeLiveResult{{
		CharacterID: result.CharacterID,
		HighScore:   result.HighScore,
	}}
}

// adaptAPIChallengeLiveStages converts Challenge Live stage rank entries to the raw type.
func adaptAPIChallengeLiveStages(stages []sekai.AnotherUserChallengeLiveSoloStage) []snapshot.RawChallengeLiveStage {
	result := make([]snapshot.RawChallengeLiveStage, len(stages))
	for i, s := range stages {
		result[i] = snapshot.RawChallengeLiveStage{
			CharacterID: s.CharacterID,
			Rank:        s.Rank,
		}
	}
	return result
}

// parseFramesJSON parses the raw bytes from a userPlayerFrames snapshot payload into the
// RawUserFrame slice used by buildFramePaths. Returns nil on empty input or parse error so
// that the caller renders without a player frame.
func parseFramesJSON(data []byte) []snapshot.RawUserFrame {
	if len(data) == 0 {
		return nil
	}
	var frames []snapshot.RawUserFrame
	if err := sonic.Unmarshal(data, &frames); err != nil {
		return nil
	}
	return frames
}

func snapshotFrames(snap snapshot.Snapshot) []snapshot.RawUserFrame {
	if snap == nil {
		return nil
	}
	if err := snap.Require(); err != nil {
		return nil
	}
	raw := snap.RawData()
	if raw == nil || len(raw.UserFrames) == 0 {
		return nil
	}
	frames := make([]snapshot.RawUserFrame, len(raw.UserFrames))
	copy(frames, raw.UserFrames)
	return frames
}

func snapshotRawData(snap snapshot.Snapshot) *snapshot.RawUserData {
	if snap == nil {
		return nil
	}
	if err := snap.Require(); err != nil {
		return nil
	}
	return snap.RawData()
}

type profileRenderState struct {
	leaderCardID      int
	leaderTrainedArt  bool
	userCards         []snapshot.RawUserCard
	decks             []snapshot.RawUserDeck
	activeDeckID      int
	detailedUserCards []any
}

func resolveProfileRenderState(resp *sekai.GetAnotherProfileResponse, snap snapshot.Snapshot) profileRenderState {
	state := profileRenderState{
		leaderCardID:      resp.UserDeck.Leader,
		leaderTrainedArt:  isAPICardTrainedArt(findAPIUserCard(resp.UserCards, resp.UserDeck.Leader)),
		userCards:         adaptAPICards(resp.UserCards),
		decks:             adaptAPIDeckAsList(resp.UserDeck),
		activeDeckID:      resp.UserDeck.DeckID,
		detailedUserCards: buildAPIUserCardEntries(resp.UserCards, resp.UserDeck),
	}

	raw := snapshotRawData(snap)
	if raw == nil {
		return state
	}

	// Profile current-deck information comes from the public game API. Snapshot
	// data can still supplement missing card records, but it should not replace
	// the API-selected deck or leader display state.
	if len(raw.UserCards) > 0 {
		state.userCards = mergeProfileUserCards(state.userCards, raw.UserCards)
		if len(state.detailedUserCards) == 0 {
			state.detailedUserCards = buildSnapshotUserCardEntries(raw.UserCards)
		}
		if state.leaderCardID > 0 && findAPIUserCard(resp.UserCards, state.leaderCardID) == nil {
			state.leaderTrainedArt = isSnapshotCardTrainedArt(snapshot.FindUserCard(raw.UserCards, state.leaderCardID))
		}
	}
	if state.activeDeckID == 0 && len(raw.UserDecks) > 0 {
		state.decks = slices.Clone(raw.UserDecks)
		activeDeck := snapshot.FindActiveDeck(raw.UserDecks, raw.UserGamedata.Deck)
		state.activeDeckID = raw.UserGamedata.Deck
		if state.activeDeckID == 0 {
			state.activeDeckID = activeDeck.DeckID
		}
		if activeDeck.Leader > 0 {
			state.leaderCardID = activeDeck.Leader
			state.leaderTrainedArt = isSnapshotCardTrainedArt(snapshot.FindUserCard(raw.UserCards, activeDeck.Leader))
		}
	}

	return state
}

func mergeProfileUserCards(primary []snapshot.RawUserCard, fallback []snapshot.RawUserCard) []snapshot.RawUserCard {
	if len(primary) == 0 {
		return slices.Clone(fallback)
	}

	result := slices.Clone(primary)
	seen := make(map[int]struct{}, len(primary))
	for _, card := range primary {
		if card.CardID > 0 {
			seen[card.CardID] = struct{}{}
		}
	}
	for _, card := range fallback {
		if card.CardID == 0 {
			continue
		}
		if _, ok := seen[card.CardID]; ok {
			continue
		}
		seen[card.CardID] = struct{}{}
		result = append(result, card)
	}
	return result
}

func buildSnapshotUserCardEntries(cards []snapshot.RawUserCard) []any {
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

// findAPIUserCard returns the first AnotherUserCard whose CardID matches, or nil.
func findAPIUserCard(cards []sekai.AnotherUserCard, cardID int) *sekai.AnotherUserCard {
	for i := range cards {
		if cards[i].CardID == cardID {
			return &cards[i]
		}
	}
	return nil
}

// isAPICardTrainedArt reports whether the card should currently display its
// after-training art. We intentionally key this off defaultImage only so the
// avatar stays in sync with the actual selected art, rather than the unlock
// state of the card.
func isAPICardTrainedArt(card *sekai.AnotherUserCard) bool {
	if card == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(card.DefaultImage), "special_training")
}

func isSnapshotCardTrainedArt(card *snapshot.RawUserCard) bool {
	if card == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(card.DefaultImage), "special_training")
}
