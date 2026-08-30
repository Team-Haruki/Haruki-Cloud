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
	filter := newEventListFilter(b, events, query, time.Now())
	result := make([]*masterdata.Event, 0, len(events))
	for _, eventInfo := range events {
		if filter.matches(eventInfo) {
			result = append(result, eventInfo)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartAt < result[j].StartAt
	})
	if query.Limit > 0 && len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result
}

type eventListFilter struct {
	builder               *Builder
	query                 ListQuery
	now                   time.Time
	includePast           bool
	includeFuture         bool
	worldBloomTurnByEvent map[int]int
}

func newEventListFilter(builder *Builder, events []*masterdata.Event, query ListQuery, now time.Time) eventListFilter {
	includePast, includeFuture := eventListTimeSelection(query)
	filter := eventListFilter{
		builder:       builder,
		query:         query,
		now:           now,
		includePast:   includePast,
		includeFuture: includeFuture,
	}
	if query.WorldBloomTurn > 0 {
		filter.worldBloomTurnByEvent = builder.buildWorldBloomTurnByEvent(events)
	}
	return filter
}

func eventListTimeSelection(query ListQuery) (bool, bool) {
	if query.OnlyFuture {
		return false, true
	}
	if !query.IncludePast && !query.IncludeFuture {
		return true, true
	}
	return query.IncludePast, query.IncludeFuture
}

func (f eventListFilter) matches(eventInfo *masterdata.Event) bool {
	start := time.UnixMilli(eventInfo.StartAt)
	end := time.UnixMilli(eventutil.EffectiveClosedAt(eventInfo.AggregateAt, eventInfo.ClosedAt))
	if !f.includePast && end.Before(f.now) {
		return false
	}
	if !f.includeFuture && start.After(f.now) {
		return false
	}
	if f.query.EventType != "" && !strings.EqualFold(eventInfo.EventType, f.query.EventType) {
		return false
	}
	if !f.matchesWorldBloomTurn(eventInfo) || (f.query.Year != 0 && start.Year() != f.query.Year) {
		return false
	}
	if !f.matchesBonus(eventInfo) || !f.matchesOnlyUnit(eventInfo) {
		return false
	}
	return f.matchesBanner(eventInfo)
}

func (f eventListFilter) matchesWorldBloomTurn(eventInfo *masterdata.Event) bool {
	if f.query.WorldBloomTurn <= 0 {
		return true
	}
	return strings.EqualFold(eventInfo.EventType, "world_bloom") && f.worldBloomTurnByEvent[eventInfo.ID] == f.query.WorldBloomTurn
}

func (f eventListFilter) matchesBonus(eventInfo *masterdata.Event) bool {
	bonusUnit := f.query.Unit
	if f.query.OnlyUnit {
		bonusUnit = ""
	}
	if bonusUnit == "" && !f.query.Blend && f.query.Attr == "" && f.query.CharacterID == 0 && len(f.query.CharacterIDs) == 0 {
		return true
	}
	return f.builder.matchEventBonus(eventInfo.EventType, eventInfo.ID, eventInfo.Unit, bonusUnit, f.query.Blend, f.query.Attr, f.query.CharacterID, f.query.CharacterIDs)
}

func (f eventListFilter) matchesOnlyUnit(eventInfo *masterdata.Event) bool {
	return !f.query.OnlyUnit || f.builder.eventCardsAllInUnit(eventInfo.ID, f.query.Unit)
}

func (f eventListFilter) matchesBanner(eventInfo *masterdata.Event) bool {
	if f.query.BannerCharID == nil {
		return true
	}
	bannerCharacterID, err := f.builder.source.GetEventBannerCharacterID(eventInfo.ID)
	return err == nil && bannerCharacterID == *f.query.BannerCharID && f.builder.isBoxEvent(eventInfo.ID)
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
		if attr == "" {
			attr = strings.ToLower(bonus.CardAttr)
		}
		b.addEventBonusCharacter(units, charSet, bonus.GameCharacterUnitID, bonus.GameCharacterID)
	}
	return attr, units, charSet
}

