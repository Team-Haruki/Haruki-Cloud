package sekai

import (
	"fmt"
	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type deckAutoQueryParams struct {
	EventID                      *int                               `json:"event_id,omitempty"`
	TargetBonuses                []int                              `json:"target_bonuses,omitempty"`
	Args                         string                             `json:"args,omitempty"`
	Algorithm                    string                             `json:"algorithm,omitempty"`
	LiveType                     string                             `json:"live_type,omitempty"`
	Target                       string                             `json:"target,omitempty"`
	MusicQuery                   string                             `json:"music_query,omitempty"`
	MusicID                      *int                               `json:"music_id,omitempty"`
	MusicDiff                    string                             `json:"music_diff,omitempty"`
	EventAttr                    string                             `json:"event_attr,omitempty"`
	EventUnit                    string                             `json:"event_unit,omitempty"`
	WorldBloomCharacterID        *int                               `json:"world_bloom_character_id,omitempty"`
	WorldBloomCharacterQuery     string                             `json:"world_bloom_character_query,omitempty"`
	WorldBloomEventTurn          *int                               `json:"world_bloom_event_turn,omitempty"`
	ChallengeLiveCharacterID     *int                               `json:"challenge_live_character_id,omitempty"`
	ChallengeLiveCharacterQuery  string                             `json:"challenge_live_character_query,omitempty"`
	FixedCards                   []int                              `json:"fixed_cards,omitempty"`
	FixedCharacters              []int                              `json:"fixed_characters,omitempty"`
	FixedCharacterQueries        []string                           `json:"fixed_character_queries,omitempty"`
	Rarity1Config                *renderdeck.CardConfigPatch        `json:"rarity_1_config,omitempty"`
	Rarity2Config                *renderdeck.CardConfigPatch        `json:"rarity_2_config,omitempty"`
	Rarity3Config                *renderdeck.CardConfigPatch        `json:"rarity_3_config,omitempty"`
	Rarity4Config                *renderdeck.CardConfigPatch        `json:"rarity_4_config,omitempty"`
	RarityBirthdayConfig         *renderdeck.CardConfigPatch        `json:"rarity_birthday_config,omitempty"`
	SingleCardConfigs            []renderdeck.SingleCardConfigPatch `json:"single_card_configs,omitempty"`
	MultiLiveTeammatePower       *int                               `json:"multi_live_teammate_power,omitempty"`
	MultiLiveTeammateScoreUp     *int                               `json:"multi_live_teammate_score_up,omitempty"`
	MultiLiveScoreUpLowerBound   *float64                           `json:"multi_live_score_up_lower_bound,omitempty"`
	SkillOrderChooseStrategy     string                             `json:"skill_order_choose_strategy,omitempty"`
	SkillReferenceChooseStrategy string                             `json:"skill_reference_choose_strategy,omitempty"`
	KeepAfterTrainingState       bool                               `json:"keep_after_training_state,omitempty"`
}

var (
	deckEventIDRegex = regexp.MustCompile(`(?i)(活动|event)\s*(\d+)`)
	deckWlTurnRegex  = regexp.MustCompile(`(?i)\bwl([12])\b`)
)

var deckPowerTargetKeywords = []string{"综合力", "综合", "总合力", "总和", "power"}
var deckSkillTargetKeywords = []string{"倍率", "实效", "skill", "时效"}
var deckSkillMaxKeywords = []string{"满技能", "满技", "skillmax", "技能满级", "slv4"}
var deckMasterMaxKeywords = []string{"满突破", "满破", "rankmax", "mastermax", "5破", "五破"}
var deckEpisodeReadKeywords = []string{"剧情已读", "满剧情", "前后篇已读", "前后篇", "已读"}
var deckCanvasKeywords = []string{"满画布", "全画布", "画布", "满画板", "全画板", "画板"}
var deckDisableKeywords = []string{"禁用", "disable"}
var deckKeepAfterTrainingKeywords = []string{"bfes不变", "bf不变"}
var deckTeammatePowerKeywords = []string{"队友综合力", "队友总合力", "队友综合", "队友总和"}
var deckTeammateScoreUpKeywords = []string{"队友实效", "队友技能", "队友时效"}
var deckSkillOrderKeywords = []string{"技能顺序", "技能排列"}
var deckSkillReferenceKeywords = []string{"技能抽取", "技能吸取"}
var deckMaxKeywords = []string{"最高", "最大", "最优", "最强", "最佳"}
var deckMinKeywords = []string{"最低", "最小", "最差", "最弱", "最烂"}
var deckAverageKeywords = []string{"平均", "均值", "期望"}

var deckRarityPrefixes = []struct {
	prefix string
	apply  func(*deckAutoQueryParams, renderdeck.CardConfigPatch)
}{
	{"生日", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.RarityBirthdayConfig, patch)
	}},
	{"一星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity1Config, patch)
	}},
	{"1星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity1Config, patch)
	}},
	{"二星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity2Config, patch)
	}},
	{"2星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity2Config, patch)
	}},
	{"三星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity3Config, patch)
	}},
	{"3星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity3Config, patch)
	}},
	{"四星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity4Config, patch)
	}},
	{"4星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity4Config, patch)
	}},
}

