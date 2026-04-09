package event

import (
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (b *Builder) filterEvents(query ListQuery) []*masterdata.Event {
	events := b.source.GetEvents()
	now := time.Now()
	result := make([]*masterdata.Event, 0, len(events))
	includePast := query.IncludePast
	includeFuture := query.IncludeFuture
	if !query.OnlyFuture && !includePast && !includeFuture {
		includePast = true
		includeFuture = true
	}
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
