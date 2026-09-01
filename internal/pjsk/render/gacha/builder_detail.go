package gacha

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

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

	pickupOrder := uniqueGachaPickupIDs(gachaInfo)
	cardState := newGachaDetailCardState()
	cardState.load(b.source, gachaInfo)
	weightInfo := buildGachaWeight(gachaInfo, cardState)
	pickupCards := cardState.pickupCards(b, pickupOrder, region)

	info := drawing.GachaInfo{
		ID:                  gachaInfo.ID,
		Name:                gachaInfo.Name,
		GachaType:           gachaInfo.GachaType,
		Summary:             gachaInfo.GachaInformation.Summary,
		Desc:                gachaInfo.GachaInformation.Description,
		StartAt:             gachaInfo.StartAt,
		EndAt:               gachaInfo.EndAt + gachaEndPaddingMillis,
		AssetName:           gachaInfo.AssetBundleName,
		CeilItemImgPath:     b.gachaCeilItemPath(gachaInfo, region),
		Behaviors:           b.convertBehaviors(gachaInfo, region),
		Rarity1Count:        cardState.rarityCounts["rarity_1"],
		Rarity2Count:        cardState.rarityCounts["rarity_2"],
		Rarity3Count:        cardState.rarityCounts["rarity_3"],
		Rarity4Count:        cardState.rarityCounts["rarity_4"],
		RarityBirthdayCount: cardState.rarityCounts["rarity_birthday"],
		PickupCount:         len(pickupOrder),
	}

	return &drawing.GachaDetailRequest{
		Gacha:         info,
		WeightInfo:    weightInfo,
		PickupCards:   pickupCards,
		LogoImgPath:   nonEmptyGachaPath(b.buildGachaLogoPath(gachaInfo, region)),
		BannerImgPath: nonEmptyGachaPath(b.buildGachaBannerPath(gachaInfo, region)),
		Region:        region.String(),
	}, nil
}

type gachaDetailCardState struct {
	rarityCounts  map[string]int
	cardWeight    map[int]float64
	cardRarity    map[int]string
	cardCache     map[int]*masterdata.Card
	rarityWeights map[string]float64
	rarityRates   map[string]float64
}

func newGachaDetailCardState() *gachaDetailCardState {
	return &gachaDetailCardState{
		rarityCounts:  make(map[string]int),
		cardWeight:    make(map[int]float64),
		cardRarity:    make(map[int]string),
		cardCache:     make(map[int]*masterdata.Card),
		rarityWeights: make(map[string]float64),
		rarityRates:   make(map[string]float64),
	}
}

func (s *gachaDetailCardState) load(source DataSource, gachaInfo *masterdata.Gacha) {
	for _, detail := range gachaInfo.GachaDetails {
		cardInfo, err := source.GetCardByID(detail.CardID)
		if err != nil {
			continue
		}
		s.cardCache[cardInfo.ID] = cardInfo
		rarity := strings.ToLower(cardInfo.CardRarityType)
		s.cardRarity[cardInfo.ID] = rarity
		s.rarityCounts[rarity]++
		s.cardWeight[detail.CardID] += float64(detail.Weight)
		s.rarityWeights[rarity] += float64(detail.Weight)
	}
}

func uniqueGachaPickupIDs(gachaInfo *masterdata.Gacha) []int {
	result := make([]int, 0, len(gachaInfo.GachaPickups))
	seen := make(map[int]struct{}, len(gachaInfo.GachaPickups))
	for _, pickup := range gachaInfo.GachaPickups {
		if _, exists := seen[pickup.CardID]; !exists {
			seen[pickup.CardID] = struct{}{}
			result = append(result, pickup.CardID)
		}
	}
	return result
}

func gachaGuaranteedType(gachaInfo *masterdata.Gacha) string {
	guaranteedType := ""
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
	return guaranteedType
}

func buildGachaWeight(gachaInfo *masterdata.Gacha, state *gachaDetailCardState) drawing.GachaWeight {
	weight := drawing.GachaWeight{GuaranteedRates: map[string]float64{}}
	for _, rate := range gachaInfo.GachaCardRarityRates {
		if !strings.EqualFold(rate.LotteryType, "normal") {
			continue
		}
		rarity := strings.ToLower(rate.CardRarityType)
		fraction := rate.Rate / 100.0
		state.rarityRates[rarity] = fraction
		setGachaRarityRate(&weight, rarity, fraction)
	}
	weight.GuaranteedRates = guaranteedGachaRates(state.rarityRates, gachaGuaranteedType(gachaInfo))
	return weight
}

