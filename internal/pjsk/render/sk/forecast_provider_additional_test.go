package sk

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

type additionalForecastRoundTripper func(*http.Request) (*http.Response, error)

func (fn additionalForecastRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func additionalForecastProvider(t *testing.T, responder func(*http.Request) (int, string, error)) *RemoteForecastProvider {
	t.Helper()
	client := resty.New().SetRetryCount(0)
	client.SetTransport(additionalForecastRoundTripper(func(req *http.Request) (*http.Response, error) {
		status, body, err := responder(req)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	}))
	return &RemoteForecastProvider{http: client}
}

func TestForecastHTTPPureHelpersAdditional(t *testing.T) {
	testSekaRunRowAndAssignmentHelpers(t)
	testSekaRunRowExtraction(t)
	testSekaRunScoreParsing(t)
	testForecastNumberAndTimestampHelpers(t)
}

func testSekaRunRowAndAssignmentHelpers(t *testing.T) {
	if got := parseSekaRunRow(` 1, "p", '[x]' `); !reflect.DeepEqual(got, []string{"1", "p", "x"}) {
		t.Fatalf("parseSekaRunRow() = %#v", got)
	}

	assignments := []struct {
		body string
		name string
		want string
	}{
		{"other = 1", "currentEvent", ""},
		{"currentEvent without assignment", "currentEvent", ""},
		{`currentEvent = "123"; next = 1`, "currentEvent", "123"},
		{"currentEvent='456'\nnext=1", "currentEvent", "456"},
	}
	for _, tt := range assignments {
		if got := extractSekaRunAssignment(tt.body, tt.name); got != tt.want {
			t.Errorf("extractSekaRunAssignment(%q) = %q, want %q", tt.body, got, tt.want)
		}
	}

}

func testSekaRunRowExtraction(t *testing.T) {
	current, rows, err := extractSekaRunRows(`currentEvent = "7"; data = [[7,p,0,0,1,100,1,0,1,1], [7,h,0,0,2,200,2,0,1,1]];`)
	if err != nil || current != "7" || len(rows) != 2 {
		t.Fatalf("explicit sekarun rows = %q, %#v, %v", current, rows, err)
	}
	current, rows, err = extractSekaRunRows(`currentEvent = "8"; data = [[]];`)
	if err != nil || current != "8" || len(rows) != 0 {
		t.Fatalf("empty sekarun rows = %q, %#v, %v", current, rows, err)
	}
	if _, _, err := extractSekaRunRows(`data = [[1,2]`); err == nil {
		t.Fatal("unterminated named sekarun payload unexpectedly succeeded")
	}
	current, rows, err = extractSekaRunRows(`prefix [[1,2], [3,4]] suffix`)
	if err != nil || current != "" || len(rows) != 2 {
		t.Fatalf("fallback sekarun rows = %q, %#v, %v", current, rows, err)
	}
	if _, _, err := extractSekaRunRows("no rows"); err == nil {
		t.Fatal("invalid fallback sekarun payload unexpectedly succeeded")
	}

}

func testSekaRunScoreParsing(t *testing.T) {
	scoreCases := []struct {
		values []string
		want   int
		ok     bool
	}{
		{[]string{"", "", "", "", "123.6"}, 124, true},
		{[]string{"", "", "", "", "bad", "", "", "", "100", "200"}, 150, true},
		{[]string{"", "", "", "", "0"}, 0, false},
		{[]string{"short"}, 0, false},
		{[]string{"", "", "", "", "bad", "", "", "", "bad", "200"}, 0, false},
		{[]string{"", "", "", "", "bad", "", "", "", "-2", "0"}, 0, false},
	}
	for _, tt := range scoreCases {
		got, ok := parseSekaRunScore(tt.values)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseSekaRunScore(%#v) = %d, %v", tt.values, got, ok)
		}
	}

}

func testForecastNumberAndTimestampHelpers(t *testing.T) {
	testForecastIntegerConversions(t)
	testForecastTimestampConversions(t)
}

func testForecastIntegerConversions(t *testing.T) {
	intCases := []struct {
		value any
		want  int
		ok    bool
	}{
		{int(1), 1, true}, {int64(2), 2, true}, {float64(3.9), 3, true}, {float32(4.9), 4, true},
		{" 5 ", 5, true}, {"6.9", 6, true}, {"", 0, false}, {"bad", 0, false}, {true, 0, false},
	}
	for _, tt := range intCases {
		got, ok := asInt(tt.value)
		if got != tt.want || ok != tt.ok {
			t.Errorf("asInt(%#v) = %d, %v", tt.value, got, ok)
		}
	}
	int64Cases := []struct {
		value any
		want  int64
		ok    bool
	}{
		{int(1), 1, true}, {int64(2), 2, true}, {float64(3.9), 3, true}, {float32(4.9), 4, true},
		{" 5 ", 5, true}, {"6.9", 6, true}, {"", 0, false}, {"bad", 0, false}, {true, 0, false},
	}
	for _, tt := range int64Cases {
		got, ok := asInt64(tt.value)
		if got != tt.want || ok != tt.ok {
			t.Errorf("asInt64(%#v) = %d, %v", tt.value, got, ok)
		}
	}
}

func testForecastTimestampConversions(t *testing.T) {
	if normalizeForecastTimestamp(0) != 0 || normalizeForecastTimestamp(-1) != 0 {
		t.Fatal("non-positive forecast timestamp was not normalized to zero")
	}
	if normalizeForecastTimestamp(1_700_000_000) != 1_700_000_000_000 {
		t.Fatal("seconds timestamp was not converted to milliseconds")
	}
	if normalizeForecastTimestamp(1_700_000_000_001) != 1_700_000_000_001 {
		t.Fatal("millisecond timestamp changed")
	}
	if _, ok := parseForecastRFC3339(""); ok {
		t.Fatal("empty RFC3339 timestamp unexpectedly parsed")
	}
	if _, ok := parseForecastRFC3339("invalid"); ok {
		t.Fatal("invalid RFC3339 timestamp unexpectedly parsed")
	}
	if got, ok := parseForecastRFC3339("2024-01-02T03:04:05.123Z"); !ok || got != 1_704_164_645_123 {
		t.Fatalf("RFC3339 timestamp = %d, %v", got, ok)
	}
}

func TestForecastHTTPClientBranchesAdditional(t *testing.T) {
	ctx := context.Background()
	provider := additionalForecastProvider(t, func(req *http.Request) (int, string, error) {
		if strings.TrimSpace(req.Header.Get("User-Agent")) == "" {
			t.Error("forecast request missing User-Agent")
		}
		switch req.URL.Path {
		case "/json":
			return http.StatusOK, `{"value":7}`, nil
		case "/bad-json":
			return http.StatusOK, `{`, nil
		case "/text":
			return http.StatusOK, "hello", nil
		default:
			return http.StatusBadGateway, "bad", nil
		}
	})
	var payload struct {
		Value int `json:"value"`
	}
	if err := provider.getJSON(ctx, "http://forecast.test/json", &payload); err != nil || payload.Value != 7 {
		t.Fatalf("getJSON() = %+v, %v", payload, err)
	}
	if err := provider.getJSON(ctx, "http://forecast.test/bad-json", &payload); err == nil {
		t.Fatal("malformed forecast JSON unexpectedly succeeded")
	}
	if err := provider.getJSON(ctx, "http://forecast.test/status", &payload); err == nil {
		t.Fatal("non-200 forecast JSON unexpectedly succeeded")
	}
	if got, err := provider.getText(ctx, "http://forecast.test/text"); err != nil || got != "hello" {
		t.Fatalf("getText() = %q, %v", got, err)
	}
	if _, err := provider.getText(ctx, "http://forecast.test/status"); err == nil {
		t.Fatal("non-200 forecast text unexpectedly succeeded")
	}

	transportErr := errors.New("transport failed")
	failing := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return 0, "", transportErr
	})
	if err := failing.getJSON(ctx, "http://forecast.test/json", &payload); err == nil {
		t.Fatal("JSON transport failure unexpectedly succeeded")
	}
	if _, err := failing.getText(ctx, "http://forecast.test/text"); err == nil {
		t.Fatal("text transport failure unexpectedly succeeded")
	}
}

