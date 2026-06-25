package card

import (
	"path/filepath"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

type cardDistributionBucket struct {
	count int
	owned int
}

var cardBoxAttributeOrder = []string{"cute", "cool", "pure", "happy", "mysterious"}

var cardBoxAttributeLabels = map[string]string{
	"cute":       "可爱",
	"cool":       "帅气",
	"pure":       "纯真",
	"happy":      "快乐",
	"mysterious": "神秘",
	"unknown":    "未分类",
}

var cardBoxAttributeColors = map[string]string{
	"cute":       "#FF66AA",
	"cool":       "#3D8BFF",
	"pure":       "#49C878",
	"happy":      "#FFB02E",
	"mysterious": "#9B72FF",
	"unknown":    "#9AA0A6",
}

func normalizeCardBoxGroupBy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CardBoxGroupByAttr, "attrs", "attribute", "attributes":
		return CardBoxGroupByAttr
	default:
		return ""
	}
}

func (b *Builder) buildCardBoxDistribution(items []drawing.UserCard, iconPaths map[int]string, colorCodes map[int]string, ownedData bool, _ renderregion.Value) *drawing.CardBoxDistribution {
	distribution := &drawing.CardBoxDistribution{
		OwnedData: ownedData,
	}
	if len(items) == 0 {
		return distribution
	}

	characterBuckets := make(map[int]*cardDistributionBucket)
	attributeBuckets := make(map[string]*cardDistributionBucket)
	attributeCharacterBuckets := make(map[string]map[int]*cardDistributionBucket)
	for _, item := range items {
		distribution.TotalCount++
		if item.HasCard {
			distribution.OwnedCount++
		}

		characterID := 0
		if item.Card.CharacterID != nil {
			characterID = *item.Card.CharacterID
		}
		if characterID > 0 {
			addCardDistributionCount(characterBuckets, characterID, item.HasCard)
		}

		attr := normalizeDistributionAttr(stringValue(item.Card.Attr))
		addCardDistributionCount(attributeBuckets, attr, item.HasCard)
		if characterID > 0 {
			if attributeCharacterBuckets[attr] == nil {
				attributeCharacterBuckets[attr] = make(map[int]*cardDistributionBucket)
			}
			addCardDistributionCount(attributeCharacterBuckets[attr], characterID, item.HasCard)
		}
	}

	characterStats := buildCardDistributionCharacterStats(characterBuckets, iconPaths, colorCodes, ownedData)
	distribution.MaxCharacterBarCount = maxCharacterDistributionBarCount(characterStats)
	applyCharacterDistributionRatios(characterStats, distribution.MaxCharacterBarCount, distributionBarDenominator(distribution.TotalCount, distribution.OwnedCount, ownedData))
	distribution.CharacterStats = characterStats

	attributeStats := b.buildCardDistributionAttributeStats(attributeBuckets, attributeCharacterBuckets, iconPaths, colorCodes, ownedData)
	distribution.MaxAttributeBarCount = maxAttributeDistributionBarCount(attributeStats)
	applyAttributeDistributionRatios(attributeStats, distribution.MaxAttributeBarCount, distributionBarDenominator(distribution.TotalCount, distribution.OwnedCount, ownedData))
	distribution.AttributeStats = attributeStats

	return distribution
}

func addCardDistributionCount[T comparable](buckets map[T]*cardDistributionBucket, key T, owned bool) {
	bucket := buckets[key]
	if bucket == nil {
		bucket = &cardDistributionBucket{}
		buckets[key] = bucket
	}
	bucket.count++
	if owned {
		bucket.owned++
	}
}

func buildCardDistributionCharacterStats(buckets map[int]*cardDistributionBucket, iconPaths map[int]string, colorCodes map[int]string, ownedData bool) []drawing.CardDistributionCharacterStat {
	characterIDs := make([]int, 0, len(buckets))
	for characterID := range buckets {
		characterIDs = append(characterIDs, characterID)
	}
	sort.Ints(characterIDs)

	stats := make([]drawing.CardDistributionCharacterStat, 0, len(characterIDs))
	for _, characterID := range characterIDs {
		bucket := buckets[characterID]
		if bucket == nil {
			continue
		}
		barCount := bucket.count
		if ownedData {
			barCount = bucket.owned
		}
		stats = append(stats, drawing.CardDistributionCharacterStat{
			CharacterID: characterID,
			Count:       bucket.count,
			OwnedCount:  bucket.owned,
			BarCount:    barCount,
			ColorCode:   colorCodes[characterID],
			IconPath:    iconPaths[characterID],
		})
	}
	return stats
}