type deckAliasRule struct {
	re   *regexp.Regexp
	unit string
}

var deckUnitAliasRules = newDeckUnitAliasRules()

func buildDeckQueryParams(ctx SekaiHandlerContext, mode string) (deckAutoQueryParams, error) {
	args := strings.TrimSpace(strings.ToLower(ctx.GetArgs()))
	params := deckAutoQueryParams{}
	var err error

	switch mode {
	case "deck-event":
		args, err = buildEventDeckParams(args, &params, ctx.originalTriggerCmd)
	case "deck-challenge":
		args, err = buildChallengeDeckParams(args, &params)
	case "deck-no-event":
		args, err = buildNoEventDeckParams(args, &params)
	case "deck-bonus":
		args, err = buildBonusDeckParams(args, &params, ctx.originalTriggerCmd)
	case "deck-mysekai":
		args, err = buildMysekaiDeckParams(args, &params, ctx.originalTriggerCmd)
	default:
		err = fmt.Errorf("unsupported deck mode: %s", mode)
	}
	if err != nil {
		return deckAutoQueryParams{}, err
	}
	if args = strings.TrimSpace(args); args != "" {
		params.Args = args
	}
	return params, nil
}

func buildEventDeckParams(args string, params *deckAutoQueryParams, trigger string) (string, error) {
	args, err := extractDeckCommonParams(args, params, deckCommonConfig{
		allowLiveType:      true,
		allowMultiLive:     true,
		allowTarget:        true,
		allowAlgorithm:     true,
		allowRandom:        true,
		allowFixed:         true,
		allowCardConfig:    true,
		defaultArgsTrigger: trigger,
	})
	if err != nil {
		return "", err
	}
	args, err = extractDeckEventSelection(args, params, trigger)
	if err != nil {
		return "", err
	}
	return extractDeckMusicQuery(args, params)
}

func buildChallengeDeckParams(args string, params *deckAutoQueryParams) (string, error) {
	args, err := extractDeckCommonParams(args, params, deckCommonConfig{
		allowLiveType:   true,
		allowTarget:     true,
		allowAlgorithm:  true,
		allowRandom:     true,
		allowFixed:      true,
		allowCardConfig: true,
	})
	if err != nil {
		return "", err
	}
	charID, charQuery, remaining := extractDeckCharacterCandidate(args, true)
	if charID > 0 {
		params.ChallengeLiveCharacterID = intPtr(charID)
		args = remaining
	} else if charQuery != "" {
		params.ChallengeLiveCharacterQuery = charQuery
		args = remaining
	}
	return extractDeckMusicQuery(args, params)
}

