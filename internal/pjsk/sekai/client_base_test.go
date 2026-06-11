package sekai

import (
	"errors"
	"testing"
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
			got := sanitizeNetworkError(tt.err)
			if got == nil {
				t.Fatal("expected error")
			}
			if got.Error() != "network request failed" {
				t.Fatalf("sanitizeNetworkError() = %q", got.Error())
			}
		})
	}
}

func TestSanitizeNetworkErrorKeepsNonURLMessages(t *testing.T) {
	got := sanitizeNetworkError(errors.New("connection refused"))
	if got == nil || got.Error() != "connection refused" {
		t.Fatalf("sanitizeNetworkError() = %v", got)
	}
}
