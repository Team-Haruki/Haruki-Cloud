package mysekai

import (
	"fmt"
	"time"

	"haruki-cloud/utils/drawing"
)

// pathResolver resolves a relative asset path to its full Drawing-API-relative path.
type pathResolver func(relPath string) string

func extractMysekaiGateInfo(merged map[string]any) (int, int, int) {
	visit, ok := merged["userMysekaiGateCharacterVisit"].(map[string]any)
	if !ok {
		return 1, 1, 0
	}
	gate, ok := visit["userMysekaiGate"].(map[string]any)
	if !ok {
		return 1, 1, 0
	}
	gateID := intNumber(gate["mysekaiGateId"], 1)
	gateLevel := intNumber(gate["mysekaiGateLevel"], 1)
	gateSkinID := intNumber(gate["mysekaiGateSkinId"], 0)
	if gateID <= 0 {
		gateID = 1
	}
	if gateLevel <= 0 {
		gateLevel = 1
	}
	return gateID, gateLevel, gateSkinID
}

func extractMysekaiPhenoms(resolve pathResolver, merged map[string]any) []drawing.MysekaiPhenomRequest {
	rawSchedules, ok := merged["mysekaiPhenomenaSchedules"].([]any)
	if !ok {
		return []drawing.MysekaiPhenomRequest{}
	}

	nowMs := int64Number(merged["now"], 0)
	if updated, ok := merged["updatedResources"].(map[string]any); ok {
		if v := int64Number(updated["now"], 0); v > 0 {
			nowMs = v
		}
	}
	now := time.Now()
	if nowMs > 0 {
		now = time.UnixMilli(nowMs)
	}

	const firstHour = 4
	const secondHour = 16

	phenomStart := time.Date(now.Year(), now.Month(), now.Day(), firstHour, 0, 0, 0, now.Location())
	if now.Hour() < firstHour {
		phenomStart = phenomStart.Add(-24 * time.Hour)
	}
	currentIdx := 1
	if now.Hour() >= firstHour && now.Hour() < secondHour {
		currentIdx = 0
	}

	phenoms := make([]drawing.MysekaiPhenomRequest, 0, len(rawSchedules))
	for i, item := range rawSchedules {
		schedule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		bg := []int{255, 255, 255, 75}
		fg := []int{125, 125, 125, 255}
		if i == currentIdx {
			bg = []int{255, 255, 255, 150}
			fg = []int{0, 0, 0, 255}
		}
		phenomID := intNumber(schedule["mysekaiPhenomenaId"], 1)
		phenoms = append(phenoms, drawing.MysekaiPhenomRequest{
			RefreshReason:  "natural",
			ImagePath:      resolve(fmt.Sprintf("mysekai/thumbnail/phenomena/%s.png", mysekaiPhenomIconName(phenomID))),
			BackgroundFill: bg,
			Text:           phenomStart.Add(time.Duration(i) * 12 * time.Hour).Format("15:04"),
			TextFill:       fg,
		})
	}
	return phenoms
}

func mysekaiPhenomIconName(phenomID int) string {
	icons := map[int]string{
		1:  "env_sunny",
		2:  "env_evening",
		3:  "env_night",
		4:  "env_fine",
		5:  "env_fullmoon",
		6:  "env_rain",
		7:  "env_rainnight",
		8:  "env_cloud",
		9:  "env_thunder",
		10: "env_snow",
		11: "env_snownight",
		12: "env_rainbow",
		13: "env_universe",
		14: "env_meteorshower",
		15: "env_sekai",
	}
	if icon, ok := icons[phenomID]; ok {
		return icon
	}
	return "env_default"
}
