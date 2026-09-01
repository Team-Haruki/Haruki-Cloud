//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package mysekai

import (
	"bytes"
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

type housingCoverageClient struct {
	raw       stdjson.RawMessage
	err       error
	thumbnail []byte
	thumbErr  error
	calls     int
	afterCall func()
}

func (c *housingCoverageClient) GetMySekaiHousingCompetitionList(string, int, bool) (stdjson.RawMessage, error) {
	c.calls++
	if c.afterCall != nil {
		c.afterCall()
	}
	return c.raw, c.err
}

func (c *housingCoverageClient) GetMySekaiHousingThumbnail(string, string) ([]byte, error) {
	return c.thumbnail, c.thumbErr
}

type listOnlyHousingCoverageClient struct {
	raw stdjson.RawMessage
	err error
}

func (c listOnlyHousingCoverageClient) GetMySekaiHousingCompetitionList(string, int, bool) (stdjson.RawMessage, error) {
	return c.raw, c.err
}

type housingRoundTripFunc func(*http.Request) (*http.Response, error)

func (f housingRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHousingCompetitionBuildValidationRefreshAndRenderBranches(t *testing.T) {
	validRaw := stdjson.RawMessage(`{"lotteryAt":2000,"results":[{"mysekaiHousingCompetitionId":25,"mysekaiOwnerUserId":1,"mysekaiOwnerUserName":"owner","userMysekaiHousingCompetitionName":"entry","thumbnailPath":"thumb","submittedAt":1000,"reviewCount":10}]}`)
	client := &housingCoverageClient{raw: validRaw, thumbnail: []byte("thumb")}
	var nilController *Controller
	if _, err := nilController.BuildHousingCompetitionLine(context.Background(), client, HousingCompetitionLineQuery{}); err == nil {
		t.Fatal("nil controller should fail")
	}
	if _, err := (&Controller{}).BuildHousingCompetitionLine(context.Background(), nil, HousingCompetitionLineQuery{}); err == nil {
		t.Fatal("nil API should fail")
	}
	if _, err := (&Controller{}).BuildHousingCompetitionLine(context.Background(), client, HousingCompetitionLineQuery{}); err == nil {
		t.Fatal("unconfigured masterdata should fail")
	}

	root := t.TempDir()
	writeHousingCompetitionMasterdata(t, root)
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: root, AllowFallback: true})
	if _, err := controller.BuildHousingCompetitionLine(context.Background(), client, HousingCompetitionLineQuery{Ranks: []int{0}, Now: time.UnixMilli(1500)}); err == nil {
		t.Fatal("invalid rank should fail")
	}
	if _, err := controller.BuildHousingCompetitionLine(context.Background(), client, HousingCompetitionLineQuery{HousingID: 404}); err == nil {
		t.Fatal("missing housing ID should fail")
	}
	empty := &housingCoverageClient{raw: stdjson.RawMessage(`{"lotteryAt":2000,"results":[]}`)}
	if _, err := controller.BuildHousingCompetitionLine(nil, empty, HousingCompetitionLineQuery{HousingID: 25, Ranks: []int{1}}); err == nil {
		t.Fatal("empty samples should fail")
	}
	tooShort := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: root, AllowFallback: true})
	if _, err := tooShort.BuildHousingCompetitionLine(context.Background(), client, HousingCompetitionLineQuery{HousingID: 25, Ranks: []int{2}}); err == nil {
		t.Fatal("rank beyond the sampled entries should fail")
	}
	withoutCache := *controller
	withoutCache.housingCompetitionStats = nil
	result, err := withoutCache.BuildHousingCompetitionLine(nil, client, HousingCompetitionLineQuery{HousingID: 25, Ranks: []int{1}, SampleIntervalMillis: -1})
	if err != nil || result.UniqueCount != 1 || result.Region != "jp" {
		t.Fatalf("uncached build = %+v, %v", result, err)
	}

	if err := nilController.RefreshHousingCompetitionStats(context.Background(), client, HousingCompetitionLineQuery{}); err != nil {
		t.Fatalf("nil refresh = %v", err)
	}
	if err := controller.RefreshHousingCompetitionStats(nil, nil, HousingCompetitionLineQuery{}); err != nil {
		t.Fatalf("refresh without API = %v", err)
	}
	if err := withoutCache.RefreshHousingCompetitionStats(context.Background(), client, HousingCompetitionLineQuery{}); err != nil {
		t.Fatalf("refresh without cache = %v", err)
	}
	refreshController := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: root, AllowFallback: true})
	if err := refreshController.RefreshHousingCompetitionStats(nil, client, HousingCompetitionLineQuery{HousingID: 25}); err != nil {
		t.Fatalf("active refresh = %v", err)
	}

	nilController.StartHousingCompetitionStatsRefresh(context.Background(), client, "jp")
	controller.StartHousingCompetitionStatsRefresh(context.Background(), nil, "jp")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	controller.StartHousingCompetitionStatsRefresh(canceled, client, "")
	time.Sleep(time.Millisecond)

	if _, err := nilController.RenderHousingCompetitionLine(result); err == nil {
		t.Fatal("nil drawing controller should fail")
	}
	if _, err := (&Controller{}).RenderHousingCompetitionLine(nil); err == nil {
		t.Fatal("nil result should fail after drawing is configured only")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("image")) }))
	defer server.Close()
	renderer := &Controller{drawing: drawing.NewHarukiDrawingClient(server.URL)}
	if _, err := renderer.RenderHousingCompetitionLine(nil); err == nil {
		t.Fatal("nil render result should fail")
	}
	image, err := renderer.RenderHousingCompetitionLine(&HousingCompetitionLineResult{Request: drawing.MysekaiHousingCompetitionRequest{}})
	if err != nil || string(image) != "image" {
		t.Fatalf("render housing result = %q, %v", image, err)
	}
}

