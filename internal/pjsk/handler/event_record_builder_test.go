package handler

import (
	"context"
	"encoding/json"
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

func TestBuildEventRecordFromSnapshotLeavesRegularEventRankEmptyWithoutSuiteRank(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_no_suite_rank?mode=memory&cache=shared&_fk=1")
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
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].Rank != nil {
		t.Fatalf("expected missing suite rank to stay empty, got %+v", req.EventInfo[0].Rank)
	}
}

func TestBuildEventRecordFromSnapshotBackfillsClosedEventRankDisplayFromHonor(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_honor_rank?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-48 * time.Hour).UnixMilli()
	closedAt := now.Add(-24 * time.Hour).UnixMilli()
	rewardRanges, err := json.Marshal([]map[string]any{
		{
			"fromRank": 1001,
			"toRank":   2000,
			"eventRankingRewardDetails": []map[string]any{
				{"resourceType": "honor", "resourceId": 2002},
			},
		},
		{
			"fromRank": 4001,
			"toRank":   5000,
			"eventRankingRewardDetails": []map[string]any{
				{"resourceType": "honor", "resourceId": 5005},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal reward ranges: %v", err)
	}
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("cn").
		SetGameID(9201).
		SetEventType("marathon").
		SetName("T5000 Event").
		SetAssetbundleName("honor_9201").
		SetStartAt(startAt).
		SetAggregateAt(closedAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(closedAt).
		SetEventRankingRewardRanges(rewardRanges).
		Save(ctx); err != nil {
		t.Fatalf("create event 9201: %v", err)
	}
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("cn").
		SetGameID(9202).
		SetEventType("marathon").
		SetName("High PT No Tier").
		SetAssetbundleName("honor_9202").
		SetStartAt(startAt).
		SetAggregateAt(closedAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(closedAt).
		Save(ctx); err != nil {
		t.Fatalf("create event 9202: %v", err)
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "cn",
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
				Region:          "CN",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				Now: now.UnixMilli(),
				UserGamedata: rendersnapshot.RawUserGamedata{
					UserID: 123456789,
				},
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9202, EventPoint: 999999},
					{EventID: 9201, EventPoint: 10},
				},
				UserHonors: []rendersnapshot.RawUserHonor{
					{HonorID: 5005, HonorLevel: 1},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.CN)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.EventInfo) != 2 {
		t.Fatalf("expected 2 event entries, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].ID != 9201 {
		t.Fatalf("expected T rank tier to sort before PT-only event, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].Rank != nil {
		t.Fatalf("expected honor fallback to avoid exact rank, got %+v", req.EventInfo[0].Rank)
	}
	if req.EventInfo[0].RankDisplay == nil || *req.EventInfo[0].RankDisplay != "T5000" {
		t.Fatalf("expected T5000 display, got %+v", req.EventInfo[0].RankDisplay)
	}
	if req.EventInfo[0].RankTier == nil || *req.EventInfo[0].RankTier != 5000 {
		t.Fatalf("expected rank_tier=5000, got %+v", req.EventInfo[0].RankTier)
	}
	if req.RankNote == nil || *req.RankNote != "CN/KR/TW服没有排名数据，仅显示Txxx名" {
		t.Fatalf("expected CN rank note, got %+v", req.RankNote)
	}
}

func TestBuildEventRecordFromSnapshotBackfillsClosedEventRankDisplayFromResourceBoxHonor(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_resource_box_honor?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-48 * time.Hour).UnixMilli()
	closedAt := now.Add(-24 * time.Hour).UnixMilli()
	rewardRanges, err := json.Marshal([]map[string]any{{
		"fromRank": 4001,
		"toRank":   5000,
		"eventRankingRewards": []map[string]any{
			{"resourceBoxId": 4926, "rewardConditionType": "none"},
		},
	}})
	if err != nil {
		t.Fatalf("marshal reward ranges: %v", err)
	}
	resourceBoxDetails, err := json.Marshal([]map[string]any{
		{"resourceType": "jewel", "resourceQuantity": 100},
		{"resourceType": "honor", "resourceId": 5870, "resourceQuantity": 1},
	})
	if err != nil {
		t.Fatalf("marshal resource box details: %v", err)
	}
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("cn").
		SetGameID(9203).
		SetEventType("marathon").
		SetName("Real Shape T5000 Event").
		SetAssetbundleName("honor_9203").
		SetStartAt(startAt).
		SetAggregateAt(closedAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(closedAt).
		SetEventRankingRewardRanges(rewardRanges).
		Save(ctx); err != nil {
		t.Fatalf("create event 9203: %v", err)
	}
	if _, err := sekaiClient.Resourceboxe.Create().
		SetServerRegion("cn").
		SetGameID(4926).
		SetResourceBoxPurpose("event_ranking_reward").
		SetResourceBoxType("expand").
		SetDetails(resourceBoxDetails).
		Save(ctx); err != nil {
		t.Fatalf("create resource box 4926: %v", err)
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "cn",
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
				Region:          "CN",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				Now: now.UnixMilli(),
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9203, EventPoint: 2435379},
				},
				UserHonors: []rendersnapshot.RawUserHonor{
					{HonorID: 5870, HonorLevel: 1},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.CN)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].RankDisplay == nil || *req.EventInfo[0].RankDisplay != "T5000" {
		t.Fatalf("expected resource-box honor fallback T5000, got %+v", req.EventInfo[0].RankDisplay)
	}
	if req.EventInfo[0].RankTier == nil || *req.EventInfo[0].RankTier != 5000 {
		t.Fatalf("expected rank_tier=5000, got %+v", req.EventInfo[0].RankTier)
	}
}

func TestBuildEventRecordFromSnapshotDoesNotBackfillHonorRankBeforeClosed(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_honor_open?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	rewardRanges, err := json.Marshal([]map[string]any{{
		"fromRank": 4001,
		"toRank":   5000,
		"eventRankingRewardDetails": []map[string]any{
			{"resourceType": "honor", "resourceId": 5005},
		},
	}})
	if err != nil {
		t.Fatalf("marshal reward ranges: %v", err)
	}
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("cn").
		SetGameID(9203).
		SetEventType("marathon").
		SetName("Open Event").
		SetAssetbundleName("honor_open").
		SetStartAt(now.Add(-time.Hour).UnixMilli()).
		SetAggregateAt(now.Add(time.Hour).UnixMilli()).
		SetClosedAt(now.Add(2 * time.Hour).UnixMilli()).
		SetEventRankingRewardRanges(rewardRanges).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "cn",
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
				Region:          "CN",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				Now: now.UnixMilli(),
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9203, EventPoint: 777777},
				},
				UserHonors: []rendersnapshot.RawUserHonor{
					{HonorID: 5005, HonorLevel: 1},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.CN)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].RankDisplay != nil || req.EventInfo[0].RankTier != nil {
		t.Fatalf("expected open event to avoid honor rank fallback, got %+v", req.EventInfo[0])
	}
}

