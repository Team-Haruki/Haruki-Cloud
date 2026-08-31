package handler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	eventdb "haruki-cloud/database/sekai/event"
	"haruki-cloud/internal/pjsk/eventutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderprovider "haruki-cloud/internal/pjsk/render/provider"

	"haruki-cloud/internal/pjsk/drawing"
)

func resolveDeckCharacterSelections(ctx context.Context, q *deck.AutoQuery, app *renderapp.App) error {
	if q == nil {
		return nil
	}

	region := renderregion.WithDefault(renderregion.Normalize(q.Region))

	if err := resolveDeckEventAndWorldBloomSelection(ctx, q, app, region); err != nil {
		return err
	}
	if err := resolveDeckForcedLeaderSelection(ctx, q, app, region); err != nil {
		return err
	}
	if err := resolveDeckWorldBloomCharacterQuery(ctx, q, app, region); err != nil {
		return err
	}
	if err := resolveDeckChallengeCharacterQuery(ctx, q, app, region); err != nil {
		return err
	}
	if err := resolveDeckFixedCharacterQueries(ctx, q, app, region); err != nil {
		return err
	}
	if err := validateDeckCharacterIDs(q.FixedCharacters); err != nil {
		return err
	}
	return nil
}

func resolveDeckForcedLeaderSelection(ctx context.Context, q *deck.AutoQuery, app *renderapp.App, region renderregion.Value) error {
	if !isResolvedDeckWorldBloomFinale(q) {
		q.ForcedLeaderCharacterID = nil
		q.ForcedLeaderCharacterQuery = ""
		return nil
	}
	if q.ForcedLeaderCharacterID != nil || strings.TrimSpace(q.ForcedLeaderCharacterQuery) == "" {
		return nil
	}
	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, q.ForcedLeaderCharacterQuery, "deck")
	if err != nil {
		return err
	}
	q.ForcedLeaderCharacterID = drawing.IntPtr(charID)
	q.ForcedLeaderCharacterQuery = ""
	return nil
}

func resolveDeckWorldBloomCharacterQuery(ctx context.Context, q *deck.AutoQuery, app *renderapp.App, region renderregion.Value) error {
	if q.WorldBloomCharacterID != nil || strings.TrimSpace(q.WorldBloomCharacterQuery) == "" {
		return nil
	}
	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, q.WorldBloomCharacterQuery, "deck")
	if err != nil {
		return err
	}
	q.WorldBloomCharacterID = drawing.IntPtr(charID)
	q.WorldBloomCharacterQuery = ""
	if strings.TrimSpace(q.EventUnit) == "" {
		q.EventUnit = resolveDeckCharacterUnit(charID)
	}
	return nil
}

func resolveDeckChallengeCharacterQuery(ctx context.Context, q *deck.AutoQuery, app *renderapp.App, region renderregion.Value) error {
	if q.ChallengeLiveCharacterID != nil || strings.TrimSpace(q.ChallengeLiveCharacterQuery) == "" {
		return nil
	}
	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, q.ChallengeLiveCharacterQuery, "deck")
	if err == nil {
		q.ChallengeLiveCharacterID = drawing.IntPtr(charID)
		q.ChallengeLiveCharacterQuery = ""
		return nil
	}
	if strings.TrimSpace(q.MusicQuery) == "" && isCharacterNotFoundError(err) {
		assignDeckFallbackMusicQuery(q, q.ChallengeLiveCharacterQuery)
		q.ChallengeLiveCharacterQuery = ""
		return nil
	}
	return err
}

func resolveDeckFixedCharacterQueries(ctx context.Context, q *deck.AutoQuery, app *renderapp.App, region renderregion.Value) error {
	for _, raw := range q.FixedCharacterQueries {
		charID, err := resolveGameCharacterIDByQuery(ctx, app, region, raw, "deck")
		if err != nil {
			return err
		}
		q.FixedCharacters = append(q.FixedCharacters, charID)
	}
	q.FixedCharacterQueries = nil
	return nil
}

func prepareDeckWorldBloomFinaleSimulation(q *deck.AutoQuery, turn int) error {
	if q == nil {
		return nil
	}
	if q.ForcedLeaderCharacterID == nil && strings.TrimSpace(q.ForcedLeaderCharacterQuery) == "" {
		return fmt.Errorf("wl%d 终章需要指定队长角色，例如 wl%d 终章 miku", turn, turn)
	}

	q.EventID = nil
	q.WorldBloomFinaleTurn = drawing.IntPtr(turn)
	q.WorldBloomEventTurn = nil
	q.MetadataWorldBloomFinale = true
	q.WorldBloomCharacterID = nil
	q.WorldBloomCharacterQuery = ""
	q.EventUnit = ""
	q.EventAttr = ""
	return nil
}

func resolveDeckEventAndWorldBloomSelection(ctx context.Context, q *deck.AutoQuery, app *renderapp.App, region renderregion.Value) error {
	resolver := deckEventSelectionResolver{ctx: ctx, query: q, app: app, region: region}
	return resolver.resolve()
}

type deckEventSelectionResolver struct {
	ctx       context.Context
	query     *deck.AutoQuery
	app       *renderapp.App
	region    renderregion.Value
	eventID   int
	eventInfo *sekaidb.Event
	chapters  []*sekaidb.Worldbloom
}

