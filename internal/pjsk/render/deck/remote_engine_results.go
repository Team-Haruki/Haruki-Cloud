package deck

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
	"sort"
	"strings"
)

type remoteRewarmKind int

const (
	remoteRewarmNone remoteRewarmKind = iota
	remoteRewarmMasterdata
	remoteRewarmMusicMeta
)

func convertRemoteDecks(src []remoteRecommendDeck) []RecommendDeck {
	out := make([]RecommendDeck, 0, len(src))
	for _, d := range src {
		cards := make([]RecommendCard, 0, len(d.Cards))
		for _, c := range d.Cards {
			cards = append(cards, RecommendCard{
				CardID:          c.CardID,
				Level:           c.Level,
				MasterRank:      c.MasterRank,
				DefaultImage:    c.DefaultImage,
				SkillLevel:      c.SkillLevel,
				SkillRate:       c.SkillScoreUp,
				EventBonusRate:  c.EventBonusRate,
				IsAfterStory:    c.Episode2Read,
				IsBeforeStory:   c.Episode1Read,
				IsAfterTraining: c.AfterTraining,
				HasCanvasBonus:  c.HasCanvasBonus,
			})
		}
		out = append(out, RecommendDeck{
			Cards:                cards,
			Score:                d.Score,
			LiveScore:            d.LiveScore,
			MysekaiEventPoint:    d.MysekaiEventPoint,
			TotalPower:           d.TotalPower,
			EventBonusRate:       d.EventBonusRate,
			SupportDeckBonusRate: d.SupportDeckBonusRate,
			MultiLiveScoreUp:     d.MultiLiveScoreUp,
		})
	}
	return out
}

func parseRemoteRecommendBatch(raw json.RawMessage, options []map[string]any) ([]remoteBatchRecommendResult, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("deck-service returned empty response")
	}

	if trimmed[0] == '[' {
		var items []remoteBatchRecommendResult
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		fillMissingRemoteAlgorithms(items, options)
		return items, nil
	}

	var single remoteRecommendResult
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return nil, err
	}
	item := remoteBatchRecommendResult{Result: &single}
	if len(options) > 0 {
		if alg, _ := options[0]["algorithm"].(string); strings.TrimSpace(alg) != "" {
			item.Alg = alg
		}
	}
	return []remoteBatchRecommendResult{item}, nil
}

func fillMissingRemoteAlgorithms(items []remoteBatchRecommendResult, options []map[string]any) {
	for index := range items {
		if strings.TrimSpace(items[index].Alg) != "" || index >= len(options) {
			continue
		}
		if alg, _ := options[index]["algorithm"].(string); strings.TrimSpace(alg) != "" {
			items[index].Alg = alg
		}
	}
}

type remoteRecommendPair struct {
	Deck RecommendDeck
	Alg  string
}

type remoteResultAccumulator struct {
	result   *RecommendResult
	seen     map[string]*RecommendDeck
	order    []string
	firstErr error
}

func newRemoteResultAccumulator() *remoteResultAccumulator {
	return &remoteResultAccumulator{
		result: &RecommendResult{CostTimes: make(map[string]float64), WaitTimes: make(map[string]float64)},
		seen:   make(map[string]*RecommendDeck),
	}
}

func aggregateRemoteRecommendResults(recType string, options []map[string]any, results []remoteBatchRecommendResult) (*RecommendResult, error) {
	accumulator := newRemoteResultAccumulator()
	for _, item := range results {
		accumulator.add(item, recType, options)
	}
	if len(accumulator.order) == 0 && accumulator.firstErr != nil {
		return nil, accumulator.firstErr
	}
	pairs := accumulator.pairs()
	target, _ := options[0]["target"].(string)
	sort.SliceStable(pairs, func(i, j int) bool {
		return compareRecommendDecks(strings.ToLower(strings.TrimSpace(recType)), target, pairs[i].Deck, pairs[j].Deck)
	})
	for _, pair := range pairs[:remoteRecommendLimit(options[0], len(pairs))] {
		accumulator.result.Decks = append(accumulator.result.Decks, pair.Deck)
		accumulator.result.DeckAlgs = append(accumulator.result.DeckAlgs, pair.Alg)
	}
	return accumulator.result, nil
}

func (a *remoteResultAccumulator) add(item remoteBatchRecommendResult, recType string, options []map[string]any) {
	if message := strings.TrimSpace(item.Error); message != "" {
		if a.firstErr == nil {
			a.firstErr = fmt.Errorf("%s", message)
		}
		return
	}
	decks := convertRemoteDecks(item.Decks)
	if item.Result != nil {
		decks = convertRemoteDecks(item.Result.Decks)
	}
	if len(decks) == 0 {
		return
	}
	alg := displayRecommendAlgorithm(item.Alg)
	if alg != "" {
		a.result.CostTimes[alg], a.result.WaitTimes[alg] = item.CostTime, item.WaitTime
	}
	for _, deck := range decks {
		a.addDeck(deck, alg, recType, options[0])
	}
}

func (a *remoteResultAccumulator) addDeck(deck RecommendDeck, alg, recType string, option map[string]any) {
	hash := deckHash(deck)
	if existing := a.seen[hash]; existing != nil {
		if alg != "" {
			existing.Algs = append(existing.Algs, alg)
		}
		target, _ := option["target"].(string)
		if compareRecommendDecks(recType, target, deck, *existing) {
			algs := existing.Algs
			*existing = deck
			existing.Algs = algs
		}
		return
	}
	copy := deck
	if alg != "" {
		copy.Algs = []string{alg}
	}
	a.seen[hash] = &copy
	a.order = append(a.order, hash)
}

func (a *remoteResultAccumulator) pairs() []remoteRecommendPair {
	result := make([]remoteRecommendPair, 0, len(a.order))
	for _, hash := range a.order {
		deck := a.seen[hash]
		unique := make(map[string]struct{}, len(deck.Algs))
		for _, alg := range deck.Algs {
			unique[alg] = struct{}{}
		}
		algs := make([]string, 0, len(unique))
		for alg := range unique {
			algs = append(algs, alg)
		}
		sort.Strings(algs)
		result = append(result, remoteRecommendPair{Deck: *deck, Alg: strings.Join(algs, "+")})
	}
	return result
}

func remoteRecommendLimit(option map[string]any, available int) int {
	limit, ok := option["limit"].(int)
	if !ok {
		value, _ := option["limit"].(float64)
		limit = int(value)
	}
	if limit <= 0 || limit > available {
		return available
	}
	return limit
}

func shouldRewarmRemoteService(err error) bool {
	return classifyRemoteRewarm(err) != remoteRewarmNone
}

func classifyRemoteRewarm(err error) remoteRewarmKind {
	if err == nil {
		return remoteRewarmNone
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "master data not found"):
		return remoteRewarmMasterdata
	case strings.Contains(message, "music metas not found"),
		strings.Contains(message, "music meta not found"):
		return remoteRewarmMusicMeta
	default:
		return remoteRewarmNone
	}
}

func isUnsupportedBatchProtocolError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "http 404") ||
		strings.Contains(message, "missing field `live_type`") ||
		strings.Contains(message, "unsupported media type")
}

func isMissingUserdataHashError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "userdata_hash") &&
		(strings.Contains(message, "user data not found") || strings.Contains(message, "not found"))
}

func hashPayload(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cloneRecommendOption(option map[string]any) map[string]any {
	if option == nil {
		return map[string]any{}
	}
	copied := make(map[string]any, len(option)+1)
	for k, v := range option {
		copied[k] = v
	}
	return copied
}