func buildNoEventDeckParams(args string, params *deckAutoQueryParams) (string, error) {
	args, err := extractDeckCommonParams(args, params, deckCommonConfig{
		allowLiveType:   true,
		allowMultiLive:  true,
		allowTarget:     true,
		allowAlgorithm:  true,
		allowRandom:     true,
		allowFixed:      true,
		allowCardConfig: true,
	})
	if err != nil {
		return "", err
	}
	return extractDeckMusicQuery(args, params)
}

func buildBonusDeckParams(args string, params *deckAutoQueryParams, trigger string) (string, error) {
	eventID, remaining := extractDeckEventID(args)
	if eventID != nil {
		params.EventID = eventID
		args = remaining
	}

	fields := strings.Fields(args)
	if len(fields) == 0 {
		return "", onebot11.NewReplayError("使用方式:\n%s event123 120 160", trigger)
	}
	bonuses := make([]int, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || value <= 0 {
			return "", onebot11.NewReplayError("使用方式:\n%s event123 120 160", trigger)
		}
		bonuses = append(bonuses, value)
	}
	params.TargetBonuses = bonuses
	return "", nil
}

func buildMysekaiDeckParams(args string, params *deckAutoQueryParams, trigger string) (string, error) {
	args, err := extractDeckCommonParams(args, params, deckCommonConfig{
		allowFixed:         true,
		allowCardConfig:    true,
		defaultNoChange:    true,
		defaultArgsTrigger: trigger,
	})
	if err != nil {
		return "", err
	}
	return extractDeckEventSelection(args, params, trigger)
}

type deckCommonConfig struct {
	allowLiveType      bool
	allowMultiLive     bool
	allowTarget        bool
	allowAlgorithm     bool
	allowRandom        bool
	allowFixed         bool
	allowCardConfig    bool
	defaultNoChange    bool
	defaultArgsTrigger string
}

func extractDeckCommonParams(args string, params *deckAutoQueryParams, cfg deckCommonConfig) (string, error) {
	var err error
	if cfg.allowFixed {
		args, err = extractDeckFixedTargets(args, params)
		if err != nil {
			return "", err
		}
	}
	if cfg.allowAlgorithm {
		args = extractDeckAlgorithm(args, params)
	}
	if cfg.allowLiveType {
		args = extractDeckLiveType(args, params)
	}
	if cfg.allowRandom {
		args = extractDeckRandomStrategies(args, params)
	}
	if cfg.allowMultiLive && params.LiveType != "solo" && params.LiveType != "auto" {
		args, err = extractDeckMultiliveOptions(args, params)
		if err != nil {
			return "", err
		}
	}
	if cfg.allowCardConfig {
		args = extractDeckCardConfigs(args, params, cfg.defaultNoChange)
	}
	if cfg.allowTarget {
		args = extractDeckTarget(args, params)
	}
	return strings.TrimSpace(args), nil
}

func extractDeckAlgorithm(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "dfs", "sa", "ga", "all":
			if params.Algorithm == "" {
				params.Algorithm = field
				continue
			}
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}

