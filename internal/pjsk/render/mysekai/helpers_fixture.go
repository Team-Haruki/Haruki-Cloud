package mysekai

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
)

func birthdayCharacterID(characters map[int]map[string]any, fixtureName string) int {
	for id, item := range characters {
		givenName := stringValue(item["givenName"])
		if givenName != "" && strings.HasSuffix(fixtureName, "（"+givenName+"）") {
			return id
		}
	}
	return 0
}

func fixtureThumbnailPath(resolve pathResolver, item map[string]any) string {
	assetbundleName := stringValueFrom(item, "assetbundleName", "assetbundle_name")
	if assetbundleName == "" {
		return ""
	}
	if stringValueFrom(item, "mysekaiFixtureType", "mysekai_fixture_type") == "surface_appearance" {
		layoutType := stringValueFrom(item, "mysekaiSettableLayoutType", "mysekai_settable_layout_type")
		if layoutType == "" {
			layoutType = "floor_appearance"
		}
		return resolve(fmt.Sprintf("mysekai/thumbnail/surface_appearance/%s/tex_%s_%s_1.png", assetbundleName, assetbundleName, layoutType))
	}
	return resolve(fmt.Sprintf("mysekai/thumbnail/fixture/%s_1.png", assetbundleName))
}

func fixtureColorImages(resolve pathResolver, item map[string]any) []drawing.MysekaiFixtureColorImage {
	base := fixtureThumbnailPath(resolve, item)
	if base == "" {
		return nil
	}

	var baseColorCode *string
	if code := stringValue(item["colorCode"]); code != "" {
		baseColorCode = drawing.StringPtr(code)
	}
	images := []drawing.MysekaiFixtureColorImage{{
		ImagePath: base,
		ColorCode: baseColorCode,
	}}
	rawColors, ok := item["mysekaiFixtureAnotherColors"].([]any)
	if !ok {
		return images
	}
	assetbundleName := stringValueFrom(item, "assetbundleName", "assetbundle_name")
	if assetbundleName == "" {
		return images
	}

	for index, raw := range rawColors {
		color, _ := raw.(map[string]any)
		colorCode := stringValueFrom(color, "colorCode", "color_code")
		path := resolve(fmt.Sprintf("mysekai/thumbnail/fixture/%s_%d.png", assetbundleName, index+2))
		if stringValueFrom(item, "mysekaiFixtureType", "mysekai_fixture_type") == "surface_appearance" {
			layoutType := stringValueFrom(item, "mysekaiSettableLayoutType", "mysekai_settable_layout_type")
			if layoutType == "" {
				layoutType = "floor_appearance"
			}
			path = resolve(fmt.Sprintf("mysekai/thumbnail/surface_appearance/%s/tex_%s_%s_%d.png", assetbundleName, assetbundleName, layoutType, index+2))
		}
		images = append(images, drawing.MysekaiFixtureColorImage{
			ImagePath: path,
			ColorCode: drawing.StringPtr(colorCode),
		})
		if colorCode == "" {
			images[len(images)-1].ColorCode = nil
		}
	}
	return images
}

func fixtureBasicInfo(item map[string]any) []string {
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

func fixtureBlueprintInfo(blueprint map[string]any) []string {
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

func fixtureTags(item map[string]any, tags map[int]map[string]any) []string {
	group, ok := item["mysekaiFixtureTagGroup"].(map[string]any)
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

func findFixtureBlueprint(items []map[string]any, fixtureID int) map[string]any {
	for _, item := range items {
		if stringValue(item["mysekaiCraftType"]) == "mysekai_fixture" && intNumber(item["craftTargetId"], 0) == fixtureID {
			return item
		}
	}
	return nil
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

func extractGroupCuids(group map[string]any) []int {
	result := make([]int, 0, 9)
	for i := 1; i <= 9; i++ {
		id := intNumberFrom(
			group,
			0,
			fmt.Sprintf("gameCharacterUnitId%d", i),
			fmt.Sprintf("game_character_unit_id%d", i),
			fmt.Sprintf("game_character_unit_id_%d", i),
		)
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
	_, ok := obtained[fixtureID]
	return ok
}

func percent(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) * 100 / float64(b)
}

func isMusicAvailableNow(windows []map[string]any, nowMs int64) bool {
	for _, item := range windows {
		start := int64Number(item["startAt"], 0)
		end := int64Number(item["endAt"], 0)
		if start <= nowMs && nowMs <= end {
			return true
		}
	}
	return false
}
