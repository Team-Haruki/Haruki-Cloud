package mysekai

import (
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

type fakeHousingCompetitionListClient struct {
	responses      []stdjson.RawMessage
	calls          []fakeHousingCompetitionListCall
	thumbnailCalls []fakeHousingCompetitionThumbnailCall
}

type fakeHousingCompetitionListCall struct {
	server    string
	housingID int
	isLottery bool
}

type fakeHousingCompetitionThumbnailCall struct {
	server    string
	imagePath string
}

func (f *fakeHousingCompetitionListClient) GetMySekaiHousingCompetitionList(server string, housingID int, isLottery bool) (stdjson.RawMessage, error) {
	f.calls = append(f.calls, fakeHousingCompetitionListCall{server: server, housingID: housingID, isLottery: isLottery})
	if len(f.responses) == 0 {
		return stdjson.RawMessage(`{"lotteryAt":2000,"results":[]}`), nil
	}
	idx := len(f.calls) - 1
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return f.responses[idx], nil
}

func (f *fakeHousingCompetitionListClient) GetMySekaiHousingThumbnail(server, imagePath string) ([]byte, error) {
	f.thumbnailCalls = append(f.thumbnailCalls, fakeHousingCompetitionThumbnailCall{server: server, imagePath: imagePath})
	return []byte("thumb-" + imagePath), nil
}

func TestBuildHousingCompetitionLineSamplesAndRanksByReviewCount(t *testing.T) {
	root := t.TempDir()
	writeHousingCompetitionMasterdata(t, root)
	assetRoot := t.TempDir()
	bannerPath := filepath.Join(assetRoot, "asset", "jp-assets", "ondemand", "mysekai", "effect", "ui_anim", "mysekai_housing_competition", "lottery_result", "bg_competition_contest_1.png")
	if err := os.MkdirAll(filepath.Dir(bannerPath), 0o755); err != nil {
		t.Fatalf("mkdir banner asset: %v", err)
	}
	bannerBytes := []byte("banner-image")
	if err := os.WriteFile(bannerPath, bannerBytes, 0o644); err != nil {
		t.Fatalf("write banner asset: %v", err)
	}
	statsCachePath := filepath.Join(t.TempDir(), "housing_stats.json")

	api := &fakeHousingCompetitionListClient{
		responses: []stdjson.RawMessage{
			stdjson.RawMessage(`{"lotteryAt":2000,"results":[
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":101,"mysekaiOwnerUserName":"owner-a","userMysekaiHousingCompetitionName":"entry-a","thumbnailPath":"hash/a","submittedAt":1100,"reviewCount":10},
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":102,"mysekaiOwnerUserName":"owner-b","userMysekaiHousingCompetitionName":"entry-b","thumbnailPath":"hash/b","submittedAt":1200,"reviewCount":12}
			]}`),
			stdjson.RawMessage(`{"lotteryAt":2600,"results":[
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":101,"mysekaiOwnerUserName":"owner-a","userMysekaiHousingCompetitionName":"entry-a","thumbnailPath":"hash/a","submittedAt":1100,"reviewCount":15},
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":103,"mysekaiOwnerUserName":"owner-c","userMysekaiHousingCompetitionName":"entry-c","thumbnailPath":"hash/c","submittedAt":1300,"reviewCount":1}
			]}`),
		},
	}

	controller := NewController(nil, nil, renderregion.JP, assets.NewAssetHelper(assetRoot, nil), MasterdataOptions{
		LocalDir:                         root,
		AllowFallback:                    true,
		HousingCompetitionStatsCachePath: statsCachePath,
	})
	result, err := controller.BuildHousingCompetitionLine(context.Background(), api, HousingCompetitionLineQuery{
		Region:               "jp",
		Ranks:                []int{1, 2, 3},
		SampleCount:          2,
		SampleIntervalMillis: -1,
		Now:                  time.UnixMilli(1500),
	})
	if err != nil {
		t.Fatalf("BuildHousingCompetitionLine() error = %v", err)
	}
	if len(api.calls) != 2 {
		t.Fatalf("calls = %+v", api.calls)
	}
	for _, call := range api.calls {
		if call.server != "jp" || call.housingID != 25 || !call.isLottery {
			t.Fatalf("unexpected api call: %+v", call)
		}
	}
	if result.Competition.ID != 25 || result.UniqueCount != 3 || result.SampleCount != 2 {
		t.Fatalf("unexpected result meta: %+v", result)
	}
	gotNames := []string{result.Entries[0].EntryName, result.Entries[1].EntryName, result.Entries[2].EntryName}
	if !reflect.DeepEqual(gotNames, []string{"entry-a", "entry-b", "entry-c"}) {
		t.Fatalf("unexpected rank order: %+v", gotNames)
	}
	gotScores := []int{result.Request.Entries[0].ReviewCount, result.Request.Entries[1].ReviewCount, result.Request.Entries[2].ReviewCount}
	if !reflect.DeepEqual(gotScores, []int{15, 12, 1}) {
		t.Fatalf("unexpected request scores: %+v", gotScores)
	}
	if result.Request.CompetitionID != 25 || result.Request.Name != "烤森百景 ブロックアート" {
		t.Fatalf("unexpected drawing request: %+v", result.Request)
	}
	if result.Request.Description == nil || *result.Request.Description != HousingCompetitionNotice {
		t.Fatalf("unexpected notice: %v", result.Request.Description)
	}
	if result.Request.BannerImagePath == nil || *result.Request.BannerImagePath != "asset/jp-assets/ondemand/mysekai/effect/ui_anim/mysekai_housing_competition/lottery_result/bg_competition_contest_1.png" {
		t.Fatalf("unexpected banner path: %v", result.Request.BannerImagePath)
	}
	if result.Request.BannerImageBase64 == nil || *result.Request.BannerImageBase64 != base64.StdEncoding.EncodeToString(bannerBytes) {
		t.Fatalf("unexpected banner base64: %v", result.Request.BannerImageBase64)
	}
	cachedBannerPath := filepath.Join(filepath.Dir(statsCachePath), housingCompetitionBannerCacheDirName, "asset", "jp-assets", "ondemand", "mysekai", "effect", "ui_anim", "mysekai_housing_competition", "lottery_result", "bg_competition_contest_1.png")
	cachedBanner, err := os.ReadFile(cachedBannerPath)
	if err != nil {
		t.Fatalf("read cached banner: %v", err)
	}
	if !reflect.DeepEqual(cachedBanner, bannerBytes) {
		t.Fatalf("unexpected cached banner bytes: %q", string(cachedBanner))
	}
	requestPayload, err := stdjson.Marshal(result.Request)
	if err != nil {
		t.Fatalf("marshal drawing request: %v", err)
	}
	if strings.Contains(string(requestPayload), "owner_user_id") {
		t.Fatalf("owner user id should not be sent to drawing api: %s", string(requestPayload))
	}
	if result.Request.Entries[0].NextReviewCount == nil || *result.Request.Entries[0].NextReviewCount != 12 {
		t.Fatalf("unexpected next review count: %+v", result.Request.Entries[0])
	}
	if result.Request.Entries[1].PreviousDelta == nil || *result.Request.Entries[1].PreviousDelta != 3 {
		t.Fatalf("unexpected previous delta: %+v", result.Request.Entries[1])
	}
	if result.Request.Entries[0].ThumbnailImageBase64 == nil || *result.Request.Entries[0].ThumbnailImageBase64 != base64.StdEncoding.EncodeToString([]byte("thumb-hash/a")) {
		t.Fatalf("unexpected thumbnail payload: %+v", result.Request.Entries[0].ThumbnailImageBase64)
	}
	if len(api.thumbnailCalls) != 3 {
		t.Fatalf("thumbnail calls = %+v", api.thumbnailCalls)
	}
}

func TestResolveHousingCompetitionRefreshTargetUsesReviewWindow(t *testing.T) {
	root := t.TempDir()
	writeHousingCompetitionMasterdataWithWindow(t, root, 1000, 2000, 3000)
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: root, AllowFallback: true})

	beforeReview, err := controller.resolveHousingCompetitionRefreshTarget(HousingCompetitionLineQuery{
		Region: "jp",
		Now:    time.UnixMilli(1500),
	})
	if err != nil {
		t.Fatalf("resolve before review: %v", err)
	}
	if beforeReview.Active || beforeReview.NextStartAt != 2000 {
		t.Fatalf("unexpected before-review target: %+v", beforeReview)
	}
	if _, err := controller.resolveHousingCompetition(HousingCompetitionLineQuery{
		Region: "jp",
		Now:    time.UnixMilli(1500),
	}); err == nil || err.Error() != "当前没有正在进行的烤森百景活动" {
		t.Fatalf("resolveHousingCompetition should not select a competition before reviewStartAt")
	}

	active, err := controller.resolveHousingCompetitionRefreshTarget(HousingCompetitionLineQuery{
		Region: "jp",
		Now:    time.UnixMilli(2500),
	})
	if err != nil {
		t.Fatalf("resolve active: %v", err)
	}
	if !active.Active || active.Competition.ID != 25 {
		t.Fatalf("unexpected active target: %+v", active)
	}

	afterAggregate, err := controller.resolveHousingCompetitionRefreshTarget(HousingCompetitionLineQuery{
		Region: "jp",
		Now:    time.UnixMilli(3500),
	})
	if err != nil {
		t.Fatalf("resolve after aggregate: %v", err)
	}
	if afterAggregate.Active || afterAggregate.NextStartAt != 0 {
		t.Fatalf("unexpected after-aggregate target: %+v", afterAggregate)
	}
}

