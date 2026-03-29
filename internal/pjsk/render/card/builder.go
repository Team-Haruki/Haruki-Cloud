package card

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

type Builder struct {
	source      DataSource
	translation DataSource
	events      event.DataSource
	assets      *assets.AssetHelper
}

func NewBuilder(source DataSource, translation DataSource, eventSource event.DataSource, assetHelper *assets.AssetHelper) *Builder {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	return &Builder{
		source:      source,
		translation: translation,
		events:      eventSource,
		assets:      assetHelper,
	}
}

func (b *Builder) BuildCardDetailRequest(card *masterdata.Card, region renderregion.Value) (*drawing.CardDetailRequest, error) {
	if card == nil {
		return nil, fmt.Errorf("card is required")
	}

	cardInfo := b.BuildCardBasic(card, region)

	var eventInfo *drawing.CardEventInfo
	var eventAttrIconPath *string
	var eventUnitIconPath *string
	var eventCharaIconPath *string

	if b.events != nil {
		if eventInfoModel, err := b.events.GetEventByCardID(card.ID); err == nil && eventInfoModel != nil {
			eventInfo = &drawing.CardEventInfo{
				EventID:         eventInfoModel.ID,
				EventName:       eventInfoModel.Name,
				StartAt:         eventInfoModel.StartAt,
				EndAt:           eventInfoModel.AggregateAt + 1000,
				EventBannerPath: b.buildEventBannerPath(eventInfoModel.AssetBundleName, region),
			}
			if bonuses, err := b.events.GetEventDeckBonuses(eventInfoModel.ID); err == nil {
				for _, bonus := range bonuses {
					if bonus == nil || bonus.CardAttr == "" {
						continue
					}
					attr := bonus.CardAttr
					eventInfo.BonusAttr = &attr
					path := assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("attr_icon_%s.png", bonus.CardAttr)))
					eventAttrIconPath = &path
				}

				units := make(map[string]struct{})
				for _, bonus := range bonuses {
					if bonus == nil || bonus.GameCharacterUnitID <= 0 {
						continue
					}
					gameCharacterUnit, err := b.events.GetGameCharacterUnit(bonus.GameCharacterUnitID)
					if err != nil || gameCharacterUnit == nil {
						continue
					}
					units[gameCharacterUnit.Unit] = struct{}{}
				}
				if len(units) == 1 {
					for unit := range units {
						eventInfo.Unit = &unit
						if iconName := b.getUnitIconName(unit); iconName != "" {
							path := assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, iconName+".png")
							eventUnitIconPath = &path
						}
					}
					if bannerCharacterID, err := b.events.GetEventBannerCharacterID(eventInfoModel.ID); err == nil {
						eventInfo.BannerCid = &bannerCharacterID
						path := b.BuildCharacterIconPath(bannerCharacterID, stringValue(eventInfo.Unit), region)
						if path != "" {
							eventCharaIconPath = &path
						}
					}
				}
			}
		}
	}

	var gachaInfo *drawing.CardGachaInfo
	if gachaInfoModel, err := b.source.GetGachaByCardID(card.ID); err == nil && gachaInfoModel != nil {
		gachaInfo = &drawing.CardGachaInfo{
			GachaID:         gachaInfoModel.ID,
			GachaName:       gachaInfoModel.Name,
			StartAt:         gachaInfoModel.StartAt,
			EndAt:           (gachaInfoModel.EndAt/1000 + 1) * 1000,
			GachaBannerPath: b.buildGachaBannerPath(gachaInfoModel.ID, region),
		}
	}

	return &drawing.CardDetailRequest{
		CardInfo:           cardInfo,
		Region:             region.String(),
		EventInfo:          eventInfo,
		GachaInfo:          gachaInfo,
		CardImagesPath:     b.buildCardImagePaths(card, region),
		CostumeImagesPath:  b.buildCostumeImagePaths(card, region),
		CharacterIconPath:  b.BuildCharacterIconPath(card.CharacterID, stringValue(cardInfo.Unit), region),
		UnitLogoPath:       b.buildUnitLogoPath(stringValue(cardInfo.Unit), region),
		EventAttrIconPath:  eventAttrIconPath,
		EventUnitIconPath:  eventUnitIconPath,
		EventCharaIconPath: eventCharaIconPath,
	}, nil
}