func TestHousingCompetitionPureSelectionParsingAndTimingBranches(t *testing.T) {
	testHousingCompetitionRankAndParsingBranches(t)
	testHousingCompetitionSortingBranches(t)
	testHousingCompetitionTimingBranches(t)
	testHousingCompetitionThumbnailBranches(t)
}

func testHousingCompetitionRankAndParsingBranches(t *testing.T) {
	t.Helper()
	if got, err := NormalizeHousingCompetitionRanks(nil); err != nil || !reflect.DeepEqual(got, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("default ranks = %v, %v", got, err)
	}
	if _, err := NormalizeHousingCompetitionRanks([]int{-1}); err == nil {
		t.Fatal("negative rank should fail")
	}
	if got, err := NormalizeHousingCompetitionRanks([]int{3, 1, 3, 2}); err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("normalized ranks = %v, %v", got, err)
	}
	if _, err := NormalizeHousingCompetitionRanks([]int{1, 2, 3, 4, 5, 6}); err == nil {
		t.Fatal("too many ranks should fail")
	}

	for _, raw := range []stdjson.RawMessage{stdjson.RawMessage(`{`), stdjson.RawMessage(`1`)} {
		if _, _, err := parseHousingCompetitionEntries(raw); err == nil {
			t.Fatalf("invalid entry payload %q should fail", raw)
		}
	}
	entries, lotteryAt, err := parseHousingCompetitionEntries(stdjson.RawMessage(`[
		1,
		{"isDisplayable":false,"reviewCount":100},
		{"mysekai_housing_competition_id":2,"mysekai_owner_user_name":"owner","user_mysekai_housing_competition_name":"entry","user_mysekai_housing_competition_word":"word","thumbnail_path":"path","review_count":3,"mysekai_housing_competition_tab_type":"tab"}
	]`))
	if err != nil || lotteryAt != 0 || len(entries) != 1 || entries[0].CompetitionID != 2 || entries[0].EntryName != "entry" {
		t.Fatalf("alternate entry payload = %+v, %d, %v", entries, lotteryAt, err)
	}
	if normalizeHousingCompetitionSampleCount(0) != 1 || normalizeHousingCompetitionSampleCount(100) != 10 || normalizeHousingCompetitionSampleCount(3) != 3 {
		t.Fatal("sample count normalization mismatch")
	}
}