func (r *deckEventSelectionResolver) resolve() error {
	if r.query == nil {
		return nil
	}
	if stop, err := r.resolveFinaleSelection(); stop || err != nil {
		return err
	}
	if r.app == nil {
		return nil
	}
	if stop, err := r.resolveTurnSelection(); stop || err != nil {
		return err
	}
	if r.hasStandaloneSimulationSelection() {
		return nil
	}
	if stop, err := r.ensureEventSelection(); stop || err != nil {
		return err
	}
	if stop, err := r.loadEvent(); stop || err != nil {
		return err
	}
	if !strings.EqualFold(r.eventInfo.EventType, "world_bloom") {
		return r.resolveNonWorldBloom()
	}
	return r.resolveWorldBloom()
}

func (r *deckEventSelectionResolver) resolveFinaleSelection() (bool, error) {
	q := r.query
	if q.WorldBloomFinaleTurn != nil && *q.WorldBloomFinaleTurn > 0 {
		turn := *q.WorldBloomFinaleTurn
		eventInfo, err := resolveDeckWorldBloomFinaleEventByTurn(r.ctx, r.app, r.region, turn)
		if err != nil {
			var futureTurnErr *deckFutureWorldBloomFinaleTurnError
			if errors.As(err, &futureTurnErr) && shouldKeepDeckWorldBloomSimulationSelection(q) {
				return true, prepareDeckWorldBloomFinaleSimulation(q, turn)
			}
			return true, err
		}
		q.EventID = drawing.IntPtr(int(eventInfo.GameID))
		q.WorldBloomFinaleTurn = nil
		q.MetadataWorldBloomFinale = true
	}
	if !isResolvedDeckWorldBloomFinale(q) {
		return false, nil
	}
	normalizeDeckWorldBloomFinaleSelection(q)
	return true, nil
}

func (r *deckEventSelectionResolver) resolveTurnSelection() (bool, error) {
	q := r.query
	if q.WorldBloomEventTurn == nil || *q.WorldBloomEventTurn <= 0 {
		return false, nil
	}
	turn := *q.WorldBloomEventTurn
	if err := resolveDeckWorldBloomTurnCharacterSelection(r.ctx, q, r.app, r.region); err != nil {
		return true, err
	}
	eventInfo, err := resolveDeckWorldBloomEventByTurnSelection(r.ctx, r.app, r.region, q)
	if err != nil {
		var futureTurnErr *deckFutureWorldBloomTurnError
		if errors.As(err, &futureTurnErr) && shouldKeepDeckWorldBloomSimulationSelection(q) {
			return true, nil
		}
		return true, err
	}
	q.EventID = drawing.IntPtr(int(eventInfo.GameID))
	q.MetadataWorldBloomEventTurn = drawing.IntPtr(turn)
	q.WorldBloomEventTurn = nil
	return false, nil
}

func (r *deckEventSelectionResolver) hasStandaloneSimulationSelection() bool {
	q := r.query
	return !hasPendingDeckWorldBloomSelection(q) && (strings.TrimSpace(q.EventUnit) != "" || strings.TrimSpace(q.EventAttr) != "")
}

func (r *deckEventSelectionResolver) ensureEventSelection() (bool, error) {
	q := r.query
	if q.EventID != nil && *q.EventID > 0 {
		return false, nil
	}
	if !shouldResolveDeckEventByRecommendType(q.RecommendType) {
		return true, nil
	}
	eventInfo, err := pickDeckAutoEvent(r.ctx, r.app, r.region, q.RecommendType)
	if err == nil && eventInfo != nil {
		q.EventID = drawing.IntPtr(int(eventInfo.GameID))
		return false, nil
	}
	if shouldFallbackDeckEventRecommendToNoEvent(q) {
		clearDeckAutoEventSelection(q)
		q.RecommendType = "no_event"
	}
	return true, nil
}

func (r *deckEventSelectionResolver) loadEvent() (bool, error) {
	r.eventID = *r.query.EventID
	eventInfo, err := queryDeckEventByID(r.ctx, r.app, r.region, r.eventID)
	if err != nil {
		if sekaidb.IsNotFound(err) {
			return true, nil
		}
		return true, fmt.Errorf("query deck event %d failed: %w", r.eventID, err)
	}
	if err := ensureDeckEventUnlocked(r.ctx, r.app, r.region, eventInfo); err != nil {
		return true, err
	}
	r.eventInfo = eventInfo
	return false, nil
}

func (r *deckEventSelectionResolver) resolveNonWorldBloom() error {
	q := r.query
	if q.WorldBloomCharacterQuery != "" && isDeckWorldBloomSelectorQuery(q.WorldBloomCharacterQuery) {
		return fmt.Errorf("活动 %s-%d 不是WL活动，无法指定章节", strings.ToUpper(r.region.String()), r.eventID)
	}
	q.WorldBloomCharacterID = nil
	return nil
}

func (r *deckEventSelectionResolver) resolveWorldBloom() error {
	chapters, err := queryDeckWorldBloomChapters(r.ctx, r.app, r.region, r.eventID)
	if err != nil {
		return err
	}
	r.chapters = chapters
	if deckWorldBloomHasFinaleChapter(chapters) {
		r.query.MetadataWorldBloomFinale = true
		normalizeDeckWorldBloomFinaleSelection(r.query)
		return nil
	}
	if err := r.tryMusicCharacterSelections(); err != nil {
		return err
	}
	if r.query.WorldBloomCharacterID != nil && *r.query.WorldBloomCharacterID > 0 {
		return r.resolveCharacterID()
	}
	if resolved, err := r.resolveCharacterQuery(); resolved || err != nil {
		return err
	}
	return r.selectDefaultChapter()
}

