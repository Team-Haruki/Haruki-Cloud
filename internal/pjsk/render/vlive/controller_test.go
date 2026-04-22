package vlive

import (
	"context"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

type vliveContextKey string

type fakeSource struct {
	defaultRegion renderregion.Value
	lives         map[renderregion.Value][]*Live
	characters    map[int]*masterdata.GameCharacterUnit
	events        map[int]*masterdata.Event
	resourceBoxes map[int]*provider.ResourceBox
	ctx           context.Context
	wantKey       vliveContextKey
	wantValue     string
}

func (f *fakeSource) DefaultRegion() renderregion.Value {
	return f.defaultRegion
}

func (f *fakeSource) GetLives(region renderregion.Value) ([]*Live, error) {
	if f.wantValue != "" {
		if f.ctx == nil {
			return nil, context.Canceled
		}
		value, _ := f.ctx.Value(f.wantKey).(string)
		if value != f.wantValue {
			return nil, context.Canceled
		}
	}
	return f.lives[region], nil
}

func (f *fakeSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	if f.characters == nil {
		return nil, nil
	}
	return f.characters[id], nil
}

func (f *fakeSource) GetEventByVirtualLiveID(id int) (*masterdata.Event, error) {
	if f.events == nil {
		return nil, nil
	}
	return f.events[id], nil
}

func (f *fakeSource) GetResourceBoxByPurpose(_ string, id int) *provider.ResourceBox {
	if f.resourceBoxes == nil {
		return nil
	}
	return f.resourceBoxes[id]
}

func (f *fakeSource) WithContext(ctx context.Context) DataSource {
	clone := *f
	clone.ctx = ctx
	return &clone
}

func TestRenderTextFiltersAndFormatsLives(t *testing.T) {
	now := time.Date(2026, 3, 26, 20, 0, 0, 0, time.Local)
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }

	controller := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		lives: map[renderregion.Value][]*Live{
			renderregion.JP: {
				{
					ID:      1001,
					Name:    "Future Live",
					StartAt: ms(now.Add(2 * time.Hour)),
					EndAt:   ms(now.Add(4 * time.Hour)),
					Schedules: []Schedule{
						{StartAt: ms(now.Add(2 * time.Hour)), EndAt: ms(now.Add(3 * time.Hour))},
						{StartAt: ms(now.Add(3*time.Hour + 30*time.Minute)), EndAt: ms(now.Add(4 * time.Hour))},
					},
				},
				{
					ID:      1002,
					Name:    "Ongoing Live",
					StartAt: ms(now.Add(-1 * time.Hour)),
					EndAt:   ms(now.Add(2 * time.Hour)),
					Schedules: []Schedule{
						{StartAt: ms(now.Add(-30 * time.Minute)), EndAt: ms(now.Add(30 * time.Minute))},
					},
				},
				{
					ID:      1003,
					Name:    "Too Far",
					StartAt: ms(now.Add(8 * 24 * time.Hour)),
					EndAt:   ms(now.Add(8*24*time.Hour + time.Hour)),
				},
				{
					ID:      1004,
					Name:    "Already Ended",
					StartAt: ms(now.Add(-2 * time.Hour)),
					EndAt:   ms(now.Add(-1 * time.Hour)),
				},
				{
					ID:      1005,
					Name:    "Too Long",
					StartAt: ms(now.Add(time.Hour)),
					EndAt:   ms(now.Add(31 * 24 * time.Hour)),
				},
			},
		},
	}, renderregion.JP)

	text, err := controller.RenderText(ListQuery{Now: now})
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}
	if !strings.Contains(text, "JP 虚拟Live列表") {
		t.Fatalf("missing header: %q", text)
	}
	if !strings.Contains(text, "【1001】Future Live") || !strings.Contains(text, "下一场:") {
		t.Fatalf("missing future live text: %q", text)
	}
	if !strings.Contains(text, "【1002】Ongoing Live") || !strings.Contains(text, "当前Live进行中") {
		t.Fatalf("missing ongoing live text: %q", text)
	}
	if !strings.Contains(text, "剩余场次: 2") {
		t.Fatalf("missing rest count: %q", text)
	}
	if strings.Contains(text, "Too Far") || strings.Contains(text, "Already Ended") || strings.Contains(text, "Too Long") {
		t.Fatalf("unexpected filtered lives in text: %q", text)
	}
}

func TestRenderTextReturnsEmptyMessageWhenNoUpcomingLives(t *testing.T) {
	controller := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		lives:         map[renderregion.Value][]*Live{renderregion.JP: nil},
	}, renderregion.JP)

	text, err := controller.RenderText(ListQuery{
		Now: time.Date(2026, 3, 26, 20, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}
	if text != "当前没有虚拟Live" {
		t.Fatalf("unexpected empty text: %q", text)
	}
}

func TestControllerWithContextClonesVLiveSource(t *testing.T) {
	now := time.Date(2026, 3, 26, 20, 0, 0, 0, time.Local)
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }

	controller := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		wantKey:       "trace",
		wantValue:     "vlive-list",
		lives: map[renderregion.Value][]*Live{
			renderregion.JP: {
				{ID: 1, Name: "Ctx Live", StartAt: ms(now.Add(time.Hour)), EndAt: ms(now.Add(2 * time.Hour))},
			},
		},
	}, renderregion.JP)

	ctx := context.WithValue(context.Background(), vliveContextKey("trace"), "vlive-list")
	text, err := controller.WithContext(ctx).RenderText(ListQuery{Now: now})
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}
	if !strings.Contains(text, "Ctx Live") {
		t.Fatalf("unexpected vlive text: %q", text)
	}
}

