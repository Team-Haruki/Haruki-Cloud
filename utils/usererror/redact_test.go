package usererror

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRedactForLog(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "internal url and ids",
			in:   `toolbox: Get "http://100.80.207.86:16666/api/private/game-data/cn/suite/7487590788965145370?key=abc": context deadline exceeded`,
			want: `toolbox: Get "<url>": context deadline exceeded`,
		},
		{
			name: "dial error keeps short numbers",
			in:   "query deck event 181 failed: dial tcp 10.0.0.5:5432: connect: connection refused",
			want: "query deck event 181 failed: dial tcp <ip>: connect: connection refused",
		},
		{
			name: "user ids and timestamps",
			in:   "获取玩家信息失败：user 7487590788965145370 not found at 1759888742000",
			want: "获取玩家信息失败：user <n> not found at <n>",
		},
		{
			name: "credentials",
			in:   "auth failed: Bearer eyJhbGciOiJIUzI1NiJ9.e30.x token=s3cr3t password: hunter2",
			want: "auth failed: bearer <redacted> token=<redacted> password: <redacted>",
		},
		{
			name: "email uuid and digest",
			in:   "user a.b@example.com request 973ad574-6440-4985-8719-7b8508aa690a digest 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b",
			want: "user <email> request <uuid> digest <token>",
		},
		{
			name: "multi line and control characters",
			in:   "snapshot: decode failed\n\tunexpected \x00 end",
			want: "snapshot: decode failed unexpected end",
		},
		{
			name: "invalid utf8",
			in:   "bad \xff byte",
			want: "bad ? byte",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactForLog(tt.in, 0); got != tt.want {
				t.Fatalf("RedactForLog() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedactForLogBoundsLength(t *testing.T) {
	got := RedactForLog(strings.Repeat("活动 ", 400), 10)
	if utf8.RuneCountInString(got) != 11 || !strings.HasSuffix(got, "…") {
		t.Fatalf("RedactForLog() = %q, want 10 runes plus an ellipsis", got)
	}
	if got := RedactForLog(strings.Repeat("x ", 400), 0); utf8.RuneCountInString(got) != DefaultLogMessageLimit+1 {
		t.Fatalf("default limit: got %d runes", utf8.RuneCountInString(got))
	}
}
