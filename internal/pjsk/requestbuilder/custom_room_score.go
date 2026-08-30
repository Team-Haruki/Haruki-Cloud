package requestbuilder

import (
	"encoding/csv"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
	"sort"
	"strconv"
	"strings"

	datafiles "haruki-cloud/data"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

const (
	customRoomMusicNumPerRate = 3
	customRoomMaxShownPairs   = 150
)

type customRoomScoreSelection struct {
	TargetPoint int `json:"target_point"`
}

func BuildCustomRoomScoreRequest(r *CommandInput, app *renderapp.App) (*drawing.CustomRoomScoreRequest, error) {
	if app == nil || app.Music == nil {
		return nil, fmt.Errorf("score music service unavailable: music controller is not configured")
	}

	params, err := resolveCustomRoomScoreSelection(r)
	if err != nil {
		return nil, err
	}
	if params.TargetPoint <= 0 {
		return nil, fmt.Errorf("invalid custom-room score request")
	}

	candidatePairs, err := findCustomRoomCandidatePairs(params.TargetPoint)
	if err != nil {
		return nil, err
	}
	if err := validateCustomRoomCandidatePairs(params.TargetPoint, candidatePairs); err != nil {
		return nil, err
	}
	sortCustomRoomCandidatePairs(candidatePairs)
	musicListMap, err := app.Music.ResolveCustomRoomMusicList(r.Region, customRoomEventRates(candidatePairs), customRoomMusicNumPerRate)
	if err != nil {
		return nil, err
	}

	filteredPairs := filterCustomRoomCandidatePairs(candidatePairs, musicListMap)
	if len(filteredPairs) == 0 {
		return nil, fmt.Errorf("找不到可用于自定义房间控分的歌曲")
	}
	return &drawing.CustomRoomScoreRequest{
		TargetPoint:    params.TargetPoint,
		CandidatePairs: filteredPairs,
		MusicListMap:   filterCustomRoomMusicMap(filteredPairs, musicListMap),
	}, nil
}

func validateCustomRoomCandidatePairs(targetPoint int, pairs [][]int) error {
	if len(pairs) > 0 {
		return nil
	}
	if targetPoint > 100 {
		return fmt.Errorf("该PT无法用自定义房间控分，控大于100的PT可使用\"/控分\"指令")
	}
	return fmt.Errorf("该PT无法用自定义房间控分，可能是PT过小")
}

func sortCustomRoomCandidatePairs(pairs [][]int) {
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][1] != pairs[j][1] {
			return pairs[i][1] < pairs[j][1]
		}
		return pairs[i][0] > pairs[j][0]
	})
}

func filterCustomRoomCandidatePairs(pairs [][]int, musicMap map[int][]map[string]any) [][]int {
	result := make([][]int, 0, min(len(pairs), customRoomMaxShownPairs))
	for _, pair := range pairs {
		if len(musicMap[pair[0]]) > 0 {
			result = append(result, []int{pair[0], pair[1]})
		}
		if len(result) >= customRoomMaxShownPairs {
			break
		}
	}
	return result
}

func filterCustomRoomMusicMap(pairs [][]int, musicMap map[int][]map[string]any) map[int][]map[string]any {
	result := make(map[int][]map[string]any, len(musicMap))
	for _, pair := range pairs {
		rate := pair[0]
		if _, exists := result[rate]; !exists {
			result[rate] = musicMap[rate]
		}
	}
	return result
}

func resolveCustomRoomScoreSelection(r *CommandInput) (customRoomScoreSelection, error) {
	params := customRoomScoreSelection{}
	if r != nil && r.Params != nil {
		if err := json.Unmarshal(r.Params, &params); err != nil {
			return customRoomScoreSelection{}, fmt.Errorf("bridge: unmarshal custom-room params: %w", err)
		}
	}
	if params.TargetPoint > 0 {
		return params, nil
	}
	if r == nil {
		return customRoomScoreSelection{}, fmt.Errorf("invalid custom-room score request")
	}

	target, err := strconv.Atoi(strings.TrimSpace(r.Query))
	if err != nil || target <= 0 {
		return customRoomScoreSelection{}, fmt.Errorf("invalid custom-room score request")
	}
	params.TargetPoint = target
	return params, nil
}

func findCustomRoomCandidatePairs(targetPoint int) ([][]int, error) {
	raw := strings.TrimPrefix(datafiles.CustomRoomPTCSV(), byteOrderMark)
	reader := csv.NewReader(strings.NewReader(raw))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("decode custom-room pt csv: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("custom-room pt csv is empty")
	}

	bonuses := parseCustomRoomBonuses(records[0])
	result := make([][]int, 0, 32)
	for _, row := range records[1:] {
		result = append(result, customRoomPairsFromRow(row, bonuses, targetPoint)...)
	}
	return result, nil
}

func parseCustomRoomBonuses(header []string) []int {
	if len(header) < 2 {
		return nil
	}
	bonuses := make([]int, 0, len(header)-1)
	for _, cell := range header[1:] {
		bonus, _ := parseCustomRoomBonus(cell)
		bonuses = append(bonuses, bonus)
	}
	return bonuses
}

func customRoomPairsFromRow(row []string, bonuses []int, targetPoint int) [][]int {
	if len(row) == 0 {
		return nil
	}
	eventRate, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(row[0], byteOrderMark)))
	if err != nil || eventRate <= 0 {
		return nil
	}
	result := make([][]int, 0)
	for index := 1; index < len(row) && index-1 < len(bonuses); index++ {
		point, err := strconv.Atoi(strings.TrimSpace(row[index]))
		if err == nil && point == targetPoint {
			result = append(result, []int{eventRate, bonuses[index-1]})
		}
	}
	return result
}

func parseCustomRoomBonus(raw string) (int, bool) {
	clean := strings.TrimSpace(strings.TrimPrefix(raw, byteOrderMark))
	clean = strings.TrimSuffix(clean, "%")
	if clean == "" {
		return 0, false
	}
	value, err := strconv.Atoi(clean)
	if err != nil {
		return 0, false
	}
	return value, true
}

func customRoomEventRates(pairs [][]int) []int {
	seen := make(map[int]struct{}, len(pairs))
	result := make([]int, 0, len(pairs))
	for _, pair := range pairs {
		if len(pair) < 2 || pair[0] <= 0 {
			continue
		}
		if _, ok := seen[pair[0]]; ok {
			continue
		}
		seen[pair[0]] = struct{}{}
		result = append(result, pair[0])
	}
	return result
}
