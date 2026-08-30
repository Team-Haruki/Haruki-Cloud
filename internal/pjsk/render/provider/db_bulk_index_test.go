package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"

	_ "github.com/mattn/go-sqlite3"
)

type doneObservedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestDBCardSkillFilterLoadsOneRegionIndex(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_skill_bulk_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	for _, item := range []struct {
		id         int64
		region     renderregion.Value
		spriteName string
	}{
		{id: 11, region: renderregion.JP, spriteName: "score_up"},
		{id: 12, region: renderregion.JP, spriteName: "life_recovery"},
		{id: 13, region: renderregion.TW, spriteName: "score_up"},
	} {
		if _, err := client.Skill.Create().
			SetGameID(item.id).
			SetDescriptionSpriteName(item.spriteName).
			SetServerRegion(item.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create skill %d: %v", item.id, err)
		}
	}
	for _, item := range []struct {
		id      int64
		skillID int64
	}{
		{id: 101, skillID: 11},
		{id: 102, skillID: 12},
	} {
		if _, err := client.Card.Create().
			SetGameID(item.id).
			SetSkillID(item.skillID).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create card %d: %v", item.id, err)
		}
	}

	var skillQueries atomic.Int32
	client.Skill.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			skillQueries.Add(1)
			return next.Query(ctx, query)
		})
	}))

	masterdataProvider := NewDatabaseProvider(client, renderregion.JP)
	traceCtx, trace := commandtrace.WithTrace(ctx)
	got, err := masterdataProvider.cards.Filter(traceCtx, &CardFilter{SkillType: "score_up"})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != 101 {
		t.Fatalf("unexpected filtered cards: %+v", got)
	}
	if skillQueries.Load() != 1 {
		t.Fatalf("expected one bulk skill query, got %d", skillQueries.Load())
	}
	assertOperationCount(t, trace.Snapshot(), "cards.skill_filter", 1)
	assertOperationCount(t, trace.Snapshot(), "skills.index", 1)
	assertOperationCount(t, trace.Snapshot(), "skills.index_wait", 1)

	if _, err := client.Skill.Delete().Exec(ctx); err != nil {
		t.Fatalf("delete skills: %v", err)
	}
	got, err = masterdataProvider.cards.Filter(ctx, &CardFilter{SkillType: "score_up"})
	if err != nil {
		t.Fatalf("cached Filter() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != 101 {
		t.Fatalf("unexpected cached filtered cards: %+v", got)
	}
	if skillQueries.Load() != 1 {
		t.Fatalf("expected cached filter to avoid another skill query, got %d", skillQueries.Load())
	}
}

func TestDBSkillBulkIndexRetriesAfterFailure(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_skill_retry_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	if _, err := client.Skill.Create().
		SetGameID(11).
		SetDescriptionSpriteName("score_up").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	var attempts atomic.Int32
	client.Skill.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if attempts.Add(1) == 1 {
				return nil, errors.New("synthetic skill query failure")
			}
			return next.Query(ctx, query)
		})
	}))

	provider := &dbSkillProvider{client: client, region: renderregion.JP}
	if _, err := provider.matchingTypeIDs(ctx, "score_up"); err == nil {
		t.Fatal("expected first bulk load to fail")
	}
	ids, err := provider.matchingTypeIDs(ctx, "score_up")
	if err != nil {
		t.Fatalf("retry matchingTypeIDs() error = %v", err)
	}
	if _, ok := ids[11]; !ok || len(ids) != 1 {
		t.Fatalf("unexpected matching skill IDs: %+v", ids)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected one retry after failure, got %d attempts", attempts.Load())
	}
}

