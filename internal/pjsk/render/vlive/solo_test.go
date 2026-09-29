package vlive

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

type soloFakeSource struct {
	fakeSource
	groups    map[int]*Group
	materials map[int]string
}

func (f *soloFakeSource) GetGroups(renderregion.Value) (map[int]*Group, bool) {
	return f.groups, f.groups != nil
}

func (f *soloFakeSource) GetMaterialName(id int) string { return f.materials[id] }

var soloTestNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func soloTestLives() []*Live {
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }
	start, end := soloTestNow.Add(-24*time.Hour), soloTestNow.Add(10*24*time.Hour)
	solo := func(id, cuid int, name string, withCheer bool) *Live {
		live := &Live{
			ID: id, Name: name, AssetBundleName: "vlentrance_00" + name,
			StartAt: ms(start), EndAt: ms(end),
			Schedules: []Schedule{
				{StartAt: ms(soloTestNow.Add(time.Duration(id-490) * time.Hour)), EndAt: ms(soloTestNow.Add(time.Duration(id-490)*time.Hour + 10*time.Minute))},
				{StartAt: ms(soloTestNow.Add(48 * time.Hour)), EndAt: ms(soloTestNow.Add(48*time.Hour + 10*time.Minute))},
			},
			Characters:      []Character{{GameCharacterUnitID: cuid, VirtualLivePerformanceType: "main_only"}},
			VirtualLiveType: "solo_virtual_live",
			GroupID:         2,
		}
		if withCheer {
			live.TotalCheerPointRewards = []CheerPointReward{{Threshold: 300, ResourceBoxID: 101001}, {Threshold: 900, ResourceBoxID: 101002}}
			live.SurplusReward = &SurplusReward{BasePoint: 10, ResourceBoxID: 1}
			live.OverrideCost = &OverrideCost{ResourceType: "material", ResourceID: 282, AssetBundleName: "virtual_cheer_coin"}
		}
		return live
	}
	return []*Live{
		{ID: 480, Name: "Normal Live", StartAt: ms(start), EndAt: ms(end), VirtualLiveType: "normal", GroupID: 0,
			Schedules: []Schedule{{StartAt: ms(soloTestNow.Add(3 * time.Hour)), EndAt: ms(soloTestNow.Add(4 * time.Hour))}}},
		solo(491, 1, "ソロ（一歌）", true),
		solo(492, 2, "ソロ（咲希）", true),
		solo(493, 3, "ソロ（穂波）", false),
	}
}

func newSoloController(groups map[int]*Group, lives []*Live) *Controller {
	return NewController(&soloFakeSource{
		fakeSource: fakeSource{
			defaultRegion: renderregion.JP,
			lives:         map[renderregion.Value][]*Live{renderregion.JP: lives},
			characters: map[int]*masterdata.GameCharacterUnit{
				1: {ID: 1, GameCharacterID: 1}, 2: {ID: 2, GameCharacterID: 2}, 3: {ID: 3, GameCharacterID: 3},
			},
			resourceBoxes: map[int]*provider.ResourceBox{
				101001: {ID: 101001, Details: []provider.ResourceBoxDetail{{ResourceType: "material", ResourceID: 101, ResourceQuantity: 100}}},
				101002: {ID: 101002, Details: []provider.ResourceBoxDetail{{ResourceType: "jewel", ResourceQuantity: 50}}},
				1:      {ID: 1, Details: []provider.ResourceBoxDetail{{ResourceType: "material", ResourceID: 101, ResourceQuantity: 1}}},
			},
		},
		groups:    groups,
		materials: map[int]string{282: "バーチャルエールコイン"},
	}, renderregion.JP)
}

func TestBuildListRequestCollapsesSoloGroup(t *testing.T) {
	controller := newSoloController(map[int]*Group{
		2: {ID: 2, Name: "6th Anniversary スペシャルソロライブ", Type: "solo_virtual_live", AssetBundleName: "6th_anniversary_soro_live"},
	}, soloTestLives())
	req, err := controller.BuildListRequest(ListQuery{Region: "jp", Now: soloTestNow})
	if err != nil {
		t.Fatalf("BuildListRequest() error = %v", err)
	}
	if len(req.Lives) != 2 {
		t.Fatalf("expected the normal live and one solo group entry, got %+v", req.Lives)
	}
	normal, group := req.Lives[0], req.Lives[1]
	if normal.ID != 480 || normal.GroupID != nil || normal.VirtualLiveType != "" || normal.GroupName != "" {
		t.Fatalf("normal live must be unchanged: %+v", normal)
	}
	if group.ID != 2 || group.Name != "6th Anniversary スペシャルソロライブ" || group.GroupName != group.Name ||
		group.GroupCount == nil || *group.GroupCount != 3 || group.GroupID == nil || *group.GroupID != 2 || group.VirtualLiveType != "solo_virtual_live" {
		t.Fatalf("unexpected group entry: %+v", group)
	}
	if group.RestCount != 6 || len(group.Characters) != 3 {
		t.Fatalf("group must sum rest counts and list one icon per member: %+v", group)
	}
	// The soonest upcoming schedule across the group (live 491, +1h).
	if group.CurrentStartAt != soloTestNow.Add(time.Hour).UnixMilli() {
		t.Fatalf("current window = %v", group.CurrentStartAt)
	}
}

