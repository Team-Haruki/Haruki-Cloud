package sk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

type controllerImageIndex struct{}

func (controllerImageIndex) LookupRender(_ context.Context, key string) (imagecache.RenderIndexEntry, bool, error) {
	return imagecache.RenderIndexEntry{
		RequestKey: key, ContentHash: strings.Repeat("a", 64),
		Entry: imagecache.ImageEntry{CDNPath: "pjsk/sk/cached.png", StorageBackend: imagecache.BackendGarage, MediaType: "image/png"},
	}, true, nil
}

func (controllerImageIndex) TouchRender(context.Context, []string) (int64, error) { return 0, nil }
func (controllerImageIndex) DeleteExpiredRender(context.Context, []string, time.Time) (int64, error) {
	return 0, nil
}

func newImageResultController(t *testing.T) (*Controller, *storagetest.Memory) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("cache hit must not request Drawing")
		http.Error(w, "unexpected render", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	objects := storagetest.NewMemory()
	objects.Seed(map[string][]byte{"pjsk/sk/cached.png": []byte("cached-image")})
	client := drawing.NewHarukiDrawingClient(server.URL, drawing.WithRetryCount(0))
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{TTL: time.Hour, Index: controllerImageIndex{}, Artifacts: objects})
	client.SetRenderCache(cache)
	t.Cleanup(func() { _ = client.Close() })
	return NewController(client).WithContext(t.Context()), objects
}

func TestImageRenderersPreserveArtifactUntilBytesRequested(t *testing.T) {
	controller, objects := newImageResultController(t)
	ranks := []drawing.RankInfo{{Rank: 1}}
	calls := map[string]func() (drawing.ImageResult, error){
		"line": func() (drawing.ImageResult, error) {
			return controller.RenderLineImage(LineRequest{SklRequest: drawing.SklRequest{Region: "jp", Ranks: ranks}, Full: true})
		},
		"query": func() (drawing.ImageResult, error) {
			return controller.RenderQueryImage(drawing.SKRequest{Region: "jp", Ranks: ranks})
		},
		"check-room": func() (drawing.ImageResult, error) {
			return controller.RenderCheckRoomImage(drawing.CFRequest{Region: "jp", Ranks: ranks})
		},
		"csb": func() (drawing.ImageResult, error) {
			return controller.RenderCSBImage(drawing.CSBRequest{Region: "jp", Ranks: ranks})
		},
		"speed": func() (drawing.ImageResult, error) {
			return controller.RenderSpeedImage(drawing.SpeedRequest{Region: "jp", Ranks: []drawing.SpeedInfo{{Rank: 1}}})
		},
		"player-trace": func() (drawing.ImageResult, error) {
			return controller.RenderPlayerTraceImage(drawing.PlayerTraceRequest{Region: "jp", Ranks: ranks})
		},
		"rank-trace": func() (drawing.ImageResult, error) {
			return controller.RenderRankTraceImage(drawing.RankTraceRequest{Region: "jp", Ranks: ranks})
		},
		"winrate": func() (drawing.ImageResult, error) {
			return controller.RenderWinRateImage(drawing.WinRateRequest{TeamInfo: []drawing.TeamInfo{{TeamID: 1}}})
		},
	}
	for name, render := range calls {
		t.Run(name, func(t *testing.T) {
			image, err := render()
			if err != nil || image.Ref() == nil {
				t.Fatalf("image ref = %+v, err = %v", image.Ref(), err)
			}
			if got := objects.Calls(); len(got) != 0 {
				t.Fatalf("Image renderer downloaded cached bytes: %+v", got)
			}
		})
	}
	data, err := controller.RenderQuery(drawing.SKRequest{Region: "jp", Ranks: ranks})
	if err != nil || string(data) != "cached-image" || len(objects.Calls()) != 1 {
		t.Fatalf("legacy renderer bytes = %q, calls = %+v, err = %v", data, objects.Calls(), err)
	}
}

func TestPredictLineImagePreservesArtifactThroughTrackerChain(t *testing.T) {
	controller, objects := newImageResultController(t)
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{ID: 101, StartAt: now - int64(time.Hour/time.Millisecond), AggregateAt: now + int64(2*time.Hour/time.Millisecond)}
	setTestTrackerIntegration(controller, &lineMetricsOnlyTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)
	controller.SetForecastProvider(&countingForecastProvider{bySource: map[string]ForecastSourceData{
		"33kit": {Scores: map[int]ForecastScore{50: {Score: 123456, Timestamp: now / 1000, Source: "33kit"}}, FetchedAt: now / 1000},
	}})
	if err := controller.forecastCache.RefreshNow(t.Context(), "jp", 101); err != nil {
		t.Fatal(err)
	}
	image, err := controller.RenderPredictLineFromTrackerImage(TrackerRankQuery{EventID: 101, Region: "jp", Ranks: []int{50}})
	if err != nil || image.Ref() == nil || len(objects.Calls()) != 0 {
		t.Fatalf("image ref = %+v, calls = %+v, err = %v", image.Ref(), objects.Calls(), err)
	}
}