func TestRemoteForecastProviderSourceParsersAdditional(t *testing.T) {
	ctx := context.Background()
	provider := additionalForecastProvider(t, forecastSourceResponder)
	test33KitSourceParser(t, ctx, provider)
	testMoesekaiSourceParsers(t, ctx, provider)
	testSekaRunSourceParser(t, ctx, provider)
}

func forecastSourceResponder(req *http.Request) (int, string, error) {
	switch req.URL.Host {
	case "sekai-data.3-3.dev":
		return http.StatusOK, `{
				"event":{"id":10},
				"data":{"ts":1700000000,"100":12345,"200":"23456","bad":999,"0":111,"300":0,"400":"bad"}
			}`, nil
	case "rk.exmeaning.com":
		return http.StatusOK, `{
				"event_id":10,"status":"ok","updated_at":"2024-01-02T03:04:05Z",
				"items":[
					{"rank":100,"score":1,"prediction":50000},
					{"rank":200,"score":40000,"is_final":true},
					{"rank":300,"score":30000},
					{"rank":0,"prediction":1},
					{"rank":400,"prediction":0}
				]
			}`, nil
	case "sekaibangdan.exmeaning.com":
		return http.StatusOK, `{
				"timestamp":"1700000001",
				"data":{"charts":[
					{"Rank":"100","PredictedScore":"60000"},
					{"Rank":200,"PredictedScore":70000},
					{"Rank":"bad","PredictedScore":1},
					{"Rank":0,"PredictedScore":1},
					{"Rank":300,"PredictedScore":0}
				]}
			}`, nil
	case "jiiku831.github.io":
		return http.StatusOK, `currentEvent = "10"; data = [[10,p,0,0,80000,100,1700000002,0,0,0], [10,h,0,0,70000,200,1700000001,0,0,0]];`, nil
	default:
		return http.StatusNotFound, "", nil
	}
}

