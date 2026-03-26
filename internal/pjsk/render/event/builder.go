package event

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
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

func (b *Builder) BuildEventDetailRequest(query DetailQuery) (*drawing.EventDetailRequest, error) {
	if query.EventID == 0 {
		return nil, fmt.Errorf("event id is required")
	}

	eventInfo, err := b.source.GetEventByID(query.EventID)
	if err != nil {
		return nil, err
	}
	region := query.Region
	if region.IsZero() {
		region = b.source.DefaultRegion()
	}

	cards, err := b.source.GetEventCards(eventInfo.ID)
	if err != nil {
		return nil, err
	}
	cardThumbs := make([]drawing.CardFullThumbnailRequest, 0, len(cards))
	for _, card := range cards {
		cardThumbs = append(cardThumbs, common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{AfterTraining: false}))
	}

	info, err := b.buildEventInfo(eventInfo)
	if err != nil {
		return nil, err
	}

	return &drawing.EventDetailRequest{
		Region:      region.String(),
		EventInfo:   info,
		EventAssets: b.buildEventAssets(eventInfo, info, region),
		EventCards:  cardThumbs,
	}, nil
}

func (b *Builder) BuildEventListRequest(query ListQuery) (*drawing.EventListRequest, error) {
	events := b.filterEvents(query)
	if len(events) == 0 {
		return nil, fmt.Errorf("no events matched filters")
	}

	region := query.Region
	if region.IsZero() {
		region = b.source.DefaultRegion()
	}

	briefs := make([]drawing.EventBrief, 0, len(events))
	for _, eventInfo := range events {
		brief, err := b.buildEventBrief(eventInfo, region)
		if err != nil {
			return nil, err
		}
		briefs = append(briefs, brief)
	}

	return &drawing.EventListRequest{
		EventInfo: briefs,
	}, nil
}

