package handler

import (
	"context"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"

	_ "github.com/mattn/go-sqlite3"
)

type eventRecordSnapshotStub struct {
	detail  *drawing.DetailedProfileCardRequest
	rawData *renderuserdata.RawUserData
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

func (s *eventRecordSnapshotStub) ChallengeLive() *renderuserdata.ChallengeLiveData { return nil }

func (s *eventRecordSnapshotStub) RawBytes() ([]byte, error) { return nil, nil }

func (s *eventRecordSnapshotStub) RawValue(string) ([]byte, error) { return nil, nil }

func (s *eventRecordSnapshotStub) RawFilePath() string { return "" }

func (s *eventRecordSnapshotStub) RawData() *renderuserdata.RawUserData { return s.rawData }

func (s *eventRecordSnapshotStub) MusicMetaBytes() []byte { return nil }

func (s *eventRecordSnapshotStub) MusicMetaPath() string { return "" }

func TestBuildEventRecordFromSnapshotSeparatesWorldBloomTotalAndSingleRank(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_event_record_wl?mode=memory&cache=shared&_fk=1")
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

	rc := NewRequestContext(ctx, &parser.ResolvedCommand{
		Module: parser.ModuleEvent,
		Mode:   "event-record",
		Region: "jp",
	}, &renderapp.App{
		Sekai:  sekaiClient,
		Events: renderevent.NewController(nil, nil, nil),
		Assets: assets.NewAssetHelper("", nil),
		Snapshots: renderuserdata.NewStaticSnapshotProvider(&eventRecordSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				ID:              "12345678901234",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "static_images/chara_icon/miku.png",
			},
			rawData: &renderuserdata.RawUserData{
				UserEvents: []renderuserdata.RawUserEvent{
					{EventID: 9001, EventPoint: 9999},
				},
				UserEventResults: []renderuserdata.RawUserEventResult{
					{EventID: 9001, Rank: 123},
				},
				UserWorldBlooms: []renderuserdata.RawUserWorldBloom{
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
}