func setGachaRarityRate(weight *drawing.GachaWeight, rarity string, fraction float64) {
	switch rarity {
	case "rarity_1":
		weight.Rarity1Rate = float64Ptr(fraction)
	case "rarity_2":
		weight.Rarity2Rate = float64Ptr(fraction)
	case "rarity_3":
		weight.Rarity3Rate = float64Ptr(fraction)
	case "rarity_4":
		weight.Rarity4Rate = float64Ptr(fraction)
	case "rarity_birthday":
		weight.RarityBirthdayRate = float64Ptr(fraction)
	}
}

func guaranteedGachaRates(rates map[string]float64, guaranteedType string) map[string]float64 {
	if guaranteedType == "" {
		return map[string]float64{}
	}
	result := map[string]float64{
		"rarity_1": rates["rarity_1"], "rarity_2": rates["rarity_2"],
		"rarity_3": rates["rarity_3"], "rarity_4": rates["rarity_4"],
		"rarity_birthday": rates["rarity_birthday"],
	}
	result[guaranteedType] += result["rarity_2"]
	result["rarity_2"] = 0
	if guaranteedType == "rarity_4" {
		result[guaranteedType] += result["rarity_3"]
		result["rarity_3"] = 0
	}
	return result
}

func (s *gachaDetailCardState) cardRate(cardID int) float64 {
	rarity := s.cardRarity[cardID]
	total := s.rarityWeights[rarity]
	base := s.rarityRates[rarity]
	if rarity == "" || total <= 0 || base == 0 {
		return 0
	}
	return (s.cardWeight[cardID] / total) * base
}

func (s *gachaDetailCardState) pickupCards(builder *Builder, pickupIDs []int, region renderregion.Value) []drawing.GachaCardWeight {
	result := make([]drawing.GachaCardWeight, 0, len(pickupIDs))
	for _, cardID := range pickupIDs {
		cardInfo := s.cardCache[cardID]
		if cardInfo == nil {
			var err error
			cardInfo, err = builder.source.GetCardByID(cardID)
			if err != nil {
				continue
			}
			s.cardCache[cardID] = cardInfo
			s.cardRarity[cardID] = strings.ToLower(cardInfo.CardRarityType)
		}
		result = append(result, drawing.GachaCardWeight{
			ID: cardInfo.ID, Rarity: cardInfo.CardRarityType, Rate: s.cardRate(cardInfo.ID),
			ThumbnailRequest: builder.buildGachaThumbnail(cardInfo, region),
		})
	}
	return result
}

func (b *Builder) gachaCeilItemPath(gachaInfo *masterdata.Gacha, region renderregion.Value) *string {
	if gachaInfo.GachaCeilItemID == nil || *gachaInfo.GachaCeilItemID == 0 {
		return nil
	}
	return nonEmptyGachaPath(b.buildCeilItemIconPath(*gachaInfo.GachaCeilItemID, region))
}

func nonEmptyGachaPath(path string) *string {
	if path == "" {
		return nil
	}
	return &path
}

func (b *Builder) buildGachaLogoPath(gachaInfo *masterdata.Gacha, region renderregion.Value) string {
	if gachaInfo == nil {
		return ""
	}
	idText := strconv.Itoa(gachaInfo.ID)
	var candidates []string

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

func (b *Builder) buildCeilItemIconPath(ceilItemID int, region renderregion.Value) string {
	if ceilItemID <= 0 {
		return ""
	}
	assetbundleName, err := b.source.GetGachaCeilItemAssetbundleName(ceilItemID)
	if err != nil || strings.TrimSpace(assetbundleName) == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(
		b.assets,
		region.String(),
		filepath.Join("thumbnail", "gacha_item", assetbundleName+".png"),
		filepath.Join("thumbnail", "material", assetbundleName+".png"),
		filepath.Join("thumbnail", "common_material", assetbundleName+".png"),
	)
}

func (b *Builder) convertBehaviors(gachaInfo *masterdata.Gacha, region renderregion.Value) []drawing.GachaBehavior {
	behaviors := make([]drawing.GachaBehavior, 0, len(gachaInfo.GachaBehaviors))
	jewelIcon := assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, "jewel.png")
	gachaTicketIcon := assets.ResolveRegionAssetPath(
		b.assets,
		region.String(),
		filepath.Join("thumbnail", "gacha_ticket", "gacha_ticket.png"),
	)
	for _, behavior := range gachaInfo.GachaBehaviors {
		var costType *string
		var costQty *int
		var costIcon *string
		if behavior.CostResourceType != "" {
			value := behavior.CostResourceType
			lowerValue := strings.ToLower(value)
			costType = &value
			switch {
			case strings.Contains(lowerValue, "jewel") && jewelIcon != "":
				costIcon = new(jewelIcon)
			case strings.Contains(lowerValue, "ticket"):
				costIcon = new(gachaTicketIcon)
			}
		}
		if behavior.CostResourceQuantity != 0 {
			costQty = new(behavior.CostResourceQuantity)
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