func (r *deckEventSelectionResolver) tryMusicCharacterSelections() error {
	q := r.query
	if hasNoDeckWorldBloomCharacter(q) {
		if err := tryResolveDeckMusicQueryAsWorldBloomCharacter(r.ctx, q, r.app, r.region, r.chapters); err != nil {
			return err
		}
	}
	if hasNoDeckWorldBloomCharacter(q) {
		if err := tryResolveDeckMusicCompareQueryAsWorldBloomCharacter(r.ctx, q, r.app, r.region, r.chapters); err != nil {
			return err
		}
	}
	if hasNoDeckWorldBloomCharacter(q) {
		return tryResolveDeckMusicQueryPrefixAsWorldBloomCharacter(r.ctx, q, r.app, r.region, r.chapters)
	}
	return nil
}

func hasNoDeckWorldBloomCharacter(q *deck.AutoQuery) bool {
	return q.WorldBloomCharacterID == nil && strings.TrimSpace(q.WorldBloomCharacterQuery) == ""
}

func (r *deckEventSelectionResolver) resolveCharacterID() error {
	charID := *r.query.WorldBloomCharacterID
	if !trackerWorldBloomHasCharacter(r.chapters, charID) {
		return fmt.Errorf("活动 %s-%d 没有角色 %d 的 World Link 章节", strings.ToUpper(r.region.String()), r.eventID, charID)
	}
	ensureDeckWorldBloomEventTurnMetadata(r.ctx, r.app, r.region, r.eventInfo, r.chapters, r.query)
	return nil
}

func (r *deckEventSelectionResolver) resolveCharacterQuery() (bool, error) {
	q := r.query
	query := strings.TrimSpace(q.WorldBloomCharacterQuery)
	if query == "" {
		return false, nil
	}
	chapter, err := resolveTrackerWorldBloomChapterSelection(r.ctx, r.app, r.region, r.eventInfo, r.chapters, query)
	if err != nil {
		if !isCharacterNotFoundError(err) {
			return true, err
		}
		if strings.TrimSpace(q.MusicQuery) == "" {
			assignDeckFallbackMusicQuery(q, query)
		}
		q.WorldBloomCharacterQuery = ""
		return false, nil
	}
	return true, r.applyChapterCharacter(chapter)
}

func (r *deckEventSelectionResolver) selectDefaultChapter() error {
	chapter := pickDeckDefaultWorldBloomChapter(r.eventInfo, r.chapters)
	if chapter == nil {
		return missingDeckWorldBloomChapterError(r.query, r.eventID)
	}
	return r.applyChapterCharacter(chapter)
}

func (r *deckEventSelectionResolver) applyChapterCharacter(chapter *sekaidb.Worldbloom) error {
	if chapter.GameCharacterID <= 0 {
		return fmt.Errorf("活动 %s-%d 的 World Link 章节缺少角色信息", strings.ToUpper(r.region.String()), r.eventID)
	}
	charID := int(chapter.GameCharacterID)
	r.query.WorldBloomCharacterID = drawing.IntPtr(charID)
	r.query.WorldBloomCharacterQuery = ""
	if strings.TrimSpace(r.query.EventUnit) == "" {
		r.query.EventUnit = resolveDeckCharacterUnit(charID)
	}
	ensureDeckWorldBloomEventTurnMetadata(r.ctx, r.app, r.region, r.eventInfo, r.chapters, r.query)
	return nil
}

func isResolvedDeckWorldBloomFinale(q *deck.AutoQuery) bool {
	return q != nil && (q.MetadataWorldBloomFinale || (q.EventID != nil && *q.EventID == 180))
}

func normalizeDeckWorldBloomFinaleSelection(q *deck.AutoQuery) {
	if q == nil {
		return
	}
	q.MetadataWorldBloomFinale = true
	if q.ForcedLeaderCharacterID == nil && q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0 {
		q.ForcedLeaderCharacterID = drawing.IntPtr(*q.WorldBloomCharacterID)
	}
	if strings.TrimSpace(q.ForcedLeaderCharacterQuery) == "" && q.WorldBloomCharacterQuery != "" && !isDeckWorldBloomSelectorQuery(q.WorldBloomCharacterQuery) {
		q.ForcedLeaderCharacterQuery = q.WorldBloomCharacterQuery
	}
	q.WorldBloomCharacterID = nil
	q.WorldBloomCharacterQuery = ""
	q.WorldBloomEventTurn = nil
	q.WorldBloomFinaleTurn = nil
	q.EventUnit = ""
}

func ensureDeckWorldBloomEventTurnMetadata(ctx context.Context, app *renderapp.App, region renderregion.Value, eventInfo *sekaidb.Event, chapters []*sekaidb.Worldbloom, q *deck.AutoQuery) {
	if q == nil || q.MetadataWorldBloomEventTurn != nil {
		return
	}
	if turn := resolveDeckWorldBloomEventTurn(ctx, app, region, eventInfo, chapters, q); turn > 0 {
		q.MetadataWorldBloomEventTurn = drawing.IntPtr(turn)
	}
}

func resolveDeckWorldBloomEventTurn(ctx context.Context, app *renderapp.App, region renderregion.Value, eventInfo *sekaidb.Event, chapters []*sekaidb.Worldbloom, q *deck.AutoQuery) int {
	if eventInfo == nil || q == nil {
		return 0
	}
	eventID := int(eventInfo.GameID)
	charID := 0
	if q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0 {
		charID = *q.WorldBloomCharacterID
	} else if chapter := pickDeckDefaultWorldBloomChapter(eventInfo, chapters); chapter != nil && chapter.GameCharacterID > 0 {
		charID = int(chapter.GameCharacterID)
	}
	if charID > 0 {
		if turn, err := resolveDeckWorldBloomCharacterTurnForEvent(ctx, app, region, eventID, charID); err == nil && turn > 0 {
			return turn
		}
	}

	unit := strings.TrimSpace(q.EventUnit)
	if unit == "" {
		unit = normalizeDeckUnit(eventInfo.Unit)
	}
	if unit != "" {
		if turn, err := resolveDeckWorldBloomUnitTurnForEvent(ctx, app, region, eventID, unit); err == nil && turn > 0 {
			return turn
		}
	}
	return 0
}

