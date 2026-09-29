package sekai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"haruki-cloud/config"
)

func TestToolboxSuiteFieldsConditionalRead(t *testing.T) {
	fields := []string{"upload_time", "userGamedata", "userMysekaiColorfulPass"}
	const projected = `{"upload_time":1710000000,"userGamedata":{"userId":339871638031728641},"userMysekaiColorfulPass":null}`
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/api/private/game-data/jp/suite/123456789" || query.Get("key") != "upload_time,userGamedata,userMysekaiColorfulPass" {
			t.Errorf("unexpected projected request: %s", r.URL)
		}
		if query.Get("platform") != "qq" {
			t.Errorf("missing platform")
		}
		queries = append(queries, query.Get("known_upload_time"))
		if query.Get("platform_user_id") == "revoked" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"invalid platform or platform_user_id"}`))
			return
		}
		if query.Get("known_upload_time") == "1710000000" {
			w.Header().Set("X-Upload-Time", "1710000000")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(projected))
	}))
	defer server.Close()
	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, ConditionalFetch: true})
	for _, known := range []int64{0, 1710000000, 1700000000} {
		data, notModified, err := client.GetSuiteDataFieldsConditionalContext(context.Background(), "jp", 123456789, "qq", "allowed", known, fields)
		if err != nil {
			t.Fatal(err)
		}
		if known == 1710000000 {
			if !notModified || len(data) != 0 {
				t.Fatal("warm projection did not preserve 304")
			}
		} else if notModified || string(data) != projected {
			t.Fatalf("projected body changed: %s", data)
		}
	}
	if _, _, err := client.GetSuiteDataFieldsConditionalContext(context.Background(), "jp", 123456789, "qq", "revoked", 1710000000, fields); !errors.Is(err, ErrInvalidPlatformUser) {
		t.Fatalf("warm authorization error = %v", err)
	}
	if !reflect.DeepEqual(queries, []string{"", "1710000000", "1700000000", "1710000000"}) {
		t.Fatalf("conditional reads must stay one-hop: %v", queries)
	}
}

func TestToolboxSuiteFieldsLegacyProbeKeepsProjection(t *testing.T) {
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		keys = append(keys, key)
		if key == "upload_time" {
			_, _ = w.Write([]byte(`1710000001`))
			return
		}
		_, _ = w.Write([]byte(`{"upload_time":1710000001,"userGamedata":{"userId":123}}`))
	}))
	defer server.Close()
	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
	fields := []string{"upload_time", "userGamedata"}
	data, unchanged, err := client.GetSuiteDataFieldsConditionalContext(context.Background(), "jp", 123, "qq", "allowed", 1710000000, fields)
	if err != nil || unchanged || len(data) == 0 {
		t.Fatalf("changed read = %s, %t, %v", data, unchanged, err)
	}
	data, unchanged, err = client.GetSuiteDataFieldsConditionalContext(context.Background(), "jp", 123, "qq", "allowed", 1710000001, fields)
	if err != nil || !unchanged || len(data) != 0 {
		t.Fatalf("unchanged read = %s, %t, %v", data, unchanged, err)
	}
	if !reflect.DeepEqual(keys, []string{"upload_time", "upload_time,userGamedata", "upload_time"}) {
		t.Fatalf("legacy request keys = %v", keys)
	}
}
