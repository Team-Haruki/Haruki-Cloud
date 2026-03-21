package mysekai

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/utils/drawing"
)

func intNumber(value interface{}, fallback int) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

func int64Number(value interface{}, fallback int64) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func boolValue(value interface{}) bool {
	v, ok := value.(bool)
	return ok && v
}

func stringValue(value interface{}) string {
	v, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

func nestedList(root map[string]interface{}, key string) []interface{} {
	if root == nil {
		return nil
	}
	if items, ok := root[key].([]interface{}); ok {
		return items
	}
	if updated, ok := root["updatedResources"].(map[string]interface{}); ok {
		if items, ok := updated[key].([]interface{}); ok {
			return items
		}
	}
	return nil
}

func nestedInt(root map[string]interface{}, parent, child string) int {
	if root == nil {
		return 0
	}
	parentMap, ok := root[parent].(map[string]interface{})
	if !ok {
		return 0
	}
	return intNumber(parentMap[child], 0)
}

func parseIntTokens(query string) []int {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return r == ',' || r == ' ' || r == '，' || r == '\t' || r == '\n'
	})
	result := make([]int, 0, len(fields))
	seen := make(map[int]struct{}, len(fields))
	for _, field := range fields {
		id, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func extractMysekaiGate(merged map[string]interface{}) (int, int) {
	visit, ok := merged["userMysekaiGateCharacterVisit"].(map[string]interface{})
	if !ok {
		return 1, 1
	}
	gate, ok := visit["userMysekaiGate"].(map[string]interface{})
	if !ok {
		return 1, 1
	}
	gateID := intNumber(gate["mysekaiGateId"], 1)
	gateLevel := intNumber(gate["mysekaiGateLevel"], 1)
	if gateID <= 0 {
		gateID = 1
	}
	if gateLevel <= 0 {
		gateLevel = 1
	}
	return gateID, gateLevel
}

func extractMysekaiPhenoms(merged map[string]interface{}) []drawing.MysekaiPhenomRequest {
	rawSchedules, ok := merged["mysekaiPhenomenaSchedules"].([]interface{})
	if !ok {
		return []drawing.MysekaiPhenomRequest{}
	}

	nowMs := int64Number(merged["now"], 0)
	if updated, ok := merged["updatedResources"].(map[string]interface{}); ok {
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
		schedule, ok := item.(map[string]interface{})
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
			ImagePath:      fmt.Sprintf("mysekai/thumbnail/phenomena/%s.png", mysekaiPhenomIconName(phenomID)),
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

func resourceTextColor(key string, materialRarityMap map[int]string) []int {
	switch resourceRarity(key, materialRarityMap) {
	case 2:
		return []int{200, 50, 0}
	case 1:
		return []int{50, 0, 200}
	default:
		return []int{100, 100, 100}
	}
}

func resourceRarity(key string, materialRarityMap map[int]string) int {
	mostRare := map[string]struct{}{
		"mysekai_material_5":  {},
		"mysekai_material_12": {},
		"mysekai_material_20": {},
		"mysekai_material_24": {},
		"mysekai_fixture_121": {},
		"material_17":         {},
		"material_170":        {},
	}
	rare := map[string]struct{}{
		"mysekai_material_32": {},
		"mysekai_material_33": {},
		"mysekai_material_34": {},
		"mysekai_material_61": {},
		"mysekai_material_64": {},
		"mysekai_material_65": {},
		"mysekai_material_66": {},
	}
	if _, ok := mostRare[key]; ok {
		return 2
	}
	if _, ok := rare[key]; ok {
		return 1
	}
	if strings.HasPrefix(key, "mysekai_music_record") {
		return 1
	}
	if strings.HasPrefix(key, "mysekai_material_") {
		parts := strings.Split(key, "_")
		id := intNumber(parts[len(parts)-1], 0)
		switch materialRarityMap[id] {
		case "rarity_3":
			return 2
		case "rarity_2":
			return 1
		}
	}
	return 0
}

func musicRecordIconPath(hasRecord bool) *string {
	if !hasRecord {
		return nil
	}
	path := "mysekai/music_record.png"
	return &path
}

func birthdayCharacterID(characters map[int]map[string]interface{}, fixtureName string) int {
	for id, item := range characters {
		givenName := stringValue(item["givenName"])
		if givenName != "" && strings.HasSuffix(fixtureName, "（"+givenName+"）") {
			return id
		}
	}
	return 0
}

func fixtureThumbnailPath(item map[string]interface{}) string {
	assetbundleName := stringValue(item["assetbundleName"])
	if assetbundleName == "" {
		return ""
	}
	if stringValue(item["mysekaiFixtureType"]) == "surface_appearance" {
		layoutType := stringValue(item["mysekaiSettableLayoutType"])
		if layoutType == "" {
			layoutType = "floor_appearance"
		}
		return fmt.Sprintf("mysekai/thumbnail/surface_appearance/%s/tex_%s_%s_1.png", assetbundleName, assetbundleName, layoutType)
	}
	return fmt.Sprintf("mysekai/thumbnail/fixture/%s_1.png", assetbundleName)
}

func fixtureColorImages(item map[string]interface{}) []drawing.MysekaiFixtureColorImage {
	base := fixtureThumbnailPath(item)
	if base == "" {
		return nil
	}

	images := []drawing.MysekaiFixtureColorImage{{ImagePath: base}}
	rawColors, ok := item["mysekaiFixtureAnotherColors"].([]interface{})
	if !ok {
		return images
	}
	assetbundleName := stringValue(item["assetbundleName"])
	if assetbundleName == "" {
		return images
	}

	for index, raw := range rawColors {
		color, _ := raw.(map[string]interface{})
		colorCode := stringValue(color["colorCode"])
		path := fmt.Sprintf("mysekai/thumbnail/fixture/%s_%d.png", assetbundleName, index+2)
		if stringValue(item["mysekaiFixtureType"]) == "surface_appearance" {
			layoutType := stringValue(item["mysekaiSettableLayoutType"])
			if layoutType == "" {
				layoutType = "floor_appearance"
			}
			path = fmt.Sprintf("mysekai/thumbnail/surface_appearance/%s/tex_%s_%s_%d.png", assetbundleName, assetbundleName, layoutType, index+2)
		}
		var codePtr *string
		if colorCode != "" {
			code := colorCode
			codePtr = &code
		}
		images = append(images, drawing.MysekaiFixtureColorImage{
			ImagePath: path,
			ColorCode: codePtr,
		})
	}
	return images
}

func fixtureBasicInfo(item map[string]interface{}) []string {
	boolLabel := func(ok bool, yes, no string) string {
		if ok {
			return yes
		}
		return no
	}
	info := []string{
		boolLabel(boolValue(item["isAssembled"]), "【🔨可制作】", "【❌不可制作】"),
		boolLabel(boolValue(item["isDisassembled"]), "【♻️可回收】", "【❌不可回收】"),
	}
	playerAction := stringValue(item["mysekaiFixturePlayerActionType"]) != "" && stringValue(item["mysekaiFixturePlayerActionType"]) != "no_action"
	info = append(info, boolLabel(playerAction, "【👋玩家可交互】", "【❌玩家不可交互】"))
	info = append(info, boolLabel(boolValue(item["isGameCharacterAction"]), "【🎡角色可交互】", "【❌角色无交互】"))
	return info
}

func fixtureBlueprintInfo(blueprint map[string]interface{}) []string {
	boolLabel := func(ok bool, yes, no string) string {
		if ok {
			return yes
		}
		return no
	}
	limit := intNumber(blueprint["craftCountLimit"], 0)
	info := []string{
		boolLabel(boolValue(blueprint["isEnableSketch"]), "【📝蓝图可抄写】", "【蓝图不可抄写】"),
		boolLabel(boolValue(blueprint["isObtainedByConvert"]), "【🎁蓝图可合成】", "【蓝图不可合成】"),
	}
	if limit > 0 {
		info = append(info, fmt.Sprintf("【最多制作%d次】", limit))
		return info
	}
	return append(info, "【无制作次数限制】")
}

func fixtureTags(item map[string]interface{}, tags map[int]map[string]interface{}) []string {
	group, ok := item["mysekaiFixtureTagGroup"].(map[string]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		id := intNumber(group[fmt.Sprintf("mysekaiFixtureTagId%d", i)], 0)
		if id == 0 {
			continue
		}
		tag := tags[id]
		name := stringValue(tag["name"])
		if name != "" {
			result = append(result, name)
		}
	}
	return result
}

func findFixtureBlueprint(items []map[string]interface{}, fixtureID int) map[string]interface{} {
	for _, item := range items {
		if stringValue(item["mysekaiCraftType"]) == "mysekai_fixture" && intNumber(item["craftTargetId"], 0) == fixtureID {
			return item
		}
	}
	return nil
}

func formatMysekaiQuantity(quantity int) string {
	if quantity >= 10000 {
		return fmt.Sprintf("%dk", quantity/1000)
	}
	if quantity >= 1000 {
		return fmt.Sprintf("%dk%d", quantity/1000, (quantity%1000)/100)
	}
	return strconv.Itoa(quantity)
}

func charaIconName(cuid int) string {
	names := map[int]string{
		1: "ick", 2: "saki", 3: "hnm", 4: "shiho", 5: "mnr", 6: "hrk", 7: "airi", 8: "szk",
		9: "khn", 10: "an", 11: "akt", 12: "toya", 13: "tks", 14: "emu", 15: "nene", 16: "rui",
		17: "knd", 18: "mfy", 19: "ena", 20: "mzk", 21: "miku", 22: "rin", 23: "len", 24: "luka",
		25: "meiko", 26: "kaito", 27: "miku_light_sound", 28: "miku_idol", 29: "miku_street",
		30: "miku_theme_park", 31: "miku_school_refusal", 32: "rin", 33: "rin", 34: "rin", 35: "rin",
		36: "rin", 37: "len", 38: "len", 39: "len", 40: "len", 41: "len", 42: "luka", 43: "luka",
		44: "luka", 45: "luka", 46: "luka", 47: "meiko", 48: "meiko", 49: "meiko", 50: "meiko",
		51: "meiko", 52: "kaito", 53: "kaito", 54: "kaito", 55: "kaito", 56: "kaito",
	}
	if name, ok := names[cuid]; ok {
		return name
	}
	return "miku"
}

func extractGroupCuids(group map[string]interface{}) []int {
	result := make([]int, 0, 9)
	for i := 1; i <= 9; i++ {
		id := intNumber(group[fmt.Sprintf("gameCharacterUnitId%d", i)], 0)
		if id != 0 {
			result = append(result, id)
		}
	}
	return result
}

func containsInt(items []int, target int) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasFixture(obtained map[int]struct{}, fixtureID int) bool {
	if len(obtained) == 0 {
		return true
	}
	_, ok := obtained[fixtureID]
	return ok
}

func percent(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) * 100 / float64(b)
}

func isMusicAvailableNow(windows []map[string]interface{}, nowMs int64) bool {
	for _, item := range windows {
		start := int64Number(item["startAt"], 0)
		end := int64Number(item["endAt"], 0)
		if start <= nowMs && nowMs <= end {
			return true
		}
	}
	return false
}

func sortKeysByResource(counts map[string]int, materialRarityMap map[int]string) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		leftRarity := resourceRarity(keys[i], materialRarityMap)
		rightRarity := resourceRarity(keys[j], materialRarityMap)
		if leftRarity != rightRarity {
			return leftRarity > rightRarity
		}
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