func resolveDeckWorldBloomCharacterTurnForEvent(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID, charID int) (int, error) {
	if eventID <= 0 || charID <= 0 {
		return 0, nil
	}
	worldBloomEvents, err := queryDeckWorldBloomEvents(ctx, app, region)
	if err != nil {
		return 0, err
	}
	turn := 0
	for _, item := range worldBloomEvents {
		if item == nil {
			continue
		}
		chapters, err := queryDeckWorldBloomChapters(ctx, app, region, int(item.GameID))
		if err != nil {
			return 0, err
		}
		if !trackerWorldBloomHasCharacter(chapters, charID) {
			continue
		}
		turn++
		if int(item.GameID) == eventID {
			return turn, nil
		}
	}
	return 0, nil
}

func resolveDeckWorldBloomUnitTurnForEvent(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int, unit string) (int, error) {
	if eventID <= 0 {
		return 0, nil
	}
	unit = normalizeDeckUnit(unit)
	if unit == "" {
		return 0, nil
	}
	worldBloomEvents, err := queryDeckWorldBloomEvents(ctx, app, region)
	if err != nil {
		return 0, err
	}
	turn := 0
	for _, item := range worldBloomEvents {
		if item == nil {
			continue
		}
		eventUnit := normalizeDeckUnit(item.Unit)
		hasUnit := eventUnit == unit
		if !hasUnit {
			chapters, err := queryDeckWorldBloomChapters(ctx, app, region, int(item.GameID))
			if err != nil {
				return 0, err
			}
			hasUnit = deckWorldBloomHasUnit(chapters, unit)
		}
		if !hasUnit {
			continue
		}
		turn++
		if int(item.GameID) == eventID {
			return turn, nil
		}
	}
	return 0, nil
}

func resolveDeckWorldBloomTurnCharacterSelection(ctx context.Context, q *deck.AutoQuery, app *renderapp.App, region renderregion.Value) error {
	if q == nil || app == nil {
		return nil
	}
	if q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0 {
		return nil
	}

	query := strings.TrimSpace(q.WorldBloomCharacterQuery)
	if query == "" || isDeckWorldBloomSelectorQuery(query) {
		return nil
	}

	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, query, "deck")
	if err != nil {
		return err
	}

	q.WorldBloomCharacterID = drawing.IntPtr(charID)
	q.WorldBloomCharacterQuery = ""
	if strings.TrimSpace(q.EventUnit) == "" {
		q.EventUnit = resolveDeckCharacterUnit(charID)
	}
	return nil
}

func hasPendingDeckWorldBloomSelection(q *deck.AutoQuery) bool {
	if q == nil {
		return false
	}
	return (q.WorldBloomEventTurn != nil && *q.WorldBloomEventTurn > 0) ||
		(q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0) ||
		strings.TrimSpace(q.WorldBloomCharacterQuery) != ""
}

func tryResolveDeckMusicQueryAsWorldBloomCharacter(
	ctx context.Context,
	q *deck.AutoQuery,
	app *renderapp.App,
	region renderregion.Value,
	chapters []*sekaidb.Worldbloom,
) error {
	if q == nil || app == nil {
		return nil
	}

	query := strings.TrimSpace(q.MusicQuery)
	if query == "" {
		return nil
	}

	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, query, "deck")
	if err != nil {
		if isCharacterNotFoundError(err) {
			return nil
		}
		return err
	}
	if !trackerWorldBloomHasCharacter(chapters, charID) {
		return nil
	}

	q.WorldBloomCharacterID = drawing.IntPtr(charID)
	q.MusicQuery = ""
	if strings.TrimSpace(q.EventUnit) == "" {
		q.EventUnit = resolveDeckCharacterUnit(charID)
	}
	return nil
}

func tryResolveDeckMusicQueryPrefixAsWorldBloomCharacter(
	ctx context.Context,
	q *deck.AutoQuery,
	app *renderapp.App,
	region renderregion.Value,
	chapters []*sekaidb.Worldbloom,
) error {
	if q == nil || app == nil {
		return nil
	}

	query := strings.TrimSpace(q.MusicQuery)
	fields := strings.Fields(query)
	if len(fields) < 2 {
		return nil
	}

	for split := len(fields) - 1; split >= 1; split-- {
		charID, musicQuery, matched, err := resolveDeckMusicPrefixCandidate(ctx, app, region, chapters, fields, split)
		if err != nil {
			return err
		}
		if !matched {
			continue
		}
		q.WorldBloomCharacterID = drawing.IntPtr(charID)
		q.MusicQuery = musicQuery
		if strings.TrimSpace(q.EventUnit) == "" {
			q.EventUnit = resolveDeckCharacterUnit(charID)
		}
		return nil
	}

	return nil
}

