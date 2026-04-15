package deck

import (
	"fmt"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) applyCommonRecommendMetadata(request *drawing.DeckRequest, region renderregion.Value, recType string, option map[string]any, query AutoQuery) {
	if request == nil {
		return
	}

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

	if optionHasUnitAttrEvent(option) && recType == "event" {
		request.RecommendType = "unit_attr"
	}
	if optionHasWorldBloom(option) {
		request.IsWl = true
		switch recType {
		case "event":
			request.RecommendType = "wl"
		case "bonus":
			request.RecommendType = "wl_bonus"
		}
		if cid := optionInt(option, "world_bloom_character_id"); cid > 0 {
			if icon := c.resolveCharacterIconPath(cid); icon != "" {
				request.WlCharaIconPath = &icon
				if request.CharaIconPath == nil {
					request.CharaIconPath = &icon
				}
			}
			if name := c.resolveCharacterName(region, cid); name != "" {
				request.WlCharaName = &name
				if request.CharaName == nil {
					request.CharaName = &name
				}
			}
		}
	}
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

func optionHasSimulatedEvent(option map[string]any) bool {
	if option == nil {
		return false
	}
	return optionHasUnitAttrEvent(option) || optionInt(option, "world_bloom_event_turn") > 0
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
	return optionInt(option, "world_bloom_character_id") > 0 || optionInt(option, "world_bloom_event_turn") > 0
}