func extractDeckLiveType(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if params.LiveType == "" {
			switch field {
			case "多人", "协力", "multi":
				params.LiveType = "multi"
				continue
			case "单人", "solo":
				params.LiveType = "solo"
				continue
			case "自动", "auto":
				params.LiveType = "auto"
				continue
			}
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}

func extractDeckRandomStrategies(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for idx := 0; idx < len(fields); idx++ {
		field := fields[idx]
		if strategy, consumed := resolveDeckStrategyField(field, idx, fields, deckSkillOrderKeywords); consumed > 0 {
			params.SkillOrderChooseStrategy = strategy
			idx += consumed - 1
			continue
		}
		if strategy, consumed := resolveDeckStrategyField(field, idx, fields, deckSkillReferenceKeywords); consumed > 0 {
			params.SkillReferenceChooseStrategy = strategy
			idx += consumed - 1
			continue
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}

func resolveDeckStrategyField(field string, index int, fields []string, keywords []string) (string, int) {
	for _, keyword := range keywords {
		if strings.Contains(field, keyword) {
			if strategy := resolveDeckStrategyValue(field); strategy != "" {
				return strategy, 1
			}
			if field == keyword && index+1 < len(fields) {
				if strategy := resolveDeckStrategyValue(fields[index+1]); strategy != "" {
					return strategy, 2
				}
			}
		}
	}
	return "", 0
}

func resolveDeckStrategyValue(raw string) string {
	switch {
	case containsDeckKeyword(raw, deckMaxKeywords):
		return "max"
	case containsDeckKeyword(raw, deckMinKeywords):
		return "min"
	case containsDeckKeyword(raw, deckAverageKeywords):
		return "average"
	default:
		return ""
	}
}

func extractDeckMultiliveOptions(args string, params *deckAutoQueryParams) (string, error) {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if value, ok, err := extractDeckKeywordNumber(field, deckTeammatePowerKeywords, parseMusicBoardLargeNumber); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的队友综合力")
			}
			params.MultiLiveTeammatePower = intPtr(value)
			continue
		}
		if value, ok, err := extractDeckKeywordNumber(field, deckTeammateScoreUpKeywords, parseDeckInt); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的队友实效")
			}
			params.MultiLiveTeammateScoreUp = intPtr(value)
			continue
		}
		if value, ok, err := extractDeckKeywordNumber(field, deckSkillTargetKeywords, parseDeckInt); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的队友实效")
			}
			params.MultiLiveTeammateScoreUp = intPtr(value)
			f := float64(value)
			params.MultiLiveScoreUpLowerBound = &f
			continue
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " ")), nil
}

func extractDeckFixedTargets(args string, params *deckAutoQueryParams) (string, error) {
	args = strings.ReplaceAll(args, "＃", "#")
	if !strings.Contains(args, "#") {
		return strings.TrimSpace(args), nil
	}

	prefix, suffix, _ := strings.Cut(args, "#")
	fields := strings.Fields(strings.TrimSpace(suffix))
	if len(fields) == 0 {
		return "", fmt.Errorf("固定卡牌或固定角色不能为空")
	}

	fixedCards := make([]int, 0, len(fields))
	allInts := true
	for _, field := range fields {
		value, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || value <= 0 {
			allInts = false
			break
		}
		fixedCards = append(fixedCards, value)
	}
	if allInts {
		if err := validateDeckUniqueIDs(fixedCards, 5, "固定卡牌"); err != nil {
			return "", err
		}
		params.FixedCards = fixedCards
		return strings.TrimSpace(prefix), nil
	}

	fixedCharacters := make([]int, 0, len(fields))
	fixedCharacterQueries := make([]string, 0, len(fields))
	for _, field := range fields {
		charID, charQuery := resolveDeckCharacterToken(field)
		if charID <= 0 {
			if charQuery == "" {
				return "", fmt.Errorf("格式错误，#后面请填写卡牌ID或角色")
			}
			fixedCharacterQueries = append(fixedCharacterQueries, charQuery)
			continue
		}
		fixedCharacters = append(fixedCharacters, charID)
	}
	if len(fixedCharacters)+len(fixedCharacterQueries) > 5 {
		return "", fmt.Errorf("固定角色数量不能超过5个")
	}
	if len(fixedCharacterQueries) == 0 {
		if err := validateDeckUniqueIDs(fixedCharacters, 5, "固定角色"); err != nil {
			return "", err
		}
	}
	if len(fixedCharacters) == 0 && len(fixedCharacterQueries) == 0 {
		return "", fmt.Errorf("固定角色不能为空")
	}
	params.FixedCharacters = fixedCharacters
	params.FixedCharacterQueries = fixedCharacterQueries
	return strings.TrimSpace(prefix), nil
}

func validateDeckUniqueIDs(values []int, limit int, label string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s不能为空", label)
	}
	if len(values) > limit {
		return fmt.Errorf("%s数量不能超过%d个", label, limit)
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s不能重复", label)
		}
		seen[value] = struct{}{}
	}
	return nil
}

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
	globalPatch := renderdeck.CardConfigPatch{}
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