func resolveDeckMusicPrefixCandidate(ctx context.Context, app *renderapp.App, region renderregion.Value, chapters []*sekaidb.Worldbloom, fields []string, split int) (int, string, bool, error) {
	charQuery := strings.TrimSpace(strings.Join(fields[:split], " "))
	musicQuery := strings.TrimSpace(strings.Join(fields[split:], " "))
	if charQuery == "" || musicQuery == "" {
		return 0, "", false, nil
	}
	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, charQuery, "deck")
	if err != nil {
		if isCharacterNotFoundError(err) {
			return 0, "", false, nil
		}
		return 0, "", false, err
	}
	if !trackerWorldBloomHasCharacter(chapters, charID) {
		return 0, "", false, nil
	}
	return charID, musicQuery, true, nil
}

func tryResolveDeckMusicCompareQueryAsWorldBloomCharacter(
	ctx context.Context,
	q *deck.AutoQuery,
	app *renderapp.App,
	region renderregion.Value,
	chapters []*sekaidb.Worldbloom,
) error {
	if q == nil || !q.MusicCompare || len(q.MusicCompareQueries) == 0 {
		return nil
	}

	queries := make([]string, 0, len(q.MusicCompareQueries))
	for _, raw := range q.MusicCompareQueries {
		if query := strings.TrimSpace(raw); query != "" {
			queries = append(queries, query)
		}
	}
	if len(queries) == 0 {
		return nil
	}

	for split := len(queries); split >= 1; split-- {
		charQuery := strings.TrimSpace(strings.Join(queries[:split], " "))
		charID, err := resolveDeckWorldBloomCharacterCandidateID(ctx, app, region, charQuery)
		if err != nil {
			return err
		}
		if charID <= 0 || !trackerWorldBloomHasCharacter(chapters, charID) {
			continue
		}

		q.WorldBloomCharacterID = drawing.IntPtr(charID)
		q.MusicCompareQueries = append([]string(nil), queries[split:]...)
		if strings.TrimSpace(q.EventUnit) == "" {
			q.EventUnit = resolveDeckCharacterUnit(charID)
		}
		return nil
	}

	return nil
}

func resolveDeckWorldBloomCharacterCandidateID(ctx context.Context, app *renderapp.App, region renderregion.Value, raw string) (int, error) {
	if charID, _ := resolveDeckCharacterToken(raw); charID > 0 {
		return charID, nil
	}
	if app == nil || app.Sekai == nil {
		return 0, nil
	}

	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, raw, "deck")
	if err != nil {
		if isCharacterNotFoundError(err) {
			return 0, nil
		}
		return 0, err
	}
	return charID, nil
}

func shouldResolveDeckEventByRecommendType(recommendType string) bool {
	switch strings.ToLower(strings.TrimSpace(recommendType)) {
	case "event", "bonus", "mysekai":
		return true
	default:
		return false
	}
}

func pickCurrentOrNextDeckEvent(ctx context.Context, app *renderapp.App, region renderregion.Value) (*sekaidb.Event, error) {
	return pickDeckAutoEvent(ctx, app, region, "")
}

