package gacha

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

const gachaEndPaddingMillis = int64(time.Minute / time.Millisecond)

var (
	gachaRereleasePrefixes = []string{"[it's back]", "[재등장]", "[复刻]", "[復刻]"}
	gachaRecallPrefixes    = []string{"[回响]"}
)

type Builder struct {
	source DataSource
	assets *assets.AssetHelper
}

func NewBuilder(source DataSource, assetHelper *assets.AssetHelper) *Builder {
	return &Builder{
		source: source,
		assets: assetHelper,
	}
}

func (b *Builder) BuildGachaListRequest(query ListQuery) (*drawing.GachaListRequest, error) {
	page := query.Page
	if page < 0 {
		page = 0
	}
	pageSize := query.PageSize
	if pageSize < 0 {
		pageSize = 0
	}

	now := time.Now()
	var filtered []*masterdata.Gacha
	all := b.source.GetGachas()

	cardFilter := query.CardID
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))

	for _, item := range all {
		if query.Year > 0 && time.UnixMilli(item.StartAt).Year() != query.Year {
			continue
		}
		if !query.IncludeFuture && time.UnixMilli(item.StartAt).After(now) {
			continue
		}
		if !query.IncludePast && time.UnixMilli(item.EndAt).Before(now) {
			continue
		}
		if cardFilter > 0 && !gachaContainsCard(item, cardFilter) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(item.Name), keyword) {
			continue
		}
		if query.IsRerelease && !hasAnyPrefixFold(item.Name, gachaRereleasePrefixes) {
			continue
		}
		if query.IsRecall && !hasAnyPrefixFold(item.Name, gachaRecallPrefixes) {
			continue
		}
		if query.OnlyCurrent {
			if time.UnixMilli(item.StartAt).After(now) || time.UnixMilli(item.EndAt).Before(now) {
				continue
			}
		}
		filtered = append(filtered, item)
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no gacha data matched filters")
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].StartAt == filtered[j].StartAt {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].StartAt < filtered[j].StartAt
	})

	region := query.Region
	if region.IsZero() {
		region = b.source.DefaultRegion()
	}

	briefs := make([]drawing.GachaBrief, 0, len(filtered))
	logos := make(map[int]string, len(filtered))
	for _, item := range filtered {
		briefs = append(briefs, drawing.GachaBrief{
			ID:        item.ID,
			Name:      item.Name,
			GachaType: item.GachaType,
			StartAt:   item.StartAt,
			EndAt:     item.EndAt,
			AssetName: item.AssetBundleName,
		})
		logos[item.ID] = b.buildGachaLogoPath(item, region)
	}

	return &drawing.GachaListRequest{
		Gachas:     briefs,
		PageSize:   pageSize,
		Region:     region.String(),
		GachaLogos: logos,
		Filter: drawing.GachaFilter{
			Page: page,
		},
	}, nil
}

