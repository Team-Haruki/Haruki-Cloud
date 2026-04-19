package handler

import (
	"context"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	_ "github.com/mattn/go-sqlite3"
)

type eventRecordSnapshotStub struct {
	detail  *drawing.DetailedProfileCardRequest
	rawData *rendersnapshot.RawUserData
}

func (s *eventRecordSnapshotStub) Require() error { return nil }

func (s *eventRecordSnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return s.detail
}

func (s *eventRecordSnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return nil
}

func (s *eventRecordSnapshotStub) MusicResults(string) map[int]string { return nil }

func (s *eventRecordSnapshotStub) GetMusicResult(int, string) string { return "" }

func (s *eventRecordSnapshotStub) ChallengeLive() *rendersnapshot.ChallengeLiveData { return nil }

func (s *eventRecordSnapshotStub) RawBytes() ([]byte, error) { return nil, nil }

func (s *eventRecordSnapshotStub) RawValue(string) ([]byte, error) { return nil, nil }

func (s *eventRecordSnapshotStub) RawFilePath() string { return "" }

func (s *eventRecordSnapshotStub) RawData() *rendersnapshot.RawUserData { return s.rawData }

func (s *eventRecordSnapshotStub) MusicMetaBytes() []byte { return nil }

func (s *eventRecordSnapshotStub) MusicMetaPath() string { return "" }

func TestBuildEventRecordFromSnapshotSeparatesWorldBloomTotalAndSingleRank(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_wl?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	startAt := time.Now().Add(-2 * time.Hour).UnixMilli()
	aggregateAt := time.Now().Add(-time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("jp").
		SetGameID(9001).
		SetEventType("world_bloom").
		SetName("WL Event").
		SetAssetbundleName("wl_9001").
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + 1000).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "12345678901234",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9001, EventPoint: 9999},
				},
				UserEventResults: []rendersnapshot.RawUserEventResult{
					{EventID: 9001, Rank: 123},
				},
				UserWorldBlooms: []rendersnapshot.RawUserWorldBloom{
					{EventID: 9001, GameCharacterID: 21, WorldBloomChapterPoint: 111, Rank: 10},
					{EventID: 9001, GameCharacterID: 22, WorldBloomChapterPoint: 222, Rank: 20},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.JP)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}

	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 total event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].EventPoint != 9999 {
		t.Fatalf("unexpected total WL point: %+v", req.EventInfo[0])
	}
	if req.EventInfo[0].Rank == nil || *req.EventInfo[0].Rank != 123 {
		t.Fatalf("unexpected total WL rank: %+v", req.EventInfo[0].Rank)
	}

	if len(req.WlEventInfo) != 2 {
		t.Fatalf("expected 2 WL single-rank entries, got %+v", req.WlEventInfo)
	}
	if req.WlEventInfo[0].EventPoint != 111 || req.WlEventInfo[1].EventPoint != 222 {
		t.Fatalf("unexpected WL single points: %+v", req.WlEventInfo)
	}
	if req.WlEventInfo[0].Rank == nil || *req.WlEventInfo[0].Rank != 10 {
		t.Fatalf("unexpected first WL single rank: %+v", req.WlEventInfo[0].Rank)
	}
	if req.WlEventInfo[1].Rank == nil || *req.WlEventInfo[1].Rank != 20 {
		t.Fatalf("unexpected second WL single rank: %+v", req.WlEventInfo[1].Rank)
	}
	if req.WlEventInfo[0].WlCharaIconPath == nil || req.WlEventInfo[1].WlCharaIconPath == nil {
		t.Fatalf("expected WL single-rank icons: %+v", req.WlEventInfo)
	}
	if got := *req.WlEventInfo[0].WlCharaIconPath; got != "asset/jp-assets/startapp/character/character_sd_l/chr_sp_21.png" {
		t.Fatalf("unexpected first WL single SD path: %q", got)
	}
	if got := *req.WlEventInfo[1].WlCharaIconPath; got != "asset/jp-assets/startapp/character/character_sd_l/chr_sp_22.png" {
		t.Fatalf("unexpected second WL single SD path: %q", got)
	}
}

