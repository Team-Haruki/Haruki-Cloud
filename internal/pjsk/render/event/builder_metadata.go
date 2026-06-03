package event

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

func (b *Builder) buildEventInfo(eventInfo *masterdata.Event) (drawing.EventInfo, error) {
	isWLEvent := strings.EqualFold(eventInfo.EventType, "world_bloom")
	info := drawing.EventInfo{
		ID:            eventInfo.ID,
		EventType:     eventInfo.EventType,
		EventTypeName: b.displayEventType(eventInfo.EventType),
		StartAt:       eventInfo.StartAt,
		EndAt:         eventInfo.AggregateAt + 1000,
		IsWlEvent:     isWLEvent,
		BonusCharaID:  []int{},
	}

	if !isWLEvent && b.isBoxEvent(eventInfo.ID) {
		if bannerCID, err := b.source.GetEventBannerCharacterID(eventInfo.ID); err == nil && bannerCID != 0 {
			info.BannerCid = bannerCID
			if idx := b.getBannerIndex(bannerCID, eventInfo.ID); idx != nil {
				info.BannerIndex = *idx
			}
		}
	}

	if attr, chars := b.extractEventBonuses(eventInfo.ID); attr != "" {
		info.BonusAttr = attr
		if chars != nil {
			info.BonusCharaID = chars
		}
	}
	if wlTimeline := b.buildWorldBloomTimeline(eventInfo.ID); len(wlTimeline) > 0 {
		info.WlTimeList = wlTimeline
	}

	return info, nil
}

func (b *Builder) buildEventAssets(eventInfo *masterdata.Event, info drawing.EventInfo, region renderregion.Value) drawing.EventAssets {
	assetName := eventInfo.AssetBundleName
	result := drawing.EventAssets{
		EventBgPath: assets.ResolveRegionAssetPath(b.assets, region.String(),
			filepath.Join("event", assetName, "screen", "bg.png"),
			filepath.Join("event", assetName, "bg.png"),
		),
		EventLogoPath: assets.ResolveRegionAssetPath(b.assets, region.String(),
			filepath.Join("event", assetName, "logo", "logo.png"),
			filepath.Join("event", assetName, "logo.png"),
		),
		BonusCharaPath: []string{},
	}

	if !strings.EqualFold(eventInfo.EventType, "world_bloom") {
		result.EventStoryBgPath = assets.ResolveRegionAssetPath(b.assets, region.String(), filepath.Join("event_story", assetName, "screen_image", "story_bg.png"))
		result.EventBanCharaImg = assets.ResolveRegionAssetPath(b.assets, region.String(), filepath.Join("event", assetName, "screen", "character.png"))
	}
	if info.BonusAttr != "" {
		result.EventAttrImagePath = assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("attr_icon_%s.png", strings.ToLower(info.BonusAttr))))
	}
	if info.BannerCid != 0 {
		result.BanCharaIconPath = b.characterIconPath(info.BannerCid, region)
	}
	for _, cid := range info.BonusCharaID {
		result.BonusCharaPath = append(result.BonusCharaPath, b.characterIconPath(cid, region))
	}
	if result.BanCharaIconPath == "" && len(info.BonusCharaID) > 0 {
		result.BanCharaIconPath = b.characterIconPath(info.BonusCharaID[0], region)
	}

	return result
}

func (b *Builder) buildEventBrief(eventInfo *masterdata.Event, region renderregion.Value) (drawing.EventBrief, error) {
	brief := drawing.EventBrief{
		ID:              eventInfo.ID,
		EventName:       eventInfo.Name,
		EventType:       b.displayEventType(eventInfo.EventType),
		EventTypeName:   b.displayEventType(eventInfo.EventType),
		StartAt:         eventInfo.StartAt,
		EndAt:           eventInfo.AggregateAt + 1000,
		EventBannerPath: assets.ResolveEventBannerPath(b.assets, region.String(), eventInfo.AssetBundleName),
	}

	cards, err := b.source.GetEventCards(eventInfo.ID)
	if err == nil && len(cards) > 0 {
		maxCards := len(cards)
		if maxCards > 6 {
			maxCards = 6
		}
		for i := 0; i < maxCards; i++ {
			brief.EventCards = append(brief.EventCards, common.BuildCardThumbnail(b.assets, cards[i], region, common.ThumbnailOptions{}))
		}
	}
	if attr, _ := b.extractEventBonuses(eventInfo.ID); attr != "" {
		brief.EventAttrPath = new(assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("attr_%s.png", strings.ToLower(attr)))))
	}

	isWLEvent := strings.EqualFold(eventInfo.EventType, "world_bloom")
	if !isWLEvent {
		if bannerCID, err := b.source.GetEventBannerCharacterID(eventInfo.ID); err == nil && bannerCID != 0 {
			brief.EventCharaPath = new(b.characterIconPath(bannerCID, region))
			if unit := b.unitIconPathByCharacter(bannerCID, region); unit != "" {
				brief.EventUnitPath = &unit
			}
		}
		return brief, nil
	}

	if len(cards) > 0 && len(cards) <= 6 {
		if unit := b.unitIconPathByCharacter(cards[0].CharacterID, region); unit != "" {
			brief.EventUnitPath = &unit
		}
	}
	return brief, nil
}

func (b *Builder) characterIconPath(charID int, _ renderregion.Value) string {
	if nickname, ok := assets.CharacterIDToNickname[charID]; ok {
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", nickname+".png"))
	}
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
}

func (b *Builder) characterDisplayName(charID int) string {
	character, err := b.source.GetCharacterByID(charID)
	if err != nil || character == nil {
		return ""
	}
	return strings.TrimSpace(character.FirstName + character.GivenName)
}

func (b *Builder) unitIconPathByCharacter(charID int, region renderregion.Value) string {
	character, err := b.source.GetCharacterByID(charID)
	if err != nil || character == nil {
		return ""
	}
	unitIcon := assets.UnitIconFilename(character.Unit)
	if unitIcon == "" {
		return ""
	}
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, unitIcon+".png")
}

func (b *Builder) displayEventType(code string) string {
	switch strings.ToLower(code) {
	case "marathon":
		return "马拉松"
	case "cheerful_carnival":
		return "5v5"
	case "world_bloom":
		return "WorldLink"
	default:
		return code
	}
}