func (b *Builder) BuildCardListRequest(cardIDs []int, region renderregion.Value) (*drawing.CardListRequest, error) {
	if len(cardIDs) == 0 {
		return nil, fmt.Errorf("card ids are required")
	}

	cards := make([]drawing.CardBasic, 0, len(cardIDs))
	for _, cardID := range cardIDs {
		card, err := b.source.GetCardByID(cardID)
		if err != nil || card == nil {
			continue
		}
		cardInfo := b.BuildCardBasic(card, region)
		if cardInfo.SupplyType != nil && *cardInfo.SupplyType == "甯搁┗" {
			normal := "normal"
			cardInfo.SupplyType = &normal
		}
		cards = append(cards, cardInfo)
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("no valid cards found from provided ids")
	}

	return &drawing.CardListRequest{
		Cards:  cards,
		Region: region.String(),
	}, nil
}

func (b *Builder) BuildCardBoxRequest(cards []*masterdata.Card, region renderregion.Value, detailedProfile *drawing.DetailedProfileCardRequest, showID, showBox, useAfterTraining bool) (*drawing.CardBoxRequest, error) {
	if len(cards) == 0 {
		return nil, fmt.Errorf("cards are required")
	}

	ownedCards := extractOwnedCards(detailedProfile)
	items := make([]drawing.UserCard, 0, len(cards))
	characterIconPaths := make(map[int]string)
	characterColorCodes := make(map[int]string)
	for _, card := range cards {
		if card == nil {
			continue
		}
		cardInfo := b.BuildCardBasic(card, region)
		if userCard, ok := ownedCards[card.ID]; ok {
			cardInfo.ThumbnailInfo = b.buildBoxThumbnailInfo(card, region, &userCard, useAfterTraining)
			cardInfo.IsAfterTraining = boolPtr(strings.EqualFold(userCard.DefaultImage, "special_training"))
		} else {
			cardInfo.ThumbnailInfo = b.buildBoxThumbnailInfo(card, region, nil, useAfterTraining)
			cardInfo.IsAfterTraining = boolPtr(useAfterTraining && hasAfterTrainingCard(card) && !onlyHasAfterTrainingCard(card))
			if onlyHasAfterTrainingCard(card) {
				cardInfo.IsAfterTraining = boolPtr(true)
			}
		}
		items = append(items, drawing.UserCard{
			Card:    cardInfo,
			HasCard: ownedCards[card.ID].CardID != 0,
		})
		characterIconPaths[card.CharacterID] = b.BuildCharacterIconPath(card.CharacterID, stringValue(cardInfo.Unit), region)
		if colorCode, ok := b.source.GetCharacterColorCode(card.CharacterID); ok {
			characterColorCodes[card.CharacterID] = colorCode
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("cards are required")
	}

	return &drawing.CardBoxRequest{
		Cards:               items,
		Region:              region.String(),
		ShowID:              showID,
		ShowBox:             showBox,
		CharacterIconPaths:  characterIconPaths,
		CharacterColorCodes: characterColorCodes,
	}, nil
}

func hasOwnedCardData(detailedProfile *drawing.DetailedProfileCardRequest) bool {
	return len(extractOwnedCards(detailedProfile)) > 0
}

type ownedCardState struct {
	CardID                int
	Level                 int
	MasterRank            int
	SpecialTrainingStatus string
	DefaultImage          string
}

func extractOwnedCards(detailedProfile *drawing.DetailedProfileCardRequest) map[int]ownedCardState {
	if detailedProfile == nil || len(detailedProfile.UserCards) == 0 {
		return nil
	}
	owned := make(map[int]ownedCardState, len(detailedProfile.UserCards))
	for _, raw := range detailedProfile.UserCards {
		switch item := raw.(type) {
		case map[string]interface{}:
			state := ownedCardState{
				CardID:                intValue(item["cardId"], item["card_id"]),
				Level:                 intValue(item["level"]),
				MasterRank:            intValue(item["masterRank"], item["master_rank"]),
				SpecialTrainingStatus: stringValueAny(item["specialTrainingStatus"], item["special_training_status"]),
				DefaultImage:          stringValueAny(item["defaultImage"], item["default_image"]),
			}
			if state.CardID > 0 {
				owned[state.CardID] = state
			}
		}
	}
	return owned
}

func resolveCardBoxAfterTraining(card drawing.CardBasic, state ownedCardState, useAfterTraining bool, hasOwnedState bool) bool {
	if len(card.ThumbnailInfo) <= 1 {
		return false
	}
	if !hasOwnedState {
		return useAfterTraining
	}
	if strings.TrimSpace(state.DefaultImage) == "" && strings.TrimSpace(state.SpecialTrainingStatus) == "" {
		return useAfterTraining
	}
	if !strings.EqualFold(strings.TrimSpace(state.SpecialTrainingStatus), "done") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(state.DefaultImage), "special_training")
}
func (b *Builder) buildBoxThumbnailInfo(card *masterdata.Card, region renderregion.Value, userCard *ownedCardState, useAfterTraining bool) []drawing.CardFullThumbnailRequest {
	if card == nil {
		return nil
	}

	if userCard != nil {
		afterTraining := strings.EqualFold(userCard.DefaultImage, "special_training") && hasAfterTrainingCard(card)
		rareImageType := "normal"
		if strings.EqualFold(userCard.SpecialTrainingStatus, "done") {
			rareImageType = "after_training"
		}
		fileSuffix := "_normal.png"
		memberFile := "card_normal.png"
		if afterTraining {
			fileSuffix = "_after_training.png"
			memberFile = "card_after_training.png"
		}
		thumbnailPath := assets.ResolveRegionAssetPath(b.assets, region.String(),
			filepath.Join("thumbnail", "chara", card.AssetBundleName+fileSuffix),
			filepath.Join("character", "member", card.AssetBundleName, memberFile),
		)
		rareImgPath := assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "rare_star_"+rareImageType+".png"))
		if card.CardRarityType == "rarity_birthday" {
			rareImgPath = assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "rare_birthday.png"))
		}
		return []drawing.CardFullThumbnailRequest{
			common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{
				AfterTraining: afterTraining,
				ThumbnailPath: thumbnailPath,
				RareImgPath:   rareImgPath,
				TrainRank:     drawing.IntPtr(userCard.MasterRank),
				Level:         drawing.IntPtr(userCard.Level),
				IsPcard:       true,
			}),
		}
	}

	afterTraining := useAfterTraining && hasAfterTrainingCard(card)
	if onlyHasAfterTrainingCard(card) {
		afterTraining = true
	}
	return []drawing.CardFullThumbnailRequest{
		common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{
			AfterTraining: afterTraining,
			TrainedArt:    afterTraining,
		}),
	}
}

