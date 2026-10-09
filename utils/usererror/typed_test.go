package usererror

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
)

func TestTypedErrorShowsOnlyUserText(t *testing.T) {
	cause := errors.New("tracker api error: status 503")
	err := Unavailable(i18n.FeatureRanking, cause)
	if err.Error() != i18n.Unavailable(i18n.FeatureRanking).String() || strings.Contains(err.Error(), "503") {
		t.Fatalf("Error() = %q", err.Error())
	}
	if err.Text(i18n.ZhCN) != err.Error() {
		t.Fatalf("Text() = %q", err.Text(i18n.ZhCN))
	}
	if !errors.Is(err, cause) || errors.Unwrap(err) != cause {
		t.Fatal("cause must stay reachable through Unwrap")
	}
	if got := LogText(err); got != "unavailable common.unavailable: tracker api error: status 503" {
		t.Fatalf("LogText() = %q", got)
	}
	if got := LogText(ReadOnly()); got != "read_only common.read_only" {
		t.Fatalf("LogText(no cause) = %q", got)
	}
	if LogText(nil) != "" || LogText(errors.New("plain")) != "plain" {
		t.Fatal("LogText of untyped errors")
	}
	wrapped := fmt.Errorf("bind: %w", err)
	if CodeOf(wrapped) != CodeUnavailable || CodeOf(cause) != "" {
		t.Fatalf("CodeOf() = %q", CodeOf(wrapped))
	}
	withCause := ReadOnly().WithCause(cause)
	if withCause.Cause != cause || withCause.Code != CodeReadOnly {
		t.Fatalf("WithCause() = %+v", withCause)
	}
}

func TestTypedErrorBuildersAndCodes(t *testing.T) {
	cases := []struct {
		err      *Error
		code     Code
		id       string
		input    bool
		expected bool
	}{
		{Invalid(i18n.M("moderation.kill.self")), CodeInput, "moderation.kill.self", true, true},
		{BadParam("x", i18n.M("moderation.qq_invalid")), CodeBadParam, "common.bad_param", true, true},
		{Usage(i18n.M("moderation.back.usage_reason"), "/back"), CodeUsage, "common.with_usage", true, true},
		{NotFound(i18n.Verbatim("歌曲"), "q"), CodeNotFound, "common.not_found", true, true},
		{Ambiguous(i18n.Verbatim("歌曲"), "q"), CodeAmbiguous, "common.ambiguous", true, true},
		{OutOfRange(i18n.Verbatim("序号"), 1, 3), CodeOutOfRange, "common.out_of_range", true, true},
		{Forbidden(i18n.M("moderation.not_admin")), CodeForbidden, "moderation.not_admin", false, true},
		{ReadOnly(), CodeReadOnly, "common.read_only", false, true},
		{Unavailable(i18n.FeatureRender, nil), CodeUnavailable, "common.unavailable", false, false},
		{Timeout(i18n.FeatureRender, nil), CodeTimeout, "common.timeout", false, false},
		{Misconfigured(nil), CodeMisconfigured, "common.misconfigured", false, false},
		{Internal(nil), CodeInternal, "common.request_failed", false, false},
	}
	for _, tc := range cases {
		if tc.err.Code != tc.code || tc.err.Message.ID != tc.id {
			t.Errorf("%s: got %s/%s", tc.id, tc.err.Code, tc.err.Message.ID)
		}
		if IsInput(tc.err) != tc.input || IsExpected(tc.err) != tc.expected {
			t.Errorf("%s: IsInput=%v IsExpected=%v", tc.id, IsInput(tc.err), IsExpected(tc.err))
		}
		if strings.TrimSpace(tc.err.Error()) == "" {
			t.Errorf("%s: empty user text", tc.id)
		}
	}
	if !IsExpected(Inputf("legacy")) || IsExpected(errors.New("plain")) {
		t.Fatal("IsExpected must keep legacy input errors")
	}
	if _, ok := As(errors.New("plain")); ok {
		t.Fatal("As matched an untyped error")
	}
}