func TestHousingCompetitionBannerCacheFallsBackToAssetsBaseURL(t *testing.T) {
	root := t.TempDir()
	writeHousingCompetitionMasterdata(t, root)
	cachePath := filepath.Join(t.TempDir(), "housing_stats.json")
	bannerBytes := []byte("remote-banner")

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		if r.URL.Path != "/jp-assets/ondemand/mysekai/effect/ui_anim/mysekai_housing_competition/lottery_result/bg_competition_contest_1.png" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(bannerBytes)
	}))
	defer server.Close()

	api := &fakeHousingCompetitionListClient{
		responses: []stdjson.RawMessage{
			stdjson.RawMessage(`{"lotteryAt":2000,"results":[
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":101,"mysekaiOwnerUserName":"owner-a","userMysekaiHousingCompetitionName":"entry-a","thumbnailPath":"hash/a","submittedAt":1100,"reviewCount":10}
			]}`),
		},
	}
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:                         root,
		AllowFallback:                    true,
		AssetsBaseURL:                    server.URL,
		HousingCompetitionStatsCachePath: cachePath,
	})
	result, err := controller.BuildHousingCompetitionLine(context.Background(), api, HousingCompetitionLineQuery{
		Region: "jp",
		Ranks:  []int{1},
		Now:    time.UnixMilli(1500),
	})
	if err != nil {
		t.Fatalf("BuildHousingCompetitionLine() error = %v", err)
	}
	if result.Request.BannerImageBase64 == nil || *result.Request.BannerImageBase64 != base64.StdEncoding.EncodeToString(bannerBytes) {
		t.Fatalf("unexpected remote banner base64: %v", result.Request.BannerImageBase64)
	}
	if requestedPath == "" {
		t.Fatalf("expected banner fetch from assets base url")
	}
	cachedBannerPath := filepath.Join(filepath.Dir(cachePath), housingCompetitionBannerCacheDirName, "asset", "jp-assets", "ondemand", "mysekai", "effect", "ui_anim", "mysekai_housing_competition", "lottery_result", "bg_competition_contest_1.png")
	cachedBanner, err := os.ReadFile(cachedBannerPath)
	if err != nil {
		t.Fatalf("read cached remote banner: %v", err)
	}
	if !reflect.DeepEqual(cachedBanner, bannerBytes) {
		t.Fatalf("unexpected cached remote banner bytes: %q", string(cachedBanner))
	}
}

