package deck

import (
	"context"
	"fmt"
	"slices"

	"haruki-cloud/internal/pjsk/render/snapshot"
)

func (c *Controller) prepareChallengeRecommend(query AutoQuery, option map[string]any) error {
	if option == nil {
		return nil
	}

	charID := optionInt(option, "challenge_live_character_id")
	if query.MusicCompare && charID <= 0 {
		return fmt.Errorf("挑战组卡必须指定一个角色才能进行歌曲比较")
	}

	if !query.UseCurrentDeck {
		return nil
	}
	if charID <= 0 {
		return fmt.Errorf("需要指定挑战组卡角色才能使用\"当前\"参数")
	}

	raw := c.snapshot.RawData()
	if raw == nil {
		return fmt.Errorf("raw user snapshot is unavailable")
	}

	deckInfo := snapshot.FindChallengeLiveDeck(raw.UserChallengeLiveSoloDecks, charID)
	if deckInfo == nil {
		return fmt.Errorf("找不到你的该角色的当前挑战卡组（更新当前挑战卡组需要抓包）")
	}

	cards, ok := snapshot.ChallengeLiveDeckCardIDs(deckInfo)
	if !ok {
		return fmt.Errorf("你的该角色的当前挑战卡组不足5张，无法使用\"当前\"参数（更新当前挑战卡组需要抓包）")
	}

	option["fixed_cards"] = slices.Clone(cards)
	delete(option, "fixed_characters")
	option["best_skill_as_leader"] = false
	return nil
}

func shouldRunChallengeAll(option map[string]any) bool {
	return optionInt(option, "challenge_live_character_id") <= 0
}

type rawBatchChallengeRecommender interface {
	RecommendBatch(req RecommendRequest) ([]remoteBatchRecommendResult, error)
}

func (c *Controller) recommendChallengeAll(ctx context.Context, recommender PjskDeckRecommender, req RecommendRequest, option map[string]any) (*RecommendResult, error) {
	if recommender == nil {
		return nil, fmt.Errorf("deck recommender is not configured")
	}

	if batchRecommender, ok := recommender.(rawBatchChallengeRecommender); ok {
		return c.recommendChallengeAllBatch(ctx, recommender, batchRecommender, req, option)
	}
	return c.recommendChallengeAllSequential(ctx, recommender, req, option)
}

func (c *Controller) recommendChallengeAllBatch(ctx context.Context, recommender PjskDeckRecommender, batchRecommender rawBatchChallengeRecommender, req RecommendRequest, option map[string]any) (*RecommendResult, error) {
	agg := &RecommendResult{
		CostTimes: make(map[string]float64),
		WaitTimes: make(map[string]float64),
	}
	costSamples := make(map[string][]float64)
	waitSamples := make(map[string][]float64)
	raw := c.snapshot.RawData()

	type characterBatch struct {
		charID  int
		options []map[string]any
	}

	characters := make([]characterBatch, 0, challengeCharacterCount)
	batchOptions := make([]map[string]any, 0, challengeCharacterCount)
	for charID := 1; charID <= challengeCharacterCount; charID++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		challengeOption := cloneRecommendOption(option)
		challengeOption["challenge_live_character_id"] = charID
		challengeOption["limit"] = 1

		expanded := recommender.ExpandAlgorithms(challengeOption)
		if len(expanded) == 0 {
			return nil, fmt.Errorf("deck recommend service returned no algorithms for challenge character %d", charID)
		}

		characters = append(characters, characterBatch{
			charID:  charID,
			options: expanded,
		})
		batchOptions = append(batchOptions, expanded...)
	}

	results, err := recommendBatchWithContext(ctx, batchRecommender, RecommendRequest{
		Region:            req.Region,
		UserData:          req.UserData,
		UserDataFilePath:  req.UserDataFilePath,
		MusicMeta:         req.MusicMeta,
		MusicMetaFilePath: req.MusicMetaFilePath,
		BatchOption:       batchOptions,
	})
	if err != nil {
		return nil, err
	}
	if len(results) != len(batchOptions) {
		return nil, fmt.Errorf("deck recommend service returned %d batch results, expected %d", len(results), len(batchOptions))
	}

	offset := 0
	for _, character := range characters {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nextOffset := offset + len(character.options)
		result, err := aggregateRemoteRecommendResults("challenge", character.options, results[offset:nextOffset])
		if err != nil {
			return nil, err
		}
		offset = nextOffset

		applyChallengeScoreDelta(result, character.charID, raw)
		if len(result.Decks) > 0 {
			agg.Decks = append(agg.Decks, result.Decks[0])
			if len(result.DeckAlgs) > 0 {
				agg.DeckAlgs = append(agg.DeckAlgs, result.DeckAlgs[0])
			} else {
				agg.DeckAlgs = append(agg.DeckAlgs, "")
			}
		}

		for alg, cost := range result.CostTimes {
			costSamples[alg] = append(costSamples[alg], cost)
		}
		for alg, wait := range result.WaitTimes {
			waitSamples[alg] = append(waitSamples[alg], wait)
		}
	}

	if len(agg.Decks) == 0 {
		return nil, fmt.Errorf("deck recommend service returned no deck results")
	}

	for alg, values := range costSamples {
		agg.CostTimes[alg] = averageChallengeSamples(values)
	}
	for alg, values := range waitSamples {
		agg.WaitTimes[alg] = averageChallengeSamples(values)
	}
	return agg, nil
}