func (b *Builder) addEventBonusCharacter(units map[string]struct{}, characters map[int]struct{}, characterUnitID, characterID int) {
	if characterUnitID == 0 {
		if characterID != 0 {
			characters[characterID] = struct{}{}
		}
		return
	}
	unit, err := b.source.GetGameCharacterUnit(characterUnitID)
	if err != nil || unit == nil {
		return
	}
	if unitName := strings.ToLower(strings.TrimSpace(unit.Unit)); unitName != "" {
		units[unitName] = struct{}{}
	}
	characters[unit.GameCharacterID] = struct{}{}
}

func (b *Builder) isBoxEvent(eventID int) bool {
	_, units, _ := b.extractEventBonusMeta(eventID)
	return len(units) == 1
}

func (b *Builder) matchEventBonus(eventType string, eventID int, eventUnit string, unit string, blend bool, attr string, charID int, charIDs []int) bool {
	if unit == "" && !blend && attr == "" && charID == 0 && len(charIDs) == 0 {
		return true
	}
	if (unit != "" || blend || attr != "") && !b.matchEventBonusMeta(eventType, eventID, eventUnit, unit, blend, attr) {
		return false
	}
	if charID != 0 || len(charIDs) > 0 {
		return b.eventHasCardCharacters(eventID, charID, charIDs)
	}
	return true
}

func (b *Builder) matchEventBonusMeta(eventType string, eventID int, eventUnit, unit string, blend bool, attr string) bool {
	bonusAttr, units, characters := b.extractEventBonusMeta(eventID)
	if len(units) == 0 && len(characters) == 0 && bonusAttr == "" {
		return false
	}
	if attr != "" && !strings.EqualFold(bonusAttr, attr) {
		return false
	}
	return b.matchEventBonusUnit(eventType, eventID, eventUnit, unit, blend, units)
}

func (b *Builder) matchEventBonusUnit(eventType string, eventID int, eventUnit, unit string, blend bool, units map[string]struct{}) bool {
	if blend || strings.EqualFold(strings.TrimSpace(unit), "blend") {
		return len(units) > 1
	}
	if unit == "" {
		return true
	}
	normalizedUnit := strings.ToLower(strings.TrimSpace(unit))
	if !strings.EqualFold(eventType, "world_bloom") {
		_, matched := units[normalizedUnit]
		return matched
	}
	return b.matchWorldBloomBonusUnit(eventID, eventUnit, normalizedUnit, units)
}

func (b *Builder) matchWorldBloomBonusUnit(eventID int, eventUnit, normalizedUnit string, units map[string]struct{}) bool {
	if normalizedEventUnit := strings.ToLower(strings.TrimSpace(eventUnit)); normalizedEventUnit != "" {
		return normalizedUnit == normalizedEventUnit
	}
	chapterUnits, hasChapterData := b.extractWorldBloomChapterUnits(eventID)
	if hasChapterData {
		_, matched := chapterUnits[normalizedUnit]
		return matched
	}
	_, matched := units[normalizedUnit]
	return matched
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
		if chapter != nil {
			timeline = append(timeline, b.buildWorldBloomTimelineItem(chapter))
		}
	}
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i]["start_at"].(int64) < timeline[j]["start_at"].(int64)
	})
	return timeline
}

func (b *Builder) buildWorldBloomTimelineItem(chapter *masterdata.WorldBloom) map[string]any {
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
	b.populateWorldBloomTimelineCharacter(item, chapter.GameCharacterID)
	if chapter.ChapterType != "" {
		item["chapter_type"] = chapter.ChapterType
	}
	if chapter.IsSupplemental {
		item["is_supplemental"] = true
	}
	return item
}

func (b *Builder) populateWorldBloomTimelineCharacter(item map[string]any, characterIDValue *int) {
	if characterIDValue == nil || *characterIDValue == 0 {
		return
	}
	characterID := *characterIDValue
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

func resolveWorldBloomChapterEndAt(chapter *masterdata.WorldBloom) int64 {
	if chapter == nil {
		return 0
	}
	if chapter.AggregateAt > 0 {
		return chapter.AggregateAt + 1000
	}
	if chapter.ChapterEndAt > 0 {
		return chapter.ChapterEndAt
	}
	return 0
}