func hasAnyPrefixFold(text string, prefixes []string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func (b *Builder) BuildGachaDetailRequest(query DetailQuery) (*drawing.GachaDetailRequest, error) {
	if query.GachaID == 0 {
		return nil, fmt.Errorf("gacha id is required")
	}
	gachaInfo, err := b.source.GetGachaByID(query.GachaID)
	if err != nil {
		return nil, err
	}

	region := query.Region
	if region.IsZero() {
		region = b.source.DefaultRegion()
	}

	rarityCounts := map[string]int{
		"rarity_1":        0,
		"rarity_2":        0,
		"rarity_3":        0,
		"rarity_4":        0,
		"rarity_birthday": 0,
	}
	cardWeight := make(map[int]float64)
	cardRarity := make(map[int]string)
	cardCache := make(map[int]*masterdata.Card)
	rarityWeights := make(map[string]float64)
	pickupSet := make(map[int]struct{})
	pickupOrder := make([]int, 0, len(gachaInfo.GachaPickups))
	for _, pickup := range gachaInfo.GachaPickups {
		if _, exists := pickupSet[pickup.CardID]; exists {
			continue
		}
		pickupSet[pickup.CardID] = struct{}{}
		pickupOrder = append(pickupOrder, pickup.CardID)
	}

	var guaranteedType string
	for _, behavior := range gachaInfo.GachaBehaviors {
		switch strings.ToLower(behavior.GachaBehaviorType) {
		case "over_rarity_4_once":
			guaranteedType = "rarity_4"
		case "over_rarity_3_once":
			if guaranteedType != "rarity_4" {
				guaranteedType = "rarity_3"
			}
		}
	}

	for _, detail := range gachaInfo.GachaDetails {
		cardInfo, err := b.source.GetCardByID(detail.CardID)
		if err != nil {
			continue
		}
		cardCache[cardInfo.ID] = cardInfo
		rarity := strings.ToLower(cardInfo.CardRarityType)
		cardRarity[cardInfo.ID] = rarity
		rarityCounts[rarity]++
		cardWeight[detail.CardID] += float64(detail.Weight)
		rarityWeights[rarity] += float64(detail.Weight)
	}

	rarityRateFraction := map[string]float64{}
	weightInfo := drawing.GachaWeight{
		GuaranteedRates: map[string]float64{},
	}
	for _, rate := range gachaInfo.GachaCardRarityRates {
		rarity := strings.ToLower(rate.CardRarityType)
		if !strings.EqualFold(rate.LotteryType, "normal") {
			continue
		}
		fraction := rate.Rate / 100.0
		switch rarity {
		case "rarity_1":
			weightInfo.Rarity1Rate = float64Ptr(fraction)
		case "rarity_2":
			weightInfo.Rarity2Rate = float64Ptr(fraction)
		case "rarity_3":
			weightInfo.Rarity3Rate = float64Ptr(fraction)
		case "rarity_4":
			weightInfo.Rarity4Rate = float64Ptr(fraction)
		case "rarity_birthday":
			weightInfo.RarityBirthdayRate = float64Ptr(fraction)
		}
		rarityRateFraction[rarity] = fraction
	}

	if guaranteedType != "" {
		guaranteedRates := map[string]float64{
			"rarity_1":        0,
			"rarity_2":        0,
			"rarity_3":        0,
			"rarity_4":        0,
			"rarity_birthday": 0,
		}
		for rarity, fraction := range rarityRateFraction {
			guaranteedRates[rarity] = fraction
		}
		if guaranteedType == "rarity_4" || guaranteedType == "rarity_3" {
			guaranteedRates[guaranteedType] += guaranteedRates["rarity_2"]
			guaranteedRates["rarity_2"] = 0
		}
		if guaranteedType == "rarity_4" {
			guaranteedRates[guaranteedType] += guaranteedRates["rarity_3"]
			guaranteedRates["rarity_3"] = 0
		}
		weightInfo.GuaranteedRates = guaranteedRates
	}

	computeCardRate := func(cardID int) float64 {
		rarity := cardRarity[cardID]
		if rarity == "" {
			return 0
		}
		total := rarityWeights[rarity]
		if total <= 0 {
			return 0
		}
		base := rarityRateFraction[rarity]
		if base == 0 {
			return 0
		}
		return (cardWeight[cardID] / total) * base
	}

	pickupCards := make([]drawing.GachaCardWeight, 0, len(pickupOrder))
	for _, cardID := range pickupOrder {
		cardInfo := cardCache[cardID]
		if cardInfo == nil {
			var err error
			cardInfo, err = b.source.GetCardByID(cardID)
			if err != nil {
				continue
			}
			cardCache[cardID] = cardInfo
			cardRarity[cardID] = strings.ToLower(cardInfo.CardRarityType)
		}
		pickupCards = append(pickupCards, drawing.GachaCardWeight{
			ID:               cardInfo.ID,
			Rarity:           cardInfo.CardRarityType,
			Rate:             computeCardRate(cardInfo.ID),
			ThumbnailRequest: b.buildGachaThumbnail(cardInfo, region),
		})
	}

	var bannerPath *string
	if path := b.buildGachaBannerPath(gachaInfo, region); path != "" {
		bannerPath = &path
	}
	var logoPath *string
	if path := b.buildGachaLogoPath(gachaInfo, region); path != "" {
		logoPath = &path
	}
	var ceilPath *string
	if gachaInfo.GachaCeilItemID != nil && *gachaInfo.GachaCeilItemID != 0 {
		if path := b.buildCeilItemIconPath(*gachaInfo.GachaCeilItemID, region); path != "" {
			ceilPath = &path
		}
	}

	info := drawing.GachaInfo{
		ID:                  gachaInfo.ID,
		Name:                gachaInfo.Name,
		GachaType:           gachaInfo.GachaType,
		Summary:             gachaInfo.GachaInformation.Summary,
		Desc:                gachaInfo.GachaInformation.Description,
		StartAt:             gachaInfo.StartAt,
		EndAt:               gachaInfo.EndAt + gachaEndPaddingMillis,
		AssetName:           gachaInfo.AssetBundleName,
		CeilItemImgPath:     ceilPath,
		Behaviors:           b.convertBehaviors(gachaInfo, region),
		Rarity1Count:        rarityCounts["rarity_1"],
		Rarity2Count:        rarityCounts["rarity_2"],
		Rarity3Count:        rarityCounts["rarity_3"],
		Rarity4Count:        rarityCounts["rarity_4"],
		RarityBirthdayCount: rarityCounts["rarity_birthday"],
		PickupCount:         len(pickupSet),
	}

	return &drawing.GachaDetailRequest{
		Gacha:         info,
		WeightInfo:    weightInfo,
		PickupCards:   pickupCards,
		LogoImgPath:   logoPath,
		BannerImgPath: bannerPath,
		Region:        region.String(),
	}, nil
}

func (b *Builder) buildGachaLogoPath(gachaInfo *masterdata.Gacha, region renderregion.Value) string {
	if gachaInfo == nil {
		return ""
	}
	idText := strconv.Itoa(gachaInfo.ID)
	candidates := []string{}

	if assetName := strings.TrimSpace(gachaInfo.AssetBundleName); assetName != "" {
		candidates = append(candidates,
			filepath.Join("gacha", assetName, "logo", "logo.png"),
			filepath.Join("logo", assetName+".png"),
		)
		if digits := extractNumericToken(assetName); digits != "" {
			candidates = append(candidates, filepath.Join("logo", "banner_logo"+digits+".png"))
		}
	}

	candidates = append(candidates,
		filepath.Join("gacha", "ab_gacha_"+idText, "logo", "logo.png"),
		filepath.Join("logo", fmt.Sprintf("banner_logo%d.png", gachaInfo.Seq)),
		filepath.Join("logo", "banner_logo"+idText+".png"),
	)
	return assets.ResolveRegionAssetPath(b.assets, region.String(), candidates...)
}

func (b *Builder) buildGachaBannerPath(gachaInfo *masterdata.Gacha, region renderregion.Value) string {
	if gachaInfo == nil {
		return ""
	}
	idText := strconv.Itoa(gachaInfo.ID)
	return assets.ResolveRegionAssetPath(b.assets, region.String(),
		filepath.Join("home", "banner", "banner_gacha"+idText, "banner_gacha"+idText+".png"),
		filepath.Join("gacha", "ab_gacha_"+idText, "screen", "texture", "bg_gacha"+idText+".png"),
		filepath.Join("home", "banner", gachaInfo.AssetBundleName, gachaInfo.AssetBundleName+".png"),
		filepath.Join("gacha", gachaInfo.AssetBundleName+".png"),
		filepath.Join("gacha", "banner_gacha"+idText+".png"),
	)
}

func (b *Builder) buildGachaThumbnail(cardInfo *masterdata.Card, region renderregion.Value) drawing.CardFullThumbnailRequest {
	return common.BuildCardThumbnail(b.assets, cardInfo, region, common.ThumbnailOptions{AfterTraining: false})
}

func (b *Builder) buildCeilItemIconPath(_ int, region renderregion.Value) string {
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, "shard.png")
}

