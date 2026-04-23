package deck

import (
	"math"
	"strings"
)

func expandRecommendBatchOptions(recommender PjskDeckRecommender, recType string, option map[string]any) []map[string]any {
	if recommender == nil || option == nil {
		return nil
	}
	if shouldTuneMysekaiRLGaOptions(recommender, recType, option) {
		option = applyMysekaiRLTunedGaOptions(option)
	}

	expanded := recommender.ExpandAlgorithms(option)
	if !shouldUseMysekaiRLFallback(recType, option) {
		return expanded
	}

	seen := make(map[string]struct{}, len(expanded)+2)
	result := make([]map[string]any, 0, len(expanded)+2)
	appendOption := func(item map[string]any) {
		if item == nil {
			return
		}
		alg := normalizeRecommendAlgorithmForService(optionString(item, "algorithm"))
		if alg == "" {
			return
		}
		if _, ok := seen[alg]; ok {
			return
		}
		seen[alg] = struct{}{}
		result = append(result, item)
	}

	for _, item := range expanded {
		appendOption(item)
	}
	for _, alg := range []string{"ga", "dfs_ga"} {
		fallback := cloneRecommendOption(option)
		fallback["algorithm"] = alg
		appendOption(fallback)
	}
	return result
}

func shouldUseMysekaiRLFallback(recType string, option map[string]any) bool {
	return recType == "mysekai" && normalizeRecommendAlgorithmForService(optionString(option, "algorithm")) == "rl"
}

func shouldTuneMysekaiRLGaOptions(recommender PjskDeckRecommender, recType string, option map[string]any) bool {
	if recommender == nil || recType != "mysekai" || option == nil {
		return false
	}
	algorithm := strings.ToLower(strings.TrimSpace(optionString(option, "algorithm")))
	if normalizeRecommendAlgorithmForService(algorithm) == "rl" {
		return true
	}
	if algorithm != "all" {
		return false
	}
	for _, item := range recommender.ExpandAlgorithms(option) {
		if normalizeRecommendAlgorithmForService(optionString(item, "algorithm")) == "rl" {
			return true
		}
	}
	return false
}

func applyMysekaiRLTunedGaOptions(option map[string]any) map[string]any {
	tuned := cloneRecommendOption(option)
	gaOptions := map[string]any{
		"max_iter":            256,
		"max_no_improve_iter": 6,
		"pop_size":            8000,
		"parent_size":         800,
		"elite_size":          80,
	}

	if existing, ok := option["ga_options"].(map[string]any); ok && len(existing) > 0 {
		merged := make(map[string]any, len(gaOptions)+len(existing))
		for k, v := range gaOptions {
			merged[k] = v
		}
		for k, v := range existing {
			merged[k] = v
		}
		tuned["ga_options"] = merged
		return tuned
	}

	tuned["ga_options"] = gaOptions
	return tuned
}

func compareRecommendDecks(recType, target string, left, right RecommendDeck) bool {
	if recType == "mysekai" {
		return compareMysekaiDecks(left, right)
	}
	if target == "power" {
		return left.TotalPower > right.TotalPower
	}
	if target == "skill" {
		return left.MultiLiveScoreUp > right.MultiLiveScoreUp
	}
	if target == "bonus" {
		if left.EventBonusRate != right.EventBonusRate {
			return left.EventBonusRate > right.EventBonusRate
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		return left.MultiLiveScoreUp > right.MultiLiveScoreUp
	}
	leftScore := left.Score
	rightScore := right.Score
	if recType == "no_event" {
		if left.LiveScore > 0 {
			leftScore = left.LiveScore
		}
		if right.LiveScore > 0 {
			rightScore = right.LiveScore
		}
	}
	if leftScore != rightScore {
		return leftScore > rightScore
	}
	return left.MultiLiveScoreUp > right.MultiLiveScoreUp
}

func compareMysekaiDecks(left, right RecommendDeck) bool {
	if left.MysekaiEventPoint != right.MysekaiEventPoint {
		return left.MysekaiEventPoint > right.MysekaiEventPoint
	}

	leftInternal := mysekaiInternalPoint(left)
	rightInternal := mysekaiInternalPoint(right)
	if !floatAlmostEqual(leftInternal, rightInternal) {
		return leftInternal > rightInternal
	}

	leftBonus := mysekaiCombinedBonusRate(left)
	rightBonus := mysekaiCombinedBonusRate(right)
	if !floatAlmostEqual(leftBonus, rightBonus) {
		return leftBonus > rightBonus
	}

	if left.TotalPower != right.TotalPower {
		return left.TotalPower > right.TotalPower
	}
	if !floatAlmostEqual(left.SupportDeckBonusRate, right.SupportDeckBonusRate) {
		return left.SupportDeckBonusRate > right.SupportDeckBonusRate
	}
	if !floatAlmostEqual(left.EventBonusRate, right.EventBonusRate) {
		return left.EventBonusRate > right.EventBonusRate
	}
	return left.MultiLiveScoreUp > right.MultiLiveScoreUp
}

func mysekaiInternalPoint(deck RecommendDeck) float64 {
	powerBonus := math.Floor((1+float64(deck.TotalPower)/450000.0)*10+1e-6) / 10.0
	eventBonus := math.Floor(mysekaiCombinedBonusRate(deck)+1e-6) / 100.0
	return powerBonus * (1 + eventBonus) * 500
}

func mysekaiCombinedBonusRate(deck RecommendDeck) float64 {
	return deck.EventBonusRate + deck.SupportDeckBonusRate
}

func floatAlmostEqual(left, right float64) bool {
	return math.Abs(left-right) < 1e-9
}
