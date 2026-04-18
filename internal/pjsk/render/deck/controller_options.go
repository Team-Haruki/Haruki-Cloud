package deck

import (
	"slices"
	"strings"
)

func applyRecommendOptionOverrides(option map[string]any, recType string, query AutoQuery) {
	if option == nil {
		return
	}
	if algorithm := normalizeRecommendAlgorithmForService(query.Algorithm); algorithm != "" {
		option["algorithm"] = algorithm
	}
	if liveType := normalizeRecommendLiveType(recType, query.LiveType); liveType != "" {
		option["live_type"] = liveType
	}
	if target := normalizeRecommendTarget(query.Target); target != "" {
		option["target"] = target
	}
	if query.MusicID != nil && *query.MusicID > 0 {
		option["music_id"] = *query.MusicID
	}
	if diff := normalizeRecommendDifficulty(query.MusicDiff); diff != "" {
		option["music_diff"] = diff
	}
	if len(query.TargetBonuses) > 0 {
		option["target_bonus_list"] = slices.Clone(query.TargetBonuses)
	}
	if query.Boost != nil && *query.Boost >= 0 {
		option["boost"] = *query.Boost
	}
	if query.AreaItemLevel != nil && *query.AreaItemLevel > 0 {
		option["area_item_level"] = *query.AreaItemLevel
	}
	if query.UnitFilter != "" {
		option["unit_filter"] = query.UnitFilter
	}
	if query.AttrFilter != "" {
		option["attr_filter"] = query.AttrFilter
	}
	if len(query.ExcludedCards) > 0 {
		option["excluded_cards"] = slices.Clone(query.ExcludedCards)
	}
	if query.UseCurrentDeck {
		option["use_current_deck"] = true
	}
	if query.MaxProfile {
		option["max_profile"] = true
	}
	if query.SubMaxProfile {
		option["sub_max_profile"] = true
	}
	if query.MusicCompare {
		option["music_compare"] = true
	}
	if len(query.MusicCompareQueries) > 0 {
		option["music_compare_queries"] = slices.Clone(query.MusicCompareQueries)
	}
	if len(query.SpecificSkillOrder) > 0 {
		if strings.TrimSpace(query.SkillOrderChooseStrategy) == "" {
			option["skill_order_choose_strategy"] = "specific"
		}
		option["specific_skill_order"] = slices.Clone(query.SpecificSkillOrder)
	}

	explicitEventID := query.EventID != nil && *query.EventID > 0
	if explicitEventID {
		option["event_id"] = *query.EventID
	}

	attr := normalizeRecommendAttr(query.EventAttr)
	unit := normalizeRecommendUnit(query.EventUnit)
	hasSimulatedWorldBloomTurn := query.WorldBloomEventTurn != nil && *query.WorldBloomEventTurn > 0
	fakeEvent := hasSimulatedWorldBloomTurn || (!explicitEventID && (attr != "" || unit != ""))

	if attr != "" && fakeEvent {
		option["event_attr"] = attr
	}
	if unit != "" && (hasSimulatedWorldBloomTurn || !explicitEventID) {
		option["event_unit"] = unit
	}
	if hasSimulatedWorldBloomTurn {
		option["world_bloom_event_turn"] = *query.WorldBloomEventTurn
		option["event_type"] = "world_bloom"
	}
	if query.WorldBloomCharacterID != nil && *query.WorldBloomCharacterID > 0 {
		option["world_bloom_character_id"] = *query.WorldBloomCharacterID
	}
	if fakeEvent {
		option["event_id"] = nil
	}

	if query.ChallengeLiveCharacterID != nil && *query.ChallengeLiveCharacterID > 0 {
		option["challenge_live_character_id"] = *query.ChallengeLiveCharacterID
	}
	if len(query.FixedCards) > 0 {
		option["fixed_cards"] = slices.Clone(query.FixedCards)
	}
	if len(query.FixedCharacters) > 0 {
		option["fixed_characters"] = slices.Clone(query.FixedCharacters)
	}

	applyDeckConfigPatch(option, "rarity_1_config", query.Rarity1Config)
	applyDeckConfigPatch(option, "rarity_2_config", query.Rarity2Config)
	applyDeckConfigPatch(option, "rarity_3_config", query.Rarity3Config)
	applyDeckConfigPatch(option, "rarity_4_config", query.Rarity4Config)
	applyDeckConfigPatch(option, "rarity_birthday_config", query.RarityBirthdayConfig)
	if len(query.SingleCardConfigs) > 0 {
		option["single_card_configs"] = toSingleCardConfigInterfaces(query.SingleCardConfigs)
	}

	if query.MultiLiveTeammatePower != nil && *query.MultiLiveTeammatePower > 0 {
		option["multi_live_teammate_power"] = *query.MultiLiveTeammatePower
	}
	if query.MultiLiveTeammateScoreUp != nil && *query.MultiLiveTeammateScoreUp >= 0 {
		option["multi_live_teammate_score_up"] = *query.MultiLiveTeammateScoreUp
	}
	if query.MultiLiveScoreUpLowerBound != nil && *query.MultiLiveScoreUpLowerBound >= 0 {
		option["multi_live_score_up_lower_bound"] = *query.MultiLiveScoreUpLowerBound
	}
	if strategy := normalizeRecommendStrategy(query.SkillOrderChooseStrategy); strategy != "" {
		if strategy != "specific" || len(query.SpecificSkillOrder) > 0 {
			option["skill_order_choose_strategy"] = strategy
		}
	}
	if strategy := normalizeRecommendStrategy(query.SkillReferenceChooseStrategy); strategy != "" {
		option["skill_reference_choose_strategy"] = strategy
	}
	if query.KeepAfterTrainingState {
		option["keep_after_training_state"] = true
	}
}

