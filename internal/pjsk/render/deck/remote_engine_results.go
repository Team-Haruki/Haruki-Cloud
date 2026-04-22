package deck

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
		for i := range items {
			if strings.TrimSpace(items[i].Alg) == "" && i < len(options) {
				if alg, _ := options[i]["algorithm"].(string); strings.TrimSpace(alg) != "" {
					items[i].Alg = alg
				}
			}
		}
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

func aggregateRemoteRecommendResults(options []map[string]any, results []remoteBatchRecommendResult) (*RecommendResult, error) {
	agg := &RecommendResult{
		CostTimes: make(map[string]float64),
		WaitTimes: make(map[string]float64),
	}
	seen := make(map[string]*RecommendDeck)
	var order []string
	var firstErr error

	for _, item := range results {
		if strings.TrimSpace(item.Error) != "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s", strings.TrimSpace(item.Error))
			}
			continue
		}

		var decks []RecommendDeck
		if item.Result != nil {
			decks = convertRemoteDecks(item.Result.Decks)
		} else {
			decks = convertRemoteDecks(item.Decks)
		}
		if len(decks) == 0 {
			continue
		}

		alg := displayRecommendAlgorithm(item.Alg)
		if alg != "" {
			agg.CostTimes[alg] = item.CostTime
			agg.WaitTimes[alg] = item.WaitTime
		}

		for _, deck := range decks {
			h := deckHash(deck)
			if existing, ok := seen[h]; ok {
				if alg != "" {
					existing.Algs = append(existing.Algs, alg)
				}
				continue
			}
			deckCopy := deck
			if alg != "" {
				deckCopy.Algs = []string{alg}
			}
			seen[h] = &deckCopy
			order = append(order, h)
		}
	}
	if len(order) == 0 && firstErr != nil {
		return nil, firstErr
	}

	type pair struct {
		Deck RecommendDeck
		Alg  string
	}
	var pairs []pair
	for _, h := range order {
		deck := seen[h]
		algsMap := make(map[string]struct{})
		for _, a := range deck.Algs {
			algsMap[a] = struct{}{}
		}
		var algs []string
		for alg := range algsMap {
			algs = append(algs, alg)
		}
		sort.Strings(algs)
		pairs = append(pairs, pair{Deck: *deck, Alg: strings.Join(algs, "+")})
	}

	liveType, _ := options[0]["live_type"].(string)
	target, _ := options[0]["target"].(string)
	sort.SliceStable(pairs, func(i, j int) bool {
		d1 := pairs[i].Deck
		d2 := pairs[j].Deck
		return compareRecommendDecks(liveType, target, d1, d2)
	})

	limitFloat, _ := options[0]["limit"].(float64)
	limitInt, ok := options[0]["limit"].(int)
	if !ok {
		limitInt = int(limitFloat)
	}
	if limitInt <= 0 {
		limitInt = len(pairs)
	}
	if limitInt > len(pairs) {
		limitInt = len(pairs)
	}

	for i := 0; i < limitInt; i++ {
		agg.Decks = append(agg.Decks, pairs[i].Deck)
		agg.DeckAlgs = append(agg.DeckAlgs, pairs[i].Alg)
	}
	return agg, nil
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
