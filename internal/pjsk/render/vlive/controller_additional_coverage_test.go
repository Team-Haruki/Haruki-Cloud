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
	"haruki-cloud/internal/testutil"
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
	testutil.RequireArgs(t, !(nilController.WithContext(context.Background()) != nil), "nil controller context clone should stay nil")
	{

		_, _, err := nilController.ResolveLives(ListQuery{})
		testutil.RequireArgs(t, !(err == nil), "nil controller should reject resolution")
	}
	{

		_, err := nilController.RenderList(ListQuery{})
		testutil.RequireArgs(t, !(err == nil), "nil controller should reject rendering")
	}
	{

		got := nilController.resolveRegion("")
		testutil.Require(t, !(got != renderregion.JP), "unexpected default region: %s", got)
	}

	controller := NewController(failingVLiveSource{err: errors.New("boom")}, renderregion.JP)
	controller.RegisterSource(nil)
	{
		_, _, err := controller.ResolveLives(ListQuery{})
		{
			testutil.Require(t, !(err == nil), "expected source error, got %v", err)
			testutil.Require(t, !(err.Error() != "boom"), "expected source error, got %v", err)
		}
	}
	{

		_, err := controller.RenderList(ListQuery{})
		testutil.RequireArgs(t, !(err == nil), "controller without drawing client should fail")
	}

	empty := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		lives:         map[renderregion.Value][]*Live{renderregion.JP: nil},
	}, renderregion.JP)
	{
		_, err := empty.BuildListRequest(ListQuery{Now: time.Now()})
		testutil.Require(t, errors.Is(err, ErrNoLives), "expected ErrNoLives, got %v", err)
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
	testutil.RequireArgs(t, !(err != nil), err)
	{

		testutil.Require(t, !(region != renderregion.JP), "unexpected result: region=%s lives=%d", region, len(lives))
		testutil.Require(t, !(len(lives) != 3), "unexpected result: region=%s lives=%d", region, len(lives))
	}
	{
		testutil.Require(t, !(lives[0].ID != 3), "unexpected ongoing live: %+v", lives[0])
		testutil.Require(t, lives[0].Living, "unexpected ongoing live: %+v", lives[0])
		testutil.Require(t, !(lives[0].Name != "ongoing"), "unexpected ongoing live: %+v", lives[0])
	}
	{
		testutil.Require(t, !(lives[1].ID != 1), "same-time lives should sort by id: %+v", lives)
		testutil.Require(t, !(lives[2].ID != 2), "same-time lives should sort by id: %+v", lives)
	}
	{
		testutil.Require(t, !(lives[1].Current == nil), "future live should have a non-living fallback window: %+v", lives[1])
		testutil.Require(t, !(lives[1].Living), "future live should have a non-living fallback window: %+v", lives[1])
	}

}