func normalizeRecommendLiveOptions(option map[string]any) {
	if option == nil {
		return
	}
	if optionString(option, "live_type") == "multi" {
		if _, ok := option["multi_live_teammate_power"]; !ok {
			option["multi_live_teammate_power"] = 250000
		}
		if _, ok := option["multi_live_teammate_score_up"]; !ok {
			option["multi_live_teammate_score_up"] = 200
		}
		return
	}
	delete(option, "multi_live_teammate_power")
	delete(option, "multi_live_teammate_score_up")
	delete(option, "multi_live_score_up_lower_bound")
}

func applyRecommendStrategyDefaults(option map[string]any, recType string) {
	if option == nil {
		return
	}
	if _, ok := option["skill_order_choose_strategy"]; !ok {
		option["skill_order_choose_strategy"] = defaultRecommendSkillOrderStrategy(option, recType)
	}
	if _, ok := option["skill_reference_choose_strategy"]; !ok {
		option["skill_reference_choose_strategy"] = defaultRecommendSkillReferenceStrategy(option, recType)
	}
}

func defaultRecommendSkillOrderStrategy(option map[string]any, recType string) string {
	liveType := optionString(option, "live_type")
	if recType == "challenge" && liveType != "challenge_auto" && liveType != "auto" {
		return "max"
	}
	return "average"
}

func defaultRecommendSkillReferenceStrategy(option map[string]any, recType string) string {
	liveType := optionString(option, "live_type")
	if recType == "challenge" && liveType != "challenge_auto" && liveType != "auto" {
		return "max"
	}
	return "average"
}

func applyDeckConfigPatch(option map[string]any, key string, patch *CardConfigPatch) {
	if option == nil || patch == nil {
		return
	}
	cfg, _ := option[key].(map[string]any)
	if cfg == nil {
		cfg = noChangeDeckConfig()
	}
	if patch.Disable {
		cfg["disable"] = true
	}
	if patch.LevelMax {
		cfg["level_max"] = true
	}
	if patch.EpisodeRead {
		cfg["episode_read"] = true
	}
	if patch.MasterMax {
		cfg["master_max"] = true
	}
	if patch.SkillMax {
		cfg["skill_max"] = true
	}
	if patch.Canvas {
		cfg["canvas"] = true
	}
	option[key] = cfg
}

func toSingleCardConfigInterfaces(cfgs []SingleCardConfigPatch) []any {
	if len(cfgs) == 0 {
		return nil
	}
	result := make([]any, 0, len(cfgs))
	for _, cfg := range cfgs {
		if cfg.CardID <= 0 {
			continue
		}
		result = append(result, map[string]any{
			"card_id":      cfg.CardID,
			"disable":      cfg.Disable,
			"level_max":    cfg.LevelMax,
			"episode_read": cfg.EpisodeRead,
			"master_max":   cfg.MasterMax,
			"skill_max":    cfg.SkillMax,
			"canvas":       cfg.Canvas,
		})
	}
	return result
}