func TestDBSkillBulkIndexRefreshesAfterTTL(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_skill_ttl_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	provider := &dbSkillProvider{client: client, region: renderregion.JP}
	if _, err := provider.matchingTypeIDs(ctx, "score_up"); err != nil {
		t.Fatalf("initial matchingTypeIDs() error = %v", err)
	}
	if _, err := client.Skill.Create().
		SetGameID(11).
		SetDescriptionSpriteName("score_up").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	provider.mu.Lock()
	provider.allLoadedAt = time.Now().Add(-dbBulkIndexTTL)
	provider.mu.Unlock()
	ids, err := provider.matchingTypeIDs(ctx, "score_up")
	if err != nil {
		t.Fatalf("refreshed matchingTypeIDs() error = %v", err)
	}
	if _, ok := ids[11]; !ok {
		t.Fatalf("refreshed skill index did not contain new skill: %+v", ids)
	}
}

func TestDBSkillPositiveCacheRefreshesAfterTTL(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_skill_positive_ttl_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	entity, err := client.Skill.Create().
		SetGameID(11).
		SetDescriptionSpriteName("score_up").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	provider := &dbSkillProvider{client: client, region: renderregion.JP}
	if skillInfo, err := provider.GetByID(ctx, 11); err != nil || skillInfo.DescriptionSpriteName != "score_up" {
		t.Fatalf("initial skill = %+v, err=%v", skillInfo, err)
	}
	if _, err := entity.Update().SetDescriptionSpriteName("life_recovery").Save(ctx); err != nil {
		t.Fatalf("update skill: %v", err)
	}
	provider.mu.Lock()
	provider.cacheLoadedAt[11] = time.Now().Add(-dbBulkIndexTTL)
	provider.mu.Unlock()
	skillInfo, err := provider.GetByID(ctx, 11)
	if err != nil {
		t.Fatalf("refreshed GetByID() error = %v", err)
	}
	if skillInfo.DescriptionSpriteName != "life_recovery" {
		t.Fatalf("refreshed skill sprite = %q", skillInfo.DescriptionSpriteName)
	}
}

func TestDBCardPositiveCacheRefreshesAfterTTL(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_card_positive_ttl_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	entity, err := client.Card.Create().
		SetGameID(101).
		SetCharacterID(1).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	provider := &dbCardProvider{client: client, region: renderregion.JP}
	if cardInfo, err := provider.GetByID(ctx, 101); err != nil || cardInfo.CharacterID != 1 {
		t.Fatalf("initial card = %+v, err=%v", cardInfo, err)
	}
	if _, err := entity.Update().SetCharacterID(2).Save(ctx); err != nil {
		t.Fatalf("update card: %v", err)
	}
	provider.cardMu.Lock()
	provider.cardCachedAt[101] = time.Now().Add(-dbBulkIndexTTL)
	provider.cardMu.Unlock()
	cardInfo, err := provider.GetByID(ctx, 101)
	if err != nil {
		t.Fatalf("refreshed GetByID() error = %v", err)
	}
	if cardInfo.CharacterID != 2 {
		t.Fatalf("refreshed card character_id = %d", cardInfo.CharacterID)
	}
}

func TestDBCardEpisodesLoadOneRegionIndex(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_episode_bulk_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	for _, item := range []struct {
		id     int64
		seq    int64
		cardID int64
		region renderregion.Value
	}{
		{id: 10002, seq: 2, cardID: 1001, region: renderregion.JP},
		{id: 10001, seq: 1, cardID: 1001, region: renderregion.JP},
		{id: 20001, seq: 1, cardID: 2001, region: renderregion.TW},
	} {
		if _, err := client.Cardepisode.Create().
			SetGameID(item.id).
			SetSeq(item.seq).
			SetCardID(item.cardID).
			SetServerRegion(item.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create card episode %d: %v", item.id, err)
		}
	}

	var episodeQueries atomic.Int32
	client.Cardepisode.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			episodeQueries.Add(1)
			return next.Query(ctx, query)
		})
	}))

	provider := &dbCardProvider{client: client, region: renderregion.JP}
	episodes, err := provider.GetEpisodesByCardID(ctx, 1001)
	if err != nil {
		t.Fatalf("GetEpisodesByCardID() error = %v", err)
	}
	if len(episodes) != 2 || episodes[0].ID != 10001 || episodes[1].ID != 10002 {
		t.Fatalf("unexpected episodes: %+v", episodes)
	}
	if episodeQueries.Load() != 1 {
		t.Fatalf("expected one bulk episode query, got %d", episodeQueries.Load())
	}

	episodes[0].ID = -1
	cached, err := provider.GetEpisodesByCardID(ctx, 1001)
	if err != nil {
		t.Fatalf("cached GetEpisodesByCardID() error = %v", err)
	}
	if len(cached) != 2 || cached[0].ID != 10001 {
		t.Fatalf("cached episodes were mutated by caller: %+v", cached)
	}
	missing, err := provider.GetEpisodesByCardID(ctx, 2001)
	if err != nil {
		t.Fatalf("missing GetEpisodesByCardID() error = %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("cross-region episode leaked into JP index: %+v", missing)
	}
	if episodeQueries.Load() != 1 {
		t.Fatalf("expected cached lookups to avoid another episode query, got %d", episodeQueries.Load())
	}
}