func test33KitSourceParser(t *testing.T, ctx context.Context, provider *RemoteForecastProvider) {
	for _, query := range []ForecastQuery{
		{Region: "jp", EventID: 10, Scope: ForecastScopeChapter},
		{Region: "cn", EventID: 10, Scope: ForecastScopeTotal},
	} {
		if got, err := provider.fetch33KitByQuery(ctx, query, nil); err != nil || len(got) != 0 {
			t.Errorf("33kit unsupported query = %#v, %v", got, err)
		}
	}
	got, err := provider.fetch33KitByQuery(ctx, ForecastQuery{Region: " JP ", EventID: 10}, map[int]struct{}{100: {}, 200: {}, 400: {}})
	if err != nil || len(got) != 2 || got[100].Score != 12_345 || got[200].Score != 23_456 || got[100].Timestamp != 1_700_000_000_000 {
		t.Fatalf("33kit scores = %#v, %v", got, err)
	}

}

func testMoesekaiSourceParsers(t *testing.T, ctx context.Context, provider *RemoteForecastProvider) {
	got, err := provider.fetchMoe(ctx, "jp", 10, map[int]struct{}{100: {}, 200: {}, 300: {}, 400: {}})
	if err != nil || len(got) != 2 || got[100].Score != 50_000 || got[200].Score != 40_000 || got[100].Source != "moesekai" {
		t.Fatalf("moesekai scores = %#v, %v", got, err)
	}
	got, err = provider.fetchSnowyLegacy(ctx, "cn", 10, map[int]struct{}{100: {}, 200: {}, 300: {}})
	if err != nil || len(got) != 2 || got[100].Score != 60_000 || got[200].Score != 70_000 || got[100].Timestamp != 1_700_000_001_000 {
		t.Fatalf("legacy moesekai scores = %#v, %v", got, err)
	}
	if _, err := provider.fetchSnowyLegacy(ctx, "jp", 10, map[int]struct{}{100: {}}); err != nil {
		t.Fatalf("JP legacy forecast failed: %v", err)
	}

}

