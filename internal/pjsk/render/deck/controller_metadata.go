package deck

import (
	"fmt"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) applyCommonRecommendMetadata(request *drawing.DeckRequest, region renderregion.Value, recType string, option map[string]any, query AutoQuery) {
	if request == nil {
		return
	}
	applyRecommendLiveMetadata(request, recType, option)
	finalEventID := c.resolveRecommendMetadataEventID(region, recType, option, query)
	if optionHasUnitAttrEvent(option) && recType == "event" {
		request.RecommendType = "unit_attr"
	}
	c.applyWorldBloomRecommendMetadata(request, region, recType, option, query)
	c.applyChallengeRecommendMetadata(request, region, recType, option)
	c.applyRecommendFilterMetadata(request, option)
	c.applyRecommendEventMetadata(request, region, finalEventID)
}

func applyRecommendLiveMetadata(request *drawing.DeckRequest, recType string, option map[string]any) {
	liveType := optionString(option, "live_type")
	if liveType == "" {
		switch recType {
		case "challenge":
			liveType = "challenge"
		case "mysekai":
			liveType = "mysekai"
		default:
			liveType = "multi"
		}
	}
	switch liveType {
	case "solo", "challenge":
		request.LiveType = drawing.StringPtr("solo")
		request.LiveName = drawing.StringPtr("单人")
	case "auto", "challenge_auto":
		request.LiveType = drawing.StringPtr("auto")
		request.LiveName = drawing.StringPtr("自动")
	case "mysekai":
		request.LiveType = drawing.StringPtr("mysekai")
		request.LiveName = drawing.StringPtr("烤森")
	default:
		request.LiveType = drawing.StringPtr("multi")
		request.LiveName = drawing.StringPtr("协力")
	}
}

func (c *Controller) resolveRecommendMetadataEventID(region renderregion.Value, recType string, option map[string]any, query AutoQuery) int {
	finalEventID := 0
	if option != nil {
		finalEventID = optionInt(option, "event_id")
	}
	if finalEventID <= 0 && query.EventID != nil && *query.EventID > 0 {
		finalEventID = *query.EventID
	}
	if finalEventID <= 0 && shouldUseFallbackEventMetadata(recType, option) {
		finalEventID = c.pickCurrentOrNextEventID(region)
	}
	return finalEventID
}

func (c *Controller) applyWorldBloomRecommendMetadata(request *drawing.DeckRequest, region renderregion.Value, recType string, option map[string]any, query AutoQuery) {
	if optionHasWorldBloom(option) || queryHasWorldBloomMetadata(query) {
		request.IsWl = true
		applyWorldBloomRecommendType(request, recType)
		c.applyWorldBloomCharacterMetadata(request, region, worldBloomMetadataCharacterID(option, query))
	}
}

func applyWorldBloomRecommendType(request *drawing.DeckRequest, recType string) {
	switch recType {
	case "event":
		request.RecommendType = "wl"
	case "bonus":
		request.RecommendType = "wl_bonus"
	}
}

func worldBloomMetadataCharacterID(option map[string]any, query AutoQuery) int {
	if cid := optionInt(option, "world_bloom_character_id"); cid > 0 {
		return cid
	}
	if query.WorldBloomCharacterID != nil && *query.WorldBloomCharacterID > 0 {
		return *query.WorldBloomCharacterID
	}
	if query.MetadataWorldBloomCharacterID != nil && *query.MetadataWorldBloomCharacterID > 0 {
		return *query.MetadataWorldBloomCharacterID
	}
	return 0
}

func (c *Controller) applyWorldBloomCharacterMetadata(request *drawing.DeckRequest, region renderregion.Value, characterID int) {
	if characterID <= 0 {
		return
	}
	if icon := c.resolveCharacterIconPath(characterID); icon != "" {
		request.WlCharaIconPath = &icon
		if request.CharaIconPath == nil {
			request.CharaIconPath = &icon
		}
	}
	if name := c.resolveCharacterName(region, characterID); name != "" {
		request.WlCharaName = &name
		if request.CharaName == nil {
			request.CharaName = &name
		}
	}
}

func (c *Controller) applyChallengeRecommendMetadata(request *drawing.DeckRequest, region renderregion.Value, recType string, option map[string]any) {
	if recType == "challenge" {
		if cid := optionInt(option, "challenge_live_character_id"); cid > 0 {
			if icon := c.resolveCharacterIconPath(cid); icon != "" {
				request.CharaIconPath = &icon
			}
			if name := c.resolveCharacterName(region, cid); name != "" {
				request.CharaName = &name
			}
		}
	}
}

func (c *Controller) applyRecommendFilterMetadata(request *drawing.DeckRequest, option map[string]any) {
	unit := optionString(option, "event_unit")
	if unit == "" {
		unit = optionString(option, "unit_filter")
	}
	if unit != "" {
		if path := c.resolveUnitIconPath(unit); path != "" {
			request.UnitLogoPath = &path
		}
	}
	attr := optionString(option, "event_attr")
	if attr == "" {
		attr = optionString(option, "attr_filter")
	}
	if attr != "" {
		if path := c.resolveAttrIconPath(attr); path != "" {
			request.AttrIconPath = &path
		}
	}
}

func (c *Controller) applyRecommendEventMetadata(request *drawing.DeckRequest, region renderregion.Value, finalEventID int) {
	if finalEventID > 0 {
		request.EventID = drawing.IntPtr(finalEventID)
		eventName := fmt.Sprintf("Event #%d", finalEventID)
		request.EventName = &eventName
		if _, eventSource, ok := c.resolveEventSource(region); ok {
			if eventInfo, err := eventSource.GetEventByID(finalEventID); err == nil && eventInfo != nil {
				eventName = eventInfo.Name
				request.EventName = &eventName
				if bannerPath := c.resolveEventBannerPath(eventInfo.AssetBundleName, region); bannerPath != "" {
					request.EventBannerPath = &bannerPath
				}
			}
		}
	}
}

func shouldUseFallbackEventMetadata(recType string, option map[string]any) bool {
	switch recType {
	case "event", "bonus":
		return !optionHasSimulatedEvent(option)
	default:
		return false
	}
}

func queryHasWorldBloomMetadata(query AutoQuery) bool {
	return query.MetadataWorldBloomFinale ||
		(query.WorldBloomCharacterID != nil && *query.WorldBloomCharacterID > 0) ||
		(query.MetadataWorldBloomCharacterID != nil && *query.MetadataWorldBloomCharacterID > 0)
}

func optionHasSimulatedEvent(option map[string]any) bool {
	if option == nil {
		return false
	}
	return optionHasUnitAttrEvent(option) ||
		optionInt(option, "world_bloom_event_turn") > 0 ||
		optionInt(option, "world_bloom_finale_turn") > 0
}

func optionHasUnitAttrEvent(option map[string]any) bool {
	if option == nil {
		return false
	}
	return optionString(option, "event_unit") != "" && optionString(option, "event_attr") != ""
}

func optionHasWorldBloom(option map[string]any) bool {
	if option == nil {
		return false
	}
	return optionInt(option, "world_bloom_character_id") > 0 ||
		optionInt(option, "world_bloom_event_turn") > 0 ||
		optionInt(option, "world_bloom_finale_turn") > 0 ||
		optionInt(option, "event_id") == 180
}