func pickDeckAutoEvent(ctx context.Context, app *renderapp.App, region renderregion.Value, recommendType string) (*sekaidb.Event, error) {
	if app == nil {
		return nil, fmt.Errorf("deck event resolve requires sekai client")
	}

	events, err := queryDeckEvents(ctx, app, region)
	if err != nil {
		return nil, fmt.Errorf("query deck events failed: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("当前没有可用活动")
	}

	dbEventIDs, err := queryDeckDBEventIDs(ctx, app, region)
	if err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	isJPEventFallback := region == renderregion.JP && strings.EqualFold(strings.TrimSpace(recommendType), "event")
	candidates := deckAutoEventCandidates{ctx: ctx, app: app, region: region, dbEventIDs: dbEventIDs, now: now}
	for _, eventInfo := range events {
		candidates.observe(eventInfo)
	}
	return candidates.selectEvent(isJPEventFallback)
}

type deckAutoEventCandidates struct {
	ctx         context.Context
	app         *renderapp.App
	region      renderregion.Value
	dbEventIDs  map[int]struct{}
	now         int64
	active      *sekaidb.Event
	current     *sekaidb.Event
	next        *sekaidb.Event
	blockedNext *sekaidb.Event
}

func (c *deckAutoEventCandidates) observe(eventInfo *sekaidb.Event) {
	if eventInfo == nil {
		return
	}
	if eventutil.IsCurrent(eventInfo.StartAt, eventInfo.AggregateAt, eventInfo.ClosedAt, c.now) {
		c.active = laterDeckEvent(c.active, eventInfo)
	}
	if eventutil.IsRankingOpen(eventInfo.StartAt, eventInfo.AggregateAt, c.now) {
		c.current = laterDeckEvent(c.current, eventInfo)
		return
	}
	if eventInfo.StartAt > c.now {
		c.observeFuture(eventInfo)
	}
}

func (c *deckAutoEventCandidates) observeFuture(eventInfo *sekaidb.Event) {
	if isDeckFutureEventAvailable(c.ctx, c.app, c.region, eventInfo, c.dbEventIDs, c.now) {
		c.next = earlierDeckEvent(c.next, eventInfo)
		return
	}
	c.blockedNext = earlierDeckEvent(c.blockedNext, eventInfo)
}

func laterDeckEvent(current *sekaidb.Event, candidate *sekaidb.Event) *sekaidb.Event {
	if current == nil || candidate.StartAt > current.StartAt {
		return candidate
	}
	return current
}

func earlierDeckEvent(current *sekaidb.Event, candidate *sekaidb.Event) *sekaidb.Event {
	if current == nil || candidate.StartAt < current.StartAt {
		return candidate
	}
	return current
}

func (c deckAutoEventCandidates) selectEvent(isJPEventFallback bool) (*sekaidb.Event, error) {
	if c.current != nil {
		return c.current, nil
	}
	if c.next != nil {
		return c.next, nil
	}
	if isJPEventFallback {
		return nil, nil
	}
	if c.blockedNext != nil {
		return nil, &deckEventLockedError{EventID: int(c.blockedNext.GameID)}
	}
	if c.active != nil {
		return c.active, nil
	}
	return nil, fmt.Errorf("当前没有可用活动")
}

func ensureDeckEventUnlocked(ctx context.Context, app *renderapp.App, region renderregion.Value, eventInfo *sekaidb.Event) error {
	now := time.Now().UnixMilli()
	if eventInfo == nil || eventInfo.StartAt <= now {
		return nil
	}
	dbEventIDs, err := queryDeckDBEventIDs(ctx, app, region)
	if err != nil {
		return err
	}
	if isDeckFutureEventAvailable(ctx, app, region, eventInfo, dbEventIDs, now) {
		return nil
	}
	return &deckEventLockedError{EventID: int(eventInfo.GameID)}
}

func pickDeckDefaultWorldBloomChapter(eventInfo *sekaidb.Event, chapters []*sekaidb.Worldbloom) *sekaidb.Worldbloom {
	if len(chapters) == 0 || eventInfo == nil {
		return nil
	}
	if len(chapters) == 1 {
		return chapters[0]
	}

	now := time.Now().UnixMilli()
	if now > eventInfo.AggregateAt {
		return latestDeckWorldBloomChapter(chapters)
	}
	if now < eventInfo.StartAt {
		return earliestDeckWorldBloomChapter(chapters)
	}
	return activeDeckWorldBloomChapter(chapters, now)
}

func latestDeckWorldBloomChapter(chapters []*sekaidb.Worldbloom) *sekaidb.Worldbloom {
	var latest *sekaidb.Worldbloom
	for _, chapter := range chapters {
		if chapter != nil && (latest == nil || chapter.ChapterStartAt > latest.ChapterStartAt) {
			latest = chapter
		}
	}
	return latest
}

func earliestDeckWorldBloomChapter(chapters []*sekaidb.Worldbloom) *sekaidb.Worldbloom {
	var earliest *sekaidb.Worldbloom
	for _, chapter := range chapters {
		if chapter != nil && (earliest == nil || chapter.ChapterStartAt < earliest.ChapterStartAt) {
			earliest = chapter
		}
	}
	return earliest
}

func activeDeckWorldBloomChapter(chapters []*sekaidb.Worldbloom, now int64) *sekaidb.Worldbloom {
	var selected *sekaidb.Worldbloom
	for _, chapter := range chapters {
		if chapter == nil {
			continue
		}
		chapterEnd := chapter.AggregateAt + 1000
		if chapter.ChapterStartAt <= now && now <= chapterEnd {
			if selected == nil || chapter.ChapterStartAt > selected.ChapterStartAt {
				selected = chapter
			}
		}
	}
	return selected
}

func isDeckWorldBloomSelectorQuery(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "wl" {
		return true
	}
	if _, ok := parseTrackerWorldBloomChapterNo(query); ok {
		return true
	}
	return strings.HasPrefix(query, "wl")
}

func shouldFallbackDeckEventRecommendToNoEvent(q *deck.AutoQuery) bool {
	if q == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(q.RecommendType), "event")
}

func clearDeckAutoEventSelection(q *deck.AutoQuery) {
	if q == nil {
		return
	}
	q.EventID = nil
	q.EventUnit = ""
	q.EventAttr = ""
	q.WorldBloomEventTurn = nil
	q.WorldBloomFinaleTurn = nil
	q.MetadataWorldBloomFinale = false
	q.WorldBloomCharacterID = nil
	q.WorldBloomCharacterQuery = ""
}

func resolveDeckWorldBloomFinaleEventByTurn(ctx context.Context, app *renderapp.App, region renderregion.Value, turn int) (*sekaidb.Event, error) {
	if turn < 2 {
		return nil, fmt.Errorf("WL终章从 wl2 开始，无法解析 wl%d 终章", turn)
	}
	if turn == 2 {
		return &sekaidb.Event{GameID: 180}, nil
	}
	if app == nil {
		return nil, &deckFutureWorldBloomFinaleTurnError{Turn: turn, Available: 2}
	}

	worldBloomEvents, err := queryDeckWorldBloomEvents(ctx, app, region)
	if err != nil {
		return nil, err
	}
	finaleEvents := []*sekaidb.Event{{GameID: 180}}
	for _, eventInfo := range worldBloomEvents {
		if eventInfo == nil || eventInfo.GameID == 180 {
			continue
		}
		chapters, err := queryDeckWorldBloomChapters(ctx, app, region, int(eventInfo.GameID))
		if err != nil {
			return nil, err
		}
		if deckWorldBloomHasFinaleChapter(chapters) {
			finaleEvents = append(finaleEvents, eventInfo)
		}
	}

	index := turn - 2
	if index >= len(finaleEvents) {
		return nil, &deckFutureWorldBloomFinaleTurnError{Turn: turn, Available: len(finaleEvents) + 1}
	}
	return finaleEvents[index], nil
}

type deckFutureWorldBloomFinaleTurnError struct {
	Turn      int
	Available int
}

func (e *deckFutureWorldBloomFinaleTurnError) Error() string {
	if e == nil {
		return "无法解析未来 WL 终章"
	}
	return fmt.Sprintf("当前 masterdata 仅包含到 wl%d 终章，无法解析 wl%d 终章", e.Available, e.Turn)
}

