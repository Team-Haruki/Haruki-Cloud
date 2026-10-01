package drawing

import (
	"context"
	"testing"
	"time"
)

func infoPanelCacheKey(t *testing.T, request any, now time.Time) string {
	t.Helper()
	prepared := prepareDrawingRequestBody(InfoPanelEndpoint, request, now, context.Background())
	policy, err := buildRenderCachePolicy(InfoPanelEndpoint, preparedRenderCachePayload{payload: prepared})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	key, err := buildRenderCacheKey(policy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey: %v", err)
	}
	return key
}

func TestInfoPanelRenderCacheRuleReusesOnlyWithinAMinute(t *testing.T) {
	rule := resolveRenderCacheRule(InfoPanelEndpoint)
	if !rule.Enabled || rule.TTL != renderCacheTTLTwoHour {
		t.Fatalf("info panel rule = %+v", rule)
	}
	updated := int64(1790838000000)
	request := &ProfileCardRequest{
		Profile:     &BasicProfile{ID: "1", Region: "JP", Nickname: "Panel"},
		DataSources: []ProfileDataSource{{Name: "Suite数据", UpdateTime: &updated}},
	}
	base := time.UnixMilli(1790841600000) // on a minute boundary
	first := infoPanelCacheKey(t, request, base)
	if got := infoPanelCacheKey(t, request, base.Add(30*time.Second)); got != first {
		t.Fatal("renders in the same minute should share a cache key")
	}
	if got := infoPanelCacheKey(t, request, base.Add(time.Minute)); got == first {
		t.Fatal("the next minute's DT must not reuse the cached panel")
	}
}

func TestInfoPanelRequestNormalizesRootDataSourceTimes(t *testing.T) {
	seconds := int64(1790838000)
	request := &ProfileCardRequest{
		Profile:     &BasicProfile{ID: "1", Region: "JP", Nickname: "Panel"},
		DataSources: []ProfileDataSource{{Name: "MySekai数据", UpdateTime: &seconds}},
	}
	prepared := prepareDrawingRequestBody(InfoPanelEndpoint, request, time.UnixMilli(1790841600000), context.Background())
	root := mapAt(prepared)
	if root == nil {
		t.Fatalf("prepared = %#v", prepared)
	}
	if got := root["dt"]; got != int64(1790841600000) {
		t.Fatalf("dt = %#v", got)
	}
	source := mapAt(sliceAt(root, "data_sources")[0])
	if got := source["update_time"]; got != seconds*1000 {
		t.Fatalf("data source update_time = %#v, want ms", got)
	}
}
