package drawing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"haruki-cloud/internal/core/upstreamerr"
)

// TestDrawingResponsesClassifyByContract drives the Drawing client with the
// error answers Drawing sends and checks the classification the replies
// depend on (see upstreamerr/contract.go).
func TestDrawingResponsesClassifyByContract(t *testing.T) {
	cases := []struct {
		status int
		body   string
		kind   upstreamerr.Kind
	}{
		{http.StatusBadRequest, `{"detail":"single positional indexer is out-of-bounds"}`, upstreamerr.KindDataInsufficient},
		{http.StatusInternalServerError, `{"detail":"data insufficient"}`, upstreamerr.KindDataInsufficient},
		{http.StatusBadRequest, `{"detail":"canvas size is too large"}`, upstreamerr.KindContentTooLarge},
		{http.StatusBadRequest, `{"detail":"target file not found: x.png"}`, upstreamerr.KindAssetMissing},
		{http.StatusNotFound, `{"detail":"Not Found"}`, upstreamerr.KindIncompatible},
		{http.StatusInternalServerError, `{"detail":"boom"}`, upstreamerr.KindBadResponse},
		{http.StatusBadGateway, ``, upstreamerr.KindUnavailable},
		// Drawing's structured codes, for any status, before the text.
		{http.StatusInternalServerError, `{"detail":"图片文件不存在: a.png","code":"asset_missing"}`, upstreamerr.KindAssetMissing},
		{http.StatusInternalServerError, `{"detail":"boom","code":"asset_broken"}`, upstreamerr.KindAssetBroken},
		{http.StatusInternalServerError, `{"detail":"list index out of range","code":"data_insufficient"}`, upstreamerr.KindDataInsufficient},
		{http.StatusInternalServerError, `{"detail":"Canvas size is too large (1x1)","code":"content_too_large"}`, upstreamerr.KindContentTooLarge},
		{http.StatusBadRequest, `{"detail":"not enough data","code":"asset_missing"}`, upstreamerr.KindAssetMissing},
		{http.StatusInternalServerError, `{"detail":"boom","code":"made_up"}`, upstreamerr.KindBadResponse},
	}
	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		client := NewHarukiDrawingClient(server.URL, WithRetryCount(0)).WithContext(context.Background())
		_, err := client.postPreparedOnce("/api/pjsk/test", map[string]any{"x": 1})
		server.Close()
		class, ok := upstreamerr.Classify(err)
		if !ok || class.Service != upstreamerr.ServiceRender || class.Kind != tc.kind {
			t.Errorf("drawing %d %s: Classify = %+v (ok %v), error %v, want %s", tc.status, tc.body, class, ok, err, tc.kind)
		}
		if tc.kind == upstreamerr.KindDataInsufficient && !errors.Is(err, ErrDrawingDataInsufficient) {
			t.Errorf("drawing %d %s: error must wrap ErrDrawingDataInsufficient", tc.status, tc.body)
		}
	}
}

func TestDrawingNotConfiguredAndTransportFailures(t *testing.T) {
	var client *HarukiDrawingClient
	if _, err := client.postPreparedOnce("/x", nil); !errors.Is(err, ErrNotConfigured) || !upstreamerr.Is(err, upstreamerr.KindNotConfigured) {
		t.Fatalf("nil client error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()
	_, err := NewHarukiDrawingClient(url, WithRetryCount(0)).WithContext(context.Background()).postPreparedOnce("/x", map[string]any{})
	if class, ok := upstreamerr.Classify(err); !ok || class.Service != upstreamerr.ServiceRender || class.Kind != upstreamerr.KindUnavailable {
		t.Fatalf("unreachable drawing: Classify = %+v (ok %v), error %v", class, ok, err)
	}
}

func TestStatusErrorTextIsUnchanged(t *testing.T) {
	cases := map[string]*StatusError{
		"drawing request failed with status 500":                                        {StatusCode: 500},
		"drawing request failed with status 400: bad":                                   {StatusCode: 400, Detail: "bad"},
		"drawing request failed with status 400: drawing response data is insufficient": {StatusCode: 400, insufficient: true},
	}
	for want, err := range cases {
		if err.Error() != want {
			t.Errorf("Error() = %q, want %q", err.Error(), want)
		}
	}
}
