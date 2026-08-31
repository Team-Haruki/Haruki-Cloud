package deck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"haruki-cloud/internal/core/upstream"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

func TestRemoteRecommendBatchContextCoversEarlyErrors(t *testing.T) {
	request := testRemoteRecommendRequest()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	remote := newStandaloneTestRemoteDeckRecommender("http://127.0.0.1:1", http.DefaultClient)
	if _, err := remote.RecommendBatchContext(canceled, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request error = %v", err)
	}
	request.BatchOption = nil
	if _, err := remote.RecommendBatchContext(context.Background(), request); err == nil {
		t.Fatal("expected empty batch_options validation error")
	}
	unconfigured := &RemoteDeckRecommender{client: http.DefaultClient, pool: upstream.NewPool(nil)}
	if _, err := unconfigured.RecommendBatchContext(context.Background(), testRemoteRecommendRequest()); err == nil {
		t.Fatal("expected unconfigured pool error")
	}
}

func TestDoRecommendBatchCoversProtocolErrors(t *testing.T) {
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cache_userdata":
			if mode.Load() == 0 {
				_, _ = w.Write([]byte(`{"userdata_hash":""}`))
				return
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"coverage-hash"}`))
		case "/recommend":
			http.Error(w, "recommend failed", http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	remote := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	remote.logger = logger.NewLogger("DeckRemoteCoverage", "ERROR", nil)
	exec := testRemoteExecution(t, remote)
	defer exec.Release()
	request := testRemoteRecommendRequest()
	if _, err := remote.doRecommendBatch(context.Background(), exec, request); err == nil || !strings.Contains(err.Error(), "empty userdata_hash") {
		t.Fatalf("empty hash error = %v", err)
	}
	mode.Store(1)
	if _, err := remote.doRecommendBatch(context.Background(), exec, request); err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("recommend error = %v", err)
	}
	request.BatchOption = []map[string]any{{"unsupported": make(chan int)}}
	if _, err := remote.doRecommendBatch(context.Background(), exec, request); err == nil {
		t.Fatal("expected recommend payload encoding error")
	}
}

func TestDoRecommendLegacyOptionCoversInputBranches(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode legacy request: %v", err)
		}
		capturedPath, _ = body["user_data_file_path"].(string)
		_, _ = w.Write([]byte(`{"decks":[]}`))
	}))
	defer server.Close()

	remote := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	remote.logger = logger.NewLogger("DeckRemoteCoverage", "ERROR", nil)
	exec := testRemoteExecution(t, remote)
	defer exec.Release()
	request := testRemoteRecommendRequest()
	request.UserData = nil
	request.UserDataFilePath = "/tmp/user.json"
	if _, err := remote.doRecommendLegacyOption(context.Background(), exec, request, request.BatchOption[0]); err != nil {
		t.Fatalf("file-path legacy request error = %v", err)
	}
	if capturedPath != "/tmp/user.json" {
		t.Fatalf("captured user data path = %q", capturedPath)
	}
	request.UserDataFilePath = ""
	if _, err := remote.doRecommendLegacyOption(context.Background(), exec, request, request.BatchOption[0]); err == nil {
		t.Fatal("expected missing user data error")
	}
}

func TestWaitForReadyFlightCoversResultErrors(t *testing.T) {
	boom := errors.New("boom")
	result := make(chan singleflight.Result, 1)
	result <- singleflight.Result{Err: boom}
	if _, err := waitForReadyFlight(context.Background(), result, new(remoteReadyFlightToken)); !errors.Is(err, boom) {
		t.Fatalf("flight error = %v", err)
	}
	result = make(chan singleflight.Result, 1)
	result <- singleflight.Result{Val: "unexpected"}
	if _, err := waitForReadyFlight(context.Background(), result, new(remoteReadyFlightToken)); err == nil {
		t.Fatal("expected unexpected flight result error")
	}
	result = make(chan singleflight.Result, 1)
	result <- singleflight.Result{Val: remoteReadyFlightResult{err: boom}}
	if _, err := waitForReadyFlight(context.Background(), result, new(remoteReadyFlightToken)); !errors.Is(err, boom) {
		t.Fatalf("shared work error = %v", err)
	}
}
