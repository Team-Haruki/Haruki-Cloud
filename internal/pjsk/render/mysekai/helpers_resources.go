package mysekai

import (
	"sort"
	"strconv"
	"strings"
)

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
		"material_173":        {},
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
	if matchesResourceIDRange(key, "material_", 174, 203) || matchesResourceIDRange(key, "mysekai_material_", 67, 92) {
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

func matchesResourceIDRange(key, prefix string, minID, maxID int) bool {
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	id := intNumber(strings.TrimPrefix(key, prefix), 0)
	return id >= minID && id <= maxID
}

func musicRecordIconPath(resolve pathResolver, hasRecord bool) *string {
	if !hasRecord {
		return nil
	}
	path := resolve("mysekai/music_record.png")
	return &path
}

func formatMysekaiQuantity(quantity int) string {
	if quantity >= 10000 {
		return strconv.Itoa(quantity/1000) + "k"
	}
	if quantity >= 1000 {
		return strconv.Itoa(quantity/1000) + "k" + strconv.Itoa((quantity%1000)/100)
	}
	return strconv.Itoa(quantity)
}

func sortKeysByResource(counts map[string]int, materialRarityMap map[int]string) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		leftScore := adjustedResourceSortScore(keys[i], counts[keys[i]], materialRarityMap)
		rightScore := adjustedResourceSortScore(keys[j], counts[keys[j]], materialRarityMap)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return keys[i] < keys[j]
	})
	return keys
}

func adjustedResourceSortScore(key string, count int, materialRarityMap map[int]string) int {
	switch resourceRarity(key, materialRarityMap) {
	case 2:
		return count + 1000000
	case 1:
		return count + 100000
	default:
		return count
	}
}
