package requestbuilder

import (
	"encoding/csv"
	json "github.com/bytedance/sonic"
	"fmt"
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
	if len(candidatePairs) == 0 {
		if params.TargetPoint > 100 {
			return nil, fmt.Errorf("该PT无法用自定义房间控分，控大于100的PT可使用\"/控分\"指令")
		}
		return nil, fmt.Errorf("该PT无法用自定义房间控分，可能是PT过小")
	}

	sort.Slice(candidatePairs, func(i, j int) bool {
		if candidatePairs[i][1] != candidatePairs[j][1] {
			return candidatePairs[i][1] < candidatePairs[j][1]
		}
		return candidatePairs[i][0] > candidatePairs[j][0]
	})

	musicListMap, err := app.Music.ResolveCustomRoomMusicList(r.Region, customRoomEventRates(candidatePairs), customRoomMusicNumPerRate)
	if err != nil {
		return nil, err
	}

	filteredPairs := make([][]int, 0, len(candidatePairs))
	for _, pair := range candidatePairs {
		if len(musicListMap[pair[0]]) == 0 {
			continue
		}
		filteredPairs = append(filteredPairs, []int{pair[0], pair[1]})
		if len(filteredPairs) >= customRoomMaxShownPairs {
			break
		}
	}
	if len(filteredPairs) == 0 {
		return nil, fmt.Errorf("找不到可用于自定义房间控分的歌曲")
	}

	filteredMusicMap := make(map[int][]map[string]any, len(musicListMap))
	for _, pair := range filteredPairs {
		rate := pair[0]
		if _, ok := filteredMusicMap[rate]; ok {
			continue
		}
		filteredMusicMap[rate] = musicListMap[rate]
	}

	return &drawing.CustomRoomScoreRequest{
		TargetPoint:    params.TargetPoint,
		CandidatePairs: filteredPairs,
		MusicListMap:   filteredMusicMap,
	}, nil
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
	raw := strings.TrimPrefix(datafiles.CustomRoomPTCSV(), "\ufeff")
	reader := csv.NewReader(strings.NewReader(raw))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("decode custom-room pt csv: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("custom-room pt csv is empty")
	}

	bonuses := make([]int, 0, len(records[0])-1)
	for _, cell := range records[0][1:] {
		bonus, ok := parseCustomRoomBonus(cell)
		if !ok {
			bonuses = append(bonuses, 0)
			continue
		}
		bonuses = append(bonuses, bonus)
	}

	result := make([][]int, 0, 32)
	for _, row := range records[1:] {
		if len(row) == 0 {
			continue
		}
		eventRate, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(row[0], "\ufeff")))
		if err != nil || eventRate <= 0 {
			continue
		}
		for idx := 1; idx < len(row) && idx-1 < len(bonuses); idx++ {
			pt, err := strconv.Atoi(strings.TrimSpace(row[idx]))
			if err != nil || pt != targetPoint {
				continue
			}
			result = append(result, []int{eventRate, bonuses[idx-1]})
		}
	}

	return result, nil
}

func parseCustomRoomBonus(raw string) (int, bool) {
	clean := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
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
