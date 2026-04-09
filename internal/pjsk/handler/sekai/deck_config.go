package sekai

import (
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"strings"
)

func extractDeckCardConfigs(args string, params *deckAutoQueryParams, defaultNoChange bool) string {
	_ = defaultNoChange
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if applied := applyDeckRarityConfig(field, params); applied {
			continue
		}
		if applied := applyDeckSingleCardConfig(field, params); applied {
			continue
		}
		remaining = append(remaining, field)
	}

	args = strings.TrimSpace(strings.Join(remaining, " "))
	var globalPatch renderdeck.CardConfigPatch
	args, globalPatch = extractGlobalDeckCardConfig(args)
	applyGlobalDeckCardConfig(params, globalPatch)
	if containsDeckKeyword(args, deckKeepAfterTrainingKeywords) {
		params.KeepAfterTrainingState = true
		for _, keyword := range deckKeepAfterTrainingKeywords {
			args = strings.Replace(args, keyword, "", 1)
		}
	}
	return normalizeDeckSpaces(args)
}

func applyDeckRarityConfig(field string, params *deckAutoQueryParams) bool {
	for _, item := range deckRarityPrefixes {
		if !strings.HasPrefix(field, item.prefix) {
			continue
		}
		patch, ok := parseDeckCardConfigPatch(field[len(item.prefix):])
		if !ok {
			return false
		}
		item.apply(params, patch)
		return true
	}
	return false
}

func applyDeckSingleCardConfig(field string, params *deckAutoQueryParams) bool {
	cardID := deckLeadingDigits(field)
	if cardID <= 0 {
		return false
	}
	patch, ok := parseDeckCardConfigPatch(field)
	if !ok {
		return false
	}
	upsertDeckSingleCardConfig(params, cardID, patch)
	return true
}

func extractGlobalDeckCardConfig(args string) (string, renderdeck.CardConfigPatch) {
	patch := renderdeck.CardConfigPatch{}
	for _, keyword := range deckDisableKeywords {
		if strings.Contains(args, keyword) {
			patch.Disable = true
			args = strings.Replace(args, keyword, "", 1)
			break
		}
	}
	for _, keyword := range deckSkillMaxKeywords {
		if strings.Contains(args, keyword) {
			patch.SkillMax = true
			args = strings.Replace(args, keyword, "", 1)
			break
		}
	}
	for _, keyword := range deckMasterMaxKeywords {
		if strings.Contains(args, keyword) {
			patch.MasterMax = true
			args = strings.Replace(args, keyword, "", 1)
			break
		}
	}
	for _, keyword := range deckEpisodeReadKeywords {
		if strings.Contains(args, keyword) {
			patch.EpisodeRead = true
			args = strings.Replace(args, keyword, "", 1)
			break
		}
	}
	for _, keyword := range deckCanvasKeywords {
		if strings.Contains(args, keyword) {
			patch.Canvas = true
			args = strings.Replace(args, keyword, "", 1)
			break
		}
	}
	return normalizeDeckSpaces(args), patch
}

func parseDeckCardConfigPatch(segment string) (renderdeck.CardConfigPatch, bool) {
	patch := renderdeck.CardConfigPatch{}
	for _, keyword := range deckDisableKeywords {
		if strings.Contains(segment, keyword) {
			patch.Disable = true
			break
		}
	}
	for _, keyword := range deckSkillMaxKeywords {
		if strings.Contains(segment, keyword) {
			patch.SkillMax = true
			break
		}
	}
	for _, keyword := range deckMasterMaxKeywords {
		if strings.Contains(segment, keyword) {
			patch.MasterMax = true
			break
		}
	}
	for _, keyword := range deckEpisodeReadKeywords {
		if strings.Contains(segment, keyword) {
			patch.EpisodeRead = true
			break
		}
	}
	for _, keyword := range deckCanvasKeywords {
		if strings.Contains(segment, keyword) {
			patch.Canvas = true
			break
		}
	}
	return patch, patch.Disable || patch.SkillMax || patch.MasterMax || patch.EpisodeRead || patch.Canvas
}

func applyGlobalDeckCardConfig(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
	if !hasDeckCardConfigPatch(patch) {
		return
	}
	mergeDeckCardConfigPatch(&params.Rarity1Config, patch)
	mergeDeckCardConfigPatch(&params.Rarity2Config, patch)
	mergeDeckCardConfigPatch(&params.Rarity3Config, patch)
	mergeDeckCardConfigPatch(&params.Rarity4Config, patch)
	mergeDeckCardConfigPatch(&params.RarityBirthdayConfig, patch)
}

func hasDeckCardConfigPatch(patch renderdeck.CardConfigPatch) bool {
	return patch.Disable || patch.LevelMax || patch.EpisodeRead || patch.MasterMax || patch.SkillMax || patch.Canvas
}

func mergeDeckCardConfigPatch(target **renderdeck.CardConfigPatch, patch renderdeck.CardConfigPatch) {
	if !hasDeckCardConfigPatch(patch) {
		return
	}
	if *target == nil {
		*target = &renderdeck.CardConfigPatch{}
	}
	if patch.Disable {
		(*target).Disable = true
	}
	if patch.LevelMax {
		(*target).LevelMax = true
	}
	if patch.EpisodeRead {
		(*target).EpisodeRead = true
	}
	if patch.MasterMax {
		(*target).MasterMax = true
	}
	if patch.SkillMax {
		(*target).SkillMax = true
	}
	if patch.Canvas {
		(*target).Canvas = true
	}
}

func upsertDeckSingleCardConfig(params *deckAutoQueryParams, cardID int, patch renderdeck.CardConfigPatch) {
	if cardID <= 0 {
		return
	}
	for idx := range params.SingleCardConfigs {
		if params.SingleCardConfigs[idx].CardID != cardID {
			continue
		}
		params.SingleCardConfigs[idx].LevelMax = true
		if patch.Disable {
			params.SingleCardConfigs[idx].Disable = true
		}
		if patch.EpisodeRead {
			params.SingleCardConfigs[idx].EpisodeRead = true
		}
		if patch.MasterMax {
			params.SingleCardConfigs[idx].MasterMax = true
		}
		if patch.SkillMax {
			params.SingleCardConfigs[idx].SkillMax = true
		}
		if patch.Canvas {
			params.SingleCardConfigs[idx].Canvas = true
		}
		return
	}
	params.SingleCardConfigs = append(params.SingleCardConfigs, renderdeck.SingleCardConfigPatch{
		CardID:      cardID,
		LevelMax:    true,
		Disable:     patch.Disable,
		EpisodeRead: patch.EpisodeRead,
		MasterMax:   patch.MasterMax,
		SkillMax:    patch.SkillMax,
		Canvas:      patch.Canvas,
	})
}