func (c *Controller) recommendChallengeAllSequential(ctx context.Context, recommender PjskDeckRecommender, req RecommendRequest, option map[string]any) (*RecommendResult, error) {
	agg := &RecommendResult{
		CostTimes: make(map[string]float64),
		WaitTimes: make(map[string]float64),
	}
	costSamples := make(map[string][]float64)
	waitSamples := make(map[string][]float64)
	raw := c.snapshot.RawData()

	for charID := 1; charID <= challengeCharacterCount; charID++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		challengeOption := cloneRecommendOption(option)
		challengeOption["challenge_live_character_id"] = charID
		challengeOption["limit"] = 1

		result, err := recommendWithContext(ctx, recommender, RecommendRequest{
			Region:            req.Region,
			UserData:          req.UserData,
			UserDataFilePath:  req.UserDataFilePath,
			MusicMeta:         req.MusicMeta,
			MusicMetaFilePath: req.MusicMetaFilePath,
			BatchOption:       recommender.ExpandAlgorithms(challengeOption),
		})
		if err != nil {
			return nil, err
		}

		applyChallengeScoreDelta(result, charID, raw)
		if len(result.Decks) > 0 {
			agg.Decks = append(agg.Decks, result.Decks[0])
			if len(result.DeckAlgs) > 0 {
				agg.DeckAlgs = append(agg.DeckAlgs, result.DeckAlgs[0])
			} else {
				agg.DeckAlgs = append(agg.DeckAlgs, "")
			}
		}

		for alg, cost := range result.CostTimes {
			costSamples[alg] = append(costSamples[alg], cost)
		}
		for alg, wait := range result.WaitTimes {
			waitSamples[alg] = append(waitSamples[alg], wait)
		}
	}

	if len(agg.Decks) == 0 {
		return nil, fmt.Errorf("deck recommend service returned no deck results")
	}

	for alg, values := range costSamples {
		agg.CostTimes[alg] = averageChallengeSamples(values)
	}
	for alg, values := range waitSamples {
		agg.WaitTimes[alg] = averageChallengeSamples(values)
	}
	return agg, nil
}

func applyChallengeScoreDelta(result *RecommendResult, charID int, raw *snapshot.RawUserData) {
	if result == nil || charID <= 0 {
		return
	}
	highScore := challengeHighScore(raw, charID)
	for index := range result.Decks {
		result.Decks[index].ChallengeScoreDelta = result.Decks[index].Score - highScore
	}
}

func challengeHighScore(raw *snapshot.RawUserData, charID int) int {
	if raw == nil || charID <= 0 {
		return 0
	}
	for _, item := range raw.UserChallengeLiveSoloResults {
		if item.CharacterID == charID {
			return item.HighScore
		}
	}
	return 0
}

func averageChallengeSamples(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}