func (b *Builder) buildCardDistributionAttributeStats(
	attributeBuckets map[string]*cardDistributionBucket,
	attributeCharacterBuckets map[string]map[int]*cardDistributionBucket,
	iconPaths map[int]string,
	colorCodes map[int]string,
	ownedData bool,
) []drawing.CardDistributionAttributeStat {
	attrs := append([]string(nil), cardBoxAttributeOrder...)
	for attr := range attributeBuckets {
		if !isKnownCardBoxAttribute(attr) {
			attrs = append(attrs, attr)
		}
	}
	sort.SliceStable(attrs, func(i, j int) bool {
		return cardBoxAttributeSortKey(attrs[i]) < cardBoxAttributeSortKey(attrs[j])
	})

	stats := make([]drawing.CardDistributionAttributeStat, 0, len(attrs))
	for _, attr := range attrs {
		bucket := attributeBuckets[attr]
		if bucket == nil {
			bucket = &cardDistributionBucket{}
		}
		barCount := bucket.count
		if ownedData {
			barCount = bucket.owned
		}
		characterStats := buildCardDistributionCharacterStats(attributeCharacterBuckets[attr], iconPaths, colorCodes, ownedData)
		maxCharacterBarCount := maxCharacterDistributionBarCount(characterStats)
		applyCharacterDistributionRatios(characterStats, maxCharacterBarCount, barCount)
		stats = append(stats, drawing.CardDistributionAttributeStat{
			Attr:           attr,
			Label:          cardBoxAttributeLabel(attr),
			Count:          bucket.count,
			OwnedCount:     bucket.owned,
			BarCount:       barCount,
			ColorCode:      cardBoxAttributeColor(attr),
			AttrIconPath:   b.buildCardDistributionAttrIconPath(attr),
			CharacterStats: characterStats,
		})
	}
	return stats
}

func normalizeDistributionAttr(attr string) string {
	attr = strings.ToLower(strings.TrimSpace(attr))
	if attr == "" {
		return "unknown"
	}
	if isKnownCardBoxAttribute(attr) {
		return attr
	}
	return "unknown"
}

func isKnownCardBoxAttribute(attr string) bool {
	for _, item := range cardBoxAttributeOrder {
		if attr == item {
			return true
		}
	}
	return false
}

func cardBoxAttributeSortKey(attr string) int {
	for i, item := range cardBoxAttributeOrder {
		if attr == item {
			return i
		}
	}
	return len(cardBoxAttributeOrder)
}

func cardBoxAttributeLabel(attr string) string {
	if label := cardBoxAttributeLabels[attr]; label != "" {
		return label
	}
	return attr
}

func cardBoxAttributeColor(attr string) string {
	if colorCode := cardBoxAttributeColors[attr]; colorCode != "" {
		return colorCode
	}
	return cardBoxAttributeColors["unknown"]
}

func (b *Builder) buildCardDistributionAttrIconPath(attr string) string {
	if !isKnownCardBoxAttribute(attr) {
		return ""
	}
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "attr_icon_"+attr+".png"))
}

func distributionBarDenominator(totalCount, ownedCount int, ownedData bool) int {
	if ownedData {
		return ownedCount
	}
	return totalCount
}

func maxCharacterDistributionBarCount(stats []drawing.CardDistributionCharacterStat) int {
	maxCount := 0
	for _, stat := range stats {
		if stat.BarCount > maxCount {
			maxCount = stat.BarCount
		}
	}
	return maxCount
}

func maxAttributeDistributionBarCount(stats []drawing.CardDistributionAttributeStat) int {
	maxCount := 0
	for _, stat := range stats {
		if stat.BarCount > maxCount {
			maxCount = stat.BarCount
		}
	}
	return maxCount
}

func applyCharacterDistributionRatios(stats []drawing.CardDistributionCharacterStat, maxCount, denominator int) {
	for i := range stats {
		if maxCount > 0 {
			stats[i].BarRatio = float64(stats[i].BarCount) / float64(maxCount)
		}
		if denominator > 0 {
			stats[i].Share = float64(stats[i].BarCount) / float64(denominator)
		}
	}
}

func applyAttributeDistributionRatios(stats []drawing.CardDistributionAttributeStat, maxCount, denominator int) {
	for i := range stats {
		if maxCount > 0 {
			stats[i].BarRatio = float64(stats[i].BarCount) / float64(maxCount)
		}
		if denominator > 0 {
			stats[i].Share = float64(stats[i].BarCount) / float64(denominator)
		}
	}
}