func (b *Builder) convertBehaviors(gachaInfo *masterdata.Gacha, region renderregion.Value) []drawing.GachaBehavior {
	behaviors := make([]drawing.GachaBehavior, 0, len(gachaInfo.GachaBehaviors))
	jewelIcon := assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, "jewel.png")
	for _, behavior := range gachaInfo.GachaBehaviors {
		var costType *string
		var costQty *int
		var costIcon *string
		if behavior.CostResourceType != "" {
			value := behavior.CostResourceType
			costType = &value
			if strings.Contains(strings.ToLower(value), "jewel") && jewelIcon != "" {
				icon := jewelIcon
				costIcon = &icon
			}
		}
		if behavior.CostResourceQuantity != 0 {
			value := behavior.CostResourceQuantity
			costQty = &value
		}
		behaviors = append(behaviors, drawing.GachaBehavior{
			Type:         behavior.GachaBehaviorType,
			SpinCount:    behavior.SpinCount,
			CostType:     costType,
			CostIconPath: costIcon,
			CostQuantity: costQty,
			ExecuteLimit: behavior.ExecuteLimit,
			ColorfulPass: strings.EqualFold(behavior.GachaSpinnableType, "colorful_pass"),
		})
	}
	return behaviors
}

func gachaContainsCard(gachaInfo *masterdata.Gacha, cardID int) bool {
	for _, detail := range gachaInfo.GachaDetails {
		if detail.CardID == cardID {
			return true
		}
	}
	for _, pickup := range gachaInfo.GachaPickups {
		if pickup.CardID == cardID {
			return true
		}
	}
	return false
}

func extractNumericToken(assetName string) string {
	var current strings.Builder
	last := ""
	for _, char := range assetName {
		if char >= '0' && char <= '9' {
			current.WriteRune(char)
			continue
		}
		if current.Len() > 0 {
			last = current.String()
			current.Reset()
		}
	}
	if current.Len() > 0 {
		last = current.String()
	}
	return last
}

func float64Ptr(value float64) *float64 {
	return &value
}