func deckWorldBloomHasFinaleChapter(chapters []*sekaidb.Worldbloom) bool {
	for _, chapter := range chapters {
		if chapter != nil && strings.EqualFold(strings.TrimSpace(chapter.WorldBloomChapterType), "finale") {
			return true
		}
	}
	return false
}

func missingDeckWorldBloomChapterError(q *deck.AutoQuery, eventID int) error {
	switch strings.ToLower(strings.TrimSpace(q.RecommendType)) {
	case "mysekai":
		return fmt.Errorf("请指定一个要查询的WL角色章节，例如 烤森组卡 wl1 miku 或 烤森组卡 event%d miku", eventID)
	default:
		return fmt.Errorf("请指定一个要查询的WL角色章节，例如 wl1 miku 或 event%d miku", eventID)
	}
}

func resolveDeckWorldBloomEventByTurnSelection(ctx context.Context, app *renderapp.App, region renderregion.Value, q *deck.AutoQuery) (*sekaidb.Event, error) {
	if q == nil || q.WorldBloomEventTurn == nil || *q.WorldBloomEventTurn <= 0 {
		return nil, fmt.Errorf("无效的 WL 活动序号")
	}

	turn := *q.WorldBloomEventTurn
	if q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0 {
		return resolveDeckWorldBloomEventByCharacterTurn(ctx, app, region, turn, *q.WorldBloomCharacterID)
	}

	unit := strings.TrimSpace(q.EventUnit)
	if unit != "" {
		return resolveDeckWorldBloomEventByUnitTurn(ctx, app, region, turn, unit)
	}

	return nil, fmt.Errorf("wl%d 需要指定角色，例如 wl%d miku", turn, turn)
}

func resolveDeckWorldBloomEventByCharacterTurn(ctx context.Context, app *renderapp.App, region renderregion.Value, turn, charID int) (*sekaidb.Event, error) {
	if charID <= 0 {
		return nil, fmt.Errorf("无效的 WL 角色ID: %d", charID)
	}

	worldBloomEvents, err := queryDeckWorldBloomEvents(ctx, app, region)
	if err != nil {
		return nil, err
	}

	matched := make([]*sekaidb.Event, 0, len(worldBloomEvents))
	for _, eventInfo := range worldBloomEvents {
		chapters, err := queryDeckWorldBloomChapters(ctx, app, region, int(eventInfo.GameID))
		if err != nil {
			return nil, err
		}
		if trackerWorldBloomHasCharacter(chapters, charID) {
			matched = append(matched, eventInfo)
		}
	}

	if turn > len(matched) {
		return nil, &deckFutureWorldBloomTurnError{
			Turn:      turn,
			Available: len(matched),
			Character: charID,
		}
	}
	return matched[turn-1], nil
}

func resolveDeckWorldBloomEventByUnitTurn(ctx context.Context, app *renderapp.App, region renderregion.Value, turn int, unit string) (*sekaidb.Event, error) {
	unit = normalizeDeckUnit(unit)
	if unit == "" {
		return nil, fmt.Errorf("wl%d 需要指定角色或团名", turn)
	}

	worldBloomEvents, err := queryDeckWorldBloomEvents(ctx, app, region)
	if err != nil {
		return nil, err
	}

	matched := make([]*sekaidb.Event, 0, len(worldBloomEvents))
	for _, eventInfo := range worldBloomEvents {
		eventUnit := normalizeDeckUnit(eventInfo.Unit)
		if eventUnit != "" {
			if eventUnit == unit {
				matched = append(matched, eventInfo)
			}
			continue
		}

		chapters, err := queryDeckWorldBloomChapters(ctx, app, region, int(eventInfo.GameID))
		if err != nil {
			return nil, err
		}
		if deckWorldBloomHasUnit(chapters, unit) {
			matched = append(matched, eventInfo)
		}
	}

	if turn > len(matched) {
		return nil, &deckFutureWorldBloomTurnError{
			Turn:      turn,
			Available: len(matched),
			Unit:      unit,
		}
	}
	return matched[turn-1], nil
}

type deckFutureWorldBloomTurnError struct {
	Turn      int
	Available int
	Character int
	Unit      string
}

func (e *deckFutureWorldBloomTurnError) Error() string {
	if e == nil {
		return "无法解析未来 WL 轮次"
	}
	if e.Character > 0 {
		return fmt.Sprintf("角色 %d 当前仅有 %d 次 WL，无法解析 wl%d", e.Character, e.Available, e.Turn)
	}
	return fmt.Sprintf("团 %s 当前仅有 %d 次 WL，无法解析 wl%d", e.Unit, e.Available, e.Turn)
}

func shouldKeepDeckWorldBloomSimulationSelection(q *deck.AutoQuery) bool {
	if q == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(q.RecommendType), "event")
}

func queryDeckWorldBloomEvents(ctx context.Context, app *renderapp.App, region renderregion.Value) ([]*sekaidb.Event, error) {
	events, err := queryDeckEvents(ctx, app, region)
	if err != nil {
		return nil, fmt.Errorf("query deck world bloom events failed: %w", err)
	}

	worldBloomEvents := make([]*sekaidb.Event, 0, len(events))
	for _, eventInfo := range events {
		if eventInfo == nil || !strings.EqualFold(eventInfo.EventType, "world_bloom") {
			continue
		}
		worldBloomEvents = append(worldBloomEvents, eventInfo)
	}
	if len(worldBloomEvents) == 0 {
		return nil, fmt.Errorf("当前没有可用的 WL 活动")
	}
	return worldBloomEvents, nil
}

