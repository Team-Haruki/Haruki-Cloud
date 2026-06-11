package sekai

import (
	"strings"
	"testing"
)

func TestAPIErrorHidesURLMessages(t *testing.T) {
	err := (&APIError{
		StatusCode: 502,
		Message:    "Fetch failed from https://production-game-api.sekai.colorfulpalette.org/api/jp/user/123/profile?token=secret",
	}).Error()
	if err != "sekai api error: status 502" {
		t.Fatalf("unexpected error: %q", err)
	}
	if strings.Contains(err, "http://") || strings.Contains(err, "https://") || strings.Contains(err, "token") {
		t.Fatalf("error leaked upstream detail: %q", err)
	}
}

func TestAPIErrorKeepsNonURLMessages(t *testing.T) {
	err := (&APIError{StatusCode: 401, Message: "Invalid token"}).Error()
	if err != `sekai api error: status 401, message: "Invalid token"` {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestToolboxAndTrackerErrorsHideURLMessages(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "toolbox",
			err:  &ToolboxAPIError{StatusCode: 500, Message: "proxy failed: http://100.80.207.86:16666/api/private/game-data/jp/suite/123"},
			want: "toolbox api error: status 500",
		},
		{
			name: "tracker",
			err:  &TrackerAPIError{StatusCode: 500, Message: "tracker failed: https://tracker.internal/api/jp/event/1"},
			want: "tracker api error: status 500",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Error()
			if got != tc.want {
				t.Fatalf("unexpected error: %q", got)
			}
			if strings.Contains(got, "http://") || strings.Contains(got, "https://") {
				t.Fatalf("error leaked upstream URL: %q", got)
			}
		})
	}
}
