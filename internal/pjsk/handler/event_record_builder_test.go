package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	renderprovider "haruki-cloud/internal/pjsk/render/provider"
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
	if eventHistoryPoint(req.EventInfo[0]) != 9999 {
		t.Fatalf("unexpected total WL point: %+v", req.EventInfo[0])
	}
	if req.EventInfo[0].Rank == nil || *req.EventInfo[0].Rank != 123 {
		t.Fatalf("unexpected total WL rank: %+v", req.EventInfo[0].Rank)
	}

	if len(req.WlEventInfo) != 2 {
		t.Fatalf("expected 2 WL single-rank entries, got %+v", req.WlEventInfo)
	}
	if eventHistoryPoint(req.WlEventInfo[0]) != 111 || eventHistoryPoint(req.WlEventInfo[1]) != 222 {
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

func TestBuildEventRecordFromSnapshotOverlaysSuiteRankAfterHonorScan(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_honor_then_rank?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-48 * time.Hour).UnixMilli()
	closedAt := now.Add(-24 * time.Hour).UnixMilli()
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
		SetServerRegion("jp").
		SetGameID(9301).
		SetEventType("marathon").
		SetName("Rank Overlay Event").
		SetAssetbundleName("honor_9301").
		SetStartAt(startAt).
		SetAggregateAt(closedAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(closedAt).
		SetEventRankingRewardRanges(rewardRanges).
		Save(ctx); err != nil {
		t.Fatalf("create event 9301: %v", err)
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
				Now: now.UnixMilli(),
				UserEvents: []rendersnapshot.RawUserEvent{
					{EventID: 9301, EventPoint: 123456},
				},
				UserEventResults: []rendersnapshot.RawUserEventResult{
					{EventID: 9301, Rank: 4971},
				},
				UserHonors: []rendersnapshot.RawUserHonor{
					{HonorID: 5005, HonorLevel: 1},
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
	if req.EventInfo[0].Rank == nil || *req.EventInfo[0].Rank != 4971 {
		t.Fatalf("expected exact suite rank to overlay honor display, got %+v", req.EventInfo[0].Rank)
	}
	if req.EventInfo[0].RankDisplay != nil || req.EventInfo[0].RankTier != nil {
		t.Fatalf("expected exact rank to hide T display, got %+v", req.EventInfo[0])
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

func TestBuildEventRecordFromSnapshotAddsBadgeOnlyEventRankDisplayFromHonor(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_badge_only_honor?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-48 * time.Hour).UnixMilli()
	closedAt := now.Add(-24 * time.Hour).UnixMilli()
	rewardRanges, err := json.Marshal([]map[string]any{{
		"fromRank": 4001,
		"toRank":   5000,
		"eventRankingRewardDetails": []map[string]any{
			{"resourceType": "honor", "resourceId": 6205},
		},
	}})
	if err != nil {
		t.Fatalf("marshal reward ranges: %v", err)
	}
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("cn").
		SetGameID(9204).
		SetEventType("marathon").
		SetName("Badge Only Event").
		SetAssetbundleName("badge_only_9204").
		SetStartAt(startAt).
		SetAggregateAt(closedAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(closedAt).
		SetEventRankingRewardRanges(rewardRanges).
		Save(ctx); err != nil {
		t.Fatalf("create event 9204: %v", err)
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
				UserHonors: []rendersnapshot.RawUserHonor{
					{HonorID: 6205, HonorLevel: 1},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.CN)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected badge-only event entry, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].ID != 9204 {
		t.Fatalf("expected badge-only event 9204, got %+v", req.EventInfo[0])
	}
	if req.EventInfo[0].EventPoint != nil {
		t.Fatalf("expected badge-only event point to be omitted, got %+v", req.EventInfo[0].EventPoint)
	}
	if req.EventInfo[0].RankDisplay == nil || *req.EventInfo[0].RankDisplay != "T5000" {
		t.Fatalf("expected badge-only T5000 display, got %+v", req.EventInfo[0].RankDisplay)
	}
	if req.EventInfo[0].RankTier == nil || *req.EventInfo[0].RankTier != 5000 {
		t.Fatalf("expected badge-only rank_tier=5000, got %+v", req.EventInfo[0].RankTier)
	}
}

func TestBuildEventRecordFromSnapshotBackfillsClosedWorldBloomChapterRankDisplayFromHonor(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_wl_chapter_honor?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-72 * time.Hour).UnixMilli()
	chapterEndAt := now.Add(-48 * time.Hour).UnixMilli()
	closedAt := now.Add(-24 * time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("cn").
		SetGameID(9501).
		SetEventType("world_bloom").
		SetName("WL Chapter Honor Event").
		SetAssetbundleName("wl_chapter_honor").
		SetStartAt(startAt).
		SetAggregateAt(closedAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(closedAt).
		Save(ctx); err != nil {
		t.Fatalf("create wl event: %v", err)
	}
	if _, err := sekaiClient.Worldbloom.Create().
		SetServerRegion("cn").
		SetGameID(950101).
		SetEventID(9501).
		SetGameCharacterID(21).
		SetWorldBloomChapterType("game_character").
		SetChapterNo(1).
		SetChapterStartAt(startAt).
		SetAggregateAt(chapterEndAt - 1000).
		SetChapterEndAt(chapterEndAt).
		Save(ctx); err != nil {
		t.Fatalf("create world bloom chapter 21: %v", err)
	}
	if _, err := sekaiClient.Worldbloom.Create().
		SetServerRegion("cn").
		SetGameID(950102).
		SetEventID(9501).
		SetGameCharacterID(22).
		SetWorldBloomChapterType("game_character").
		SetChapterNo(2).
		SetChapterStartAt(startAt).
		SetAggregateAt(chapterEndAt - 1000).
		SetChapterEndAt(chapterEndAt).
		Save(ctx); err != nil {
		t.Fatalf("create world bloom chapter 22: %v", err)
	}

	root := t.TempDir()
	writeJSONFile(t, filepath.Join(root, "worldBloomChapterRankingRewardRanges.json"), []map[string]any{
		{
			"id":              95010101,
			"eventId":         9501,
			"gameCharacterId": 21,
			"fromRank":        4001,
			"toRank":          5000,
			"resourceBoxId":   95010101,
		},
		{
			"id":              95010201,
			"eventId":         9501,
			"gameCharacterId": 22,
			"fromRank":        4001,
			"toRank":          5000,
			"resourceBoxId":   95010201,
		},
	})
	writeJSONFile(t, filepath.Join(root, "resourceBoxes.json"), []map[string]any{
		{
			"id":                 95010101,
			"resourceBoxPurpose": "world_bloom_chapter_ranking_reward",
			"resourceBoxType":    "expand",
			"details": []map[string]any{
				{"resourceType": "honor", "resourceId": 950121},
			},
		},
		{
			"id":                 95010201,
			"resourceBoxPurpose": "world_bloom_chapter_ranking_reward",
			"resourceBoxType":    "expand",
			"details": []map[string]any{
				{"resourceType": "honor", "resourceId": 950122},
			},
		},
	})

	provider := renderprovider.NewDatabaseProvider(sekaiClient, renderregion.CN)
	provider.SetLocalMasterdataDir(root, false)
	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "cn",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:    sekaiClient,
		Provider: provider,
		Events:   renderevent.NewController(nil, nil, nil),
		Assets:   assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "CN",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				Now: now.UnixMilli(),
				UserWorldBlooms: []rendersnapshot.RawUserWorldBloom{
					{EventID: 9501, GameCharacterID: 21, WorldBloomChapterPoint: 123456},
					{EventID: 9501, GameCharacterID: 22, WorldBloomChapterPoint: 999999},
				},
				UserHonors: []rendersnapshot.RawUserHonor{
					{HonorID: 950121, HonorLevel: 1},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.CN)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.WlEventInfo) != 2 {
		t.Fatalf("expected 2 WL entries, got %+v", req.WlEventInfo)
	}
	var char21, char22 *drawing.EventHistory
	for i := range req.WlEventInfo {
		item := &req.WlEventInfo[i]
		switch eventHistoryPoint(*item) {
		case 123456:
			char21 = item
		case 999999:
			char22 = item
		}
	}
	if char21 == nil || char22 == nil {
		t.Fatalf("expected both character entries, got %+v", req.WlEventInfo)
	}
	if char21.RankDisplay == nil || *char21.RankDisplay != "T5000" {
		t.Fatalf("expected character 21 T5000 display, got %+v", char21.RankDisplay)
	}
	if char21.RankTier == nil || *char21.RankTier != 5000 {
		t.Fatalf("expected character 21 rank_tier=5000, got %+v", char21.RankTier)
	}
	if char22.RankDisplay != nil || char22.RankTier != nil {
		t.Fatalf("expected character 22 to avoid character 21 honor fallback, got %+v", char22)
	}
}

func TestBuildEventRecordFromSnapshotAddsBadgeOnlyWorldBloomChapterRankDisplayFromHonor(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_wl_chapter_badge_only?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-72 * time.Hour).UnixMilli()
	chapterEndAt := now.Add(-48 * time.Hour).UnixMilli()
	closedAt := now.Add(-24 * time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("cn").
		SetGameID(9503).
		SetEventType("world_bloom").
		SetName("WL Chapter Badge Only Event").
		SetAssetbundleName("wl_chapter_badge_only").
		SetStartAt(startAt).
		SetAggregateAt(closedAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(closedAt).
		Save(ctx); err != nil {
		t.Fatalf("create wl event: %v", err)
	}
	if _, err := sekaiClient.Worldbloom.Create().
		SetServerRegion("cn").
		SetGameID(950301).
		SetEventID(9503).
		SetGameCharacterID(21).
		SetWorldBloomChapterType("game_character").
		SetChapterNo(1).
		SetChapterStartAt(startAt).
		SetAggregateAt(chapterEndAt - 1000).
		SetChapterEndAt(chapterEndAt).
		Save(ctx); err != nil {
		t.Fatalf("create world bloom chapter: %v", err)
	}

	root := t.TempDir()
	writeJSONFile(t, filepath.Join(root, "worldBloomChapterRankingRewardRanges.json"), []map[string]any{
		{
			"id":              95030101,
			"eventId":         9503,
			"gameCharacterId": 21,
			"fromRank":        4001,
			"toRank":          5000,
			"resourceBoxId":   95030101,
		},
	})
	writeJSONFile(t, filepath.Join(root, "resourceBoxes.json"), []map[string]any{
		{
			"id":                 95030101,
			"resourceBoxPurpose": "world_bloom_chapter_ranking_reward",
			"resourceBoxType":    "expand",
			"details": []map[string]any{
				{"resourceType": "honor", "resourceId": 950321},
			},
		},
	})

	provider := renderprovider.NewDatabaseProvider(sekaiClient, renderregion.CN)
	provider.SetLocalMasterdataDir(root, false)
	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "cn",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:    sekaiClient,
		Provider: provider,
		Events:   renderevent.NewController(nil, nil, nil),
		Assets:   assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "CN",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				Now: now.UnixMilli(),
				UserHonors: []rendersnapshot.RawUserHonor{
					{HonorID: 950321, HonorLevel: 1},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.CN)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.WlEventInfo) != 1 {
		t.Fatalf("expected badge-only WL entry, got %+v", req.WlEventInfo)
	}
	if req.WlEventInfo[0].ID != 9503 {
		t.Fatalf("expected badge-only WL event 9503, got %+v", req.WlEventInfo[0])
	}
	if req.WlEventInfo[0].EventPoint != nil {
		t.Fatalf("expected badge-only WL point to be omitted, got %+v", req.WlEventInfo[0].EventPoint)
	}
	if req.WlEventInfo[0].RankDisplay == nil || *req.WlEventInfo[0].RankDisplay != "T5000" {
		t.Fatalf("expected badge-only WL T5000 display, got %+v", req.WlEventInfo[0].RankDisplay)
	}
	if req.WlEventInfo[0].RankTier == nil || *req.WlEventInfo[0].RankTier != 5000 {
		t.Fatalf("expected badge-only WL rank_tier=5000, got %+v", req.WlEventInfo[0].RankTier)
	}
	if req.WlEventInfo[0].WlCharaIconPath == nil {
		t.Fatalf("expected badge-only WL character icon, got %+v", req.WlEventInfo[0])
	}
}

func TestBuildEventRecordFromSnapshotOverlaysWorldBloomChapterSuiteRankAfterHonorScan(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_event_record_wl_chapter_rank_overlay?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	now := time.Now()
	startAt := now.Add(-72 * time.Hour).UnixMilli()
	chapterEndAt := now.Add(-48 * time.Hour).UnixMilli()
	closedAt := now.Add(-24 * time.Hour).UnixMilli()
	if _, err := sekaiClient.Event.Create().
		SetServerRegion("cn").
		SetGameID(9502).
		SetEventType("world_bloom").
		SetName("WL Chapter Rank Overlay Event").
		SetAssetbundleName("wl_chapter_rank_overlay").
		SetStartAt(startAt).
		SetAggregateAt(closedAt - int64(time.Hour/time.Millisecond)).
		SetClosedAt(closedAt).
		Save(ctx); err != nil {
		t.Fatalf("create wl event: %v", err)
	}
	if _, err := sekaiClient.Worldbloom.Create().
		SetServerRegion("cn").
		SetGameID(950201).
		SetEventID(9502).
		SetGameCharacterID(21).
		SetWorldBloomChapterType("game_character").
		SetChapterNo(1).
		SetChapterStartAt(startAt).
		SetAggregateAt(chapterEndAt - 1000).
		SetChapterEndAt(chapterEndAt).
		Save(ctx); err != nil {
		t.Fatalf("create world bloom chapter: %v", err)
	}

	root := t.TempDir()
	writeJSONFile(t, filepath.Join(root, "worldBloomChapterRankingRewardRanges.json"), []map[string]any{
		{
			"id":              95020101,
			"eventId":         9502,
			"gameCharacterId": 21,
			"fromRank":        4001,
			"toRank":          5000,
			"resourceBoxId":   95020101,
		},
	})
	writeJSONFile(t, filepath.Join(root, "resourceBoxes.json"), []map[string]any{
		{
			"id":                 95020101,
			"resourceBoxPurpose": "world_bloom_chapter_ranking_reward",
			"resourceBoxType":    "expand",
			"details": []map[string]any{
				{"resourceType": "honor", "resourceId": 950221},
			},
		},
	})

	provider := renderprovider.NewDatabaseProvider(sekaiClient, renderregion.CN)
	provider.SetLocalMasterdataDir(root, false)
	rc := NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "cn",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:    sekaiClient,
		Provider: provider,
		Events:   renderevent.NewController(nil, nil, nil),
		Assets:   assets.NewAssetHelper("", nil),
		Snapshots: rendersnapshot.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "123456789",
				Region:          "CN",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &rendersnapshot.RawUserData{
				Now: now.UnixMilli(),
				UserWorldBlooms: []rendersnapshot.RawUserWorldBloom{
					{EventID: 9502, GameCharacterID: 21, WorldBloomChapterPoint: 123456, Rank: 4971},
				},
				UserHonors: []rendersnapshot.RawUserHonor{
					{HonorID: 950221, HonorLevel: 1},
				},
			},
		}),
	})

	req, err := buildEventRecordFromSnapshot(rc, renderregion.CN)
	if err != nil {
		t.Fatalf("buildEventRecordFromSnapshot() error = %v", err)
	}
	if len(req.WlEventInfo) != 1 {
		t.Fatalf("expected 1 WL entry, got %+v", req.WlEventInfo)
	}
	if req.WlEventInfo[0].Rank == nil || *req.WlEventInfo[0].Rank != 4971 {
		t.Fatalf("expected exact suite WL rank to overlay honor display, got %+v", req.WlEventInfo[0].Rank)
	}
	if req.WlEventInfo[0].RankDisplay != nil || req.WlEventInfo[0].RankTier != nil {
		t.Fatalf("expected exact WL rank to hide T display, got %+v", req.WlEventInfo[0])
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
		{ID: 1, RankDisplay: stringPtr("T5000"), EventPoint: intPtr(10)},
		{ID: 2, Rank: intPtr(8000), EventPoint: intPtr(999999)},
		{ID: 3, EventPoint: intPtr(1000000)},
		{ID: 4, Rank: intPtr(3000), EventPoint: intPtr(1)},
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

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