func TestBuildEventRecordFromSnapshotBackfillsRegularEventRankFromTracker(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_tracker?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	startAt := time.Now().Add(-24 * time.Hour).UnixMilli()
	aggregateAt := time.Now().Add(-23 * time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("jp").
		SetGameID(9101).
		SetEventType("marathon").
		SetName("Regular Event").
		SetAssetbundleName("regular_9101").
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + 1000).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}

	originalLookup := eventRecordTrackerRankLookup
	t.Cleanup(func() {
		eventRecordTrackerRankLookup = originalLookup
	})

	var calls int
	eventRecordTrackerRankLookup = func(ctx context.Context, _ *sekaiapi.TrackerClient, region string, eventID int, userID int64) (*int, error) {
		calls++
		if region != "jp" || eventID != 9101 || userID != 123456789 {
			t.Fatalf("unexpected tracker lookup args: region=%s event=%d user=%d", region, eventID, userID)
		}
		return new(456), nil
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserGamedata: rendersnapshot.RawUserGamedata{UserID: 123456789},
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9101, EventPoint: 777777},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.JP)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected tracker rank lookup once, got %d", calls)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].Rank == nil || *req.EventInfo[0].Rank != 456 {
		t.Fatalf("expected tracker-backed regular event rank, got %+v", req.EventInfo[0].Rank)
	}
}

func TestBuildEventRecordFromSnapshotUsesEmbeddedRegularEventRankWithoutTracker(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_embedded_rank?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	startAt := time.Now().Add(-24 * time.Hour).UnixMilli()
	aggregateAt := time.Now().Add(-23 * time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("jp").
		SetGameID(9104).
		SetEventType("marathon").
		SetName("Regular Event With Embedded Rank").
		SetAssetbundleName("regular_9104").
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + 1000).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}

	originalLookup := eventRecordTrackerRankLookup
	t.Cleanup(func() {
		eventRecordTrackerRankLookup = originalLookup
	})
	eventRecordTrackerRankLookup = func(context.Context, *sekaiapi.TrackerClient, string, int, int64) (*int, error) {
		t.Fatal("tracker fallback should not be used when userEvents already embeds rank")
		return nil, nil
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserGamedata: rendersnapshot.RawUserGamedata{UserID: 123456789},
				UserEvents: []rendersnapshot.RawUserEvent{
					{
						EventID:                 9104,
						EventPoint:              666666,
						Rank:                    908,
						RankingRewardReceivedAt: 1776164672910,
					},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.JP)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].Rank == nil || *req.EventInfo[0].Rank != 908 {
		t.Fatalf("expected embedded regular event rank, got %+v", req.EventInfo[0].Rank)
	}
}

func TestBuildEventRecordFromSnapshotSkipsTrackerFallbackWhenSnapshotHasRank(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_snapshot_rank?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	startAt := time.Now().Add(-24 * time.Hour).UnixMilli()
	aggregateAt := time.Now().Add(-23 * time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("jp").
		SetGameID(9102).
		SetEventType("marathon").
		SetName("Regular Event With Snapshot Rank").
		SetAssetbundleName("regular_9102").
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + 1000).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}

	originalLookup := eventRecordTrackerRankLookup
	t.Cleanup(func() {
		eventRecordTrackerRankLookup = originalLookup
	})
	eventRecordTrackerRankLookup = func(context.Context, *sekaiapi.TrackerClient, string, int, int64) (*int, error) {
		t.Fatal("tracker fallback should not be used when snapshot already has rank")
		return nil, nil
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserGamedata: rendersnapshot.RawUserGamedata{UserID: 123456789},
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9102, EventPoint: 888888},
				},
				UserEventResults: []rendersnapshot.RawUserEventResult{
					{EventID: 9102, Rank: 321},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.JP)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].Rank == nil || *req.EventInfo[0].Rank != 321 {
		t.Fatalf("expected snapshot rank to be preserved, got %+v", req.EventInfo[0].Rank)
	}
}

