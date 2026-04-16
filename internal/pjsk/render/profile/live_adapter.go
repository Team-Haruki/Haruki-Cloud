package profile

import (
	"strings"

	"haruki-cloud/internal/pjsk/render/userdata"
	sekai "haruki-cloud/internal/pjsk/sekai"

	"github.com/bytedance/sonic"
)

// adaptAPICards converts GetAnotherProfileResponse card list to the raw type used by the
// existing buildPCards helper.
func adaptAPICards(cards []sekai.AnotherUserCard) []userdata.RawUserCard {
	result := make([]userdata.RawUserCard, len(cards))
	for i, c := range cards {
		result[i] = userdata.RawUserCard{
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
func adaptAPIDeckAsList(deck sekai.UserDeck) []userdata.RawUserDeck {
	return []userdata.RawUserDeck{{
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
func adaptAPIUserHonors(honors []sekai.UserHonor) []userdata.RawUserHonor {
	result := make([]userdata.RawUserHonor, len(honors))
	for i, h := range honors {
		result[i] = userdata.RawUserHonor{
			HonorID:    h.HonorID,
			HonorLevel: h.Level,
		}
	}
	return result
}

// adaptAPIProfileHonors converts the profile-slot honors to the raw type.
// HonorId2 (dual bonds-honor fallback) is not present in the API response and defaults to 0.
func adaptAPIProfileHonors(honors []sekai.UserProfileHonor) []userdata.RawUserProfileHonor {
	result := make([]userdata.RawUserProfileHonor, len(honors))
	for i, h := range honors {
		result[i] = userdata.RawUserProfileHonor{
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
func adaptAPICharacters(chars []sekai.AnotherUserCharacter) []userdata.RawUserCharacter {
	result := make([]userdata.RawUserCharacter, len(chars))
	for i, c := range chars {
		result[i] = userdata.RawUserCharacter{
			CharacterID:   c.CharacterID,
			CharacterRank: c.CharacterRank,
		}
	}
	return result
}

// adaptAPIMusicClearCount converts the aggregate clear counts to the raw type used by
// buildMusicCounts.
func adaptAPIMusicClearCount(counts []sekai.AnotherUserMusicDifficultyClearCount) []userdata.RawMusicClear {
	result := make([]userdata.RawMusicClear, len(counts))
	for i, c := range counts {
		result[i] = userdata.RawMusicClear{
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
func adaptAPIChallengeLiveResult(result sekai.UserChallengeLiveSoloResult) []userdata.RawChallengeLiveResult {
	if result.CharacterID == 0 {
		return nil
	}
	return []userdata.RawChallengeLiveResult{{
		CharacterID: result.CharacterID,
		HighScore:   result.HighScore,
	}}
}

// adaptAPIChallengeLiveStages converts Challenge Live stage rank entries to the raw type.
func adaptAPIChallengeLiveStages(stages []sekai.AnotherUserChallengeLiveSoloStage) []userdata.RawChallengeLiveStage {
	result := make([]userdata.RawChallengeLiveStage, len(stages))
	for i, s := range stages {
		result[i] = userdata.RawChallengeLiveStage{
			CharacterID: s.CharacterID,
			Rank:        s.Rank,
		}
	}
	return result
}

// parseFramesJSON parses the raw bytes from a userPlayerFrames snapshot payload into the
// RawUserFrame slice used by buildFramePaths. Returns nil on empty input or parse error so
// that the caller renders without a player frame.
func parseFramesJSON(data []byte) []userdata.RawUserFrame {
	if len(data) == 0 {
		return nil
	}
	var frames []userdata.RawUserFrame
	if err := sonic.Unmarshal(data, &frames); err != nil {
		return nil
	}
	return frames
}

func snapshotFrames(snapshot userdata.Snapshot) []userdata.RawUserFrame {
	if snapshot == nil {
		return nil
	}
	if err := snapshot.Require(); err != nil {
		return nil
	}
	raw := snapshot.RawData()
	if raw == nil || len(raw.UserFrames) == 0 {
		return nil
	}
	frames := make([]userdata.RawUserFrame, len(raw.UserFrames))
	copy(frames, raw.UserFrames)
	return frames
}

func snapshotRawData(snapshot userdata.Snapshot) *userdata.RawUserData {
	if snapshot == nil {
		return nil
	}
	if err := snapshot.Require(); err != nil {
		return nil
	}
	return snapshot.RawData()
}

type profileRenderState struct {
	leaderCardID      int
	leaderTrainedArt  bool
	userCards         []userdata.RawUserCard
	decks             []userdata.RawUserDeck
	activeDeckID      int
	detailedUserCards []any
}

func resolveProfileRenderState(resp *sekai.GetAnotherProfileResponse, snapshot userdata.Snapshot) profileRenderState {
	state := profileRenderState{
		leaderCardID:      resp.UserDeck.Leader,
		leaderTrainedArt:  isAPICardTrainedArt(findAPIUserCard(resp.UserCards, resp.UserDeck.Leader)),
		userCards:         adaptAPICards(resp.UserCards),
		decks:             adaptAPIDeckAsList(resp.UserDeck),
		activeDeckID:      resp.UserDeck.DeckID,
		detailedUserCards: buildAPIUserCardEntries(resp.UserCards, resp.UserDeck),
	}

	raw := snapshotRawData(snapshot)
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
			state.leaderTrainedArt = isSnapshotCardTrainedArt(userdata.FindUserCard(raw.UserCards, state.leaderCardID))
		}
	}
	if state.activeDeckID == 0 && len(raw.UserDecks) > 0 {
		state.decks = append([]userdata.RawUserDeck(nil), raw.UserDecks...)
		activeDeck := userdata.FindActiveDeck(raw.UserDecks, raw.UserGamedata.Deck)
		state.activeDeckID = raw.UserGamedata.Deck
		if state.activeDeckID == 0 {
			state.activeDeckID = activeDeck.DeckID
		}
		if activeDeck.Leader > 0 {
			state.leaderCardID = activeDeck.Leader
			state.leaderTrainedArt = isSnapshotCardTrainedArt(userdata.FindUserCard(raw.UserCards, activeDeck.Leader))
		}
	}

	return state
}

func mergeProfileUserCards(primary []userdata.RawUserCard, fallback []userdata.RawUserCard) []userdata.RawUserCard {
	if len(primary) == 0 {
		return append([]userdata.RawUserCard(nil), fallback...)
	}

	result := append([]userdata.RawUserCard(nil), primary...)
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

func buildSnapshotUserCardEntries(cards []userdata.RawUserCard) []any {
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

func isSnapshotCardTrainedArt(card *userdata.RawUserCard) bool {
	if card == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(card.DefaultImage), "special_training")
}
