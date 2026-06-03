package event

import (
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/eventutil"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (b *Builder) filterEvents(query ListQuery) []*masterdata.Event {
	events := b.source.GetEvents()
	now := time.Now()
	result := make([]*masterdata.Event, 0, len(events))
	var worldBloomTurnByEvent map[int]int
	if query.WorldBloomTurn > 0 {
		worldBloomTurnByEvent = b.buildWorldBloomTurnByEvent(events)
	}
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
		end := time.UnixMilli(eventutil.EffectiveClosedAt(eventInfo.AggregateAt, eventInfo.ClosedAt))
		if !includePast && end.Before(now) {
			continue
		}
		if !includeFuture && start.After(now) {
			continue
		}
		if query.EventType != "" && !strings.EqualFold(eventInfo.EventType, query.EventType) {
			continue
		}
		if query.WorldBloomTurn > 0 && !strings.EqualFold(eventInfo.EventType, "world_bloom") {
			continue
		}
		if query.WorldBloomTurn > 0 && worldBloomTurnByEvent[eventInfo.ID] != query.WorldBloomTurn {
			continue
		}
		if query.Year != 0 && start.Year() != query.Year {
			continue
		}
		bonusUnit := query.Unit
		if query.OnlyUnit {
			bonusUnit = ""
		}
		if bonusUnit != "" || query.Blend || query.Attr != "" || query.CharacterID != 0 || len(query.CharacterIDs) > 0 {
			if !b.matchEventBonus(eventInfo.EventType, eventInfo.ID, eventInfo.Unit, bonusUnit, query.Blend, query.Attr, query.CharacterID, query.CharacterIDs) {
				continue
			}
		}
		if query.OnlyUnit {
			if !b.eventCardsAllInUnit(eventInfo.ID, query.Unit) {
				continue
			}
		}
		if query.BannerCharID != nil {
			bannerCID, err := b.source.GetEventBannerCharacterID(eventInfo.ID)
			if err != nil || bannerCID != *query.BannerCharID || !b.isBoxEvent(eventInfo.ID) {
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

func (b *Builder) buildWorldBloomTurnByEvent(events []*masterdata.Event) map[int]int {
	worldBloomEvents := make([]*masterdata.Event, 0)
	for _, eventInfo := range events {
		if eventInfo == nil || !strings.EqualFold(eventInfo.EventType, "world_bloom") {
			continue
		}
		if b.isWorldBloomFinaleEvent(eventInfo.ID) {
			continue
		}
		worldBloomEvents = append(worldBloomEvents, eventInfo)
	}
	sort.Slice(worldBloomEvents, func(i, j int) bool {
		return worldBloomEvents[i].StartAt < worldBloomEvents[j].StartAt
	})

	turnByEvent := make(map[int]int, len(worldBloomEvents))
	turnByUnit := make(map[string]int)
	for _, eventInfo := range worldBloomEvents {
		key := strings.ToLower(strings.TrimSpace(eventInfo.Unit))
		if key == "" {
			key = "unknown"
		}
		turnByUnit[key]++
		turnByEvent[eventInfo.ID] = turnByUnit[key]
	}
	return turnByEvent
}

func (b *Builder) isWorldBloomFinaleEvent(eventID int) bool {
	for _, chapter := range b.source.GetWorldBloomChapters(eventID) {
		if chapter != nil && strings.EqualFold(strings.TrimSpace(chapter.ChapterType), "finale") {
			return true
		}
	}
	return false
}

func (b *Builder) extractEventBonuses(eventID int) (string, []int) {
	attr, _, charSet := b.extractEventBonusMeta(eventID)
	if len(charSet) == 0 {
		return attr, nil
	}

	chars := make([]int, 0, len(charSet))
	for cid := range charSet {
		chars = append(chars, cid)
	}
	sort.Ints(chars)
	return attr, chars
}

func (b *Builder) extractEventBonusMeta(eventID int) (string, map[string]struct{}, map[int]struct{}) {
	bonuses, err := b.source.GetEventDeckBonuses(eventID)
	if err != nil {
		return "", nil, nil
	}

	attr := ""
	units := make(map[string]struct{})
	charSet := make(map[int]struct{})
	for _, bonus := range bonuses {
		if attr == "" && bonus.CardAttr != "" {
			attr = strings.ToLower(bonus.CardAttr)
		}
		if bonus.GameCharacterUnitID != 0 {
			if unit, err := b.source.GetGameCharacterUnit(bonus.GameCharacterUnitID); err == nil && unit != nil {
				unitName := strings.ToLower(strings.TrimSpace(unit.Unit))
				if unitName != "" {
					units[unitName] = struct{}{}
				}
				charSet[unit.GameCharacterID] = struct{}{}
			}
		} else if bonus.GameCharacterID != 0 {
			charSet[bonus.GameCharacterID] = struct{}{}
		}
	}
	return attr, units, charSet
}

func (b *Builder) isBoxEvent(eventID int) bool {
	_, units, _ := b.extractEventBonusMeta(eventID)
	return len(units) == 1
}

func (b *Builder) matchEventBonus(eventType string, eventID int, eventUnit string, unit string, blend bool, attr string, charID int, charIDs []int) bool {
	if unit == "" && !blend && attr == "" && charID == 0 && len(charIDs) == 0 {
		return true
	}

	if unit != "" || blend || attr != "" {
		bonusAttr, units, charSet := b.extractEventBonusMeta(eventID)
		if len(units) == 0 && len(charSet) == 0 && bonusAttr == "" {
			return false
		}
		attrMatched := attr == "" || strings.EqualFold(bonusAttr, attr)

		unitMatched := unit == ""
		if unit != "" {
			normalizedUnit := strings.ToLower(strings.TrimSpace(unit))
			if strings.EqualFold(eventType, "world_bloom") {
				if normalizedEventUnit := strings.ToLower(strings.TrimSpace(eventUnit)); normalizedEventUnit != "" {
					unitMatched = normalizedUnit == normalizedEventUnit
				} else if chapterUnits, hasChapterData := b.extractWorldBloomChapterUnits(eventID); hasChapterData {
					_, unitMatched = chapterUnits[normalizedUnit]
				} else {
					_, unitMatched = units[normalizedUnit]
				}
			} else {
				_, unitMatched = units[normalizedUnit]
			}
		}
		if blend || strings.EqualFold(strings.TrimSpace(unit), "blend") {
			unitMatched = len(units) > 1
		}
		if !attrMatched || !unitMatched {
			return false
		}
	}

	if charID != 0 || len(charIDs) > 0 {
		return b.eventHasCardCharacters(eventID, charID, charIDs)
	}

	return true
}

func (b *Builder) eventHasCardCharacters(eventID int, charID int, charIDs []int) bool {
	cards, err := b.source.GetEventCards(eventID)
	if err != nil || len(cards) == 0 {
		return false
	}

	cardChars := make(map[int]struct{}, len(cards))
	for _, cardInfo := range cards {
		if cardInfo == nil || cardInfo.CharacterID == 0 {
			continue
		}
		cardChars[cardInfo.CharacterID] = struct{}{}
	}
	if charID != 0 {
		if _, ok := cardChars[charID]; !ok {
			return false
		}
	}
	for _, cid := range charIDs {
		if cid == 0 {
			continue
		}
		if _, ok := cardChars[cid]; !ok {
			return false
		}
	}
	return true
}

func (b *Builder) eventCardsAllInUnit(eventID int, unit string) bool {
	targetUnit := normalizeEventUnit(unit)
	if targetUnit == "" {
		return false
	}

	cards, err := b.source.GetEventCards(eventID)
	if err != nil || len(cards) == 0 {
		return false
	}

	for _, cardInfo := range cards {
		cardUnit, ok := b.eventCardUnit(cardInfo)
		if !ok || cardUnit != targetUnit {
			return false
		}
	}
	return true
}

func (b *Builder) eventCardUnit(cardInfo *masterdata.Card) (string, bool) {
	if cardInfo == nil || cardInfo.CharacterID == 0 {
		return "", false
	}

	character, err := b.source.GetCharacterByID(cardInfo.CharacterID)
	if err != nil || character == nil {
		return "", false
	}
	mainUnit := normalizeEventUnit(character.Unit)
	if mainUnit == "piapro" {
		supportUnit := normalizeEventUnit(cardInfo.SupportUnit)
		if supportUnit != "" && supportUnit != "none" {
			return supportUnit, true
		}
		return "piapro", true
	}
	if mainUnit != "" {
		return mainUnit, true
	}
	supportUnit := normalizeEventUnit(cardInfo.SupportUnit)
	if supportUnit != "" && supportUnit != "none" {
		return supportUnit, true
	}
	return "", false
}

func normalizeEventUnit(unit string) string {
	return strings.ToLower(strings.TrimSpace(unit))
}

func (b *Builder) extractWorldBloomChapterUnits(eventID int) (map[string]struct{}, bool) {
	chapters := b.source.GetWorldBloomChapters(eventID)
	if len(chapters) == 0 {
		return nil, false
	}

	units := make(map[string]struct{})
	for _, chapter := range chapters {
		if chapter == nil || chapter.GameCharacterID == nil || *chapter.GameCharacterID == 0 {
			continue
		}
		character, err := b.source.GetCharacterByID(*chapter.GameCharacterID)
		if err != nil || character == nil {
			continue
		}
		unit := strings.ToLower(strings.TrimSpace(character.Unit))
		if unit == "" {
			continue
		}
		units[unit] = struct{}{}
	}
	return units, true
}

func (b *Builder) getBannerIndex(charID, eventID int) *int {
	events := b.source.GetBanEvents(charID)
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartAt < events[j].StartAt
	})
	for idx, eventInfo := range events {
		if eventInfo.ID == eventID {
			return new(idx + 1)
		}
	}
	return nil
}

func (b *Builder) buildWorldBloomTimeline(eventID int) []map[string]any {
	chapters := b.source.GetWorldBloomChapters(eventID)
	if len(chapters) == 0 {
		return nil
	}

	timeline := make([]map[string]any, 0, len(chapters))
	for _, chapter := range chapters {
		if chapter == nil {
			continue
		}
		chapterEndAt := resolveWorldBloomChapterEndAt(chapter)
		item := map[string]any{
			"start_at":             chapter.ChapterStartAt,
			"aggregate_at":         chapter.AggregateAt,
			"end_at":               chapterEndAt,
			"chapter_start_at":     chapter.ChapterStartAt,
			"chapter_aggregate_at": chapter.AggregateAt,
			"chapter_end_at":       chapterEndAt,
		}
		if chapter.ChapterNo != 0 {
			item["chapter_id"] = chapter.ChapterNo
			item["chapter_no"] = chapter.ChapterNo
		}
		if chapter.GameCharacterID != nil && *chapter.GameCharacterID != 0 {
			characterID := *chapter.GameCharacterID
			item["game_character_id"] = characterID
			item["character_icon_path"] = b.characterIconPath(characterID, b.source.DefaultRegion())
			if characterName := b.characterDisplayName(characterID); characterName != "" {
				item["character_name"] = characterName
			}
			if colorCode, ok := b.source.GetCharacterColorCode(characterID); ok {
				item["color_code"] = colorCode
				item["character_color_code"] = colorCode
			}
		}
		if chapter.ChapterType != "" {
			item["chapter_type"] = chapter.ChapterType
		}
		if chapter.IsSupplemental {
			item["is_supplemental"] = true
		}
		timeline = append(timeline, item)
	}
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i]["start_at"].(int64) < timeline[j]["start_at"].(int64)
	})
	return timeline
}

func resolveWorldBloomChapterEndAt(chapter *masterdata.WorldBloom) int64 {
	if chapter == nil {
		return 0
	}
	if chapter.ChapterEndAt > 0 {
		return chapter.ChapterEndAt
	}
	if chapter.AggregateAt > 0 {
		return chapter.AggregateAt + 1000
	}
	return 0
}