func TestBuildEventRecordFromSnapshotIgnoresMissingTrackerRank(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_missing_tracker?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	startAt := time.Now().Add(-24 * time.Hour).UnixMilli()
	aggregateAt := time.Now().Add(-23 * time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("jp").
		SetGameID(9103).
		SetEventType("marathon").
		SetName("Regular Event Missing Tracker Rank").
		SetAssetbundleName("regular_9103").
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + 1000).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}

	originalLookup := eventRecordTrackerRankLookup
	t.Cleanup(func() {
		eventRecordTrackerRankLookup = originalLookup
	})
	eventRecordTrackerRankLookup = func(context.Context, *sekaiapi.TrackerClient, string, int, int64) (*int, error) {
		return nil, sekaiapi.ErrRankingNotFound
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserGamedata: rendersnapshot.RawUserGamedata{UserID: 123456789},
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9103, EventPoint: 999999},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.JP)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].Rank != nil {
		t.Fatalf("expected missing tracker rank to stay empty, got %+v", req.EventInfo[0].Rank)
	}
}

func TestBuildEventRecordFromSnapshotLimitsTrackerFallbackToRecentEvents(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_limit_tracker?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	userEvents := make([]rendersnapshot.RawUserEvent, 0, eventRecordTrackerFallbackEventLimit+2)
	for eventID := 9201; eventID <= 9210; eventID++ {
		startAt := now.Add(-time.Duration(9210-eventID+1) * time.Hour).UnixMilli()
		aggregateAt := startAt + int64(time.Hour/time.Millisecond)
		if _, err := sekaiClient.Event.Create().
			SetServerRegion("jp").
			SetGameID(int64(eventID)).
			SetEventType("marathon").
			SetName("Tracker Limit Event").
			SetAssetbundleName("tracker_limit").
			SetStartAt(startAt).
			SetAggregateAt(aggregateAt).
			SetClosedAt(aggregateAt + 1000).
			Save(ctx); err != nil {
			t.Fatalf("create event %d: %v", eventID, err)
		}
		userEvents = append(userEvents, rendersnapshot.RawUserEvent{EventID: eventID, EventPoint: eventID * 100})
	}

	originalLookup := eventRecordTrackerRankLookup
	t.Cleanup(func() {
		eventRecordTrackerRankLookup = originalLookup
	})

	calls := make([]int, 0, len(userEvents))
	eventRecordTrackerRankLookup = func(_ context.Context, _ *sekaiapi.TrackerClient, _ string, _ int, _ int64) (*int, error) {
		calls = append(calls, len(calls))
		return nil, sekaiapi.ErrRankingNotFound
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserGamedata: rendersnapshot.RawUserGamedata{UserID: 123456789},
				UserEvents:   userEvents,
			},
		}),
	})

	if _, err := buildEventRecordFromSnapshot(rc, renderregion.JP); err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(calls) != eventRecordTrackerFallbackEventLimit {
		t.Fatalf("expected %d tracker calls, got %d", eventRecordTrackerFallbackEventLimit, len(calls))
	}
}

