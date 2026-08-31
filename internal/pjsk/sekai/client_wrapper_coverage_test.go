package sekai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/config"
)

func TestSekaiAPIContextAndThumbnailWrappers(t *testing.T) {
	var nilClient *HarukiSekaiAPIClient
	if _, err := nilClient.GetUserProfileContext(context.Background(), "jp", "1"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("nil contextual profile error = %v", err)
	}
	if _, err := nilClient.GetCustomProfileCardThumbnail("jp", "image"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("nil thumbnail error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/profile"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"user":{"userId":123}}`))
		case strings.Contains(r.URL.Path, "/custom-profile-card/thumbnail/"):
			_, _ = w.Write([]byte("thumbnail"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL, Token: "token"})
	profile, err := client.GetUserProfileContext(context.Background(), "jp", "1")
	if err != nil || profile.User.UserID != 123 {
		t.Fatalf("contextual profile = %+v,%v", profile, err)
	}
	thumbnail, err := client.GetCustomProfileCardThumbnail("jp", "hash/id")
	if err != nil || string(thumbnail) != "thumbnail" {
		t.Fatalf("thumbnail = %q,%v", thumbnail, err)
	}
}

func TestToolboxConvenienceWrappers(t *testing.T) {
	client := newToolboxConvenienceTestClient(t)
	ctx := context.Background()

	conditional, notModified, err := client.GetMySekaiDataConditionalContext(ctx, "jp", 123, "qq", "456", 0)
	if err != nil || notModified || !strings.Contains(string(conditional), "snapshot") {
		t.Fatalf("conditional mysekai = %q,%v,%v", conditional, notModified, err)
	}

	assertToolboxBytesCall(t, "suite", func() ([]byte, error) {
		return client.GetSuiteData("jp", 123, "qq", "456")
	})
	assertToolboxBytesCall(t, "mysekai", func() ([]byte, error) {
		return client.GetMySekaiData("jp", 123, "qq", "456")
	})
	assertToolboxBytesCall(t, "mysekai ctx", func() ([]byte, error) {
		return client.GetMySekaiDataContext(ctx, "jp", 123, "qq", "456")
	})
	assertToolboxBytesCall(t, "values ctx", func() ([]byte, error) {
		return client.GetPrivateDataValuesContext(ctx, "jp", ToolboxDataTypeSuite, 123, "qq", "456", "a", "b")
	})
	assertToolboxBytesCall(t, "upload wrapper", func() ([]byte, error) {
		return client.GetUploadTime("jp", ToolboxDataTypeSuite, 123, "qq", "456")
	})
}

func TestToolboxUploadTimeWrappers(t *testing.T) {
	client := newToolboxConvenienceTestClient(t)
	ctx := context.Background()

	if got, err := client.GetSuiteUploadTimeContext(ctx, "jp", 123, "qq", "456"); err != nil || got != 123 {
		t.Fatalf("suite upload time = %d,%v", got, err)
	}
	if got, err := client.GetMySekaiUploadTimeContext(ctx, "jp", 123, "qq", "456"); err != nil || got != 123 {
		t.Fatalf("mysekai upload time = %d,%v", got, err)
	}
}

func TestToolboxBindingWrappers(t *testing.T) {
	client := newToolboxConvenienceTestClient(t)
	ctx := context.Background()

	bindings, err := client.GetToolboxUserFastVerificationGameAccountBindings("qq", "456")
	if err != nil || len(bindings) != 1 || bindings[0].GameUserID != "123" {
		t.Fatalf("bindings wrapper = %+v,%v", bindings, err)
	}
	bindings, err = client.GetToolboxUserFastVerificationGameAccountBindingsContext(ctx, "qq", "456")
	if err != nil || len(bindings) != 1 {
		t.Fatalf("bindings context wrapper = %+v,%v", bindings, err)
	}
}

func newToolboxConvenienceTestClient(t *testing.T) *HarukiToolboxClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/private/game-binding" {
			_, _ = w.Write([]byte(`[{"server":"jp","gameUserId":"123"}]`))
			return
		}
		if !strings.Contains(r.URL.Path, "/api/private/game-data/") {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("key") {
		case "upload_time":
			_, _ = w.Write([]byte("123"))
		case "a,b":
			_, _ = w.Write([]byte(`{"a":1,"b":2}`))
		default:
			_, _ = w.Write([]byte(`{"snapshot":true}`))
		}
	}))
	t.Cleanup(server.Close)
	return NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, APIToken: "token"})
}

func assertToolboxBytesCall(t *testing.T, name string, call func() ([]byte, error)) {
	t.Helper()
	data, err := call()
	if err != nil || len(data) == 0 {
		t.Fatalf("%s = %q,%v", name, data, err)
	}
}
