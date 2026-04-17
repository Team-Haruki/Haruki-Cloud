package deck

import (
	"slices"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) prepareMusicCompareSelections(region renderregion.Value, recType string, query AutoQuery, option map[string]any, musicMeta []byte, musicMetaPath string) ([]MusicCompareSelection, int, error) {
	if !query.MusicCompare {
		return nil, 0, nil
	}
	if option != nil {
		option["best_skill_as_leader"] = false
	}

	if len(query.MusicCompareSelections) > 0 {
		return cloneMusicCompareSelections(query.MusicCompareSelections), len(query.MusicCompareSelections), nil
	}
	if len(query.MusicCompareQueries) > 0 {
		return nil, 0, fmt.Errorf("歌曲比较的歌曲列表尚未解析")
	}
	if !hasCompleteFixedDeckOption(option) {
		return nil, 0, fmt.Errorf("%s", strings.TrimSpace(`
如果不限定要比较的歌曲，则必须固定一个卡组！
1. 固定5个卡牌ID:
/指令 ... 歌曲比较 #1 2 3 4 5
2. 固定为你的主队配置(实时更新):
/指令 ... 歌曲比较 当前
3. 限定比较的歌曲（难度默认ma）:
/指令 ... 歌曲比较 龙 虾ex 群青apd
4. 限定比较的歌曲并固定卡组:
/指令 ... 歌曲比较 龙 虾ex #1 2 3 4 5
`))
	}

	payload, err := resolveMusicCompareMetaPayload(musicMeta, musicMetaPath)
	if err != nil {
		return nil, 0, err
	}
	selections, err := c.buildMusicCompareCandidateSelections(region, recType, option, payload)
	if err != nil {
		return nil, 0, err
	}
	return selections, musicCompareDefaultShowNum, nil
}