func TestHousingCompetitionStatsCachePersistsWithoutOwnerUserID(t *testing.T) {
	root := t.TempDir()
	writeHousingCompetitionMasterdata(t, root)
	cachePath := filepath.Join(t.TempDir(), "housing_stats.json")

	api := &fakeHousingCompetitionListClient{
		responses: []stdjson.RawMessage{
			stdjson.RawMessage(`{"lotteryAt":2000,"results":[
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":777777777777,"mysekaiOwnerUserName":"owner-a","userMysekaiHousingCompetitionName":"entry-a","thumbnailPath":"hash/a","submittedAt":1100,"reviewCount":10}
			]}`),
		},
	}
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:                         root,
		AllowFallback:                    true,
		HousingCompetitionStatsCachePath: cachePath,
	})
	if _, err := controller.BuildHousingCompetitionLine(context.Background(), api, HousingCompetitionLineQuery{
		Region: "jp",
		Ranks:  []int{1},
		Now:    time.UnixMilli(1500),
	}); err != nil {
		t.Fatalf("initial BuildHousingCompetitionLine() error = %v", err)
	}
	if len(api.calls) != 1 {
		t.Fatalf("initial api calls = %+v", api.calls)
	}
	payload, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if strings.Contains(string(payload), "777777777777") || strings.Contains(string(payload), "owner_user_id") {
		t.Fatalf("cache file should not expose owner user id: %s", string(payload))
	}

	cachedAPI := &fakeHousingCompetitionListClient{}
	loaded := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:                         root,
		AllowFallback:                    true,
		HousingCompetitionStatsCachePath: cachePath,
	})
	result, err := loaded.BuildHousingCompetitionLine(context.Background(), cachedAPI, HousingCompetitionLineQuery{
		Region: "jp",
		Ranks:  []int{1},
		Now:    time.UnixMilli(1500),
	})
	if err != nil {
		t.Fatalf("cached BuildHousingCompetitionLine() error = %v", err)
	}
	if len(cachedAPI.calls) != 0 {
		t.Fatalf("fresh persisted cache should avoid list api calls: %+v", cachedAPI.calls)
	}
	if result.Entries[0].OwnerUserID != 0 {
		t.Fatalf("cached entry should not retain owner user id: %+v", result.Entries[0])
	}
}