func TestBannerRewardAndCharacterBranchHelpers(t *testing.T) {
	controller := NewController(nil, renderregion.JP)
	{
		got := controller.bannerPath(nil, renderregion.JP, ResolvedLive{})
		testutil.Require(t, !(got != ""), "unexpected empty banner: %q", got)
	}
	{

		got := controller.eventBannerCandidates(nil, 1)
		testutil.Require(t, !(got != nil), "nil source returned candidates: %v", got)
	}
	{

		got := controller.eventBannerCandidates(&noEventVLiveSource{}, 1)
		testutil.Require(t, !(got != nil), "non-event source returned candidates: %v", got)
	}

	source := &fakeSource{events: map[int]*masterdata.Event{
		1: {VirtualLiveID: 1, AssetBundleName: " event_bundle "},
		2: {VirtualLiveID: 2},
	}}
	{
		got := controller.eventBannerCandidates(source, 0)
		testutil.Require(t, !(got != nil), "invalid id returned candidates: %v", got)
	}
	{

		got := controller.eventBannerCandidates(source, 2)
		testutil.Require(t, !(got != nil), "blank event bundle returned candidates: %v", got)
	}
	{

		got := controller.eventBannerCandidates(source, 3)
		testutil.Require(t, !(got != nil), "missing event returned candidates: %v", got)
	}
	{

		got := controller.eventBannerCandidates(source, 1)
		{
			testutil.Require(t, !(len(got) != 3), "unexpected event banner candidates: %v", got)
			testutil.Require(t, strings.Contains(got[0], "event_bundle"), "unexpected event banner candidates: %v", got)
		}
	}
	{

		got := controller.buildRewardItems(nil, ResolvedLive{})
		testutil.Require(t, !(got != nil), "nil source returned rewards: %v", got)
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
	{
		testutil.Require(t, !(len(rewards) != 1), "unexpected rewards: %+v", rewards)
		testutil.Require(t, !(rewards[0].Quantity != 1), "unexpected rewards: %+v", rewards)
		testutil.Require(t, strings.Contains(rewards[0].ImagePath, "jewel.png"), "unexpected rewards: %+v", rewards)
	}
	{

		got := controller.rewardImagePath("material", 0)
		testutil.Require(t, !(got != ""), "invalid material produced path: %q", got)
	}
	{

		got := controller.rewardImagePath("virtual_coin", 0)
		testutil.Require(t, strings.Contains(got, "virtual_coin.png"), "unexpected virtual coin path: %q", got)
	}
	{

		got := controller.rewardImagePath("other", 1)
		testutil.Require(t, !(got != ""), "unknown reward produced path: %q", got)
	}
	{

		got := controller.buildCharacterItems(nil, ResolvedLive{})
		testutil.Require(t, !(got != nil), "nil source returned characters: %v", got)
	}

	characters := controller.buildCharacterItems(failingVLiveSource{}, ResolvedLive{Characters: []Character{
		{GameCharacterUnitID: 1, VirtualLivePerformanceType: "guest"},
		{GameCharacterUnitID: 2, VirtualLivePerformanceType: "both"},
	}})
	testutil.Require(t, !(len(characters) != 0), "failed character lookups produced items: %+v", characters)

	source.characters = map[int]*masterdata.GameCharacterUnit{
		1: {ID: 1, GameCharacterID: 0},
		2: {ID: 2, GameCharacterID: 999},
	}
	characters = controller.buildCharacterItems(source, ResolvedLive{Characters: []Character{
		{GameCharacterUnitID: 1},
		{GameCharacterUnitID: 2},
		{GameCharacterUnitID: 2},
	}})
	{
		testutil.Require(t, !(len(characters) != 1), "unexpected character items: %+v", characters)
		testutil.Require(t, strings.Contains(characters[0].IconPath, "chr_icon_999.png"), "unexpected character items: %+v", characters)
	}

}

func TestScheduleAndNameHelpersCoverBoundaries(t *testing.T) {
	second := int64(1_700_000_000)
	millis := second * 1000
	{
		testutil.RequireArgs(t, !(unixTime(0) != (time.Time{})), "unixTime did not handle boundary formats")
		testutil.RequireArgs(t, !(unixTime(second).Unix() != second), "unixTime did not handle boundary formats")
		testutil.RequireArgs(t, !(unixTime(millis).UnixMilli() != millis), "unixTime did not handle boundary formats")
	}

	items := normalizeSchedules([]Schedule{
		{},
		{StartAt: millis + 3000, EndAt: millis + 2000},
		{StartAt: millis + 1000, EndAt: millis + 3000},
		{StartAt: millis + 1000, EndAt: millis + 2000},
	})
	{
		testutil.Require(t, !(len(items) != 2), "unexpected normalized schedules: %+v", items)
		testutil.Require(t, items[0].EndAt.Before(items[1].EndAt), "unexpected normalized schedules: %+v", items)
	}
	{

		got := fallbackLiveName("   ", 7)
		testutil.Require(t, !(got != "Virtual Live #7"), "unexpected fallback name: %q", got)
	}
	{

		got := fallbackLiveName("named", 7)
		testutil.Require(t, !(got != "named"), "unexpected explicit name: %q", got)
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
		{
			err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
			testutil.RequireArgs(t, !(err != nil), err)
		}

	}

	adapter := NewProviderAdapter(provider.NewLocalProvider(dir, renderregion.JP))
	testutil.RequireArgs(t, !((*ProviderAdapter)(nil).WithContext(context.Background()) != nil), "nil adapter context clone should stay nil")

	contextual, ok := adapter.WithContext(context.Background()).(*ProviderAdapter)
	testutil.RequireArgs(t, ok, "context clone did not return a provider adapter")

	lives, err := contextual.GetLives(renderregion.JP)
	testutil.RequireArgs(t, !(err != nil), err)
	{

		testutil.Require(t, !(len(lives) != 1), "adapter did not map nested data: %+v", lives)
		testutil.Require(t, !(len(lives[0].Schedules) != 1), "adapter did not map nested data: %+v", lives)
		testutil.Require(t, !(len(lives[0].Rewards) != 1), "adapter did not map nested data: %+v", lives)
		testutil.Require(t, !(len(lives[0].Characters) != 1), "adapter did not map nested data: %+v", lives)
	}
	{

		unit, err := contextual.GetGameCharacterUnit(21)
		{
			testutil.Require(t, !(err != nil), "unexpected character unit: unit=%+v err=%v", unit, err)
			testutil.Require(t, !(unit.GameCharacterID != 21), "unexpected character unit: unit=%+v err=%v", unit, err)
		}
	}
	{

		event, err := contextual.GetEventByVirtualLiveID(11)
		{
			testutil.Require(t, !(err != nil), "unexpected event: event=%+v err=%v", event, err)
			testutil.Require(t, !(event.ID != 3), "unexpected event: event=%+v err=%v", event, err)
		}
	}
	{

		_, err := contextual.GetEventByVirtualLiveID(0)
		testutil.RequireArgs(t, !(err == nil), "zero live id should fail")
	}
	{

		_, err := contextual.GetEventByVirtualLiveID(999)
		testutil.RequireArgs(t, !(err == nil), "missing live event should fail")
	}
	{

		box := contextual.GetResourceBoxByPurpose("virtual_live_reward", 7)
		{
			testutil.Require(t, !(box == nil), "unexpected resource box: %+v", box)
			testutil.Require(t, !(box.ID != 7), "unexpected resource box: %+v", box)
		}
	}

}
