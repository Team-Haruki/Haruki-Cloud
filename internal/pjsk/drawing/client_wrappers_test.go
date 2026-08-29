package drawing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/core/upstream"
)

func TestDrawingClientConstructorsAndNilBehavior(t *testing.T) {
	if got := NewHarukiDrawingClientWithTargets("", nil); got != nil {
		t.Fatalf("strict client without targets = %#v", got)
	}
	client := NewHarukiDrawingClient("", nil, WithTimeout(3*time.Second), WithRetryCount(2))
	if client == nil || client.client.GetClient().Timeout != 3*time.Second || client.client.RetryCount != 2 {
		t.Fatalf("configured client = %#v", client)
	}
	if _, err := client.postPrepared("/render", map[string]any{}); err == nil || !strings.Contains(err.Error(), "base_url is empty") {
		t.Fatalf("empty base URL error = %v", err)
	}
	client.SetRenderCache(nil)
	var nilClient *HarukiDrawingClient
	nilClient.SetRenderCache(nil)
	if nilClient.WithContext(context.Background()) != nil {
		t.Fatal("nil client context clone is non-nil")
	}
	if _, err := nilClient.postPrepared("/render", nil); err == nil {
		t.Fatal("nil client post succeeded")
	}
	data, err := nilClient.RenderWithCache("/api/pjsk/profile", map[string]any{"id": 1}, func(any) ([]byte, error) {
		return []byte("nil-render"), nil
	})
	if err != nil || string(data) != "nil-render" {
		t.Fatalf("nil client custom render = %q, %v", data, err)
	}
	wantErr := errors.New("prepare failed")
	_, err = nilClient.RenderWithCacheAndPrepare("/api/pjsk/profile", map[string]any{"id": 2}, func(any) error {
		return wantErr
	}, func(any) ([]byte, error) {
		return nil, nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("nil client prepare error = %v", err)
	}

	targetClient := NewHarukiDrawingClientWithTargetsAndResources("", []upstream.TargetConfig{{BaseURL: "http://drawing.invalid"}}, nil)
	if targetClient == nil || targetClient.baseURL != "http://drawing.invalid" {
		t.Fatalf("target client = %#v", targetClient)
	}
}

func TestDrawingClientEndpointWrappers(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		seen[request.URL.RequestURI()]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("image"))
	}))
	defer server.Close()
	client := NewHarukiDrawingClient(server.URL).WithContext(context.Background())

	calls := []struct {
		name string
		call func() ([]byte, error)
	}{
		{"music detail", func() ([]byte, error) { return client.GenerateMusicDetail(nil) }},
		{"music brief", func() ([]byte, error) { return client.GenerateMusicBriefList(nil) }},
		{"music list", func() ([]byte, error) { return client.GenerateMusicList(nil, true, false) }},
		{"music progress", func() ([]byte, error) { return client.GeneratePlayProgress(nil) }},
		{"music rewards detail", func() ([]byte, error) { return client.GenerateDetailMusicRewards(nil) }},
		{"music rewards basic", func() ([]byte, error) { return client.GenerateBasicMusicRewards(nil) }},
		{"profile", func() ([]byte, error) { return client.GenerateProfile(nil) }},
		{"modular profile", func() ([]byte, error) { return client.GenerateModularProfile(nil) }},
		{"custom profile", func() ([]byte, error) { return client.GenerateCustomProfileCard(nil) }},
		{"card detail", func() ([]byte, error) { return client.GenerateCardDetail(nil) }},
		{"card list", func() ([]byte, error) { return client.GenerateCardList(nil) }},
		{"card box", func() ([]byte, error) { return client.GenerateCardBox(nil) }},
		{"costume list", func() ([]byte, error) { return client.GenerateCostumeList(nil) }},
		{"costume detail", func() ([]byte, error) { return client.GenerateCostumeDetail(nil) }},
		{"costume prepared", func() ([]byte, error) {
			return client.GenerateCostumeDetailWithPrepare(map[string]any{"variant": 1}, nil, func(any) error { return nil })
		}},
		{"costume context prepared", func() ([]byte, error) {
			return client.GenerateCostumeDetailWithContextPrepare(map[string]any{"variant": 2}, nil, func(context.Context, any) error { return nil })
		}},
		{"deck", func() ([]byte, error) { return client.GenerateDeckRecommendation(nil) }},
		{"challenge", func() ([]byte, error) { return client.GenerateChallengeLiveDetails(nil) }},
		{"power bonus", func() ([]byte, error) { return client.GeneratePowerBonusDetail(nil) }},
		{"area item", func() ([]byte, error) { return client.GenerateAreaItemUpgradeMaterials(nil) }},
		{"bonds", func() ([]byte, error) { return client.GenerateBonds(nil) }},
		{"leader count", func() ([]byte, error) { return client.GenerateLeaderCount(nil) }},
		{"mission overview", func() ([]byte, error) { return client.GenerateCharacterMissionOverview(nil) }},
		{"mission all", func() ([]byte, error) { return client.GenerateCharacterMissionAll(nil) }},
		{"inventory", func() ([]byte, error) { return client.GenerateInventoryList(nil) }},
		{"event detail", func() ([]byte, error) { return client.GenerateEventDetail(nil) }},
		{"event record", func() ([]byte, error) { return client.GenerateEventRecord(nil) }},
		{"event list", func() ([]byte, error) { return client.GenerateEventList(nil) }},
		{"event planner", func() ([]byte, error) { return client.GenerateEventPlanner(nil) }},
		{"vlive", func() ([]byte, error) { return client.GenerateVLiveList(nil) }},
		{"gacha list", func() ([]byte, error) { return client.GenerateGachaList(nil) }},
		{"gacha detail", func() ([]byte, error) { return client.GenerateGachaDetail(nil) }},
		{"honor", func() ([]byte, error) { return client.GenerateHonor(nil) }},
		{"birthday", func() ([]byte, error) { return client.GenerateCharacterBirthday(nil) }},
		{"alias", func() ([]byte, error) { return client.GenerateAliasList(nil) }},
		{"help", func() ([]byte, error) { return client.GenerateCommandHelp(nil) }},
		{"mysekai resource", func() ([]byte, error) { return client.GenerateMysekaiResource(nil) }},
		{"mysekai map", func() ([]byte, error) { return client.GenerateMysekaiMap(nil) }},
		{"fixture list", func() ([]byte, error) { return client.GenerateMysekaiFixtureList(nil) }},
		{"fixture detail", func() ([]byte, error) { return client.GenerateMysekaiFixtureDetail(nil) }},
		{"door upgrade", func() ([]byte, error) { return client.GenerateMysekaiDoorUpgrade(nil) }},
		{"music record", func() ([]byte, error) { return client.GenerateMysekaiMusicRecord(nil) }},
		{"talk list", func() ([]byte, error) { return client.GenerateMysekaiTalkList(nil) }},
		{"housing", func() ([]byte, error) { return client.GenerateMysekaiHousingCompetition(nil) }},
		{"score control", func() ([]byte, error) { return client.GenerateScoreControl(nil) }},
		{"custom room", func() ([]byte, error) { return client.GenerateCustomRoomScore(nil) }},
		{"music meta", func() ([]byte, error) { return client.GenerateMusicMeta(nil) }},
		{"music board", func() ([]byte, error) { return client.GenerateMusicBoard(nil) }},
		{"stamp", func() ([]byte, error) { return client.GenerateStampList(nil) }},
		{"chart", func() ([]byte, error) { return client.GenerateMusicChart(nil) }},
		{"sk line", func() ([]byte, error) { return client.GenerateSKLine(nil, true) }},
		{"sk query", func() ([]byte, error) { return client.GenerateSKQuery(nil) }},
		{"sk room", func() ([]byte, error) { return client.GenerateSKCheckRoom(nil) }},
		{"sk csb", func() ([]byte, error) { return client.GenerateSKCSB(nil) }},
		{"sk speed", func() ([]byte, error) { return client.GenerateSKSpeed(nil) }},
		{"sk player trace", func() ([]byte, error) { return client.GenerateSKPlayerTrace(nil) }},
		{"sk rank trace", func() ([]byte, error) { return client.GenerateSKRankTrace(nil) }},
		{"sk winrate", func() ([]byte, error) { return client.GenerateSKWinRate(nil) }},
	}
	for _, item := range calls {
		t.Run(item.name, func(t *testing.T) {
			data, err := item.call()
			if err != nil || string(data) != "image" {
				t.Fatalf("result = %q, %v", data, err)
			}
		})
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{
		"/api/pjsk/music/list?show_id=true&show_leak=false",
		"/api/pjsk/profile",
		"/api/pjsk/event/detail",
		"/api/pjsk/mysekai/housing-competition",
		"/api/pjsk/sk/line?full=true",
	} {
		if seen[path] == 0 {
			t.Fatalf("endpoint %q was not requested; seen=%v", path, seen)
		}
	}
}

func TestDrawingResponseClassificationHelpers(t *testing.T) {
	if got := drawingResponseErrorDetail([]byte("not json")); got != "" {
		t.Fatalf("invalid detail = %q", got)
	}
	if got := drawingResponseErrorDetail([]byte(`{"detail":""}`)); got != "" {
		t.Fatalf("empty detail = %q", got)
	}
	got := drawingResponseErrorDetail([]byte(`{"detail":"line\nbreak\t` + strings.Repeat("x", 600) + `"}`))
	if strings.ContainsAny(got, "\n\t") || !strings.HasSuffix(got, "...") || len(got) > drawingErrorDetailMaxBytes+3 {
		t.Fatalf("bounded detail = %q (len=%d)", got, len(got))
	}
	for _, body := range []string{"DATA INSUFFICIENT", "not enough data", "数据不足", "index out of range"} {
		if !drawingResponseIndicatesInsufficientData([]byte(body)) {
			t.Fatalf("insufficient marker %q not detected", body)
		}
	}
	if drawingResponseIndicatesInsufficientData([]byte("ordinary error")) {
		t.Fatal("ordinary error classified as insufficient data")
	}
}
