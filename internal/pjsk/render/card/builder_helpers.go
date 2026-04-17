package card

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (b *Builder) BuildCardBasic(card *masterdata.Card, region renderregion.Value) drawing.CardBasic {
	info := drawing.CardBasic{
		CardID:          card.ID,
		CharacterID:     new(card.CharacterID),
		Rare:            new(card.CardRarityType),
		Attr:            new(card.Attr),
		Prefix:          new(card.Prefix),
		AssetBundleName: new(card.AssetBundleName),
		ReleaseAt:       new(card.ReleaseAt),
		IsAfterTraining: common.BoolPtr(false),
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

	supplyType := formatSupplyTypeForList(b.source.GetCardSupplyType(card))
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

func (b *Builder) buildCardImagePaths(card *masterdata.Card, region renderregion.Value) (paths []string) {
	if card == nil {
		return nil
	}

	if !onlyHasAfterTrainingCard(card) {
		if path := common.ResolveCardMemberImagePath(b.assets, region, card.AssetBundleName, "card_normal.png"); strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	if card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4" {
		if path := common.ResolveCardMemberImagePath(b.assets, region, card.AssetBundleName, "card_after_training.png"); strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
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
		assetBundleName := strings.TrimSpace(costume.AssetBundleName)
		if assetBundleName == "" {
			continue
		}
		paths = append(paths, assets.ResolveRegionAssetPath(b.assets, region.String(), filepath.Join("thumbnail", "costume", assetBundleName+".png")))
	}
	return paths
}

func (b *Builder) BuildCharacterIconPath(characterID int, unit string, region renderregion.Value) string {
	switch characterID {
	case 27:
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_light_sound.png"))
	case 28:
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_idol.png"))
	case 29:
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_street.png"))
	case 30:
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_theme_park.png"))
	case 31:
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_school_refusal.png"))
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
	return new(assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, fmt.Sprintf("skill_%s.png", skillType)))
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

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