func testHousingCompetitionSortingBranches(t *testing.T) {
	t.Helper()
	items := []HousingCompetitionEntry{
		{CacheKey: "z", ReviewCount: 1, SubmittedAt: 3, EntryName: "z"},
		{CacheKey: "b", ReviewCount: 2, SubmittedAt: 2, EntryName: "b"},
		{CacheKey: "a", ReviewCount: 2, SubmittedAt: 2, EntryName: "a"},
		{CompetitionID: 1, ReviewCount: 2, SubmittedAt: 1, EntryName: "derived"},
	}
	sortHousingCompetitionEntries(items)
	if items[0].SubmittedAt != 1 || items[1].CacheKey != "a" || items[2].CacheKey != "b" {
		t.Fatalf("sorted entries = %+v", items)
	}
	if items[1].uniqueKey() != "a" || items[0].uniqueKey() == "" {
		t.Fatal("entry unique keys should use explicit and derived forms")
	}
}

func testHousingCompetitionTimingBranches(t *testing.T) {
	t.Helper()
	if stringPtrIfNotEmpty(" ") != nil || *stringPtrIfNotEmpty(" x ") != "x" {
		t.Fatal("stringPtrIfNotEmpty mismatch")
	}
	if housingCompetitionListStartAt(HousingCompetitionInfo{SubmitStartAt: 10}) != 10 || housingCompetitionListStartAt(HousingCompetitionInfo{SubmitStartAt: 10, ReviewStartAt: 20}) != 20 {
		t.Fatal("competition list start mismatch")
	}
	now := time.UnixMilli(1_000)
	if got := (housingCompetitionRefreshTarget{}).waitDuration(now, 0); got != housingCompetitionIdleCheckInterval {
		t.Fatalf("default idle wait = %v", got)
	}
	if got := (housingCompetitionRefreshTarget{NextStartAt: 900}).waitDuration(now, time.Second); got != 0 {
		t.Fatalf("past start wait = %v", got)
	}
	if got := (housingCompetitionRefreshTarget{NextStartAt: 5_000}).waitDuration(now, time.Second); got != time.Second {
		t.Fatalf("capped wait = %v", got)
	}
	if got := (housingCompetitionRefreshTarget{NextStartAt: 1_500}).waitDuration(now, time.Second); got != 500*time.Millisecond {
		t.Fatalf("short wait = %v", got)
	}
	if got := (housingCompetitionRefreshTarget{}).activeRefreshWait(now, 0); got != DefaultHousingCompetitionRefreshInterval {
		t.Fatalf("default active wait = %v", got)
	}
	if got := (housingCompetitionRefreshTarget{Competition: HousingCompetitionInfo{AggregateAt: 900}}).activeRefreshWait(now, time.Second); got != 0 {
		t.Fatalf("ended active wait = %v", got)
	}
	if got := (housingCompetitionRefreshTarget{Competition: HousingCompetitionInfo{AggregateAt: 1_500}}).activeRefreshWait(now, time.Second); got != 500*time.Millisecond {
		t.Fatalf("short active wait = %v", got)
	}
	if got := (housingCompetitionRefreshTarget{Competition: HousingCompetitionInfo{AggregateAt: 5_000}}).activeRefreshWait(now, time.Second); got != time.Second {
		t.Fatalf("normal active wait = %v", got)
	}
}

func testHousingCompetitionThumbnailBranches(t *testing.T) {
	t.Helper()
	noThumb := housingCompetitionThumbnailBase64(listOnlyHousingCoverageClient{}, "jp", "path")
	if noThumb != nil || housingCompetitionThumbnailBase64(&housingCoverageClient{}, "jp", " ") != nil {
		t.Fatal("thumbnail should require a path and thumbnail-capable client")
	}
	if housingCompetitionThumbnailBase64(&housingCoverageClient{thumbErr: errors.New("failed")}, "jp", "path") != nil || housingCompetitionThumbnailBase64(&housingCoverageClient{}, "jp", "path") != nil {
		t.Fatal("thumbnail errors and empty data should return nil")
	}
	encoded := housingCompetitionThumbnailBase64(&housingCoverageClient{thumbnail: []byte("x")}, "jp", "path")
	if encoded == nil || *encoded != base64.StdEncoding.EncodeToString([]byte("x")) {
		t.Fatalf("thumbnail base64 = %v", encoded)
	}
}

