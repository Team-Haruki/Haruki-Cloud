package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/onebot11"
	"strings"
)

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
		args, err = buildNoEventDeckParams(args, &params, ctx.originalTriggerCmd)
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
	if err := validateDeckQueryParams(&params); err != nil {
		return deckAutoQueryParams{}, err
	}
	if args = strings.TrimSpace(args); args != "" {
		params.Args = args
	}
	return params, nil
}

func validateDeckQueryParams(params *deckAutoQueryParams) error {
	if params == nil {
		return nil
	}
	if len(params.MusicCompareQueries) > deckMusicCompareMaxQueries {
		return onebot11.NewReplayError("最多只能指定 %d 首歌曲进行比较", deckMusicCompareMaxQueries)
	}
	if params.SkillOrderChooseStrategy != "specific" && len(params.SpecificSkillOrder) == 0 {
		return nil
	}
	if params.SkillOrderChooseStrategy == "specific" && len(params.SpecificSkillOrder) == 0 {
		return onebot11.NewReplayError("%s", strings.TrimSpace(deckSpecificSkillOrderUsage))
	}
	if deckHasCompleteFixedCards(params) {
		return nil
	}
	return onebot11.NewReplayError("仅在使用固定队伍（例如添加\"当前\"参数）时可指定特定技能顺序")
}

func deckHasCompleteFixedCards(params *deckAutoQueryParams) bool {
	if params == nil {
		return false
	}
	return len(params.FixedCards) == 5 || params.UseCurrentDeck
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
	comparePrefix, compareSuffix, hasCompare := extractDeckMusicCompare(args)
	if hasCompare {
		applyDeckMusicCompareParams(params, comparePrefix, compareSuffix)
		return "", nil
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

	comparePrefix, compareSuffix, hasCompare := extractDeckMusicCompare(args)
	args = comparePrefix
	if hasCompare {
		params.MusicCompare = true
	}

	charID, charQuery, remaining := extractDeckCharacterCandidate(args, true)
	if charID > 0 {
		params.ChallengeLiveCharacterID = intPtr(charID)
		args = remaining
	} else if charQuery != "" {
		if remaining == "" && looksLikeInlineMusicQuery(charQuery) {
			args = normalizeDeckSpaces(args)
		} else {
			params.ChallengeLiveCharacterQuery = charQuery
			args = remaining
		}
	}
	if hasCompare {
		applyDeckMusicCompareParams(params, remaining, compareSuffix)
		return "", nil
	}
	return extractDeckMusicQuery(args, params)
}

func buildNoEventDeckParams(args string, params *deckAutoQueryParams, trigger string) (string, error) {
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
	if err := validateNoEventDeckArgs(args, trigger); err != nil {
		return "", err
	}
	comparePrefix, compareSuffix, hasCompare := extractDeckMusicCompare(args)
	if hasCompare {
		applyDeckMusicCompareParams(params, comparePrefix, compareSuffix)
		return "", nil
	}
	return extractDeckMusicQuery(args, params)
}

func buildBonusDeckParams(args string, params *deckAutoQueryParams, trigger string) (string, error) {
	eventID, remaining := extractDeckExplicitEventID(args)
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
		value, err := parseDeckBonusInt(strings.TrimSpace(field))
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
		defaultArgsTrigger: trigger,
	})
	if err != nil {
		return "", err
	}
	args, err = extractDeckEventSelection(args, params, trigger)
	if err != nil {
		return "", err
	}
	comparePrefix, compareSuffix, hasCompare := extractDeckMusicCompare(args)
	if hasCompare {
		applyDeckMusicCompareParams(params, comparePrefix, compareSuffix)
		return "", nil
	}
	return args, nil
}

func applyDeckMusicCompareParams(params *deckAutoQueryParams, segments ...string) {
	if params == nil {
		return
	}
	params.MusicCompare = true

	combined := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		combined = append(combined, segment)
	}
	if len(combined) == 0 {
		params.MusicCompareQueries = nil
		return
	}
	params.MusicCompareQueries = strings.Fields(strings.Join(combined, " "))
}