func deckWorldBloomHasUnit(chapters []*sekaidb.Worldbloom, unit string) bool {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return false
	}
	for _, chapter := range chapters {
		if chapter == nil || chapter.GameCharacterID <= 0 {
			continue
		}
		if resolveDeckCharacterUnit(int(chapter.GameCharacterID)) == unit {
			return true
		}
	}
	return false
}

func queryDeckEvents(ctx context.Context, app *renderapp.App, region renderregion.Value) ([]*sekaidb.Event, error) {
	if app == nil {
		return nil, fmt.Errorf("deck event resolve requires app")
	}
	if provider := deckEventProviderForRegion(app, region); provider != nil && provider.Events() != nil {
		items := provider.Events().GetAll(ctx)
		events := make([]*sekaidb.Event, 0, len(items))
		for _, item := range items {
			if item == nil {
				continue
			}
			events = append(events, deckEventFromMasterdata(item))
		}
		sort.Slice(events, func(i, j int) bool {
			return events[i].StartAt < events[j].StartAt
		})
		return events, nil
	}
	if app.Sekai == nil {
		return nil, fmt.Errorf("deck event resolve requires sekai client")
	}
	return app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String())).
		Order(eventdb.ByStartAt()).
		All(ctx)
}

func queryDeckEventByID(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int) (*sekaidb.Event, error) {
	if eventID <= 0 {
		return nil, fmt.Errorf("event id is required")
	}
	if provider := deckEventProviderForRegion(app, region); provider != nil && provider.Events() != nil {
		item, err := provider.Events().GetByID(ctx, eventID)
		if err == nil && item != nil {
			return deckEventFromMasterdata(item), nil
		}
	}
	if app == nil || app.Sekai == nil {
		return nil, fmt.Errorf("event %d not found", eventID)
	}
	return app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String()), eventdb.GameIDEQ(int64(eventID))).
		Only(ctx)
}

func queryDeckWorldBloomChapters(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int) ([]*sekaidb.Worldbloom, error) {
	if provider := deckEventProviderForRegion(app, region); provider != nil && provider.Events() != nil {
		items := provider.Events().GetWorldBloomChapters(ctx, eventID)
		if len(items) > 0 {
			chapters := make([]*sekaidb.Worldbloom, 0, len(items))
			for _, item := range items {
				if item == nil {
					continue
				}
				chapters = append(chapters, deckWorldBloomFromMasterdata(item))
			}
			return chapters, nil
		}
	}
	return queryTrackerWorldBloomChapters(ctx, app, region, eventID)
}

func queryDeckDBEventIDs(ctx context.Context, app *renderapp.App, region renderregion.Value) (map[int]struct{}, error) {
	if app == nil || app.Sekai == nil {
		return nil, nil
	}
	items, err := app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String())).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query deck db events failed: %w", err)
	}
	result := make(map[int]struct{}, len(items))
	for _, item := range items {
		if item == nil || item.GameID <= 0 {
			continue
		}
		result[int(item.GameID)] = struct{}{}
	}
	return result, nil
}

func isDeckFutureEventAvailable(ctx context.Context, app *renderapp.App, region renderregion.Value, eventInfo *sekaidb.Event, dbEventIDs map[int]struct{}, now int64) bool {
	if eventInfo == nil {
		return false
	}
	if eventInfo.StartAt <= now {
		return true
	}
	if region == renderregion.JP {
		return deckEventLeakReleased(ctx, app, int(eventInfo.GameID), now)
	}
	if dbEventIDs == nil {
		return true
	}
	if _, ok := dbEventIDs[int(eventInfo.GameID)]; ok {
		return true
	}
	return false
}

func deckEventLeakReleased(ctx context.Context, app *renderapp.App, eventID int, now int64) bool {
	provider := deckEventProviderForRegion(app, renderregion.JP)
	if provider == nil || provider.Events() == nil {
		return false
	}
	cards, err := provider.Events().GetCards(ctx, eventID)
	if err != nil || len(cards) == 0 {
		return false
	}
	earliest := int64(0)
	for _, cardInfo := range cards {
		if cardInfo == nil || cardInfo.ReleaseAt <= 0 {
			continue
		}
		if earliest == 0 || cardInfo.ReleaseAt < earliest {
			earliest = cardInfo.ReleaseAt
		}
	}
	return earliest > 0 && earliest <= now
}

func deckEventProviderForRegion(app *renderapp.App, region renderregion.Value) renderprovider.MasterDataProvider {
	if app == nil {
		return nil
	}
	return app.ProviderForRegion(region)
}

func deckEventFromMasterdata(item *masterdata.Event) *sekaidb.Event {
	if item == nil {
		return nil
	}
	return &sekaidb.Event{
		GameID:          int64(item.ID),
		EventType:       item.EventType,
		Unit:            item.Unit,
		Name:            item.Name,
		AssetbundleName: item.AssetBundleName,
		StartAt:         item.StartAt,
		AggregateAt:     item.AggregateAt,
		ClosedAt:        item.ClosedAt,
	}
}

func deckWorldBloomFromMasterdata(item *masterdata.WorldBloom) *sekaidb.Worldbloom {
	if item == nil {
		return nil
	}
	charID := int64(0)
	if item.GameCharacterID != nil {
		charID = int64(*item.GameCharacterID)
	}
	return &sekaidb.Worldbloom{
		EventID:               int64(item.EventID),
		GameCharacterID:       charID,
		WorldBloomChapterType: item.ChapterType,
		ChapterNo:             int64(item.ChapterNo),
		ChapterStartAt:        item.ChapterStartAt,
		AggregateAt:           item.AggregateAt,
		ChapterEndAt:          item.ChapterEndAt,
		IsSupplemental:        item.IsSupplemental,
	}
}