func TestHousingCompetitionStatsCacheFallbackPersistenceAndDefensiveBranches(t *testing.T) {
	fixture := newHousingStatsCoverageFixture()
	testHousingStatsCacheValidationBranches(t, fixture)
	testHousingStatsCacheFreshnessBranches(t, fixture)
	testHousingStatsBucketAndMergeBranches(t, fixture)
	testHousingStatsSamplingBranches(t, fixture)
	testHousingStatsPersistenceBranches(t)
}

type housingStatsCoverageFixture struct {
	validRaw stdjson.RawMessage
	client   *housingCoverageClient
	cache    *housingCompetitionStatsCache
	key      housingCompetitionStatsCacheKey
	now      time.Time
	bad      *housingCoverageClient
}

func newHousingStatsCoverageFixture() *housingStatsCoverageFixture {
	validRaw := stdjson.RawMessage(`{"lotteryAt":2000,"results":[{"mysekaiHousingCompetitionId":25,"mysekaiOwnerUserId":1,"userMysekaiHousingCompetitionName":"entry","submittedAt":1000,"reviewCount":10}]}`)
	client := &housingCoverageClient{raw: validRaw}
	key, _ := newHousingCompetitionStatsCacheKey("", 25)
	return &housingStatsCoverageFixture{
		validRaw: validRaw,
		client:   client,
		cache:    newHousingCompetitionStatsCache("", -1),
		key:      key,
		now:      time.Now().UTC(),
		bad:      &housingCoverageClient{err: errors.New("upstream")},
	}
}