type concurrentHousingCompetitionClient struct {
	calls     atomic.Int32
	started   chan struct{}
	startOnce sync.Once
	release   <-chan struct{}
	lotteryAt int64
}

func (c *concurrentHousingCompetitionClient) GetMySekaiHousingCompetitionList(_ string, housingID int, _ bool) (stdjson.RawMessage, error) {
	c.calls.Add(1)
	if c.started != nil {
		c.startOnce.Do(func() { close(c.started) })
	}
	if c.release != nil {
		<-c.release
	}
	return stdjson.RawMessage(fmt.Sprintf(`{"lotteryAt":%d,"results":[{"mysekaiHousingCompetitionId":%d,"mysekaiOwnerUserId":%d,"userMysekaiHousingCompetitionName":"entry-%d","thumbnailPath":"hash/%d","submittedAt":%d,"reviewCount":%d}]}`,
		c.lotteryAt, housingID, housingID, housingID, housingID, housingID, housingID)), nil
}

func TestHousingCompetitionStatsCacheSharesRefreshAndTrace(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "housing_stats.json")
	cache := newHousingCompetitionStatsCache(cachePath, time.Millisecond)
	release := make(chan struct{})
	client := &concurrentHousingCompetitionClient{
		started:   make(chan struct{}),
		release:   release,
		lotteryAt: time.Now().UTC().UnixMilli(),
	}

	const callers = 12
	start := make(chan struct{})
	var launched atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	traces := make([]*commandtrace.Trace, callers)
	for i := 0; i < callers; i++ {
		ctx, trace := commandtrace.WithTrace(context.Background())
		traces[i] = trace
		wg.Add(1)
		go func(ctx context.Context) {
			defer wg.Done()
			<-start
			launched.Add(1)
			_, _, _, err := cache.Refresh(ctx, client, "jp", 25, 1)
			errs <- err
		}(ctx)
	}
	close(start)
	<-client.started
	deadline := time.Now().Add(time.Second)
	for launched.Load() != callers && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(25 * time.Millisecond)
	if calls := client.calls.Load(); calls != 1 {
		close(release)
		wg.Wait()
		t.Fatalf("shared refresh made %d upstream calls, want 1", calls)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
	}

	shared := 0
	for index, trace := range traces {
		for _, name := range []string{
			"housing_cache.fetch",
			"housing_cache.merge",
			"housing_cache.encode",
			"housing_cache.persist",
		} {
			if count := housingCacheTraceOperationCount(trace, name); count != 1 {
				t.Fatalf("trace[%d] %s count = %d, operations=%+v", index, name, count, trace.Snapshot().Operations)
			}
		}
		if count := housingCacheTraceOperationCount(trace, "housing_cache.snapshot"); count < 1 {
			t.Fatalf("trace[%d] housing_cache.snapshot count = %d, operations=%+v", index, count, trace.Snapshot().Operations)
		}
		shared += housingCacheTraceOperationCount(trace, "housing_cache.shared")
	}
	if shared != callers-1 {
		t.Fatalf("shared trace count = %d, want %d", shared, callers-1)
	}
	cache.mu.Lock()
	generation := cache.generation
	cache.mu.Unlock()
	if generation != 1 {
		t.Fatalf("cache generation = %d, want one merged refresh", generation)
	}
}