func TestDBCardEpisodeBulkIndexRetriesAfterFailure(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_episode_retry_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	if _, err := client.Cardepisode.Create().
		SetGameID(10001).
		SetSeq(1).
		SetCardID(1001).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create card episode: %v", err)
	}

	var attempts atomic.Int32
	client.Cardepisode.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if attempts.Add(1) == 1 {
				return nil, errors.New("synthetic episode query failure")
			}
			return next.Query(ctx, query)
		})
	}))

	provider := &dbCardProvider{client: client, region: renderregion.JP}
	if _, err := provider.GetEpisodesByCardID(ctx, 1001); err == nil {
		t.Fatal("expected first bulk load to fail")
	}
	episodes, err := provider.GetEpisodesByCardID(ctx, 1001)
	if err != nil {
		t.Fatalf("retry GetEpisodesByCardID() error = %v", err)
	}
	if len(episodes) != 1 || episodes[0].ID != 10001 {
		t.Fatalf("unexpected episodes after retry: %+v", episodes)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected one retry after failure, got %d attempts", attempts.Load())
	}
}

func TestDBCardEpisodeBulkIndexRefreshesAfterTTL(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_episode_ttl_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	provider := &dbCardProvider{client: client, region: renderregion.JP}
	if episodes, err := provider.GetEpisodesByCardID(ctx, 1001); err != nil || len(episodes) != 0 {
		t.Fatalf("initial episodes = %+v, err=%v", episodes, err)
	}
	if _, err := client.Cardepisode.Create().
		SetGameID(10001).
		SetSeq(1).
		SetCardID(1001).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create card episode: %v", err)
	}
	provider.episodeMu.Lock()
	provider.episodesLoadedAt = time.Now().Add(-dbBulkIndexTTL)
	provider.episodeMu.Unlock()
	episodes, err := provider.GetEpisodesByCardID(ctx, 1001)
	if err != nil {
		t.Fatalf("refreshed GetEpisodesByCardID() error = %v", err)
	}
	if len(episodes) != 1 || episodes[0].ID != 10001 {
		t.Fatalf("refreshed episode index did not contain new episode: %+v", episodes)
	}
}