func testSekaRunSourceParser(t *testing.T, ctx context.Context, provider *RemoteForecastProvider) {
	for _, query := range []ForecastQuery{
		{Region: "en", EventID: 10, Scope: ForecastScopeChapter},
		{Region: "jp", EventID: 10},
	} {
		if got, err := provider.fetchSekaRunByQuery(ctx, query, nil); err != nil || len(got) != 0 {
			t.Errorf("sekarun unsupported query = %#v, %v", got, err)
		}
	}
	got, err := provider.fetchSekaRunByQuery(ctx, ForecastQuery{Region: "EN", EventID: 10}, map[int]struct{}{100: {}, 200: {}})
	if err != nil || len(got) != 2 || got[100].Score != 80_000 || got[200].Score != 70_000 {
		t.Fatalf("sekarun scores = %#v, %v", got, err)
	}
}

func TestRemoteForecastProviderSourceErrorsAndFallbacksAdditional(t *testing.T) {
	testForecastSourceEventMismatches(t)
	testMoesekaiLegacyFallback(t)
	testMoesekaiSourceFailures(t)
}

func testForecastSourceEventMismatches(t *testing.T) {
	ctx := context.Background()
	mismatch33 := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusOK, `{"event":{"id":11},"data":{}}`, nil
	})
	if _, err := mismatch33.fetch33KitByQuery(ctx, ForecastQuery{Region: "jp", EventID: 10}, nil); err == nil {
		t.Fatal("33kit event mismatch unexpectedly succeeded")
	}
	mismatchMoe := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusOK, `{"event_id":11,"items":[]}`, nil
	})
	if _, err := mismatchMoe.fetchMoe(ctx, "jp", 10, nil); err == nil {
		t.Fatal("moesekai event mismatch unexpectedly succeeded")
	}

}

func testMoesekaiLegacyFallback(t *testing.T) {
	ctx := context.Background()
	legacyFallback := additionalForecastProvider(t, func(req *http.Request) (int, string, error) {
		if req.URL.Host == "rk.exmeaning.com" {
			return http.StatusOK, `{"event_id":10,"items":[]}`, nil
		}
		return http.StatusOK, `{"timestamp":1700000000,"data":{"charts":[{"Rank":100,"PredictedScore":99}]}}`, nil
	})
	got, err := legacyFallback.fetchMoesekaiByQuery(ctx, ForecastQuery{Region: "jp", EventID: 10}, nil)
	if err != nil || got[100].Score != 99 {
		t.Fatalf("legacy fallback = %#v, %v", got, err)
	}
	for _, query := range []ForecastQuery{
		{Region: "jp", EventID: 10, Scope: ForecastScopeChapter},
		{Region: "en", EventID: 10},
	} {
		if got, err := legacyFallback.fetchMoesekaiByQuery(ctx, query, nil); err != nil || len(got) != 0 {
			t.Errorf("moesekai unsupported query = %#v, %v", got, err)
		}
	}

}

func testMoesekaiSourceFailures(t *testing.T) {
	ctx := context.Background()
	bothFail := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusBadGateway, "", nil
	})
	if _, err := bothFail.fetchMoesekaiByQuery(ctx, ForecastQuery{Region: "cn", EventID: 10}, nil); err == nil || !strings.Contains(err.Error(), "rk=") || !strings.Contains(err.Error(), "legacy=") {
		t.Fatalf("combined moesekai error = %v", err)
	}
	rkFail := additionalForecastProvider(t, func(req *http.Request) (int, string, error) {
		if req.URL.Host == "rk.exmeaning.com" {
			return http.StatusBadGateway, "", nil
		}
		return http.StatusOK, `{"data":{"charts":[]}}`, nil
	})
	if _, err := rkFail.fetchMoesekaiByQuery(ctx, ForecastQuery{Region: "cn", EventID: 10}, nil); err == nil || strings.Contains(err.Error(), "legacy=") {
		t.Fatalf("rk-only moesekai error = %v", err)
	}
	legacyFail := additionalForecastProvider(t, func(req *http.Request) (int, string, error) {
		if req.URL.Host == "rk.exmeaning.com" {
			return http.StatusOK, `{"event_id":10,"items":[]}`, nil
		}
		return http.StatusBadGateway, "", nil
	})
	if _, err := legacyFail.fetchMoesekaiByQuery(ctx, ForecastQuery{Region: "cn", EventID: 10}, nil); err == nil || strings.Contains(err.Error(), "rk=") {
		t.Fatalf("legacy-only moesekai error = %v", err)
	}
}