func TestHousingCompetitionStatsCacheLeaderCancellationDoesNotCancelSharedRefresh(t *testing.T) {
	cache := newHousingCompetitionStatsCache(filepath.Join(t.TempDir(), "housing_stats.json"), time.Millisecond)
	release := make(chan struct{})
	client := &concurrentHousingCompetitionClient{
		started:   make(chan struct{}),
		release:   release,
		lotteryAt: time.Now().UTC().UnixMilli(),
	}

	leaderBase, cancelLeader := context.WithCancel(context.Background())
	leaderCtx, _ := commandtrace.WithTrace(leaderBase)
	leaderResult := make(chan error, 1)
	go func() {
		_, _, _, err := cache.Refresh(leaderCtx, client, "jp", 25, 1)
		leaderResult <- err
	}()
	<-client.started

	followerCtx, followerTrace := commandtrace.WithTrace(context.Background())
	followerStarted := make(chan struct{})
	followerResult := make(chan error, 1)
	go func() {
		close(followerStarted)
		_, _, _, err := cache.Refresh(followerCtx, client, "jp", 25, 1)
		followerResult <- err
	}()
	<-followerStarted
	time.Sleep(25 * time.Millisecond)
	cancelLeader()
	if err := <-leaderResult; !errors.Is(err, context.Canceled) {
		close(release)
		t.Fatalf("leader Refresh() error = %v, want context.Canceled", err)
	}
	close(release)
	if err := <-followerResult; err != nil {
		t.Fatalf("follower Refresh() error = %v", err)
	}
	if calls := client.calls.Load(); calls != 1 {
		t.Fatalf("provider calls = %d, want one shared fetch", calls)
	}
	if count := housingCacheTraceOperationCount(followerTrace, "housing_cache.fetch"); count != 1 {
		t.Fatalf("follower fetch trace count = %d, operations=%+v", count, followerTrace.Snapshot().Operations)
	}
	if count := housingCacheTraceOperationCount(followerTrace, "housing_cache.shared"); count != 1 {
		t.Fatalf("follower shared trace count = %d, operations=%+v", count, followerTrace.Snapshot().Operations)
	}
}