func TestDBCardEpisodeBulkIndexSurvivesLeaderCancellation(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_episode_cancel_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	if _, err := client.Cardepisode.Create().
		SetGameID(10001).
		SetSeq(1).
		SetCardID(1001).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create card episode: %v", err)
	}

	queryStarted := make(chan struct{})
	releaseQuery := make(chan struct{})
	var startedOnce sync.Once
	var attempts atomic.Int32
	client.Cardepisode.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			attempts.Add(1)
			startedOnce.Do(func() { close(queryStarted) })
			<-releaseQuery
			return next.Query(ctx, query)
		})
	}))

	provider := &dbCardProvider{client: client, region: renderregion.JP}
	leaderCtx, cancelLeader := context.WithCancel(ctx)
	leaderResult := make(chan error, 1)
	go func() {
		_, err := provider.GetEpisodesByCardID(leaderCtx, 1001)
		leaderResult <- err
	}()

	select {
	case <-queryStarted:
	case <-time.After(time.Second):
		t.Fatal("bulk episode query did not start")
	}
	cancelLeader()
	select {
	case err := <-leaderResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled leader did not return")
	}

	followerCtx, followerTrace := commandtrace.WithTrace(ctx)
	followerWait := &doneObservedContext{Context: followerCtx, observed: make(chan struct{})}
	followerResult := make(chan error, 1)
	go func() {
		episodes, err := provider.GetEpisodesByCardID(followerWait, 1001)
		if err == nil && (len(episodes) != 1 || episodes[0].ID != 10001) {
			err = fmt.Errorf("unexpected follower episodes: %+v", episodes)
		}
		followerResult <- err
	}()
	select {
	case <-followerWait.observed:
	case <-time.After(time.Second):
		t.Fatal("follower did not start waiting for the shared episode index")
	}
	close(releaseQuery)
	select {
	case err := <-followerResult:
		if err != nil {
			t.Fatalf("follower GetEpisodesByCardID() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("follower did not receive shared bulk result")
	}
	if attempts.Load() != 1 {
		t.Fatalf("expected one shared episode query, got %d", attempts.Load())
	}
	assertOperationCount(t, followerTrace.Snapshot(), "cards.episode_index", 1)
	assertOperationCount(t, followerTrace.Snapshot(), "cards.episode_index_wait", 1)
}

func TestDBCardWorldLinkIndexSurvivesLeaderCancellation(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_world_link_cancel_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	if _, err := client.Event.Create().
		SetGameID(11).
		SetEventType("world_bloom").
		SetUnit("none").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, err := client.Eventcard.Create().
		SetGameID(21).
		SetEventID(11).
		SetCardID(31).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create event card: %v", err)
	}

	queryStarted := make(chan struct{})
	releaseQuery := make(chan struct{})
	var startedOnce sync.Once
	var attempts atomic.Int32
	client.Event.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			attempts.Add(1)
			startedOnce.Do(func() { close(queryStarted) })
			<-releaseQuery
			return next.Query(ctx, query)
		})
	}))

	provider := &dbCardProvider{client: client, region: renderregion.JP}
	leaderCtx, cancelLeader := context.WithCancel(ctx)
	leaderResult := make(chan bool, 1)
	go func() {
		leaderResult <- provider.loadWorldLink3Cards(leaderCtx)
	}()

	select {
	case <-queryStarted:
	case <-time.After(time.Second):
		t.Fatal("world link index query did not start")
	}
	cancelLeader()
	select {
	case loaded := <-leaderResult:
		if loaded {
			t.Fatal("canceled leader unexpectedly reported a loaded index")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled leader did not return")
	}

	followerCtx, followerTrace := commandtrace.WithTrace(ctx)
	followerWait := &doneObservedContext{Context: followerCtx, observed: make(chan struct{})}
	followerResult := make(chan bool, 1)
	go func() {
		followerResult <- provider.loadWorldLink3Cards(followerWait)
	}()
	select {
	case <-followerWait.observed:
	case <-time.After(time.Second):
		t.Fatal("follower did not start waiting for the shared world link index")
	}
	close(releaseQuery)
	select {
	case loaded := <-followerResult:
		if !loaded {
			t.Fatal("follower did not receive the shared world link index")
		}
	case <-time.After(time.Second):
		t.Fatal("follower did not receive shared world link result")
	}
	if attempts.Load() != 1 {
		t.Fatalf("expected one shared world link query, got %d", attempts.Load())
	}
	if !provider.isWorldLink3Card(ctx, 31) {
		t.Fatal("world link card missing from loaded index")
	}
	assertOperationCount(t, followerTrace.Snapshot(), "cards.supply_index", 1)
	assertOperationCount(t, followerTrace.Snapshot(), "cards.supply_index_wait", 1)
}