func TestBuildEventRecordFromSnapshotDropsNonJPRankingBoundaryRank(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_non_jp_boundary_rank?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	startAt := time.Now().Add(-24 * time.Hour).UnixMilli()
	aggregateAt := time.Now().Add(-23 * time.Hour).UnixMilli()
	rewardRanges, err := json.Marshal([]map[string]int{
		{"fromRank": 3001, "toRank": 4000},
		{"fromRank": 4001, "toRank": 5000},
	})
	if err != nil {
		t.Fatalf("marshal reward ranges: %v", err)
	}
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("tw").
		SetGameID(9105).
		SetEventType("marathon").
		SetName("TW Boundary Rank Event").
		SetAssetbundleName("tw_boundary_rank").
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + 1000).
		SetEventRankingRewardRanges(rewardRanges).
		Save(ctx); err != nil {
		t.Fatalf("create event: %v", err)
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "tw",
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
				Region:          "TW",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				UserGamedata: rendersnapshot.RawUserGamedata{UserID: 123456789},
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9105, EventPoint: 777777},
				},
				UserEventResults: []rendersnapshot.RawUserEventResult{
					{EventID: 9105, Rank: 4000},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.TW)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].Rank != nil {
		t.Fatalf("expected suspicious non-JP boundary rank to be dropped, got %+v", req.EventInfo[0].Rank)
	}
}

func TestBuildEventRecordFromSnapshotUsesEmbeddedRegularEventRank(t *testing.T) {
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

func TestBuildEventRecordFromSnapshotUsesUserEventResultRank(t *testing.T) {
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

func TestBuildEventRecordFromSnapshotLeavesWorldBloomAndRegularRanksEmptyWithoutSuiteRank(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_no_rank_wl_regular?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-2 * time.Hour).UnixMilli()
	aggregateAt := now.Add(-time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("jp").
		SetGameID(9401).
		SetEventType("world_bloom").
		SetName("WL Event").
		SetAssetbundleName("wl_no_rank").
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
		SetAssetbundleName("regular_no_rank").
		SetStartAt(startAt - int64(time.Hour/time.Millisecond)).
		SetAggregateAt(aggregateAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(aggregateAt + 1000 - int64(time.Hour/time.Millisecond)).
		Save(ctx); err != nil {
		t.Fatalf("create regular event: %v", err)
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
	if len(req.EventInfo) != 2 {
		t.Fatalf("expected 2 event entries, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].ID != 9401 || !req.EventInfo[0].IsWlEvent {
		t.Fatalf("unexpected first event info: %+v", req.EventInfo[0])
	}
	if req.EventInfo[0].Rank != nil {
		t.Fatalf("expected wl total event to keep empty rank, got %+v", req.EventInfo[0].Rank)
	}
	if req.EventInfo[1].Rank != nil {
		t.Fatalf("expected regular event to keep empty rank, got %+v", req.EventInfo[1].Rank)
	}
}

func TestSortEventHistoryUsesRankDisplayTier(t *testing.T) {
	items := []drawing.EventHistory{
		{ID: 1, RankDisplay: stringPtr("T5000"), EventPoint: 10},
		{ID: 2, Rank: intPtr(8000), EventPoint: 999999},
		{ID: 3, EventPoint: 1000000},
		{ID: 4, Rank: intPtr(3000), EventPoint: 1},
	}

	sortEventHistory(items)

	got := []int{items[0].ID.(int), items[1].ID.(int), items[2].ID.(int), items[3].ID.(int)}
	want := []int{4, 1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected T5000 to sort as 5000, got order %v", got)
		}
	}
}