func TestHousingCompetitionStatsCacheRetentionBounds(t *testing.T) {
	cache := newHousingCompetitionStatsCache("", time.Second)
	cache.entryTTL = time.Hour
	cache.maxEntries = 3
	cache.maxBucketEntries = 2
	cache.maxBuckets = 2
	now := time.Now().UTC()
	makeBucket := func(refreshedAt time.Time, keys ...string) *housingCompetitionStatsBucket {
		entries := make(map[string]HousingCompetitionEntry, len(keys))
		for index, key := range keys {
			entries[key] = HousingCompetitionEntry{CacheKey: key, LastSeenAt: refreshedAt.Add(time.Duration(index) * time.Second).UnixMilli()}
		}
		return &housingCompetitionStatsBucket{
			entries:       entries,
			refreshedAt:   refreshedAt,
			sampledAt:     refreshedAt,
			snapshotDirty: true,
		}
	}
	cache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 1}] = makeBucket(now.Add(-2*time.Hour), "stale")
	cache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 2}] = makeBucket(now.Add(-3*time.Minute), "a", "b")
	rankedBucket := makeBucket(now.Add(-2*time.Minute), "c", "d", "e")
	entry := rankedBucket.entries["c"]
	entry.ReviewCount = 100
	rankedBucket.entries["c"] = entry
	entry = rankedBucket.entries["d"]
	entry.ReviewCount = 10
	rankedBucket.entries["d"] = entry
	entry = rankedBucket.entries["e"]
	entry.ReviewCount = 1
	rankedBucket.entries["e"] = entry
	cache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 3}] = rankedBucket
	cache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 4}] = makeBucket(now.Add(-time.Minute), "f")

	cache.mu.Lock()
	cache.pruneLocked(now)
	if len(cache.buckets) != 2 {
		cache.mu.Unlock()
		t.Fatalf("bucket count = %d, want 2", len(cache.buckets))
	}
	if cache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 1}] != nil {
		cache.mu.Unlock()
		t.Fatal("expired bucket was retained")
	}
	if cache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 2}] != nil {
		cache.mu.Unlock()
		t.Fatal("oldest bucket above the hard cap was retained")
	}
	if rankedBucket.entries["c"].ReviewCount != 100 || rankedBucket.entries["e"].CacheKey != "" {
		cache.mu.Unlock()
		t.Fatal("per-bucket cap did not preserve the highest review-count entries")
	}
	total := 0
	for _, bucket := range cache.buckets {
		total += len(bucket.entries)
		if len(bucket.entries) > cache.maxBucketEntries {
			cache.mu.Unlock()
			t.Fatalf("bucket entry count = %d, want <= %d", len(bucket.entries), cache.maxBucketEntries)
		}
	}
	cache.mu.Unlock()
	if total != 3 {
		t.Fatalf("total entries = %d, want 3", total)
	}
}

func TestHousingCompetitionStatsCacheConcurrentPersistenceIsComplete(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "housing_stats.json")
	cache := newHousingCompetitionStatsCache(cachePath, time.Second)
	client := &concurrentHousingCompetitionClient{lotteryAt: time.Now().UTC().UnixMilli()}

	const refreshes = 24
	var wg sync.WaitGroup
	errs := make(chan error, refreshes)
	for housingID := 1; housingID <= refreshes; housingID++ {
		wg.Add(1)
		go func(housingID int) {
			defer wg.Done()
			_, _, _, err := cache.Refresh(context.Background(), client, "jp", housingID, 1)
			errs <- err
		}(housingID)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
	}

	payload, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	var persisted persistedHousingCompetitionStatsCache
	if err := stdjson.Unmarshal(payload, &persisted); err != nil {
		t.Fatalf("decode cache file: %v", err)
	}
	if len(persisted.Buckets) != refreshes {
		t.Fatalf("persisted buckets = %d, want %d", len(persisted.Buckets), refreshes)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".housing_stats.json.tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("orphaned temp files: %v", temps)
	}
}

func housingCacheTraceOperationCount(trace *commandtrace.Trace, name string) int {
	if trace == nil {
		return 0
	}
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name {
			return operation.Count
		}
	}
	return 0
}

func writeHousingCompetitionMasterdata(t *testing.T, root string) {
	t.Helper()
	writeHousingCompetitionMasterdataWithWindow(t, root, 1000, 1000, 3000)
}

func writeHousingCompetitionMasterdataWithWindow(t *testing.T, root string, submitStartAt, reviewStartAt, aggregateAt int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	masterdata := fmt.Sprintf(`[
		{
			"id":25,
			"name":"ブロックアート",
			"description":"test",
			"submitStartAt":%d,
			"reviewStartAt":%d,
			"submitEndAt":2500,
			"aggregateAt":%d,
			"backgroundImageAssetbundleFileName":"bg_competition_contest_1"
		}
	]`, submitStartAt, reviewStartAt, aggregateAt)
	if err := os.WriteFile(filepath.Join(root, "jp", "mysekaiHousingCompetitions.json"), []byte(masterdata), 0o644); err != nil {
		t.Fatalf("write masterdata: %v", err)
	}
}
