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

	target := resolveSKTrackerTarget(ctx, rankArgs, allowUID, selfWhenEmpty)
	ranks, userID, err := parseSKTrackerTargetRanks(target, allowUID)
	if err != nil {
		return nil, err
	}
	if len(ranks) == 0 && userID == nil && target.userID == "" {
		return nil, fmt.Errorf("请至少提供一个排名或UID")
	}
	defaultRanks := defaultSKRanksByMode(wlMode)
	if len(defaultRanksOverride) > 0 {
		defaultRanks = slices.Clone(defaultRanksOverride)
	}
	// Empty rank query should use mode-specific default lines.
	if !target.rankArgsProvided && userID == nil && target.userID == "" {
		ranks = defaultRanks
	}
	params := map[string]any{
		"region":          strings.ToLower(strings.TrimSpace(ctx.Region().String())),
		"region_explicit": ctx.HasExplicitRegion(),
	}
	if len(ranks) > 0 {
		params["ranks"] = ranks
	}
	if !target.rankArgsProvided && userID == nil && target.userID == "" && len(ranks) > 0 {
		params["default_ranks"] = true
	}
	applySKTrackerMetaParams(params, eventID, wlCharacterID, wlCharacterQuery, userID)
	applySKTrackerTargetParams(params, ctx, target)
	if full {
		params["full"] = true
	}
	return params, nil
}

type skTrackerTarget struct {
	rankArgs         string
	rankArgsProvided bool
	userID           string
	selector         string
}

func resolveSKTrackerTarget(ctx HarrukiSekaiHandlerContext, rankArgs string, allowUID, selfWhenEmpty bool) skTrackerTarget {
	target := skTrackerTarget{rankArgs: rankArgs, rankArgsProvided: strings.TrimSpace(rankArgs) != ""}
	if !allowUID {
		return target
	}
	if uidArg := strings.TrimSpace(ctx.UIDArg()); uidArg != "" && strings.TrimSpace(target.rankArgs) == "" {
		applySKUIDArgument(&target, uidArg, strings.TrimSpace(ctx.GetUserId()))
	}
	if selfWhenEmpty && strings.TrimSpace(target.rankArgs) == "" && target.userID == "" {
		target.userID = strings.TrimSpace(ctx.GetUserId())
	}
	return target
}

func applySKUIDArgument(target *skTrackerTarget, uidArg, requesterID string) {
	switch {
	case strings.HasPrefix(uidArg, "@"):
		if candidate := strings.TrimSpace(strings.TrimPrefix(uidArg, "@")); isDigits(candidate) {
			target.userID = candidate
		}
	case isBindingSelector(uidArg):
		target.userID = requesterID
		target.selector = strings.ToLower(uidArg)
	case isDigits(uidArg):
		target.rankArgs = uidArg
	}
}

func parseSKTrackerTargetRanks(target skTrackerTarget, allowUID bool) ([]int, *int64, error) {
	if strings.TrimSpace(target.rankArgs) == "" && target.userID != "" {
		return nil, nil, nil
	}
	return parseSKRanks(target.rankArgs, allowUID)
}

func applySKTrackerMetaParams(params map[string]any, eventID, wlCharacterID int, wlCharacterQuery string, userID *int64) {
	setIntParam(params, "event_id", eventID)
	setIntParam(params, "wl_character_id", wlCharacterID)
	setStringParam(params, "wl_character_query", strings.TrimSpace(wlCharacterQuery))
	if userID != nil && *userID > 0 {
		params["user_id"] = *userID
	}
}

