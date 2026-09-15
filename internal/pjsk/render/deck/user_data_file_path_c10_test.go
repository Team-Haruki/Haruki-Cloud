package deck

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/utils/logger"
)

type rawPathSnapshot struct {
	rendersnapshot.Snapshot
	path string
}

func (s rawPathSnapshot) RawFilePath() string { return s.path }

func errorRecords(logs string) []string {
	var records []string
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if strings.Contains(line, "level=ERROR") {
			records = append(records, line)
		}
	}
	return records
}

func TestRemoteRecommendLogsUserDataFilePathFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"decks":[]}`))
	}))
	defer server.Close()

	var logs bytes.Buffer
	remote := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	remote.logger = logger.NewLogger("DeckRemoteC10", "DEBUG", &logs)
	exec := testRemoteExecution(t, remote)
	defer exec.Release()
	request := testRemoteRecommendRequest()
	request.Region = "jp"
	request.UserData = nil
	request.UserDataFilePath = "/tmp/snapshots/haruki-pjsk-user-123.json"

	if _, err := remote.doRecommendLegacyOption(context.Background(), exec, request, request.BatchOption[0]); err != nil {
		t.Fatalf("legacy request error = %v", err)
	}
	records := errorRecords(logs.String())
	if len(records) != 1 {
		t.Fatalf("want exactly one ERROR record, got %d:\n%s", len(records), logs.String())
	}
	for _, want := range []string{"deck user_data_file_path fallback used", "region=jp", "path_base=haruki-pjsk-user-123.json", "count=1"} {
		if !strings.Contains(records[0], want) {
			t.Fatalf("record missing %q: %s", want, records[0])
		}
	}
	if strings.Contains(records[0], "/tmp/snapshots") {
		t.Fatalf("record must carry only the basename: %s", records[0])
	}
	if got := remote.UserDataFilePathFallbacks(); got != 1 {
		t.Fatalf("counter = %d, want 1", got)
	}
}

func TestRemoteRecommendWithUserDataDoesNotLogFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"decks":[]}`))
	}))
	defer server.Close()

	var logs bytes.Buffer
	remote := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	remote.logger = logger.NewLogger("DeckRemoteC10", "DEBUG", &logs)
	exec := testRemoteExecution(t, remote)
	defer exec.Release()
	request := testRemoteRecommendRequest()
	request.UserDataFilePath = "/tmp/user.json"

	if _, err := remote.doRecommendLegacyOption(context.Background(), exec, request, request.BatchOption[0]); err != nil {
		t.Fatalf("legacy request error = %v", err)
	}
	if records := errorRecords(logs.String()); len(records) != 0 {
		t.Fatalf("unexpected ERROR records:\n%s", logs.String())
	}
	if got := remote.UserDataFilePathFallbacks(); got != 0 {
		t.Fatalf("counter = %d, want 0", got)
	}
}

func TestUserDataFilePathFallbackNilSafety(t *testing.T) {
	var remote *RemoteDeckRecommender
	if remote.UserDataFilePathFallbacks() != 0 {
		t.Fatal("nil remote counter must be 0")
	}
	// A recommender without a logger or execution still counts.
	bare := &RemoteDeckRecommender{}
	bare.logUserDataFilePathFallback(context.Background(), nil, "en", "/x/y.json")
	if bare.UserDataFilePathFallbacks() != 1 {
		t.Fatal("bare recommender did not count")
	}
	var controller *Controller
	if controller.UserDataFilePathFallbacks() != 0 {
		t.Fatal("nil controller counter must be 0")
	}
}

func TestControllerLogsUserDataFilePathFallback(t *testing.T) {
	var logs bytes.Buffer
	controller := &Controller{
		snapshot:                  rawPathSnapshot{path: "/var/tmp/haruki-pjsk-user-9.json"},
		logger:                    logger.NewLogger("DeckC10", "DEBUG", &logs),
		userDataFilePathFallbacks: new(atomic.Int64),
	}
	clone := controller.WithSnapshot(rawPathSnapshot{path: "/var/tmp/haruki-pjsk-user-9.json"})

	if got := clone.userDataFilePathForRequest(context.Background(), renderregion.JP, []byte(`{}`)); got != "/var/tmp/haruki-pjsk-user-9.json" {
		t.Fatalf("path with bytes = %q", got)
	}
	if records := errorRecords(logs.String()); len(records) != 0 {
		t.Fatalf("bytes present must not log:\n%s", logs.String())
	}

	if got := clone.userDataFilePathForRequest(context.Background(), renderregion.JP, nil); got != "/var/tmp/haruki-pjsk-user-9.json" {
		t.Fatalf("path without bytes = %q", got)
	}
	records := errorRecords(logs.String())
	if len(records) != 1 {
		t.Fatalf("want exactly one ERROR record:\n%s", logs.String())
	}
	for _, want := range []string{"deck user_data_file_path fallback resolved", "region=jp", "path_base=haruki-pjsk-user-9.json", "count=1"} {
		if !strings.Contains(records[0], want) {
			t.Fatalf("record missing %q: %s", want, records[0])
		}
	}
	// Clones share the counter with the constructed controller.
	if controller.UserDataFilePathFallbacks() != 1 || clone.UserDataFilePathFallbacks() != 1 {
		t.Fatalf("counter = %d/%d, want 1", controller.UserDataFilePathFallbacks(), clone.UserDataFilePathFallbacks())
	}

	// No path: nothing to log. No logger / counter: global logger, no panic.
	empty := &Controller{snapshot: rawPathSnapshot{}}
	if got := empty.userDataFilePathForRequest(context.Background(), renderregion.JP, nil); got != "" {
		t.Fatalf("empty path = %q", got)
	}
	bare := &Controller{snapshot: rawPathSnapshot{path: "/a/b.json"}}
	if got := bare.userDataFilePathForRequest(context.Background(), renderregion.EN, nil); got != "/a/b.json" {
		t.Fatalf("bare path = %q", got)
	}
	if bare.UserDataFilePathFallbacks() != 0 {
		t.Fatal("controller without a counter must report 0")
	}

	constructed := NewController(nil, nil, nil, nil, nil, renderregion.JP)
	if constructed.logger == nil || constructed.userDataFilePathFallbacks == nil {
		t.Fatal("NewController must set the C10 logger and counter")
	}
}
