package handler

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func buildSKTrackerParams(ctx HarrukiSekaiHandlerContext, defaultFull bool, allowUID bool, selfWhenEmpty bool) (map[string]any, error) {
	return buildSKTrackerParamsWithDefaultRanks(ctx, defaultFull, allowUID, selfWhenEmpty, nil)
}

func buildSKTrackerParamsWithDefaultRanks(ctx HarrukiSekaiHandlerContext, defaultFull bool, allowUID bool, selfWhenEmpty bool, defaultRanksOverride []int) (map[string]any, error) {
	eventID, wlCharacterID, wlCharacterQuery, full, rankArgs := extractSKMetaArgs(
		strings.TrimSpace(ctx.GetArgs()),
		defaultFull,
		ctx.PrefixArg() == "wl",
	)
	wlMode := ctx.PrefixArg() == "wl" || wlCharacterID > 0 || strings.TrimSpace(wlCharacterQuery) != ""
	if ctx.PrefixArg() == "wl" && wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "" {
		wlCharacterQuery = "wl"
		wlMode = true
	}

	effectiveRankArgs := rankArgs
	rankArgsProvided := strings.TrimSpace(effectiveRankArgs) != ""
	targetUserID := ""
	targetSelector := ""
	if allowUID {
		if uidArg := strings.TrimSpace(ctx.UIDArg()); uidArg != "" && strings.TrimSpace(effectiveRankArgs) == "" {
			switch {
			case strings.HasPrefix(uidArg, "@"):
				candidate := strings.TrimSpace(strings.TrimPrefix(uidArg, "@"))
				if isDigits(candidate) {
					targetUserID = candidate
				}
			case isBindingSelector(uidArg):
				targetUserID = strings.TrimSpace(ctx.GetUserId())
				targetSelector = strings.ToLower(uidArg)
			case isDigits(uidArg):
				effectiveRankArgs = uidArg
			}
		}
		if selfWhenEmpty && strings.TrimSpace(effectiveRankArgs) == "" && targetUserID == "" {
			targetUserID = strings.TrimSpace(ctx.GetUserId())
		}
	}

	var (
		ranks  []int
		userID *int64
	)
	if strings.TrimSpace(effectiveRankArgs) != "" || targetUserID == "" {
		var err error
		ranks, userID, err = parseSKRanks(effectiveRankArgs, allowUID)
		if err != nil {
			return nil, err
		}
	}

	if len(ranks) == 0 && userID == nil && targetUserID == "" {
		return nil, fmt.Errorf("请至少提供一个排名或UID")
	}

	defaultRanks := defaultSKRanksByMode(wlMode)
	if len(defaultRanksOverride) > 0 {
		defaultRanks = slices.Clone(defaultRanksOverride)
	}
	// Empty rank query should use mode-specific default lines.
	if !rankArgsProvided && userID == nil && targetUserID == "" {
		ranks = defaultRanks
	}
	params := map[string]any{
		"region":          strings.ToLower(strings.TrimSpace(ctx.Region().String())),
		"region_explicit": ctx.HasExplicitRegion(),
	}
	if len(ranks) > 0 {
		params["ranks"] = ranks
	}
	if !rankArgsProvided && userID == nil && targetUserID == "" && len(ranks) > 0 {
		params["default_ranks"] = true
	}
	if eventID > 0 {
		params["event_id"] = eventID
	}
	if wlCharacterID > 0 {
		params["wl_character_id"] = wlCharacterID
	}
	if strings.TrimSpace(wlCharacterQuery) != "" {
		params["wl_character_query"] = strings.TrimSpace(wlCharacterQuery)
	}
	if userID != nil && *userID > 0 {
		params["user_id"] = *userID
	}
	if targetUserID != "" {
		params["target_platform"] = strings.ToLower(strings.TrimSpace(ctx.GetPlatform()))
		params["target_user_id"] = targetUserID
		if targetSelector != "" {
			params["target_selector"] = targetSelector
		}
	}
	if full {
		params["full"] = true
	}
	return params, nil
}

