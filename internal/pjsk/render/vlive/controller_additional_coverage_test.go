package vlive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

type failingVLiveSource struct {
	err error
}

func (s failingVLiveSource) DefaultRegion() renderregion.Value { return renderregion.JP }
func (s failingVLiveSource) GetLives(renderregion.Value) ([]*Live, error) {
	return nil, s.err
}
func (failingVLiveSource) GetGameCharacterUnit(int) (*masterdata.GameCharacterUnit, error) {
	return nil, errors.New("character lookup failed")
}
func (failingVLiveSource) GetResourceBoxByPurpose(string, int) *provider.ResourceBox { return nil }

type noEventVLiveSource struct{ fakeSource }

func TestControllerGuardAndErrorBranches(t *testing.T) {
	var nilController *Controller
	nilController.RegisterSource(nil)
	if nilController.WithContext(context.Background()) != nil {
		t.Fatal("nil controller context clone should stay nil")
	}
	if _, _, err := nilController.ResolveLives(ListQuery{}); err == nil {
		t.Fatal("nil controller should reject resolution")
	}
	if _, err := nilController.RenderList(ListQuery{}); err == nil {
		t.Fatal("nil controller should reject rendering")
	}
	if got := nilController.resolveRegion(""); got != renderregion.JP {
		t.Fatalf("unexpected default region: %s", got)
	}

	controller := NewController(failingVLiveSource{err: errors.New("boom")}, renderregion.JP)
	controller.RegisterSource(nil)
	if _, _, err := controller.ResolveLives(ListQuery{}); err == nil || err.Error() != "boom" {
		t.Fatalf("expected source error, got %v", err)
	}
	if _, err := controller.RenderList(ListQuery{}); err == nil {
		t.Fatal("controller without drawing client should fail")
	}

	empty := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		lives:         map[renderregion.Value][]*Live{renderregion.JP: nil},
	}, renderregion.JP)
	if _, err := empty.BuildListRequest(ListQuery{Now: time.Now()}); !errors.Is(err, ErrNoLives) {
		t.Fatalf("expected ErrNoLives, got %v", err)
	}
}

