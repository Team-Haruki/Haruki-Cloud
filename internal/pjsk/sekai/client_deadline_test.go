package sekai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstreamerr"
)

// A request deadline is a timeout of the called service, so the reply
// layer can say which feature timed out instead of a generic failure.
func TestRequestDeadlineIsClassifiedAsServiceTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	expired := func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		t.Cleanup(cancel)
		return ctx
	}

	t.Run("game data", func(t *testing.T) {
		client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL})
		_, err := client.WithContext(expired(t)).GetSystem("jp")
		requireServiceTimeout(t, err, upstreamerr.ServiceGameData)
	})
	t.Run("toolbox", func(t *testing.T) {
		client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
		_, err := client.GetSuiteDataContext(expired(t), "jp", 123456789, "qq", "10001")
		requireServiceTimeout(t, err, upstreamerr.ServiceToolbox)
	})
}

func requireServiceTimeout(t *testing.T, err error, service upstreamerr.Service) {
	t.Helper()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded in the chain", err)
	}
	class, ok := upstreamerr.Classify(err)
	if !ok || class.Service != service || class.Kind != upstreamerr.KindTimeout {
		t.Fatalf("Classify() = %+v, %v; want %s timeout", class, ok, service)
	}
}