func TestRemoteForecastProviderLocalBranchesAdditional(t *testing.T) {
	ctx := context.Background()
	provider := additionalForecastProvider(t, func(req *http.Request) (int, string, error) {
		if strings.HasSuffix(req.URL.Path, "/total") {
			return http.StatusBadGateway, "", nil
		}
		return http.StatusOK, `{
			"region":"tw","event_id":20,"updated_at":"1700000000","leaderboard_scope":"chapter",
			"lines":[
				{"leaderboard_scope":"total","rows":[{"rank":100,"prediction":1}]},
				{"leaderboard_scope":"chapter","character_id":21,"current_timestamp":"1700000001","rows":[{"rank":100,"prediction":111,"current_timestamp":"1700000002"}]},
				{"leaderboard_scope":"chapter","character_id":"22","rows":[
					{"rank":"100","prediction":"222","current_timestamp":"1700000003"},
					{"rank":200,"prediction":333},
					{"rank":"bad","prediction":1},
					{"rank":0,"prediction":1},
					{"rank":300,"prediction":0}
				]}
			]
		}`, nil
	})
	provider.localForecastURL = "http://local.test/"
	if _, err := provider.fetchLocalForecastByQuery(ctx, ForecastQuery{}, nil); err == nil {
		t.Fatal("invalid local forecast params unexpectedly succeeded")
	}
	withoutLocal := *provider
	withoutLocal.localForecastURL = ""
	if got, err := withoutLocal.fetchLocalForecastByQuery(ctx, ForecastQuery{Region: "tw", EventID: 20}, nil); err != nil || len(got) != 0 {
		t.Fatalf("disabled local forecast = %#v, %v", got, err)
	}
	if got := buildLocalForecastURLs("http://local", ForecastQuery{Region: "jp", Scope: ForecastScopeChapter}); !reflect.DeepEqual(got, []string{"http://local/prediction/jp/chapter", "http://local/prediction/jp"}) {
		t.Fatalf("chapter local URLs = %#v", got)
	}
	if got := buildLocalForecastURLs("http://local", ForecastQuery{Region: "jp"}); !reflect.DeepEqual(got, []string{"http://local/prediction/jp/total", "http://local/prediction/jp"}) {
		t.Fatalf("total local URLs = %#v", got)
	}

	characterID := 22
	got, err := provider.fetchLocalForecastByQuery(ctx, ForecastQuery{Region: "TW", EventID: 20, Scope: ForecastScopeChapter, WlCharacterID: &characterID}, map[int]struct{}{100: {}, 200: {}, 300: {}})
	if err != nil || len(got) != 2 || got[100].Score != 222 || got[100].Timestamp != 1_700_000_003_000 || got[200].Score != 333 {
		t.Fatalf("local chapter forecast = %#v, %v", got, err)
	}

	eventMismatch := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusOK, `{"event_id":21,"lines":[]}`, nil
	})
	if _, err := eventMismatch.fetchLocalForecastURL(ctx, "http://local", ForecastQuery{Region: "tw", EventID: 20}, nil); err == nil {
		t.Fatal("local event mismatch unexpectedly succeeded")
	}
	regionMismatch := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusOK, `{"region":"jp","event_id":20,"lines":[]}`, nil
	})
	if _, err := regionMismatch.fetchLocalForecastURL(ctx, "http://local", ForecastQuery{Region: "tw", EventID: 20}, nil); err == nil {
		t.Fatal("local region mismatch unexpectedly succeeded")
	}
	allFail := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusBadGateway, "", nil
	})
	allFail.localForecastURL = "http://local.test"
	if _, err := allFail.fetchLocalForecastByQuery(ctx, ForecastQuery{Region: "tw", EventID: 20}, nil); err == nil {
		t.Fatal("all local URL failures unexpectedly succeeded")
	}
}

