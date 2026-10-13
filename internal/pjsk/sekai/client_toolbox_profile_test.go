package sekai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"haruki-cloud/config"
)

// recordingToolbox answers every private game-data read with a small Suite
// body and records each request's query.
type recordingToolbox struct {
	mu      sync.Mutex
	queries []url.Values
}

func (r *recordingToolbox) handler(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.queries = append(r.queries, req.URL.Query())
	r.mu.Unlock()
	if req.URL.Query().Get("key") == "upload_time" {
		_, _ = w.Write([]byte(`1710000500`))
		return
	}
	_, _ = w.Write([]byte(`{"upload_time":1710000500}`))
}

func newProfileTestClient(t *testing.T, profile *string, conditional bool) (*HarukiToolboxClient, *recordingToolbox) {
	t.Helper()
	rec := &recordingToolbox{}
	server := httptest.NewServer(http.HandlerFunc(rec.handler))
	t.Cleanup(server.Close)
	return NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, ConditionalFetch: conditional, SuiteProfile: profile}), rec
}

func strPtr(s string) *string { return &s }

func TestToolboxWholeSuiteReadSendsConfiguredProfile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile *string
		want    string
	}{
		{"default", nil, "cloud"},
		{"explicit cloud", strPtr("cloud"), "cloud"},
		{"rollback to whole document", strPtr(""), ""},
	} {
		for _, known := range []int64{0, 1710000000} {
			client, rec := newProfileTestClient(t, tc.profile, true)
			if got := client.SuiteProfile(); got != tc.want {
				t.Fatalf("%s: SuiteProfile() = %q, want %q", tc.name, got, tc.want)
			}
			if _, _, err := client.GetSuiteDataConditionalContext(context.Background(), "jp", 123456789, "qq", "10001", known); err != nil {
				t.Fatalf("%s: GetSuiteDataConditionalContext() error = %v", tc.name, err)
			}
			if len(rec.queries) != 1 {
				t.Fatalf("%s: requests = %d, want 1", tc.name, len(rec.queries))
			}
			q := rec.queries[0]
			if got, present := q["profile"]; tc.want == "" && present {
				t.Fatalf("%s: whole-document read sent profile=%v", tc.name, got)
			}
			if got := q.Get("profile"); got != tc.want {
				t.Fatalf("%s known=%d: profile = %q, want %q", tc.name, known, got, tc.want)
			}
			if q.Get("key") != "" {
				t.Fatalf("%s: whole-Suite read sent key=%q", tc.name, q.Get("key"))
			}
		}
	}
}

func TestToolboxProfileIsNeverCombinedWithKeyOrMySekai(t *testing.T) {
	client, rec := newProfileTestClient(t, nil, true)
	ctx := context.Background()
	if _, _, err := client.GetSuiteDataFieldsConditionalContext(ctx, "jp", 1, "qq", "10001", 0, []string{"upload_time", "userCards"}); err != nil {
		t.Fatalf("fields read error = %v", err)
	}
	if _, _, err := client.GetMySekaiDataConditionalContext(ctx, "jp", 1, "qq", "10001", 0); err != nil {
		t.Fatalf("mysekai read error = %v", err)
	}
	if _, _, err := client.GetPrivateDataConditionalContext(ctx, "jp", ToolboxDataTypeSuite, 1, "qq", "10001", 0); err != nil {
		t.Fatalf("generic private read error = %v", err)
	}
	if _, err := client.GetPrivateDataValueContext(ctx, "jp", ToolboxDataTypeSuite, 1, "qq", "10001", "userGamedata"); err != nil {
		t.Fatalf("value read error = %v", err)
	}
	if _, err := client.GetUploadTimeContext(ctx, "jp", ToolboxDataTypeSuite, 1, "qq", "10001"); err != nil {
		t.Fatalf("upload_time read error = %v", err)
	}
	if len(rec.queries) != 5 {
		t.Fatalf("requests = %d, want 5", len(rec.queries))
	}
	for i, q := range rec.queries {
		if _, present := q["profile"]; present {
			t.Fatalf("request %d (%s) sent profile=%q", i, q.Encode(), q.Get("profile"))
		}
	}
	if got := rec.queries[0].Get("key"); got != "upload_time,userCards" {
		t.Fatalf("fields read key = %q", got)
	}
}

// Without conditional read support the 304 is emulated by a key=upload_time
// probe; only the body fetch that follows carries the profile.
func TestToolboxEmulatedConditionalReadSendsProfileOnBodyFetchOnly(t *testing.T) {
	client, rec := newProfileTestClient(t, nil, false)
	data, notModified, err := client.GetSuiteDataConditionalContext(context.Background(), "jp", 1, "qq", "10001", 1710000000)
	if err != nil || notModified || string(data) != `{"upload_time":1710000500}` {
		t.Fatalf("emulated read: data=%q notModified=%v err=%v", data, notModified, err)
	}
	if len(rec.queries) != 2 {
		t.Fatalf("requests = %d, want probe + fetch", len(rec.queries))
	}
	probe, fetch := rec.queries[0], rec.queries[1]
	if probe.Get("key") != "upload_time" || probe.Has("profile") {
		t.Fatalf("probe query = %s", probe.Encode())
	}
	if fetch.Get("profile") != "cloud" || fetch.Has("key") || fetch.Has("known_upload_time") {
		t.Fatalf("fetch query = %s", fetch.Encode())
	}
}

func TestToolboxSuiteProfileNilSafe(t *testing.T) {
	var client *HarukiToolboxClient
	if got := client.SuiteProfile(); got != "" {
		t.Fatalf("nil client SuiteProfile() = %q", got)
	}
	if got := (&HarukiToolboxClient{}).SuiteProfile(); got != "" {
		t.Fatalf("unconfigured client SuiteProfile() = %q", got)
	}
}