func hasAfterTrainingCard(card *masterdata.Card) bool {
	if card == nil {
		return false
	}
	return card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4"
}

func onlyHasAfterTrainingCard(card *masterdata.Card) bool {
	return card != nil && strings.EqualFold(card.InitialSpecialTrainingStatus, "done")
}

func intValue(values ...interface{}) int {
	for _, raw := range values {
		switch value := raw.(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		}
	}
	return 0
}

func stringValueAny(values ...interface{}) string {
	for _, raw := range values {
		if value, ok := raw.(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (b *Builder) BuildCardBasic(card *masterdata.Card, region renderregion.Value) drawing.CardBasic {
	characterID := card.CharacterID
	rare := card.CardRarityType
	attr := card.Attr
	prefix := card.Prefix
	assetBundleName := card.AssetBundleName
	releaseAt := card.ReleaseAt

	info := drawing.CardBasic{
		CardID:          card.ID,
		CharacterID:     &characterID,
		Rare:            &rare,
		Attr:            &attr,
		Prefix:          &prefix,
		AssetBundleName: &assetBundleName,
		ReleaseAt:       &releaseAt,
		IsAfterTraining: boolPtr(false),
		ThumbnailInfo:   b.buildThumbnailInfo(card, region),
		Power:           b.calculatePower(card),
	}

	if character, err := b.source.GetCharacterByID(card.CharacterID); err == nil && character != nil {
		characterName := strings.TrimSpace(character.FirstName + character.GivenName)
		if characterName != "" {
			info.CharacterName = &characterName
		}
	}
	if unit, err := b.source.GetUnitByCardID(card.ID); err == nil && strings.TrimSpace(unit) != "" {
		info.Unit = &unit
	}

	supplyType := b.source.GetCardSupplyType(card)
	if strings.TrimSpace(supplyType) != "" {
		info.SupplyType = &supplyType
	}

	if skill, err := b.source.GetSkillByID(card.SkillID); err == nil && skill != nil {
		info.Skill = &drawing.CardSkill{
			SkillID:           skill.ID,
			SkillName:         card.CardSkillName,
			SkillType:         skill.DescriptionSpriteName,
			SkillDetail:       b.buildDualSkillDetail(card, skill, region),
			SkillTypeIconPath: b.buildSkillTypeIconPath(skill.DescriptionSpriteName, region),
		}
	}
	if card.SpecialTrainingSkillID > 0 {
		if skill, err := b.source.GetSkillByID(card.SpecialTrainingSkillID); err == nil && skill != nil {
			info.SpecialSkillInfo = &drawing.CardSkill{
				SkillID:           skill.ID,
				SkillName:         card.SpecialTrainingSkillName,
				SkillType:         skill.DescriptionSpriteName,
				SkillDetail:       b.buildDualSkillDetail(card, skill, region),
				SkillTypeIconPath: b.buildSkillTypeIconPath(skill.DescriptionSpriteName, region),
			}
		}
	}

	return info
}

func (b *Builder) buildThumbnailInfo(card *masterdata.Card, region renderregion.Value) []drawing.CardFullThumbnailRequest {
	// Cards with initial_special_training_status='done' only have after_training art
	alreadyTrained := strings.EqualFold(card.InitialSpecialTrainingStatus, "done")

	if alreadyTrained {
		return []drawing.CardFullThumbnailRequest{
			common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{AfterTraining: true, TrainedArt: true}),
		}
	}

	items := []drawing.CardFullThumbnailRequest{
		common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{AfterTraining: false}),
	}
	if card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4" {
		items = append(items, common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{AfterTraining: true, TrainedArt: true}))
	}
	return items
}