func testHousingStatsCacheValidationBranches(t *testing.T, fixture *housingStatsCoverageFixture) {
	t.Helper()
	client := fixture.client
	cache := fixture.cache
	var nilCache *housingCompetitionStatsCache
	if nilCache.RefreshInterval() != DefaultHousingCompetitionRefreshInterval {
		t.Fatal("nil cache refresh interval mismatch")
	}
	if entries, _, count, err := nilCache.GetOrRefresh(nil, client, "", 25, 1); err != nil || len(entries) != 1 || count != 1 {
		t.Fatalf("nil-cache GetOrRefresh = %+v, %d, %v", entries, count, err)
	}
	if entries, _, count, err := nilCache.Refresh(nil, client, "jp", 25, 1); err != nil || len(entries) != 1 || count != 1 {
		t.Fatalf("nil-cache Refresh = %+v, %d, %v", entries, count, err)
	}
	if cache.RefreshInterval() != DefaultHousingCompetitionRefreshInterval {
		t.Fatal("default cache interval mismatch")
	}
	if _, _, _, err := cache.GetOrRefresh(context.Background(), client, "jp", 0, 1); err == nil {
		t.Fatal("invalid GetOrRefresh key should fail")
	}
	if _, _, _, err := cache.Refresh(context.Background(), client, "jp", 0, 1); err == nil {
		t.Fatal("invalid Refresh key should fail")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := cache.Refresh(canceled, client, "jp", 25, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Refresh = %v", err)
	}
	if _, _, _, err := cache.Refresh(nil, nil, "jp", 25, 1); err == nil {
		t.Fatal("nil API should fail")
	}
	if _, _, _, err := cache.Refresh(context.Background(), fixture.bad, "jp", 25, 1); err == nil {
		t.Fatal("upstream refresh error should propagate")
	}
	malformed := &housingCoverageClient{raw: stdjson.RawMessage(`{`)}
	if _, _, _, err := cache.Refresh(context.Background(), malformed, "jp", 25, 1); err == nil {
		t.Fatal("malformed refresh response should fail")
	}
}

func testHousingStatsCacheFreshnessBranches(t *testing.T, fixture *housingStatsCoverageFixture) {
	t.Helper()
	cache, key, now := fixture.cache, fixture.key, fixture.now
	key, ok := newHousingCompetitionStatsCacheKey("", 25)
	if !ok || key.Region != renderRegionDefaultString() {
		t.Fatalf("default cache key = %+v, %v", key, ok)
	}
	if _, ok := newHousingCompetitionStatsCacheKey("jp", -1); ok {
		t.Fatal("negative housing ID should not form a cache key")
	}
	cache.buckets[key] = &housingCompetitionStatsBucket{
		entries:       map[string]HousingCompetitionEntry{"entry": {CacheKey: "entry", ReviewCount: 10}},
		refreshedAt:   now,
		sampledAt:     now,
		snapshotDirty: true,
	}
	freshClient := &housingCoverageClient{err: errors.New("must not call")}
	if entries, _, count, err := cache.GetOrRefresh(context.Background(), freshClient, "jp", 25, 1); err != nil || len(entries) != 1 || count != 0 || freshClient.calls != 0 {
		t.Fatalf("fresh cached result = %+v, %d, %v calls=%d", entries, count, err, freshClient.calls)
	}
	cache.buckets[key].refreshedAt = now.Add(-time.Hour)
	cache.refreshInterval = time.Second
	if entries, _, count, err := cache.GetOrRefresh(context.Background(), fixture.bad, "jp", 25, 1); err != nil || len(entries) != 1 || count != 0 {
		t.Fatalf("stale fallback = %+v, %d, %v", entries, count, err)
	}
	delete(cache.buckets, key)
	if _, _, _, err := cache.GetOrRefresh(context.Background(), fixture.bad, "jp", 25, 1); err == nil {
		t.Fatal("error without a stale fallback should propagate")
	}
}

func testHousingStatsBucketAndMergeBranches(t *testing.T, fixture *housingStatsCoverageFixture) {
	t.Helper()
	testHousingStatsBucketBranches(t, fixture)
	testHousingStatsMergeBranches(t)
}

func testHousingStatsBucketBranches(t *testing.T, fixture *housingStatsCoverageFixture) {
	t.Helper()
	cache, key, now := fixture.cache, fixture.key, fixture.now
	var nilCache *housingCompetitionStatsCache
	if shouldRefreshHousingCompetitionStats(nil, now, 0) != true || shouldRefreshHousingCompetitionStats(&housingCompetitionStatsBucket{}, now, 0) != true {
		t.Fatal("empty buckets should refresh")
	}
	entryBucket := &housingCompetitionStatsBucket{entries: map[string]HousingCompetitionEntry{"x": {}}, refreshedAt: time.Time{}}
	if !shouldRefreshHousingCompetitionStats(entryBucket, now, 0) {
		t.Fatal("bucket without refresh time should refresh")
	}
	entryBucket.refreshedAt = now
	if shouldRefreshHousingCompetitionStats(entryBucket, now, 0) || !shouldRefreshHousingCompetitionStats(entryBucket, now.Add(time.Hour), time.Second) {
		t.Fatal("fresh/stale refresh decisions mismatch")
	}
	var nilBucket *housingCompetitionStatsBucket
	if entries, sampledAt := nilBucket.snapshot(); entries != nil || !sampledAt.IsZero() || nilBucket.snapshotView() != nil {
		t.Fatal("nil bucket snapshot mismatch")
	}
	if entries, sampledAt := nilCache.snapshotLocked(key); entries != nil || !sampledAt.IsZero() {
		t.Fatal("nil cache snapshot mismatch")
	}
	if entries, sampledAt := cache.snapshotLocked(key); entries != nil || !sampledAt.IsZero() {
		t.Fatal("missing cache snapshot mismatch")
	}
}

func testHousingStatsMergeBranches(t *testing.T) {
	t.Helper()
	merged := map[string]HousingCompetitionEntry{}
	mergeHousingCompetitionEntries(merged, []HousingCompetitionEntry{{CompetitionID: 1, OwnerUserID: 2, EntryName: "old", ReviewCount: 2}})
	for cacheKey, entry := range merged {
		mergeHousingCompetitionEntries(merged, []HousingCompetitionEntry{{CacheKey: cacheKey, CompetitionID: 3, OwnerUserID: 4, OwnerUserName: "new", EntryName: "new", EntryWord: "word", ThumbnailPath: "thumb", SubmittedAt: 5, ReviewCount: 3, TabType: "tab", LastSeenAt: entry.LastSeenAt + 1}})
		if got := merged[cacheKey]; got.CompetitionID != 3 || got.OwnerUserID != 4 || got.EntryName != "new" || got.ReviewCount != 3 {
			t.Fatalf("merged entry = %+v", got)
		}
	}
	removeLowestRankedHousingCompetitionEntries(nil, 1)
	removeLowestRankedHousingCompetitionEntries(&housingCompetitionStatsBucket{}, 0)
	removeBucket := &housingCompetitionStatsBucket{entries: map[string]HousingCompetitionEntry{"a": {ReviewCount: 1}, "b": {ReviewCount: 2}}}
	removeLowestRankedHousingCompetitionEntries(removeBucket, 99)
	if len(removeBucket.entries) != 0 {
		t.Fatalf("remove all bucket entries = %+v", removeBucket.entries)
	}
}

func testHousingStatsSamplingBranches(t *testing.T, fixture *housingStatsCoverageFixture) {
	t.Helper()
	if err := waitHousingCompetitionSampleInterval(context.Background(), 0); err != nil {
		t.Fatalf("zero wait = %v", err)
	}
	waitCtx, waitCancel := context.WithCancel(context.Background())
	waitCancel()
	if err := waitHousingCompetitionSampleInterval(waitCtx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = %v", err)
	}
	if err := waitHousingCompetitionSampleInterval(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("completed wait = %v", err)
	}

	cancelCtx, cancelWait := context.WithCancel(context.Background())
	cancelClient := &housingCoverageClient{raw: fixture.validRaw, afterCall: cancelWait}
	if _, _, count, err := fetchHousingCompetitionSamples(cancelCtx, cancelClient, "", 25, 2, 1000); !errors.Is(err, context.Canceled) || count != 1 {
		t.Fatalf("sample interval cancellation count=%d err=%v", count, err)
	}
	if _, _, _, err := fetchHousingCompetitionSamples(context.Background(), nil, "jp", 25, 1, 0); err == nil {
		t.Fatal("nil sample API should fail")
	}
	if _, _, _, err := fetchHousingCompetitionSamples(context.Background(), fixture.client, "jp", 0, 1, 0); err == nil {
		t.Fatal("invalid sample housing ID should fail")
	}
}

func testHousingStatsPersistenceBranches(t *testing.T) {
	t.Helper()
	testHousingStatsInvalidPersistenceFiles(t)
	testHousingStatsSnapshotPersistence(t)
	testHousingStatsPersistenceWriteAndTime(t)
}

func testHousingStatsInvalidPersistenceFiles(t *testing.T) {
	t.Helper()
	badFiles := []string{"", `{`, `{"version":99}`, `{"version":1,"buckets":[{"key":{"region":"jp","housing_id":0},"entries":[]},{"key":{"region":"jp","housing_id":1},"entries":[{"cache_key":""}]}]}`}
	for index, payload := range badFiles {
		path := filepath.Join(t.TempDir(), "cache.json")
		if index > 0 || payload != "" {
			if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
				t.Fatalf("write bad cache: %v", err)
			}
		}
		loaded := newHousingCompetitionStatsCache(path, time.Second)
		if len(loaded.buckets) != 0 {
			t.Fatalf("bad cache %q loaded buckets: %+v", payload, loaded.buckets)
		}
	}
}