func TestSekaRunForecastBranchesAdditional(t *testing.T) {
	body := `currentEvent = "10"; data = [[
		10,p,0,0,100.4,100,1700000000,0,0,0], [
		10,p,0,0,200.4,100,1700000001,0,0,0], [
		10,h,0,0,150,100,1699999999,0,0,0], [
		10,h,0,0,bad,200,1700000000,0,100,300], [
		10,x,0,0,999,300,1700000000,0,0,0], [
		10,p,0,0,999,bad,1700000000,0,0,0], [
		10,p,0,0,999,0,1700000000,0,0,0], [
		10,p,0,0,bad,400,1700000000,0,bad,bad], [
		11,p,0,0,999,500,1700000000,0,0,0], [
		short,row
	]];`
	got, err := parseSekaRunForecast(body, 10, map[int]struct{}{100: {}, 200: {}, 300: {}, 400: {}})
	if err != nil || len(got) != 2 || got[100].Score != 200 || got[200].Score != 200 {
		t.Fatalf("parsed sekarun forecast = %#v, %v", got, err)
	}
	if _, err := parseSekaRunForecast(`currentEvent = "11"; data = [[11,p,0,0,1,100,1,0,0,0]];`, 10, nil); err == nil {
		t.Fatal("sekarun event mismatch unexpectedly succeeded")
	}
	got, err = parseSekaRunForecast(`currentEvent = "10"; data = [[10,p,0,0,bad,100,1,0,bad,bad]];`, 10, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty matched sekarun event = %#v, %v", got, err)
	}
	if _, err := parseSekaRunForecast("invalid", 10, nil); err == nil {
		t.Fatal("invalid sekarun body unexpectedly succeeded")
	}
}

func TestRemoteForecastProviderOrchestrationAdditional(t *testing.T) {
	testRemoteForecastProviderValidation(t)
	local, bySource := testRemoteForecastProviderLocalFetches(t)
	testForecastQueryNormalization(t)
	testForecastSourceOrdering(t, local)
	testForecastFetchedAt(t, bySource)
}

func testRemoteForecastProviderValidation(t *testing.T) {
	ctx := context.Background()
	if _, err := (*RemoteForecastProvider)(nil).FetchBySourceQuery(ctx, ForecastQuery{Region: "jp", EventID: 1}); err == nil {
		t.Fatal("nil remote forecast provider unexpectedly succeeded")
	}
	provider := NewRemoteForecastProviderWithConfig(ForecastConfig{LocalBaseURL: "http://local.test/"})
	for _, query := range []ForecastQuery{{Region: "", EventID: 1}, {Region: "jp", EventID: 0}} {
		if _, err := provider.FetchBySourceQuery(ctx, query); err == nil {
			t.Errorf("invalid query %#v unexpectedly succeeded", query)
		}
	}
	if _, err := provider.FetchBySourceQuery(ctx, ForecastQuery{Region: "xx", EventID: 1}); err == nil {
		t.Fatal("unsupported region unexpectedly succeeded")
	}

}

func testRemoteForecastProviderLocalFetches(t *testing.T) (*RemoteForecastProvider, map[string]ForecastSourceData) {
	ctx := context.Background()
	local := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusOK, `{"region":"tw","event_id":20,"lines":[{"leaderboard_scope":"total","rows":[{"rank":100,"prediction":1000},{"rank":200,"prediction":2000}]}]}`, nil
	})
	local.localForecastURL = "http://local.test"
	bySource, err := local.FetchBySource(ctx, " TW ", 20, []int{-1, 100, 200})
	if err != nil || len(bySource) != 1 || len(bySource["local"].Scores) != 2 || bySource["local"].FetchedAt <= 0 {
		t.Fatalf("FetchBySource() = %#v, %v", bySource, err)
	}
	merged, err := local.Fetch(ctx, "tw", 20, []int{100})
	if err != nil || len(merged) != 1 || merged[100].Score != 1000 {
		t.Fatalf("Fetch() = %#v, %v", merged, err)
	}
	merged, err = local.FetchQuery(ctx, ForecastQuery{Region: "tw", EventID: 20, Ranks: []int{100}})
	if err != nil || merged[100].Score != 1000 {
		t.Fatalf("FetchQuery() = %#v, %v", merged, err)
	}

	empty := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusOK, `{"region":"tw","event_id":20,"lines":[]}`, nil
	})
	empty.localForecastURL = "http://local.test"
	if got, err := empty.FetchBySourceQuery(ctx, ForecastQuery{Region: "tw", EventID: 20}); err != nil || len(got) != 0 {
		t.Fatalf("empty source result = %#v, %v", got, err)
	}
	failing := additionalForecastProvider(t, func(*http.Request) (int, string, error) {
		return http.StatusBadGateway, "", nil
	})
	failing.localForecastURL = "http://local.test"
	if _, err := failing.FetchBySourceQuery(ctx, ForecastQuery{Region: "tw", EventID: 20}); err == nil || !strings.Contains(err.Error(), "all forecast sources failed") {
		t.Fatalf("all-source failure = %v", err)
	}

	return local, bySource
}

