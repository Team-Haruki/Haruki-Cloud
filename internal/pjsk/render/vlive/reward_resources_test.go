package vlive

import (
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

type rowFakeSource struct {
	*fakeSource
	rows map[string]map[int]map[string]any
}

func (s *rowFakeSource) MasterRows(filename string) (map[int]map[string]any, bool) {
	rows, ok := s.rows[filename]
	return rows, ok
}

func TestBuildRewardItemsResolvesJP700ResourceTypes(t *testing.T) {
	box := map[int]*provider.ResourceBox{
		4: {Details: []provider.ResourceBoxDetail{
			{ResourceType: "virtual_item", ResourceID: 222, ResourceQuantity: 1},
			{ResourceType: "honor_background", ResourceID: 10101, ResourceQuantity: 1},
		}},
	}
	jp := &rowFakeSource{
		fakeSource: &fakeSource{defaultRegion: renderregion.JP, resourceBoxes: box},
		rows: map[string]map[int]map[string]any{
			"virtualItems.json":     {222: {"id": 222, "assetbundleName": "fan_ichika"}},
			"honorBackgrounds.json": {10101: {"id": 10101, "assetbundleName": "honor_bg_style_01_01"}},
		},
	}
	controller := NewController(jp, renderregion.JP)
	rewards := controller.buildRewardItems(jp, ResolvedLive{Rewards: []Reward{{ResourceBoxID: 4}}})
	if len(rewards) != 2 || !strings.HasSuffix(rewards[0].ImagePath, "thumbnail/virtual_live_item/fan_ichika.png") ||
		!strings.HasSuffix(rewards[1].ImagePath, "honor_background/honor_bg_style_01_01/degree_sub.png") {
		t.Fatalf("rewards = %+v", rewards)
	}

	// A region without the tables skips the rewards exactly as before.
	old := &fakeSource{defaultRegion: renderregion.EN, resourceBoxes: box}
	if got := NewController(old, renderregion.EN).buildRewardItems(old, ResolvedLive{Rewards: []Reward{{ResourceBoxID: 4}}}); len(got) != 0 {
		t.Fatalf("old region rewards = %+v", got)
	}
}