func TestDBCardWorldLinkIndexRefreshesAfterTTL(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_world_link_ttl_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	provider := &dbCardProvider{client: client, region: renderregion.JP}
	if provider.isWorldLink3Card(ctx, 31) {
		t.Fatal("empty world link index unexpectedly contained card")
	}
	if _, err := client.Event.Create().
		SetGameID(11).
		SetEventType("world_bloom").
		SetUnit("none").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, err := client.Eventcard.Create().
		SetGameID(21).
		SetEventID(11).
		SetCardID(31).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create event card: %v", err)
	}
	provider.worldLink3Mu.Lock()
	provider.worldLink3LoadedAt = time.Now().Add(-dbBulkIndexTTL)
	provider.worldLink3Mu.Unlock()
	if !provider.isWorldLink3Card(ctx, 31) {
		t.Fatal("refreshed world link index did not contain new card")
	}
}

func assertOperationCount(t *testing.T, snapshot commandtrace.Snapshot, name string, want int) {
	t.Helper()
	for _, operation := range snapshot.Operations {
		if operation.Name == name {
			if operation.Count != want {
				t.Fatalf("operation %s count = %d, want %d", name, operation.Count, want)
			}
			return
		}
	}
	t.Fatalf("operation %s was not recorded: %+v", name, snapshot.Operations)
}

func TestDBMusicLimitedTimeMusicsLoadsOneRegionIndex(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_limited_bulk_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	for _, item := range []struct {
		id      int64
		musicID int64
		region  renderregion.Value
		startAt int64
	}{
		{id: 1, musicID: 100, region: renderregion.JP, startAt: 100},
		{id: 2, musicID: 100, region: renderregion.JP, startAt: 200},
		{id: 3, musicID: 200, region: renderregion.JP, startAt: 300},
		{id: 4, musicID: 100, region: renderregion.TW, startAt: 400},
	} {
		if _, err := client.Limitedtimemusic.Create().
			SetGameID(item.id).
			SetMusicID(item.musicID).
			SetStartAt(item.startAt).
			SetEndAt(item.startAt + 1000).
			SetServerRegion(item.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create limitedtimemusic %d: %v", item.id, err)
		}
	}

	var limitedQueries atomic.Int32
	client.Limitedtimemusic.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			limitedQueries.Add(1)
			return next.Query(ctx, query)
		})
	}))

	masterdataProvider := NewDatabaseProvider(client, renderregion.JP)
	traceCtx, trace := commandtrace.WithTrace(ctx)

	// Whole-catalog sweep: one query per music before the bulk index.
	for musicID, wantWindows := range map[int]int{100: 2, 200: 1, 300: 0} {
		got := masterdataProvider.musics.GetLimitedTimeMusics(traceCtx, musicID)
		if len(got) != wantWindows {
			t.Fatalf("GetLimitedTimeMusics(%d) returned %d windows, want %d", musicID, len(got), wantWindows)
		}
	}
	if limitedQueries.Load() != 1 {
		t.Fatalf("expected one bulk limitedtimemusic query, got %d", limitedQueries.Load())
	}
	assertOperationCount(t, trace.Snapshot(), "musics.limited_time_index", 1)
	// Only the first call consults the flight; later calls short-circuit on
	// the freshness check without recording a wait.
	assertOperationCount(t, trace.Snapshot(), "musics.limited_time_index_wait", 1)

	// TW rows must not leak into the JP index.
	jpWindows := masterdataProvider.musics.GetLimitedTimeMusics(traceCtx, 100)
	for _, window := range jpWindows {
		if window.StartAt == 400 {
			t.Fatalf("TW window leaked into JP index: %+v", window)
		}
	}

	// Returned slices are defensive copies: mutating one must not corrupt the index.
	jpWindows[0].StartAt = -1
	fresh := masterdataProvider.musics.GetLimitedTimeMusics(traceCtx, 100)
	if fresh[0].StartAt == -1 {
		t.Fatal("caller mutation leaked into the shared limitedtimemusic index")
	}
}