func extractDeckTarget(args string, params *deckAutoQueryParams) string {
	switch {
	case containsDeckKeyword(args, deckPowerTargetKeywords):
		params.Target = "power"
		args = removeDeckKeywordOnce(args, deckPowerTargetKeywords)
	case containsDeckKeyword(args, deckSkillTargetKeywords):
		params.Target = "skill"
		args = removeDeckKeywordOnce(args, deckSkillTargetKeywords)
	}
	return normalizeDeckSpaces(args)
}

func extractDeckMusicQuery(args string, params *deckAutoQueryParams) (string, error) {
	args = normalizeDeckSpaces(args)
	if args == "" {
		return "", nil
	}

	if diff, cleaned := rendermusic.ExtractMusicDifficulty(args); diff != "" {
		params.MusicDiff = diff
		args = cleaned
	}
	args = normalizeDeckSpaces(args)
	if args == "" {
		return "", nil
	}
	if musicID, ok := rendermusic.ParseExplicitMusicID(args); ok {
		params.MusicID = intPtr(musicID)
		params.MusicQuery = ""
		return "", nil
	}
	params.MusicQuery = args
	return "", nil
}

func extractDeckEventSelection(args string, params *deckAutoQueryParams, trigger string) (string, error) {
	if turn, _, charQuery, remaining := extractDeckSimulatedWorldBloom(args); turn > 0 && charQuery != "" {
		params.WorldBloomEventTurn = intPtr(turn)
		params.WorldBloomCharacterQuery = charQuery
		return remaining, nil
	}

	if eventID, remaining := extractDeckEventID(args); eventID != nil {
		params.EventID = eventID
		if _, charQuery, next := extractDeckCharacterCandidate(remaining, true); charQuery != "" {
			params.WorldBloomCharacterQuery = charQuery
			return next, nil
		}
		return remaining, nil
	}

	attr, unit, remaining := extractDeckSimulatedEvent(args)
	switch {
	case attr != "" && unit != "":
		params.EventAttr = attr
		params.EventUnit = unit
		return remaining, nil
	case attr != "" || unit != "":
		return "", onebot11.NewReplayError("使用方式:\n%s event123\n%s 团名 属性\n%s 角色名 wl1", trigger, trigger, trigger)
	default:
		return normalizeDeckSpaces(args), nil
	}
}

func extractDeckSimulatedWorldBloom(args string) (turn int, charID int, charQuery string, remaining string) {
	matches := deckWlTurnRegex.FindStringSubmatch(args)
	if len(matches) < 2 {
		return 0, 0, "", strings.TrimSpace(args)
	}
	turnValue, err := strconv.Atoi(matches[1])
	if err != nil || turnValue <= 0 {
		return 0, 0, "", strings.TrimSpace(args)
	}

	args = deckWlTurnRegex.ReplaceAllString(args, " ")
	charID, charQuery, args = extractDeckCharacterCandidate(args, true)
	if charID <= 0 && charQuery == "" {
		return 0, 0, "", strings.TrimSpace(args)
	}
	return turnValue, charID, charQuery, normalizeDeckSpaces(args)
}

func extractDeckEventID(args string) (*int, string) {
	if strings.Contains(args, "终章") {
		eventID := 180
		return &eventID, normalizeDeckSpaces(strings.Replace(args, "终章", "", 1))
	}
	matches := deckEventIDRegex.FindStringSubmatch(args)
	if len(matches) < 3 {
		return nil, normalizeDeckSpaces(args)
	}
	eventID, err := strconv.Atoi(matches[2])
	if err != nil || eventID <= 0 {
		return nil, normalizeDeckSpaces(args)
	}
	return &eventID, normalizeDeckSpaces(deckEventIDRegex.ReplaceAllString(args, " "))
}

func extractDeckSimulatedEvent(args string) (attr string, unit string, remaining string) {
	attr, args = extractDeckAttribute(args)
	unit, args = extractDeckUnit(args)
	return attr, unit, normalizeDeckSpaces(args)
}