func applySKTrackerTargetParams(params map[string]any, ctx HarrukiSekaiHandlerContext, target skTrackerTarget) {
	if target.userID == "" {
		return
	}
	params["target_platform"] = strings.ToLower(strings.TrimSpace(ctx.GetPlatform()))
	params["target_user_id"] = target.userID
	setStringParam(params, "target_selector", target.selector)
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
	wlMode := ctx.PrefixArg() == "wl" || wlCharacterID > 0 || strings.TrimSpace(wlCharacterQuery) != ""

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
		"ranks":                defaultSKRanksByMode(wlMode),
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
	args, compareRank, err := extractSKCompareRankArg(strings.TrimSpace(ctx.GetArgs()))
	if err != nil {
		return nil, err
	}
	eventID, wlCharacterID, wlCharacterQuery, _, rankArgs := extractSKMetaArgs(
		args,
		false,
		ctx.PrefixArg() == "wl",
	)
	if ctx.PrefixArg() == "wl" && wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "" {
		wlCharacterQuery = "wl"
	}
	params := newSKTraceParams(ctx, eventID, wlCharacterID, wlCharacterQuery, compareRank)
	target, effectiveRankArgs := resolveSKTraceTarget(ctx, rankArgs)
	if err := applySKTraceRanks(params, effectiveRankArgs); err != nil {
		return nil, err
	}
	applySKTraceTarget(params, target, ctx.GetPlatform())
	return params, nil
}

type skTraceTarget struct {
	userID   string
	selector string
}

func newSKTraceParams(ctx HarrukiSekaiHandlerContext, eventID int, wlCharacterID int, wlCharacterQuery string, compareRank int) map[string]any {
	params := map[string]any{
		"region":          strings.ToLower(strings.TrimSpace(ctx.Region().String())),
		"region_explicit": ctx.HasExplicitRegion(),
	}
	if eventID > 0 {
		params["event_id"] = eventID
	}
	if wlCharacterID > 0 {
		params["wl_character_id"] = wlCharacterID
	}
	if query := strings.TrimSpace(wlCharacterQuery); query != "" {
		params["wl_character_query"] = query
	}
	if compareRank > 0 {
		params["compare_rank"] = compareRank
	}
	return params
}

func resolveSKTraceTarget(ctx HarrukiSekaiHandlerContext, rankArgs string) (skTraceTarget, string) {
	uidArg := strings.TrimSpace(ctx.UIDArg())
	if uidArg == "" || strings.TrimSpace(rankArgs) != "" {
		return skTraceTarget{}, rankArgs
	}
	switch {
	case strings.HasPrefix(uidArg, "@"):
		candidate := strings.TrimSpace(strings.TrimPrefix(uidArg, "@"))
		if isDigits(candidate) {
			return skTraceTarget{userID: candidate}, rankArgs
		}
	case isBindingSelector(uidArg):
		return skTraceTarget{userID: strings.TrimSpace(ctx.GetUserId()), selector: strings.ToLower(uidArg)}, rankArgs
	case isDigits(uidArg):
		return skTraceTarget{}, uidArg
	}
	return skTraceTarget{}, rankArgs
}

func applySKTraceRanks(params map[string]any, rankArgs string) error {
	if strings.TrimSpace(rankArgs) == "" {
		return nil
	}
	ranks, userID, err := parseSKRanks(rankArgs, true)
	if err != nil {
		return err
	}
	if len(ranks) > 2 {
		return fmt.Errorf("ptr 最多支持两个排名，例如: /ptr 1 2")
	}
	if len(ranks) > 0 {
		params["ranks"] = ranks
	}
	if userID != nil && *userID > 0 {
		params["user_id"] = *userID
	}
	return nil
}

func applySKTraceTarget(params map[string]any, target skTraceTarget, platform string) {
	if target.userID == "" {
		return
	}
	params["target_platform"] = strings.ToLower(strings.TrimSpace(platform))
	params["target_user_id"] = target.userID
	if target.selector != "" {
		params["target_selector"] = target.selector
	}
}

func extractSKCompareRankArg(args string) (string, int, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return "", 0, nil
	}
	remaining := make([]string, 0, len(fields))
	compareRank := 0
	for _, raw := range fields {
		token := strings.TrimSpace(raw)
		if !strings.HasPrefix(token, "#") {
			remaining = append(remaining, raw)
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(token, "#"))
		if value == "" || !isDigits(value) {
			return "", 0, fmt.Errorf("分数线参数格式应为 #排名，例如: /ptr #100")
		}
		rank, err := strconv.Atoi(value)
		if err != nil || rank <= 0 {
			return "", 0, fmt.Errorf("分数线排名必须大于 0")
		}
		if compareRank > 0 {
			return "", 0, fmt.Errorf("ptr 只能指定一个分数线参数")
		}
		compareRank = rank
	}
	return strings.TrimSpace(strings.Join(remaining, " ")), compareRank, nil
}
