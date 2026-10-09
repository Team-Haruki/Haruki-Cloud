package sekai

import (
	"errors"
	"testing"

	"haruki-cloud/internal/core/upstreamerr"
)

func TestSanitizeNetworkErrorHidesSensitiveURLs(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "quoted go http url",
			err:  errors.New(`Get "https://production-game-api.sekai.colorfulpalette.org/api/jp/user/123/profile": EOF`),
		},
		{
			name: "plain internal url",
			err:  errors.New("connect failed: http://100.80.207.86:16666/api/private/game-data/jp/suite/123"),
		},
		{
			name: "sensitive query url",
			err:  errors.New("connect failed: https://toolbox.example.com/api?token=secret"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeNetworkError(upstreamerr.ServiceToolbox, "toolbox: request failed after retries", tt.err)
			if got == nil {
				t.Fatal("expected error")
			}
			if got.Error() != "toolbox: request failed after retries: network request failed" {
				t.Fatalf("sanitizeNetworkError() = %q", got.Error())
			}
		})
	}
}

func TestSanitizeNetworkErrorKeepsNonURLMessages(t *testing.T) {
	cause := errors.New("connection refused")
	got := sanitizeNetworkError(upstreamerr.ServiceRanking, "tracker: request failed after retries", cause)
	if got == nil || got.Error() != "tracker: request failed after retries: connection refused" {
		t.Fatalf("sanitizeNetworkError() = %v", got)
	}
	if !errors.Is(got, cause) || !upstreamerr.IsService(got, upstreamerr.ServiceRanking) {
		t.Fatalf("sanitizeNetworkError() must keep the cause and the service: %v", got)
	}
}