func testHousingStatsSnapshotPersistence(t *testing.T) {
	t.Helper()
	persistCache := newHousingCompetitionStatsCache(filepath.Join(t.TempDir(), "cache.json"), time.Second)
	persistCache.buckets[housingCompetitionStatsCacheKey{Region: "tw", HousingID: 2}] = nil
	persistCache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 2}] = &housingCompetitionStatsBucket{entries: map[string]HousingCompetitionEntry{"b": {CacheKey: "b"}}, snapshotDirty: true}
	persistCache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 1}] = &housingCompetitionStatsBucket{entries: map[string]HousingCompetitionEntry{"a": {CacheKey: "a"}}, snapshotDirty: true}
	persisted := persistCache.snapshotForPersistenceLocked()
	if len(persisted.Buckets) != 2 || persisted.Buckets[0].Key.HousingID != 1 || persisted.Buckets[1].Key.HousingID != 2 {
		t.Fatalf("sorted persisted snapshot = %+v", persisted.Buckets)
	}
	persistCache.generation = 2
	persistCache.persistLatest(context.Background(), 2)
	if persistCache.persistedGeneration != 2 {
		t.Fatalf("persisted generation = %d", persistCache.persistedGeneration)
	}
	persistCache.persistLatest(context.Background(), 1)
	var emptyPersist *housingCompetitionStatsCache
	emptyPersist.persistLatest(context.Background(), 1)
}

