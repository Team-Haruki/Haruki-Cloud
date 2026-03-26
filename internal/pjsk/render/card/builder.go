package card

import (
	"fmt"
	"path/filepath"
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
				EventBannerPath: b.buildEventBannerPath(eventInfoModel.AssetBundleName),
			}
			if bonuses, err := b.events.GetEventDeckBonuses(eventInfoModel.ID); err == nil {
				for _, bonus := range bonuses {
					if bonus == nil || bonus.CardAttr == "" {
						continue
					}
					attr := bonus.CardAttr
					eventInfo.BonusAttr = &attr
					path := assets.ResolveAssetPath(b.assets, "", filepath.Join("card", fmt.Sprintf("attr_icon_%s.png", bonus.CardAttr)))
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
							path := assets.ResolveAssetPath(b.assets, "", filepath.Join("unit", iconName+".png"))
							eventUnitIconPath = &path
						}
					}
					if bannerCharacterID, err := b.events.GetEventBannerCharacterID(eventInfoModel.ID); err == nil {
						eventInfo.BannerCid = &bannerCharacterID
						path := b.BuildCharacterIconPath(bannerCharacterID, stringValue(eventInfo.Unit))
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
			GachaBannerPath: b.buildGachaBannerPath(gachaInfoModel.ID),
		}
	}

	return &drawing.CardDetailRequest{
		CardInfo:           cardInfo,
		Region:             region.String(),
		EventInfo:          eventInfo,
		GachaInfo:          gachaInfo,
		CardImagesPath:     b.buildCardImagePaths(card),
		CostumeImagesPath:  b.buildCostumeImagePaths(card),
		CharacterIconPath:  b.BuildCharacterIconPath(card.CharacterID, stringValue(cardInfo.Unit)),
		UnitLogoPath:       b.buildUnitLogoPath(stringValue(cardInfo.Unit)),
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
		if cardInfo.SupplyType != nil && *cardInfo.SupplyType == "常驻" {
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

func (b *Builder) BuildCardBoxRequest(cards []*masterdata.Card, region renderregion.Value) (*drawing.CardBoxRequest, error) {
	if len(cards) == 0 {
		return nil, fmt.Errorf("cards are required")
	}

	items := make([]drawing.UserCard, 0, len(cards))
	characterIconPaths := make(map[int]string)
	for _, card := range cards {
		if card == nil {
			continue
		}
		cardInfo := b.BuildCardBasic(card, region)
		if len(cardInfo.ThumbnailInfo) > 1 {
			cardInfo.IsAfterTraining = boolPtr(true)
		}
		items = append(items, drawing.UserCard{
			Card:    cardInfo,
			HasCard: false,
		})
		characterIconPaths[card.CharacterID] = b.BuildCharacterIconPath(card.CharacterID, stringValue(cardInfo.Unit))
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("cards are required")
	}

	return &drawing.CardBoxRequest{
		Cards:              items,
		Region:             region.String(),
		ShowID:             false,
		ShowBox:            false,
		CharacterIconPaths: characterIconPaths,
	}, nil
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
		ThumbnailInfo:   b.buildThumbnailInfo(card),
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
			SkillTypeIconPath: b.buildSkillTypeIconPath(skill.DescriptionSpriteName),
		}
	}
	if card.SpecialTrainingSkillID > 0 {
		if skill, err := b.source.GetSkillByID(card.SpecialTrainingSkillID); err == nil && skill != nil {
			info.SpecialSkillInfo = &drawing.CardSkill{
				SkillID:           skill.ID,
				SkillName:         card.SpecialTrainingSkillName,
				SkillType:         skill.DescriptionSpriteName,
				SkillDetail:       b.buildDualSkillDetail(card, skill, region),
				SkillTypeIconPath: b.buildSkillTypeIconPath(skill.DescriptionSpriteName),
			}
		}
	}

	return info
}

func (b *Builder) buildThumbnailInfo(card *masterdata.Card) []drawing.CardFullThumbnailRequest {
	items := []drawing.CardFullThumbnailRequest{
		common.BuildCardThumbnail(b.assets, card, common.ThumbnailOptions{AfterTraining: false}),
	}
	if card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4" {
		items = append(items, common.BuildCardThumbnail(b.assets, card, common.ThumbnailOptions{AfterTraining: true, TrainedArt: true}))
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

func (b *Builder) buildCardImagePaths(card *masterdata.Card) []string {
	if card == nil {
		return nil
	}
	base := filepath.Join("character", "member", card.AssetBundleName)
	paths := []string{
		assets.ResolveAssetPath(b.assets, "", filepath.Join(base, "card_normal.png")),
	}
	if card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4" {
		paths = append(paths, assets.ResolveAssetPath(b.assets, "", filepath.Join(base, "card_after_training.png")))
	}
	return paths
}

func (b *Builder) buildCostumeImagePaths(card *masterdata.Card) []string {
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
		paths = append(paths, assets.ResolveAssetPath(b.assets, "", filepath.Join("thumbnail", "costume", costume.AssetBundleName+".png")))
	}
	return paths
}

func (b *Builder) BuildCharacterIconPath(characterID int, unit string) string {
	if characterID == 21 && unit != "" && unit != "piapro" {
		return assets.ResolveAssetPath(b.assets, "", filepath.Join("chara_icon", fmt.Sprintf("miku_%s.png", unit)))
	}
	if nickname, ok := assets.CharacterIDToNickname[characterID]; ok {
		return assets.ResolveAssetPath(b.assets, "", filepath.Join("chara_icon", nickname+".png"))
	}
	return assets.ResolveAssetPath(b.assets, "", filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)))
}

func (b *Builder) buildUnitLogoPath(unit string) string {
	if unit == "" {
		return ""
	}
	return assets.ResolveAssetPath(b.assets, "", fmt.Sprintf("logo_%s.png", unit))
}

func (b *Builder) buildSkillTypeIconPath(skillType string) *string {
	if strings.TrimSpace(skillType) == "" {
		return nil
	}
	path := assets.ResolveAssetPath(b.assets, "", filepath.Join("skill", fmt.Sprintf("skill_%s.png", skillType)))
	return &path
}

func (b *Builder) buildEventBannerPath(assetBundleName string) string {
	if strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return assets.ResolveAssetPath(b.assets, "",
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
	)
}

func (b *Builder) buildGachaBannerPath(gachaID int) string {
	if gachaID == 0 {
		return ""
	}
	return assets.ResolveAssetPath(b.assets, "",
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
