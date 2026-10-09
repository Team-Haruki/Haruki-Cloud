package pjsk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/notfound"
	"haruki-cloud/utils/usererror"
)

func TestCommandErrorTextShowsTypedReplies(t *testing.T) {
	ctx := context.Background()
	notFound := notfound.Card("662")
	if got := commandErrorText(ctx, fmt.Errorf("failed to search card: %w", notFound), "card/detail", "/card 662"); got != notFound.Message.String() {
		t.Fatalf("wrapped typed reply = %q, want %q", got, notFound.Message)
	}
	readOnly := usererror.ReadOnly()
	if got := commandErrorText(ctx, readOnly, "profile/bind", "/绑定"); got != readOnly.Message.String() {
		t.Fatalf("read-only reply = %q", got)
	}
}

func TestCommandErrorTextReplacesUnrecognizedWithRouteGuidance(t *testing.T) {
	ctx := context.Background()
	got := commandErrorText(ctx, usererror.Unrecognized(), "event", "/查活动 super-secret")
	want := i18n.WithUsage(i18n.M("guidance.event"), "/查活动").String()
	if got != want {
		t.Fatalf("unrecognized reply = %q, want %q", got, want)
	}
	if strings.Contains(got, "super-secret") {
		t.Fatalf("reply echoed the arguments: %q", got)
	}

	// A route without its own guidance keeps the generic reason.
	got = commandErrorText(ctx, usererror.Unrecognized(), "no/such-route", "/cmd")
	if want := i18n.WithUsage(i18n.Unrecognized(), "/cmd").String(); got != want {
		t.Fatalf("generic unrecognized reply = %q, want %q", got, want)
	}

	// A help pointer needs a slash command.
	got = commandErrorText(ctx, usererror.Unrecognized(), "event", "event 1")
	if want := i18n.M("guidance.event").String(); got != want {
		t.Fatalf("reply without a slash trigger = %q, want %q", got, want)
	}
}

func TestCommandErrorTextAddsHelpPointerToMisuseAndBadParams(t *testing.T) {
	ctx := context.Background()
	reason := i18n.M("deck.compare.too_many", i18n.Data{"Max": 5})
	if got, want := commandErrorText(ctx, usererror.Misuse(reason), "deck/event", "/活动组卡"), i18n.WithUsage(reason, "/活动组卡").String(); got != want {
		t.Fatalf("misuse reply = %q, want %q", got, want)
	}
	bad := usererror.BadParam("x", i18n.M("inventory.filter_unknown"))
	if got, want := commandErrorText(ctx, bad, "inventory/list", "/查背包 x"), i18n.WithUsage(bad.Message, "/查背包").String(); got != want {
		t.Fatalf("bad parameter reply = %q, want %q", got, want)
	}
	// Errors that are not about the command's shape get no help pointer.
	notFound := notfound.Music("x")
	if got := commandErrorText(ctx, notFound, "music", "/查曲 x"); got != notFound.Message.String() {
		t.Fatalf("not-found reply = %q", got)
	}
}

func TestHelpTriggerKeepsMultiWordCommands(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()
	for trigger, want := range map[string]string{
		"/pjsk vlive":     "/pjsk vlive",
		"/pjsk vlive 12":  "/pjsk vlive",
		"/jp查曲 tyw":       "/jp查曲",
		"/查活动  super":     "/查活动",
		"/not-registered": "/not-registered",
		"event 1":         "",
	} {
		if got := helpTrigger(trigger); got != want {
			t.Errorf("helpTrigger(%q) = %q, want %q", trigger, got, want)
		}
	}
}

func TestCommandErrorTextHidesUntypedErrors(t *testing.T) {
	ctx := context.Background()
	for _, err := range []error{
		errors.New(`Get "http://192.0.2.10:8080/api/private/x": connection refused`),
		errors.New("无法识别的指令: super-secret"),
		fmt.Errorf("failed to search card: %w", errors.New("sekai: card not found")),
	} {
		if got := commandErrorText(ctx, err, "card/detail", "/card"); got != i18n.RequestFailed().String() {
			t.Fatalf("untyped error %q reply = %q", err, got)
		}
	}
}

func TestCommandErrorTextClassifiesUpstreamFailures(t *testing.T) {
	ctx := context.Background()
	timeout := upstreamerr.Transport(upstreamerr.ServiceRanking, "tracker: request failed", context.DeadlineExceeded)
	if got, want := commandErrorText(ctx, timeout, "sk", "/sk"), i18n.Timeout(i18n.FeatureRanking).String(); got != want {
		t.Fatalf("upstream timeout reply = %q, want %q", got, want)
	}
}

func TestSanitizeErrorReplyDropsNonCatalogLines(t *testing.T) {
	ctx := context.Background()
	reply := i18n.M("binding.required").String()
	got := sanitizeErrorReply(ctx, reply+"\nraw upstream detail: token=abc", i18n.DefaultLocale)
	if got != reply {
		t.Fatalf("sanitized reply = %q, want %q", got, reply)
	}
	if got := sanitizeErrorReply(ctx, "raw upstream detail", i18n.DefaultLocale); got != i18n.RequestFailed().String() {
		t.Fatalf("a reply with no catalog line must become the generic reply: %q", got)
	}
}

func TestEveryRouteGuidanceHasCatalogText(t *testing.T) {
	for route, guidance := range routeGuidance {
		if text := guidance.String(); text == "" || text == guidance.ID {
			t.Errorf("route %s guidance %s has no catalog text", route, guidance.ID)
		}
	}
}

func TestCommandErrorTextRepliesTimeoutForRequestDeadline(t *testing.T) {
	ctx := context.Background()
	got := commandErrorText(ctx, fmt.Errorf("x: %w", context.DeadlineExceeded), "card/detail", "/card 1")
	if want := i18n.M("common.request_timeout").String(); got != want {
		t.Fatalf("deadline reply = %q, want %q", got, want)
	}
	transport := upstreamerr.Transport(upstreamerr.ServiceGameData, "", context.DeadlineExceeded)
	got = commandErrorText(ctx, transport, "card/detail", "/card 1")
	if want := i18n.Timeout(i18n.FeatureGameData).String(); got != want {
		t.Fatalf("game data deadline reply = %q, want %q", got, want)
	}
}