func TestBuildListRequestCollapsesSoloGroupWithoutGroupTable(t *testing.T) {
	controller := newSoloController(nil, soloTestLives())
	req, err := controller.BuildListRequest(ListQuery{Region: "jp", Now: soloTestNow})
	if err != nil {
		t.Fatalf("BuildListRequest() error = %v", err)
	}
	if len(req.Lives) != 2 || req.Lives[1].Name != "ソロ（一歌）" || req.Lives[1].GroupCount == nil || *req.Lives[1].GroupCount != 3 {
		t.Fatalf("expected a collapsed entry named after the first live, got %+v", req.Lives)
	}
	text, err := controller.RenderText(ListQuery{Region: "jp", Now: soloTestNow})
	if err != nil || !strings.Contains(text, "【2】ソロ（一歌）") || !strings.Contains(text, "共3场个人Live") || strings.Contains(text, "【492】") {
		t.Fatalf("text list = %q err=%v", text, err)
	}
}

func TestBuildListRequestOldRegionUnchanged(t *testing.T) {
	// Pre-7.0.0 data: no solo lives, virtual_message lives keep their own rows.
	lives := soloTestLives()[:1]
	message := *lives[0]
	message.ID, message.VirtualLiveType, message.GroupID = 481, "virtual_message", 1
	lives = append(lives, &message)
	controller := newSoloController(map[int]*Group{1: {ID: 1, Name: "msg", Type: "virtual_message"}}, lives)
	req, err := controller.BuildListRequest(ListQuery{Region: "jp", Now: soloTestNow})
	if err != nil {
		t.Fatalf("BuildListRequest() error = %v", err)
	}
	data, _ := json.Marshal(req)
	if len(req.Lives) != 2 || strings.Contains(string(data), "group_") || strings.Contains(string(data), "virtual_live_type") {
		t.Fatalf("old-region request changed: %s", data)
	}
}

func TestBuildDetailRequestForSoloLive(t *testing.T) {
	controller := newSoloController(map[int]*Group{
		2: {ID: 2, Name: "6th Anniversary スペシャルソロライブ", Type: "solo_virtual_live"},
	}, soloTestLives())
	req, err := controller.BuildDetailRequest(DetailQuery{Region: "jp", Query: "491", Now: soloTestNow})
	if err != nil {
		t.Fatalf("BuildDetailRequest() error = %v", err)
	}
	if req.ID != 491 || req.Title != "ソロ（一歌）" || len(req.Lives) != 3 || req.Lives[0].ID != 491 || req.Lives[2].ID != 493 {
		t.Fatalf("unexpected detail header/lives: %+v", req)
	}
	if req.Lives[0].ScheduleCount == nil || *req.Lives[0].ScheduleCount != 2 || req.Lives[0].CharacterIconPath == "" {
		t.Fatalf("unexpected live row: %+v", req.Lives[0])
	}
	if len(req.TotalCheerPointRewards) != 2 || req.TotalCheerPointRewards[0].Threshold != 300 || len(req.TotalCheerPointRewards[0].Rewards) != 1 ||
		req.TotalCheerPointRewards[0].Rewards[0].Quantity != 100 {
		t.Fatalf("unexpected cheer point rewards: %+v", req.TotalCheerPointRewards)
	}
	if req.SurplusReward == nil || req.SurplusReward.BasePoint != 10 || len(req.SurplusReward.Rewards) != 1 {
		t.Fatalf("unexpected surplus reward: %+v", req.SurplusReward)
	}
	if req.OverrideCost == nil || req.OverrideCost.Name != "バーチャルエールコイン" || req.OverrideCost.ResourceID == nil || *req.OverrideCost.ResourceID != 282 ||
		!strings.Contains(req.OverrideCost.ImagePath, "material282.png") {
		t.Fatalf("unexpected override cost: %+v", req.OverrideCost)
	}
}

func TestBuildDetailRequestForGroupAndMissingCheerData(t *testing.T) {
	controller := newSoloController(nil, soloTestLives())
	group, err := controller.BuildDetailRequest(DetailQuery{Region: "jp", Query: "2", Now: soloTestNow})
	if err != nil {
		t.Fatalf("group detail error = %v", err)
	}
	if group.ID != 2 || len(group.Lives) != 3 || group.TotalCheerPointRewards != nil || group.SurplusReward != nil || group.OverrideCost == nil {
		t.Fatalf("group detail must list lives without per-live rewards: %+v", group)
	}
	keyword, err := controller.BuildDetailRequest(DetailQuery{Region: "jp", Query: "个人", Now: soloTestNow})
	if err != nil || keyword.ID != 2 {
		t.Fatalf("keyword detail = %+v err=%v", keyword, err)
	}
	// Live 493 has no cheer-point data (columns not ingested yet).
	bare, err := controller.BuildDetailRequest(DetailQuery{Region: "jp", Query: "493", Now: soloTestNow})
	if err != nil || bare.TotalCheerPointRewards != nil || bare.SurplusReward != nil || bare.OverrideCost != nil {
		t.Fatalf("detail without cheer data = %+v err=%v", bare, err)
	}
	if _, err := controller.BuildDetailRequest(DetailQuery{Region: "jp", Query: "480", Now: soloTestNow}); !errors.Is(err, ErrSoloLiveNotFound) {
		t.Fatalf("normal live id error = %v", err)
	}
}

func TestBuildDetailRequestOldRegionHasNoSoloLives(t *testing.T) {
	controller := newSoloController(nil, soloTestLives()[:1])
	if _, err := controller.BuildDetailRequest(DetailQuery{Region: "jp", Query: "491", Now: soloTestNow}); !errors.Is(err, ErrNoSoloLives) {
		t.Fatalf("old-region detail error = %v", err)
	}
}

func TestIsDetailQuery(t *testing.T) {
	for query, want := range map[string]bool{"": false, " 491 ": true, "0": false, "ソロ": true, "个人live": true, "SOLO": true, "abc": false} {
		if got := IsDetailQuery(query); got != want {
			t.Errorf("IsDetailQuery(%q) = %v, want %v", query, got, want)
		}
	}
}