func testHousingStatsPersistenceWriteAndTime(t *testing.T) {
	t.Helper()
	blockingParent := filepath.Join(t.TempDir(), "parent-file")
	if err := os.WriteFile(blockingParent, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocking parent: %v", err)
	}
	if err := writeHousingCompetitionStatsCachePayload(filepath.Join(blockingParent, "cache.json"), []byte(`{}`)); err == nil {
		t.Fatal("cache write through a file parent should fail")
	}
	if !timeFromUnixMilli(0).IsZero() || timeFromUnixMilli(1000).UnixMilli() != 1000 || unixMilliOrZero(time.Time{}) != 0 || unixMilliOrZero(time.UnixMilli(1000)) != 1000 {
		t.Fatal("cache time conversion mismatch")
	}
}

func TestHousingCompetitionBannerCacheIOAndURLBranches(t *testing.T) {
	testHousingBannerCacheBasicIO(t)
	testHousingBannerCacheRemoteErrors(t)
	testHousingBannerCacheLocalAndPathEdges(t)
}

func testHousingBannerCacheBasicIO(t *testing.T) {
	t.Helper()
	var nilCache *housingCompetitionBannerCache
	if _, err := nilCache.Bytes("path"); err == nil || nilCache.Base64("path") != nil || nilCache.isSynced("path") {
		t.Fatal("nil banner cache behavior mismatch")
	}
	nilCache.markSynced("path")
	cache := newHousingCompetitionBannerCache(t.TempDir(), nil, "https://assets.example/base")
	if _, err := cache.Bytes(" "); err == nil {
		t.Fatal("empty banner path should fail")
	}
	if cache.isSynced(" ") {
		t.Fatal("blank banner path should not be synced")
	}
	cache.markSynced(" ")
	cache.httpClient = &http.Client{Transport: housingRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/base/jp-assets/banner.png" {
			return nil, errors.New("unexpected URL " + req.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("banner")), Header: make(http.Header)}, nil
	})}
	if err := cache.Sync("asset/jp-assets/banner.png"); err != nil {
		t.Fatalf("Sync() = %v", err)
	}
	if err := cache.Sync("asset/jp-assets/banner.png"); err != nil {
		t.Fatalf("cached Sync() = %v", err)
	}
	raw, err := cache.Bytes("asset/jp-assets/banner.png")
	if err != nil || string(raw) != "banner" {
		t.Fatalf("Bytes() = %q, %v", raw, err)
	}
	encoded := cache.Base64("asset/jp-assets/banner.png")
	if encoded == nil || *encoded != base64.StdEncoding.EncodeToString([]byte("banner")) {
		t.Fatalf("Base64() = %v", encoded)
	}
}