func (b *Builder) calculatePower(card *masterdata.Card) *drawing.CardPower {
	if card == nil {
		return nil
	}

	var power1 int
	var power2 int
	var power3 int
	for _, parameter := range card.CardParameters {
		switch parameter.CardParameterType {
		case "param1":
			if parameter.Power > power1 {
				power1 = parameter.Power
			}
		case "param2":
			if parameter.Power > power2 {
				power2 = parameter.Power
			}
		case "param3":
			if parameter.Power > power3 {
				power3 = parameter.Power
			}
		}
	}

	power1 += card.SpecialTrainingPower1BonusFixed
	power2 += card.SpecialTrainingPower2BonusFixed
	power3 += card.SpecialTrainingPower3BonusFixed

	return &drawing.CardPower{
		Power1:     power1,
		Power2:     power2,
		Power3:     power3,
		PowerTotal: power1 + power2 + power3,
	}
}

func (b *Builder) buildDualSkillDetail(card *masterdata.Card, skill *masterdata.Skill, region renderregion.Value) string {
	if card == nil || skill == nil {
		return ""
	}
	var lines []string
	if primary := strings.TrimSpace(b.source.FormatSkillDescription(skill, card.CharacterID)); primary != "" {
		lines = append(lines, primary)
	}
	if region == renderregion.JP && b.translation != nil {
		if translated, err := b.translation.GetSkillByID(skill.ID); err == nil && translated != nil {
			if secondary := strings.TrimSpace(b.translation.FormatSkillDescription(translated, card.CharacterID)); secondary != "" {
				lines = append(lines, secondary)
			}
		}
	}
	return combineSkillLines(lines...)
}