func TestResolveLivesCoversFallbackWindowsAndSorting(t *testing.T) {
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	ms := func(value time.Time) int64 { return value.UnixMilli() }
	controller := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		lives: map[renderregion.Value][]*Live{renderregion.JP: {
			nil,
			{ID: 9, StartAt: 0, EndAt: ms(now.Add(time.Hour))},
			{ID: 3, Name: " ongoing ", StartAt: ms(now.Add(-time.Hour)), EndAt: ms(now.Add(time.Hour))},
			{ID: 2, StartAt: ms(now.Add(2 * time.Hour)), EndAt: ms(now.Add(3 * time.Hour))},
			{ID: 1, StartAt: ms(now.Add(2 * time.Hour)), EndAt: ms(now.Add(4 * time.Hour))},
		}},
	}, renderregion.JP)

	lives, region, err := controller.ResolveLives(ListQuery{Region: "", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if region != renderregion.JP || len(lives) != 3 {
		t.Fatalf("unexpected result: region=%s lives=%d", region, len(lives))
	}
	if lives[0].ID != 3 || !lives[0].Living || lives[0].Name != "ongoing" {
		t.Fatalf("unexpected ongoing live: %+v", lives[0])
	}
	if lives[1].ID != 1 || lives[2].ID != 2 {
		t.Fatalf("same-time lives should sort by id: %+v", lives)
	}
	if lives[1].Current == nil || lives[1].Living {
		t.Fatalf("future live should have a non-living fallback window: %+v", lives[1])
	}
}

func TestBannerRewardAndCharacterBranchHelpers(t *testing.T) {
	controller := NewController(nil, renderregion.JP)
	if got := controller.bannerPath(nil, renderregion.JP, ResolvedLive{}); got != "" {
		t.Fatalf("unexpected empty banner: %q", got)
	}
	if got := controller.eventBannerCandidates(nil, 1); got != nil {
		t.Fatalf("nil source returned candidates: %v", got)
	}
	if got := controller.eventBannerCandidates(&noEventVLiveSource{}, 1); got != nil {
		t.Fatalf("non-event source returned candidates: %v", got)
	}
	source := &fakeSource{events: map[int]*masterdata.Event{
		1: {VirtualLiveID: 1, AssetBundleName: " event_bundle "},
		2: {VirtualLiveID: 2},
	}}
	if got := controller.eventBannerCandidates(source, 0); got != nil {
		t.Fatalf("invalid id returned candidates: %v", got)
	}
	if got := controller.eventBannerCandidates(source, 2); got != nil {
		t.Fatalf("blank event bundle returned candidates: %v", got)
	}
	if got := controller.eventBannerCandidates(source, 3); got != nil {
		t.Fatalf("missing event returned candidates: %v", got)
	}
	if got := controller.eventBannerCandidates(source, 1); len(got) != 3 || !strings.Contains(got[0], "event_bundle") {
		t.Fatalf("unexpected event banner candidates: %v", got)
	}

	if got := controller.buildRewardItems(nil, ResolvedLive{}); got != nil {
		t.Fatalf("nil source returned rewards: %v", got)
	}
	source.resourceBoxes = map[int]*provider.ResourceBox{
		4: {Details: []provider.ResourceBoxDetail{
			{ResourceType: "unknown", ResourceQuantity: 4},
			{ResourceType: "paid_jewel", ResourceQuantity: 0},
		}},
	}
	rewards := controller.buildRewardItems(source, ResolvedLive{Rewards: []Reward{
		{VirtualLiveType: "special", ResourceBoxID: 4},
		{VirtualLiveType: "normal", ResourceBoxID: 99},
		{ResourceBoxID: 4},
	}})
	if len(rewards) != 1 || rewards[0].Quantity != 1 || !strings.Contains(rewards[0].ImagePath, "jewel.png") {
		t.Fatalf("unexpected rewards: %+v", rewards)
	}
	if got := controller.rewardImagePath("material", 0); got != "" {
		t.Fatalf("invalid material produced path: %q", got)
	}
	if got := controller.rewardImagePath("virtual_coin", 0); !strings.Contains(got, "virtual_coin.png") {
		t.Fatalf("unexpected virtual coin path: %q", got)
	}
	if got := controller.rewardImagePath("other", 1); got != "" {
		t.Fatalf("unknown reward produced path: %q", got)
	}

	if got := controller.buildCharacterItems(nil, ResolvedLive{}); got != nil {
		t.Fatalf("nil source returned characters: %v", got)
	}
	characters := controller.buildCharacterItems(failingVLiveSource{}, ResolvedLive{Characters: []Character{
		{GameCharacterUnitID: 1, VirtualLivePerformanceType: "guest"},
		{GameCharacterUnitID: 2, VirtualLivePerformanceType: "both"},
	}})
	if len(characters) != 0 {
		t.Fatalf("failed character lookups produced items: %+v", characters)
	}
	source.characters = map[int]*masterdata.GameCharacterUnit{
		1: {ID: 1, GameCharacterID: 0},
		2: {ID: 2, GameCharacterID: 999},
	}
	characters = controller.buildCharacterItems(source, ResolvedLive{Characters: []Character{
		{GameCharacterUnitID: 1},
		{GameCharacterUnitID: 2},
		{GameCharacterUnitID: 2},
	}})
	if len(characters) != 1 || !strings.Contains(characters[0].IconPath, "chr_icon_999.png") {
		t.Fatalf("unexpected character items: %+v", characters)
	}
}

func TestScheduleAndNameHelpersCoverBoundaries(t *testing.T) {
	second := int64(1_700_000_000)
	millis := second * 1000
	if unixTime(0) != (time.Time{}) || unixTime(second).Unix() != second || unixTime(millis).UnixMilli() != millis {
		t.Fatal("unixTime did not handle boundary formats")
	}
	items := normalizeSchedules([]Schedule{
		{},
		{StartAt: millis + 3000, EndAt: millis + 2000},
		{StartAt: millis + 1000, EndAt: millis + 3000},
		{StartAt: millis + 1000, EndAt: millis + 2000},
	})
	if len(items) != 2 || !items[0].EndAt.Before(items[1].EndAt) {
		t.Fatalf("unexpected normalized schedules: %+v", items)
	}
	if got := fallbackLiveName("   ", 7); got != "Virtual Live #7" {
		t.Fatalf("unexpected fallback name: %q", got)
	}
	if got := fallbackLiveName("named", 7); got != "named" {
		t.Fatalf("unexpected explicit name: %q", got)
	}
}

func TestProviderAdapterMapsLocalProviderData(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"virtualLives.json": `[{
			"id":11,"name":"Mapped","assetbundleName":"mapped_bundle",
			"startAt":1700000000000,"endAt":1700003600000,
			"virtualLiveSchedules":[{"startAt":1700000000000,"endAt":1700001800000}],
			"virtualLiveRewards":[{"virtualLiveType":"normal","resourceBoxId":7}],
			"virtualLiveCharacters":[{"gameCharacterUnitId":21,"virtualLivePerformanceType":"both"}]
		}]`,
		"gameCharacterUnits.json": `[{"id":21,"gameCharacterId":21,"unit":"piapro"}]`,
		"events.json":             `[{"id":3,"name":"Event","assetbundleName":"event_bundle","virtualLiveId":11}]`,
		"resourceBoxes.json": `[{
			"ID":7,"ResourceBoxPurpose":"virtual_live_reward",
			"Details":[{"resourceType":"jewel","resourceQuantity":100}]
		}]`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	adapter := NewProviderAdapter(provider.NewLocalProvider(dir, renderregion.JP))
	if (*ProviderAdapter)(nil).WithContext(context.Background()) != nil {
		t.Fatal("nil adapter context clone should stay nil")
	}
	contextual, ok := adapter.WithContext(context.Background()).(*ProviderAdapter)
	if !ok {
		t.Fatal("context clone did not return a provider adapter")
	}
	lives, err := contextual.GetLives(renderregion.JP)
	if err != nil {
		t.Fatal(err)
	}
	if len(lives) != 1 || len(lives[0].Schedules) != 1 || len(lives[0].Rewards) != 1 || len(lives[0].Characters) != 1 {
		t.Fatalf("adapter did not map nested data: %+v", lives)
	}
	if unit, err := contextual.GetGameCharacterUnit(21); err != nil || unit.GameCharacterID != 21 {
		t.Fatalf("unexpected character unit: unit=%+v err=%v", unit, err)
	}
	if event, err := contextual.GetEventByVirtualLiveID(11); err != nil || event.ID != 3 {
		t.Fatalf("unexpected event: event=%+v err=%v", event, err)
	}
	if _, err := contextual.GetEventByVirtualLiveID(0); err == nil {
		t.Fatal("zero live id should fail")
	}
	if _, err := contextual.GetEventByVirtualLiveID(999); err == nil {
		t.Fatal("missing live event should fail")
	}
	if box := contextual.GetResourceBoxByPurpose("virtual_live_reward", 7); box == nil || box.ID != 7 {
		t.Fatalf("unexpected resource box: %+v", box)
	}
}