func TestBuildListRequestIncludesBannerRewardsAndCharacters(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }

	controller := NewControllerWithDrawing(&fakeSource{
		defaultRegion: renderregion.JP,
		lives: map[renderregion.Value][]*Live{
			renderregion.JP: {
				{
					ID:              371,
					Name:            "Turning Pain into Drive",
					AssetBundleName: "vlentrance_00371_re",
					StartAt:         ms(now.Add(-2 * time.Hour)),
					EndAt:           ms(now.Add(20 * time.Hour)),
					Schedules: []Schedule{
						{StartAt: ms(now.Add(-30 * time.Minute)), EndAt: ms(now.Add(30 * time.Minute))},
						{StartAt: ms(now.Add(2 * time.Hour)), EndAt: ms(now.Add(3 * time.Hour))},
					},
					Rewards: []Reward{
						{VirtualLiveType: "normal", ResourceBoxID: 7},
					},
					Characters: []Character{
						{GameCharacterUnitID: 21, VirtualLivePerformanceType: "main_only"},
						{GameCharacterUnitID: 22, VirtualLivePerformanceType: "both"},
						{GameCharacterUnitID: 22, VirtualLivePerformanceType: "both"},
						{GameCharacterUnitID: 25, VirtualLivePerformanceType: "guest"},
					},
				},
			},
		},
		events: map[int]*masterdata.Event{
			371: {
				ID:              9001,
				AssetBundleName: "event_painful_2022",
				VirtualLiveID:   371,
			},
		},
		characters: map[int]*masterdata.GameCharacterUnit{
			21: {ID: 21, GameCharacterID: 21, Unit: "piapro"},
			22: {ID: 22, GameCharacterID: 22, Unit: "piapro"},
			25: {ID: 25, GameCharacterID: 25, Unit: "piapro"},
		},
		resourceBoxes: map[int]*provider.ResourceBox{
			7: {
				ID: 7,
				Details: []provider.ResourceBoxDetail{
					{ResourceType: "jewel", ResourceQuantity: 300},
					{ResourceType: "material", ResourceID: 12, ResourceQuantity: 2},
				},
			},
		},
	}, nil, nil, renderregion.JP)

	req, err := controller.BuildListRequest(ListQuery{Region: "jp", TimeZone: "Asia/Tokyo", Now: now})
	if err != nil {
		t.Fatalf("BuildListRequest() error = %v", err)
	}
	if req.Region != "jp" {
		t.Fatalf("unexpected region: %q", req.Region)
	}
	if req.TimeZone != "Asia/Tokyo" {
		t.Fatalf("unexpected timezone: %q", req.TimeZone)
	}
	if req.DT != now.UnixMilli() {
		t.Fatalf("unexpected dt: %d", req.DT)
	}
	if len(req.Lives) != 1 {
		t.Fatalf("unexpected lives len: %d", len(req.Lives))
	}
	live := req.Lives[0]
	if live.BannerPath != "asset/jp-assets/startapp/home/banner/event_painful_2022/event_painful_2022.png" {
		t.Fatalf("unexpected banner path: %q", live.BannerPath)
	}
	if !live.Living || live.RestCount != 1 {
		t.Fatalf("unexpected live state: living=%v rest=%d", live.Living, live.RestCount)
	}
	if len(live.Rewards) != 2 {
		t.Fatalf("unexpected rewards len: %d", len(live.Rewards))
	}
	if live.Rewards[0].ImagePath != "asset/jp-assets/startapp/thumbnail/common_material/jewel.png" {
		t.Fatalf("unexpected jewel reward path: %q", live.Rewards[0].ImagePath)
	}
	if live.Rewards[1].ImagePath != "asset/jp-assets/startapp/thumbnail/material/material12.png" {
		t.Fatalf("unexpected material reward path: %q", live.Rewards[1].ImagePath)
	}
	if len(live.Characters) != 2 {
		t.Fatalf("unexpected characters len: %d", len(live.Characters))
	}
	if live.Characters[0].IconPath != "static_images/chara_icon/miku.png" {
		t.Fatalf("unexpected first character path: %q", live.Characters[0].IconPath)
	}
	if live.Characters[1].IconPath != "static_images/chara_icon/rin.png" {
		t.Fatalf("unexpected second character path: %q", live.Characters[1].IconPath)
	}
}

func TestBuildListRequestBirthdayLiveUsesBirthdayBanner(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }

	controller := NewControllerWithDrawing(&fakeSource{
		defaultRegion: renderregion.JP,
		lives: map[renderregion.Value][]*Live{
			renderregion.JP: {
				{
					ID:      373,
					Name:    "HAPPY BIRTHDAY演唱会 结名 2026",
					StartAt: ms(now.Add(24 * time.Hour)),
					EndAt:   ms(now.Add(72 * time.Hour)),
					Characters: []Character{
						{GameCharacterUnitID: 19, VirtualLivePerformanceType: "main_only"},
						{GameCharacterUnitID: 21, VirtualLivePerformanceType: "both"},
					},
				},
			},
		},
		characters: map[int]*masterdata.GameCharacterUnit{
			19: {ID: 19, GameCharacterID: 19, Unit: "25ji"},
			21: {ID: 21, GameCharacterID: 21, Unit: "piapro"},
		},
	}, nil, nil, renderregion.JP)

	req, err := controller.BuildListRequest(ListQuery{Region: "jp", Now: now})
	if err != nil {
		t.Fatalf("BuildListRequest() error = %v", err)
	}
	if len(req.Lives) != 1 {
		t.Fatalf("unexpected lives len: %d", len(req.Lives))
	}
	if got := req.Lives[0].BannerPath; got != "asset/jp-assets/startapp/home/banner/banner_birthday_ena_2026/banner_birthday_ena_2026.png" {
		t.Fatalf("unexpected birthday banner path: %q", got)
	}
}
