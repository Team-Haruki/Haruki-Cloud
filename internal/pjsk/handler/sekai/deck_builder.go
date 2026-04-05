package sekai

import (
	"fmt"
	"haruki-cloud/api/bot/onebot11"
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
		defaultNoChange:    true,
		defaultArgsTrigger: trigger,
	})
	if err != nil {
		return "", err
	}
	return extractDeckEventSelection(args, params, trigger)
}