func extractDeckAttribute(args string) (string, string) {
	ext := parser.NewExtractor(nil)
	result := ext.ExtractAttribute(args)
	if !result.Found {
		return "", strings.TrimSpace(args)
	}
	return result.Value, normalizeDeckSpaces(result.Remaining)
}

func extractDeckUnit(args string) (string, string) {
	for _, rule := range deckUnitAliasRules {
		if !rule.re.MatchString(args) {
			continue
		}
		remaining := rule.re.ReplaceAllString(args, " ")
		return rule.unit, normalizeDeckSpaces(remaining)
	}
	return "", strings.TrimSpace(args)
}

func extractDeckCharacter(args string) (int, string) {
	return 0, normalizeDeckSpaces(args)
}

func extractDeckCharacterCandidate(args string, allowSingleFieldFallback bool) (int, string, string) {
	charID, remaining := extractDeckCharacter(args)
	if charID > 0 {
		return charID, "", remaining
	}
	args = normalizeDeckSpaces(args)
	if !allowSingleFieldFallback || args == "" {
		return 0, "", args
	}
	if len(strings.Fields(args)) != 1 {
		return 0, "", args
	}
	return 0, args, ""
}

func resolveDeckCharacterToken(token string) (int, string) {
	raw := strings.TrimSpace(token)
	if raw == "" {
		return 0, ""
	}
	return 0, raw
}

func extractDeckKeywordNumber(field string, keywords []string, parserFn func(string) (int, error)) (int, bool, error) {
	for _, keyword := range keywords {
		if !strings.Contains(field, keyword) {
			continue
		}
		raw := strings.TrimSpace(strings.Replace(field, keyword, "", 1))
		if raw == "" {
			return 0, false, nil
		}
		value, err := parserFn(strings.TrimSuffix(raw, "%"))
		return value, true, err
	}
	return 0, false, nil
}

func parseDeckInt(raw string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(raw))
}

func containsDeckKeyword(args string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(args, keyword) {
			return true
		}
	}
	return false
}

func removeDeckKeywordOnce(args string, keywords []string) string {
	for _, keyword := range keywords {
		if strings.Contains(args, keyword) {
			return normalizeDeckSpaces(strings.Replace(args, keyword, "", 1))
		}
	}
	return normalizeDeckSpaces(args)
}

func normalizeDeckSpaces(args string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(strings.TrimSpace(args)), " "))
}

func deckLeadingDigits(raw string) int {
	var digits strings.Builder
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			break
		}
		digits.WriteRune(ch)
	}
	if digits.Len() == 0 {
		return 0
	}
	value, err := strconv.Atoi(digits.String())
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func deckCharacterUnit(charID int) string {
	switch {
	case charID >= 1 && charID <= 4:
		return "light_sound"
	case charID >= 5 && charID <= 8:
		return "idol"
	case charID >= 9 && charID <= 12:
		return "street"
	case charID >= 13 && charID <= 16:
		return "theme_park"
	case charID >= 17 && charID <= 20:
		return "school_refusal"
	case charID >= 21 && charID <= 26:
		return "piapro"
	default:
		return ""
	}
}

func newDeckUnitAliasRules() []deckAliasRule {
	aliases := make(map[string]string, len(educationAreaUnitAliases))
	for alias, unit := range educationAreaUnitAliases {
		aliases[alias] = unit
	}

	keys := make([]string, 0, len(aliases))
	for key := range aliases {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	rules := make([]deckAliasRule, 0, len(keys))
	for _, key := range keys {
		pattern := "(?i)"
		if isDeckASCIIAlias(key) {
			pattern += `\b` + regexp.QuoteMeta(key) + `\b`
		} else {
			pattern += regexp.QuoteMeta(key)
		}
		rules = append(rules, deckAliasRule{
			re:   regexp.MustCompile(pattern),
			unit: aliases[key],
		})
	}
	return rules
}

func isDeckASCIIAlias(raw string) bool {
	for _, ch := range raw {
		if ch > 127 {
			return false
		}
	}
	return true
}

func intPtr(value int) *int {
	return &value
}