func testHousingBannerCacheRemoteErrors(t *testing.T) {
	t.Helper()
	notFound := newHousingCompetitionBannerCache("", nil, "https://assets.example")
	notFound.httpClient = &http.Client{Transport: housingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("missing")), Header: make(http.Header)}, nil
	})}
	if _, err := notFound.BytesContext(nil, "banner.png"); err == nil {
		t.Fatal("HTTP error should fail banner download")
	}
	transportError := newHousingCompetitionBannerCache("", nil, "https://assets.example")
	transportError.httpClient = &http.Client{Transport: housingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport")
	})}
	if _, err := transportError.Bytes("banner.png"); err == nil {
		t.Fatal("transport error should fail banner download")
	}
	tooLarge := newHousingCompetitionBannerCache("", nil, "https://assets.example")
	tooLarge.httpClient = &http.Client{Transport: housingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(make([]byte, housingCompetitionBannerMaxBytes+1))), Header: make(http.Header)}, nil
	})}
	if _, err := tooLarge.Bytes("banner.png"); err == nil {
		t.Fatal("oversized banner should fail")
	}
	withoutSource := newHousingCompetitionBannerCache("", nil, "")
	if _, err := withoutSource.Bytes("banner.png"); err == nil {
		t.Fatal("cache without a source should fail")
	}
}

func testHousingBannerCacheLocalAndPathEdges(t *testing.T) {
	t.Helper()
	testHousingBannerLocalSourceAndPaths(t)
	testHousingBannerCacheWrites(t)
}

func testHousingBannerLocalSourceAndPaths(t *testing.T) {
	t.Helper()
	cache := newHousingCompetitionBannerCache(t.TempDir(), nil, "https://assets.example/base")
	withoutSource := newHousingCompetitionBannerCache("", nil, "")
	assetRoot := t.TempDir()
	assetPath := filepath.Join(assetRoot, "asset", "jp-assets", "ondemand", "mysekai", "banner.png")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("mkdir local banner: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("local"), 0o644); err != nil {
		t.Fatalf("write local banner: %v", err)
	}
	local := newHousingCompetitionBannerCache("", assets.NewAssetHelper(assetRoot, nil), "")
	if raw, err := local.BytesContext(context.Background(), "asset/jp-assets/ondemand/mysekai/banner.png"); err != nil || string(raw) != "local" {
		t.Fatalf("local asset banner = %q, %v", raw, err)
	}

	if _, err := cache.sourceURL("../escape.png"); err == nil {
		t.Fatal("traversal source path should fail")
	}
	invalidURL := newHousingCompetitionBannerCache("", nil, "://")
	if _, err := invalidURL.sourceURL("banner.png"); err == nil {
		t.Fatal("invalid base URL should fail")
	}
	if _, err := withoutSource.sourceURL("banner.png"); err == nil {
		t.Fatal("empty base URL should fail")
	}
	if cache.cachePath("") != "" || (*housingCompetitionBannerCache)(nil).cachePath("x") != "" {
		t.Fatal("empty/nil cache path should be blank")
	}
	regular := housingCompetitionBannerCacheRelPath("asset/jp/banner.PNG")
	hashedAbs := housingCompetitionBannerCacheRelPath(filepath.Join(t.TempDir(), "banner.PNG"))
	hashedTraversal := housingCompetitionBannerCacheRelPath("../banner")
	if regular != "asset/jp/banner.PNG" || !strings.HasPrefix(hashedAbs, "by_hash/") || !strings.HasSuffix(hashedAbs, ".png") || !strings.HasSuffix(hashedTraversal, ".bin") || housingCompetitionBannerCacheRelPath(" ") != "" {
		t.Fatalf("banner cache relative paths regular=%q abs=%q traversal=%q", regular, hashedAbs, hashedTraversal)
	}
}

func testHousingBannerCacheWrites(t *testing.T) {
	t.Helper()
	cache := newHousingCompetitionBannerCache(t.TempDir(), nil, "https://assets.example/base")
	if err := cache.write("", []byte("x")); err != nil || cache.write(filepath.Join(t.TempDir(), "x"), nil) != nil {
		t.Fatal("empty banner writes should be no-ops")
	}
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatalf("write existing cache: %v", err)
	}
	if err := cache.write(existing, []byte("new")); err != nil {
		t.Fatalf("write existing cache = %v", err)
	}
	if raw, _ := os.ReadFile(existing); string(raw) != "old" {
		t.Fatalf("existing cache overwritten with %q", raw)
	}
	blockingParent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blockingParent, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocking parent: %v", err)
	}
	if err := cache.write(filepath.Join(blockingParent, "banner"), []byte("x")); err == nil {
		t.Fatal("banner write under file parent should fail")
	}
}