func resolveMusicCompareMetaPayload(payload []byte, path string) ([]byte, error) {
	if len(payload) > 0 {
		return payload, nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("deck music compare requires music meta data")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func hasCompleteFixedDeckOption(option map[string]any) bool {
	if option == nil {
		return false
	}
	if fixedCards, ok := option["fixed_cards"].([]int); ok && len(fixedCards) == 5 {
		return true
	}
	if useCurrentDeck, _ := option["use_current_deck"].(bool); useCurrentDeck {
		return true
	}
	return false
}

func cloneMusicCompareSelections(items []MusicCompareSelection) []MusicCompareSelection {
	if len(items) == 0 {
		return nil
	}
	out := make([]MusicCompareSelection, len(items))
	copy(out, items)
	return out
}

func (c *Controller) buildMusicCompareCandidateSelections(region renderregion.Value, recType string, option map[string]any, payload []byte) ([]MusicCompareSelection, error) {
	var items []map[string]any
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, err
	}

	liveType := strings.ToLower(strings.TrimSpace(optionString(option, "live_type")))
	target := strings.ToLower(strings.TrimSpace(optionString(option, "target")))
	useEventRate := recType == "event" && target == "score"

	type candidate struct {
		value     float64
		musicID   int
		musicDiff string
	}
	values := make([]candidate, 0, len(items))
	for _, item := range items {
		musicID := compareMetaInt(item["music_id"])
		if musicID <= 0 {
			continue
		}
		diff := normalizeMusicCompareDifficulty(compareMetaString(item["difficulty"]))
		value := compareMusicCandidateValue(item, liveType, useEventRate)
		if value <= 0 {
			continue
		}
		values = append(values, candidate{
			value:     value,
			musicID:   musicID,
			musicDiff: diff,
		})
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("没有找到可比较的歌曲数据")
	}

	sort.SliceStable(values, func(i, j int) bool {
		if values[i].value != values[j].value {
			return values[i].value > values[j].value
		}
		if values[i].musicID != values[j].musicID {
			return values[i].musicID < values[j].musicID
		}
		return values[i].musicDiff < values[j].musicDiff
	})
	if len(values) > musicCompareCandidateNum {
		values = values[:musicCompareCandidateNum]
	}

	result := make([]MusicCompareSelection, 0, len(values))
	for _, item := range values {
		title, coverPath := c.resolveCompareMusicMetadata(region, item.musicID)
		result = append(result, MusicCompareSelection{
			MusicID:        item.musicID,
			MusicDiff:      item.musicDiff,
			MusicTitle:     title,
			MusicCoverPath: coverPath,
		})
	}
	return result, nil
}

func compareMusicCandidateValue(item map[string]any, liveType string, useEventRate bool) float64 {
	isMulti := liveType == "multi" || liveType == "cheerful"
	isAuto := liveType == "auto" || liveType == "challenge_auto"

	value := compareMetaFloat(item["base_score"])
	if isAuto {
		value = compareMetaFloat(item["base_score_auto"])
	}

	scoreKey := "skill_score_solo"
	switch {
	case isAuto:
		scoreKey = "skill_score_auto"
	case isMulti:
		scoreKey = "skill_score_multi"
	}
	for _, score := range compareMetaFloatSlice(item[scoreKey]) {
		if isMulti {
			value += score * 1.8
			continue
		}
		value += score
	}
	if isMulti {
		value += compareMetaFloat(item["fever_score"]) * 0.5
	}
	if useEventRate {
		value *= compareMetaFloat(item["event_rate"]) / 100.0
	}
	return value
}

func (c *Controller) recommendMusicCompare(recommender DeckRecommender, req RecommendRequest, option map[string]any, selections []MusicCompareSelection, showNum int, recType string) (*RecommendResult, []MusicCompareSelection, error) {
	if recommender == nil {
		return nil, nil, fmt.Errorf("deck recommender is not configured")
	}
	if len(selections) == 0 {
		return nil, nil, fmt.Errorf("歌曲比较未提供可用的歌曲")
	}

	items := make([]musicCompareResultItem, 0, len(selections))
	costSamples := make(map[string][]float64)
	waitSamples := make(map[string][]float64)
	challengeCharID := optionInt(option, "challenge_live_character_id")

	for _, selection := range selections {
		compareOption := cloneRecommendOption(option)
		compareOption["limit"] = 1
		compareOption["music_id"] = selection.MusicID
		compareOption["music_diff"] = normalizeMusicCompareDifficulty(selection.MusicDiff)

		result, err := recommender.Recommend(RecommendRequest{
			Region:            req.Region,
			UserData:          req.UserData,
			UserDataFilePath:  req.UserDataFilePath,
			MusicMeta:         req.MusicMeta,
			MusicMetaFilePath: req.MusicMetaFilePath,
			BatchOption:       recommender.ExpandAlgorithms(compareOption),
		})
		if err != nil {
			return nil, nil, err
		}
		if recType == "challenge" {
			applyChallengeScoreDelta(result, challengeCharID, c.snapshot.RawData())
		}
		if result == nil || len(result.Decks) == 0 {
			continue
		}

		for alg, cost := range result.CostTimes {
			costSamples[alg] = append(costSamples[alg], cost)
		}
		for alg, wait := range result.WaitTimes {
			waitSamples[alg] = append(waitSamples[alg], wait)
		}

		alg := ""
		if len(result.DeckAlgs) > 0 {
			alg = result.DeckAlgs[0]
		}
		items = append(items, musicCompareResultItem{
			selection: selection,
			deck:      result.Decks[0],
			alg:       alg,
		})
	}
	if len(items) == 0 {
		return nil, nil, fmt.Errorf("deck recommend service returned no deck results")
	}

	sort.SliceStable(items, func(i, j int) bool {
		return compareDeckSortScore(recType, items[i].deck) > compareDeckSortScore(recType, items[j].deck)
	})

	if showNum <= 0 || showNum > len(items) {
		showNum = len(items)
	}
	items = items[:showNum]

	agg := &RecommendResult{
		Decks:     make([]RecommendDeck, 0, len(items)),
		DeckAlgs:  make([]string, 0, len(items)),
		CostTimes: make(map[string]float64),
		WaitTimes: make(map[string]float64),
	}
	orderedSelections := make([]MusicCompareSelection, 0, len(items))
	for _, item := range items {
		agg.Decks = append(agg.Decks, item.deck)
		agg.DeckAlgs = append(agg.DeckAlgs, item.alg)
		orderedSelections = append(orderedSelections, item.selection)
	}
	for alg, values := range costSamples {
		agg.CostTimes[alg] = averageChallengeSamples(values)
	}
	for alg, values := range waitSamples {
		agg.WaitTimes[alg] = averageChallengeSamples(values)
	}
	return agg, orderedSelections, nil
}

func compareDeckSortScore(recType string, deck RecommendDeck) int {
	switch recType {
	case "mysekai":
		return deck.MysekaiEventPoint
	case "no_event":
		if deck.LiveScore > 0 {
			return deck.LiveScore
		}
	}
	return deck.Score
}

func normalizeMusicCompareDifficulty(diff string) string {
	diff = strings.ToLower(strings.TrimSpace(diff))
	if diff == "" {
		return "master"
	}
	return diff
}

func compareMetaString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func compareMetaFloat(value any) float64 {
	switch item := value.(type) {
	case float64:
		return item
	case float32:
		return float64(item)
	case int:
		return float64(item)
	case int64:
		return float64(item)
	default:
		return 0
	}
}

func compareMetaInt(value any) int {
	return int(compareMetaFloat(value))
}

func compareMetaFloatSlice(value any) []float64 {
	switch items := value.(type) {
	case []float64:
		return slices.Clone(items)
	case []any:
		result := make([]float64, 0, len(items))
		for _, item := range items {
			result = append(result, compareMetaFloat(item))
		}
		return result
	default:
		return nil
	}
}