func buildSKSpeedTrackerParams(ctx HarrukiSekaiHandlerContext, unit string, defaultPeriodValue int64, periodScaleSeconds int64) (map[string]any, error) {
	eventID, wlCharacterID, wlCharacterQuery, _, periodArgs := extractSKMetaArgs(
		strings.TrimSpace(ctx.GetArgs()),
		false,
		ctx.PrefixArg() == "wl",
	)
	if ctx.PrefixArg() == "wl" && wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "" {
		wlCharacterQuery = "wl"
	}

	unit = strings.ToLower(strings.TrimSpace(unit))
	if unit == "" {
		unit = "h"
	}
	if defaultPeriodValue <= 0 {
		defaultPeriodValue = 1
	}
	if periodScaleSeconds <= 0 {
		periodScaleSeconds = 60 * 60
	}

	periodValue := defaultPeriodValue
	if raw := strings.TrimSpace(periodArgs); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			periodValue = parsed
		}
	}

	params := map[string]any{
		"region":               strings.ToLower(strings.TrimSpace(ctx.Region().String())),
		"region_explicit":      ctx.HasExplicitRegion(),
		"ranks":                slices.Clone(defaultSKSpeedRanks),
		"default_ranks":        true,
		"speed_unit":           unit,
		"speed_period_seconds": periodValue * periodScaleSeconds,
	}
	if eventID > 0 {
		params["event_id"] = eventID
	}
	if wlCharacterID > 0 {
		params["wl_character_id"] = wlCharacterID
	}
	if strings.TrimSpace(wlCharacterQuery) != "" {
		params["wl_character_query"] = strings.TrimSpace(wlCharacterQuery)
	}
	return params, nil
}

func buildSKPlayerTraceParams(ctx HarrukiSekaiHandlerContext) (map[string]any, error) {
	eventID, wlCharacterID, wlCharacterQuery, _, rankArgs := extractSKMetaArgs(
		strings.TrimSpace(ctx.GetArgs()),
		false,
		ctx.PrefixArg() == "wl",
	)

	params := map[string]any{
		"region":          strings.ToLower(strings.TrimSpace(ctx.Region().String())),
		"region_explicit": ctx.HasExplicitRegion(),
	}
	if ctx.PrefixArg() == "wl" && wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "" {
		wlCharacterQuery = "wl"
	}
	if eventID > 0 {
		params["event_id"] = eventID
	}
	if wlCharacterID > 0 {
		params["wl_character_id"] = wlCharacterID
	}
	if strings.TrimSpace(wlCharacterQuery) != "" {
		params["wl_character_query"] = strings.TrimSpace(wlCharacterQuery)
	}

	targetUserID := ""
	targetSelector := ""
	if uidArg := strings.TrimSpace(ctx.UIDArg()); uidArg != "" && strings.TrimSpace(rankArgs) == "" {
		switch {
		case strings.HasPrefix(uidArg, "@"):
			candidate := strings.TrimSpace(strings.TrimPrefix(uidArg, "@"))
			if isDigits(candidate) {
				targetUserID = candidate
			}
		case isBindingSelector(uidArg):
			targetUserID = strings.TrimSpace(ctx.GetUserId())
			targetSelector = strings.ToLower(uidArg)
		case isDigits(uidArg):
			rankArgs = uidArg
		}
	}

	if strings.TrimSpace(rankArgs) != "" {
		ranks, userID, err := parseSKRanks(rankArgs, true)
		if err != nil {
			return nil, err
		}
		if len(ranks) > 2 {
			return nil, fmt.Errorf("ptr 最多支持两个排名，例如: /ptr 1 2")
		}
		if len(ranks) > 0 {
			params["ranks"] = ranks
		}
		if userID != nil && *userID > 0 {
			params["user_id"] = *userID
		}
	}

	if targetUserID != "" {
		params["target_platform"] = strings.ToLower(strings.TrimSpace(ctx.GetPlatform()))
		params["target_user_id"] = targetUserID
		if targetSelector != "" {
			params["target_selector"] = targetSelector
		}
	}

	return params, nil
}
