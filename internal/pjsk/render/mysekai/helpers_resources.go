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
		leftPriority := resourceSortPriority(keys[i])
		rightPriority := resourceSortPriority(keys[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
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

func resourceSortPriority(key string) int {
	if strings.HasPrefix(key, "mysekai_music_record_") {
		return 0
	}
	return 1
}