func TestBuildEventRecordFromSnapshotStopsTrackerFallbackAfterFatalError(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_stop_tracker?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	userEvents := []rendersnapshot.RawUserEvent{
		{EventID: 9310, EventPoint: 1000},
		{EventID: 9309, EventPoint: 900},
		{EventID: 9308, EventPoint: 800},
	}
	for index, userEvent := range userEvents {
		startAt := now.Add(-time.Duration(index+1) * time.Hour).UnixMilli()
		aggregateAt := startAt + int64(time.Hour/time.Millisecond)
		if _, err := sekaiClient.Event.Create().
			SetServerRegion("jp").
			SetGameID(int64(userEvent.EventID)).
			SetEventType("marathon").
			SetName("Tracker Stop Event").
			SetAssetbundleName("tracker_stop").
			SetStartAt(startAt).
			SetAggregateAt(aggregateAt).
			SetClosedAt(aggregateAt + 1000).
			Save(ctx); err != nil {
			t.Fatalf("create event %d: %v", userEvent.EventID, err)
		}
	}

	originalLookup := eventRecordTrackerRankLookup
	t.Cleanup(func() {
		eventRecordTrackerRankLookup = originalLookup
	})

	calls := make([]int, 0, len(userEvents))
	eventRecordTrackerRankLookup = func(_ context.Context, _ *sekaiapi.TrackerClient, _ string, eventID int, _ int64) (*int, error) {
		calls = append(calls, eventID)
		if eventID == 9310 {
			return nil, &sekaiapi.TrackerAPIError{
				StatusCode: 500,
				Message:    `{"error":"failed to fetch latest ranking: Error 1146 (42S02): Table 'jp_event_db.event_9310' doesn't exist"}`,
			}
		}
		return new(123), nil
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserGamedata: rendersnapshot.RawUserGamedata{UserID: 123456789},
				UserEvents:   userEvents,
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.JP)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(calls) != 1 || calls[0] != 9310 {
		t.Fatalf("expected fallback to stop after latest fatal error, got calls=%v", calls)
	}
	if len(req.EventInfo) != len(userEvents) {
		t.Fatalf("expected %d event entries, got %+v", len(userEvents), req.EventInfo)
	}
	for _, item := range req.EventInfo {
		if item.Rank != nil {
			t.Fatalf("expected no tracker rank after fatal fallback stop, got %+v", req.EventInfo)
		}
	}
}

func TestBuildEventRecordFromSnapshotSkipsWorldBloomTrackerFallback(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_skip_wl_tracker?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-2 * time.Hour).UnixMilli()
	aggregateAt := now.Add(-time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("jp").
		SetGameID(9401).
		SetEventType("world_bloom").
		SetName("WL Event").
		SetAssetbundleName("wl_skip_tracker").
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + 1000).
		Save(ctx); err != nil {
		t.Fatalf("create wl event: %v", err)
	}
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("jp").
		SetGameID(9400).
		SetEventType("marathon").
		SetName("Regular Event").
		SetAssetbundleName("regular_skip_tracker").
		SetStartAt(startAt - int64(time.Hour/time.Millisecond)).
		SetAggregateAt(aggregateAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(aggregateAt + 1000 - int64(time.Hour/time.Millisecond)).
		Save(ctx); err != nil {
		t.Fatalf("create regular event: %v", err)
	}

	originalLookup := eventRecordTrackerRankLookup
	t.Cleanup(func() {
		eventRecordTrackerRankLookup = originalLookup
	})

	calls := make([]int, 0, 2)
	eventRecordTrackerRankLookup = func(_ context.Context, _ *sekaiapi.TrackerClient, _ string, eventID int, _ int64) (*int, error) {
		calls = append(calls, eventID)
		return new(321), nil
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserGamedata: rendersnapshot.RawUserGamedata{UserID: 123456789},
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9401, EventPoint: 9999},
					{EventID: 9400, EventPoint: 8888},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.JP)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(calls) != 1 || calls[0] != 9400 {
		t.Fatalf("expected tracker fallback only for regular event, got calls=%v", calls)
	}
	if len(req.EventInfo) != 2 {
		t.Fatalf("expected 2 event entries, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].ID != 9401 || !req.EventInfo[0].IsWlEvent {
		t.Fatalf("unexpected first event info: %+v", req.EventInfo[0])
	}
	if req.EventInfo[0].Rank != nil {
		t.Fatalf("expected wl total event to keep empty rank, got %+v", req.EventInfo[0].Rank)
	}
	if req.EventInfo[1].Rank == nil || *req.EventInfo[1].Rank != 321 {
		t.Fatalf("expected regular event rank from tracker, got %+v", req.EventInfo[1].Rank)
	}
}