func combineSkillLines(lines ...string) string {
	seen := make(map[string]struct{})
	ordered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		ordered = append(ordered, line)
	}
	return strings.Join(ordered, "\n")
}

func (b *Builder) buildCardImagePaths(card *masterdata.Card, region renderregion.Value) []string {
	if card == nil {
		return nil
	}
	base := filepath.Join("character", "member", card.AssetBundleName)
	paths := []string{
		assets.ResolveRegionAssetPath(b.assets, region.String(), filepath.Join(base, "card_normal.png")),
	}
	if card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4" {
		paths = append(paths, assets.ResolveRegionAssetPath(b.assets, region.String(), filepath.Join(base, "card_after_training.png")))
	}
	return paths
}

func (b *Builder) buildCostumeImagePaths(card *masterdata.Card, region renderregion.Value) []string {
	if card == nil {
		return []string{}
	}
	costumes, err := b.source.GetCostume3dsByCardID(card.ID)
	if err != nil || len(costumes) == 0 {
		return []string{}
	}

	paths := make([]string, 0, len(costumes))
	for _, costume := range costumes {
		if costume == nil {
			continue
		}
		paths = append(paths, assets.ResolveRegionAssetPath(b.assets, region.String(), filepath.Join("thumbnail", "costume", costume.AssetBundleName+".png")))
	}
	return paths
}

func (b *Builder) BuildCharacterIconPath(characterID int, unit string, region renderregion.Value) string {
	if characterID == 21 && unit != "" && unit != "piapro" {
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", fmt.Sprintf("miku_%s.png", unit)))
	}
	if nickname, ok := assets.CharacterIDToNickname[characterID]; ok {
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", nickname+".png"))
	}
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)))
}

func (b *Builder) buildUnitLogoPath(unit string, region renderregion.Value) string {
	if unit == "" {
		return ""
	}
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, fmt.Sprintf("logo_%s.png", unit))
}

func (b *Builder) buildSkillTypeIconPath(skillType string, region renderregion.Value) *string {
	if strings.TrimSpace(skillType) == "" {
		return nil
	}
	path := assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, fmt.Sprintf("skill_%s.png", skillType))
	return &path
}

func (b *Builder) buildEventBannerPath(assetBundleName string, region renderregion.Value) string {
	if strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(b.assets, region.String(),
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
	)
}

func (b *Builder) buildGachaBannerPath(gachaID int, region renderregion.Value) string {
	if gachaID == 0 {
		return ""
	}
	return assets.ResolveRegionAssetPath(b.assets, region.String(),
		filepath.Join("home", "banner", fmt.Sprintf("banner_gacha%d", gachaID), fmt.Sprintf("banner_gacha%d.png", gachaID)),
		filepath.Join("gacha", fmt.Sprintf("banner_gacha%d.png", gachaID)),
	)
}

func (b *Builder) getUnitIconName(unit string) string {
	switch unit {
	case "light_sound":
		return "light_sound"
	case "idol":
		return "idol"
	case "street":
		return "street"
	case "theme_park":
		return "theme_park"
	case "school_refusal":
		return "school_refusal"
	case "piapro":
		return "piapro"
	default:
		return ""
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