func testForecastQueryNormalization(t *testing.T) {
	chapterID := -1
	normalized := normalizeForecastQuery(ForecastQuery{Region: " JP ", Ranks: []int{100, -1, 100, 200}, Scope: "invalid", WlCharacterID: &chapterID})
	if normalized.Region != "jp" || !reflect.DeepEqual(normalized.Ranks, []int{100, 200}) || normalized.Scope != ForecastScopeTotal || normalized.WlCharacterID != nil {
		t.Fatalf("normalized forecast query = %#v", normalized)
	}
	chapterID = 21
	normalized = normalizeForecastQuery(ForecastQuery{Region: "cn", Scope: ForecastScopeChapter, WlCharacterID: &chapterID})
	if normalized.Scope != ForecastScopeChapter || normalized.WlCharacterID == nil || *normalized.WlCharacterID != 21 {
		t.Fatalf("normalized chapter query = %#v", normalized)
	}

}

func testForecastSourceOrdering(t *testing.T, local *RemoteForecastProvider) {
	for _, tt := range []struct {
		query ForecastQuery
		want  []string
	}{
		{ForecastQuery{Region: "jp"}, []string{"33kit", "moesekai", "local"}},
		{ForecastQuery{Region: "cn"}, []string{"moesekai", "local"}},
		{ForecastQuery{Region: "en"}, []string{"sekarun", "local"}},
		{ForecastQuery{Region: "tw"}, []string{"local"}},
		{ForecastQuery{Region: "kr"}, []string{"local"}},
		{ForecastQuery{Region: "xx"}, []string{}},
		{ForecastQuery{Region: "jp", Scope: ForecastScopeChapter}, []string{"local"}},
		{ForecastQuery{Region: "xx", Scope: ForecastScopeChapter}, []string{}},
	} {
		sources := local.sourcesForQuery(tt.query)
		names := make([]string, 0, len(sources))
		for _, source := range sources {
			names = append(names, source.name)
		}
		if !reflect.DeepEqual(names, tt.want) {
			t.Errorf("sourcesForQuery(%#v) = %#v", tt.query, names)
		}
	}
	if names := local.sourcesForRegion("jp"); len(names) != 3 {
		t.Fatalf("sourcesForRegion(jp) = %#v", names)
	}

	if got := forecastSourceOrderForRegion("unknown"); got != nil {
		t.Fatalf("unknown source order = %#v", got)
	}
	if got := forecastSourceDisplayOrder("jp", nil); got != nil {
		t.Fatalf("empty source display order = %#v", got)
	}
	data := map[string]ForecastSourceData{"local": {}, "extra-b": {}, "33kit": {}, "extra-a": {}}
	if got := forecastSourceDisplayOrder(" JP ", data); !reflect.DeepEqual(got, []string{"33kit", "local", "extra-a", "extra-b"}) {
		t.Fatalf("source display order = %#v", got)
	}
	for region, want := range map[string][]string{
		"cn": {"moesekai", "local"}, "en": {"sekarun", "local"}, "tw": {"local"}, "kr": {"local"},
	} {
		if got := forecastSourceOrderForRegion(region); !reflect.DeepEqual(got, want) {
			t.Errorf("forecastSourceOrderForRegion(%q) = %#v", region, got)
		}
	}

}

func testForecastFetchedAt(t *testing.T, bySource map[string]ForecastSourceData) {
	start := time.Now().UTC().UnixMilli()
	if bySource["local"].FetchedAt < start-5_000 {
		t.Fatalf("unexpected fetched-at timestamp: %d", bySource["local"].FetchedAt)
	}
}