func (b *Builder) buildEventInfo(eventInfo *masterdata.Event) (drawing.EventInfo, error) {
	isWLEvent := strings.EqualFold(eventInfo.EventType, "world_bloom")
	info := drawing.EventInfo{
		ID:           eventInfo.ID,
		EventType:    b.displayEventType(eventInfo.EventType),
		StartAt:      eventInfo.StartAt,
		EndAt:        eventInfo.AggregateAt + 1000,
		IsWlEvent:    isWLEvent,
		BonusCharaID: []int{},
	}

	if !isWLEvent {
		if bannerCID, err := b.source.GetEventBannerCharacterID(eventInfo.ID); err == nil && bannerCID != 0 {
			info.BannerCid = bannerCID
			if idx := b.getBannerIndex(bannerCID, eventInfo.ID); idx != nil {
				info.BannerIndex = *idx
			} else {
				info.BannerIndex = 1
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
	assetDir := assets.RegionAssetDir(region.String())
	result := drawing.EventAssets{
		EventBgPath: assets.ResolveAssetPath(b.assets, assetDir,
			filepath.Join("event", assetName, "screen", "bg.png"),
			filepath.Join("event", assetName, "bg.png"),
		),
		EventLogoPath: assets.ResolveAssetPath(b.assets, assetDir,
			filepath.Join("event", assetName, "logo", "logo.png"),
			filepath.Join("event", assetName, "logo.png"),
		),
		BonusCharaPath: []string{},
	}

	if !strings.EqualFold(eventInfo.EventType, "world_bloom") {
		result.EventStoryBgPath = assets.ResolveAssetPath(b.assets, assetDir, filepath.Join("event_story", assetName, "screen_image", "story_bg.png"))
		result.EventBanCharaImg = assets.ResolveAssetPath(b.assets, assetDir, filepath.Join("event", assetName, "screen", "character.png"))
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
	assetDir := assets.RegionAssetDir(region.String())
	brief := drawing.EventBrief{
		ID:        eventInfo.ID,
		EventName: eventInfo.Name,
		EventType: b.displayEventType(eventInfo.EventType),
		StartAt:   eventInfo.StartAt,
		EndAt:     eventInfo.AggregateAt + 1000,
		EventBannerPath: assets.ResolveAssetPath(b.assets, assetDir,
			filepath.Join("home", "banner", eventInfo.AssetBundleName, eventInfo.AssetBundleName+".png"),
			filepath.Join("event", eventInfo.AssetBundleName, "banner.png"),
		),
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
		path := assets.ResolveAssetPath(b.assets, assetDir, filepath.Join("card", fmt.Sprintf("attr_%s.png", strings.ToLower(attr))))
		brief.EventAttrPath = &path
	}

	isWLEvent := strings.EqualFold(eventInfo.EventType, "world_bloom")
	if !isWLEvent {
		if bannerCID, err := b.source.GetEventBannerCharacterID(eventInfo.ID); err == nil && bannerCID != 0 {
			path := b.characterIconPath(bannerCID, region)
			brief.EventCharaPath = &path
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

func (b *Builder) filterEvents(query ListQuery) []*masterdata.Event {
	events := b.source.GetEvents()
	now := time.Now()
	result := make([]*masterdata.Event, 0, len(events))
	includePast := query.IncludePast
	includeFuture := query.IncludeFuture
	if query.OnlyFuture {
		includeFuture = true
		includePast = false
	}

	for _, eventInfo := range events {
		start := time.UnixMilli(eventInfo.StartAt)
		end := time.UnixMilli(eventInfo.AggregateAt + 1000)
		if !includePast && end.Before(now) {
			continue
		}
		if !includeFuture && start.After(now) {
			continue
		}
		if query.EventType != "" && !strings.EqualFold(eventInfo.EventType, query.EventType) {
			continue
		}
		if query.Year != 0 && start.Year() != query.Year {
			continue
		}
		if query.Unit != "" || query.Blend || query.Attr != "" || query.CharacterID != 0 || len(query.CharacterIDs) > 0 {
			if !b.matchEventBonus(eventInfo.ID, query.Unit, query.Blend, query.Attr, query.CharacterID, query.CharacterIDs) {
				continue
			}
		}
		if query.BannerCharID != nil {
			bannerCID, err := b.source.GetEventBannerCharacterID(eventInfo.ID)
			if err != nil || bannerCID != *query.BannerCharID {
				continue
			}
		}
		result = append(result, eventInfo)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartAt < result[j].StartAt
	})
	if query.Limit > 0 && len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result
}

func normalizeRegion(value renderregion.Value) renderregion.Value {
	return renderregion.Normalize(value.String())
}

func (b *Builder) extractEventBonuses(eventID int) (string, []int) {
	bonuses, err := b.source.GetEventDeckBonuses(eventID)
	if err != nil {
		return "", nil
	}

	attr := ""
	charSet := make(map[int]struct{})
	for _, bonus := range bonuses {
		if attr == "" && bonus.CardAttr != "" {
			attr = strings.ToLower(bonus.CardAttr)
		}
		if bonus.GameCharacterUnitID != 0 {
			if unit, err := b.source.GetGameCharacterUnit(bonus.GameCharacterUnitID); err == nil && unit != nil {
				charSet[unit.GameCharacterID] = struct{}{}
			}
		} else if bonus.GameCharacterID != 0 {
			charSet[bonus.GameCharacterID] = struct{}{}
		}
	}

	chars := make([]int, 0, len(charSet))
	for cid := range charSet {
		chars = append(chars, cid)
	}
	sort.Ints(chars)
	return attr, chars
}

func (b *Builder) matchEventBonus(eventID int, unit string, blend bool, attr string, charID int, charIDs []int) bool {
	if unit == "" && !blend && attr == "" && charID == 0 && len(charIDs) == 0 {
		return true
	}

	bonuses, err := b.source.GetEventDeckBonuses(eventID)
	if err != nil {
		return false
	}

	attrMatched := attr == ""
	units := make(map[string]struct{})
	charSet := make(map[int]struct{})

	for _, bonus := range bonuses {
		if !attrMatched && strings.EqualFold(bonus.CardAttr, attr) {
			attrMatched = true
		}
		if bonus.GameCharacterUnitID != 0 {
			gcu, gcuErr := b.source.GetGameCharacterUnit(bonus.GameCharacterUnitID)
			if gcuErr == nil && gcu != nil {
				units[strings.ToLower(strings.TrimSpace(gcu.Unit))] = struct{}{}
				charSet[gcu.GameCharacterID] = struct{}{}
			}
		} else if bonus.GameCharacterID != 0 {
			charSet[bonus.GameCharacterID] = struct{}{}
		}
	}

	unitMatched := unit == ""
	if unit != "" {
		_, unitMatched = units[strings.ToLower(strings.TrimSpace(unit))]
	}
	if blend || strings.EqualFold(strings.TrimSpace(unit), "blend") {
		unitMatched = len(units) > 1
	}

	charMatched := true
	if charID != 0 {
		_, charMatched = charSet[charID]
	}
	for _, cid := range charIDs {
		if cid == 0 {
			continue
		}
		if _, ok := charSet[cid]; !ok {
			charMatched = false
			break
		}
	}

	return attrMatched && unitMatched && charMatched
}

func (b *Builder) getBannerIndex(charID, eventID int) *int {
	events := b.source.GetBanEvents(charID)
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartAt < events[j].StartAt
	})
	for idx, eventInfo := range events {
		if eventInfo.ID == eventID {
			index := idx + 1
			return &index
		}
	}
	return nil
}

func (b *Builder) buildWorldBloomTimeline(eventID int) []map[string]interface{} {
	chapters := b.source.GetWorldBloomChapters(eventID)
	if len(chapters) == 0 {
		return nil
	}

	timeline := make([]map[string]interface{}, 0, len(chapters))
	for _, chapter := range chapters {
		item := map[string]interface{}{
			"start_at":     chapter.ChapterStartAt,
			"aggregate_at": chapter.AggregateAt,
		}
		if chapter.ChapterNo != 0 {
			item["chapter_id"] = chapter.ChapterNo
		}
		if chapter.GameCharacterID != nil && *chapter.GameCharacterID != 0 {
			item["game_character_id"] = *chapter.GameCharacterID
		}
		timeline = append(timeline, item)
	}
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i]["start_at"].(int64) < timeline[j]["start_at"].(int64)
	})
	return timeline
}

func (b *Builder) characterIconPath(charID int, _ renderregion.Value) string {
	if nickname, ok := assets.CharacterIDToNickname[charID]; ok {
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", nickname+".png"))
	}
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
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
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("unit", unitIcon+".png"))
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
