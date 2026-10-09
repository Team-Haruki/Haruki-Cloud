package pjsk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/notfound"
	"haruki-cloud/utils/usererror"
)

const paramEchoSecret = "super-secret"

// paramEchoTypedErrors are typed errors that carry the user's input. Without
// parameter echo their reply must not repeat it; with echo it may.
func paramEchoTypedErrors() map[string]struct {
	err         error
	path, typed string
} {
	return map[string]struct {
		err         error
		path, typed string
	}{
		"bad parameter":       {usererror.BadParam(paramEchoSecret, i18n.M("common.param.uid_digits")), "profile/bind", "/绑定 " + paramEchoSecret},
		"music not found":     {notfound.Music(paramEchoSecret), "music", "/查曲 " + paramEchoSecret},
		"music in region":     {notfound.InRegion(notfound.Music(paramEchoSecret), "cn", ""), "music", "/cn查曲 " + paramEchoSecret},
		"card not found":      {fmt.Errorf("failed to search card: %w", notfound.Card(paramEchoSecret)), "card/detail", "/查卡 " + paramEchoSecret},
		"card fallback query": {notfound.InRegion(notfound.Card(""), "jp", paramEchoSecret), "card/detail", "/查卡 " + paramEchoSecret},
		"generic not found":   {usererror.NotFound(i18n.M("common.param_name.value"), paramEchoSecret), "event", "/查活动 " + paramEchoSecret},
		"generic ambiguous":   {usererror.Ambiguous(i18n.M("common.param_name.value"), paramEchoSecret), "event", "/查活动 " + paramEchoSecret},
		"character":           {usererror.New(usererror.CodeNotFound, i18n.M("character.not_found", i18n.Data{"UserQuery": i18n.EchoQuery(paramEchoSecret)})), "misc/birthday", "/生日 " + paramEchoSecret},
		"alias pending": {usererror.Invalid(i18n.M("alias.already_pending", i18n.Data{
			"Kind": i18n.M("alias.kind.music"), "UserAlias": i18n.EchoQuery(paramEchoSecret),
		})), "alias/music/add", "/添加歌曲别名"},
		"virtual live": {usererror.New(usererror.CodeNotFound, i18n.M("vlive.solo.not_found", i18n.Data{"UserQuery": i18n.EchoQuery(paramEchoSecret)})), "vlive", "/vlive " + paramEchoSecret},
	}
}

func TestCommandErrorTextHidesUserInputWithoutParamEcho(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()
	for name, tc := range paramEchoTypedErrors() {
		t.Run(name, func(t *testing.T) {
			got := commandErrorText(context.Background(), tc.err, tc.path, tc.typed)
			if strings.Contains(got, paramEchoSecret) {
				t.Fatalf("reply echoed the input: %q", got)
			}
			if got == i18n.RequestFailed().String() {
				t.Fatalf("echo-free reply fell back to the generic reply")
			}
			for _, pair := range []string{"“”", "「」", "：\n"} {
				if strings.Contains(got, pair) || strings.HasSuffix(got, "：") {
					t.Fatalf("reply leaves %q: %q", pair, got)
				}
			}
		})
	}
}

func TestCommandErrorTextEchoesUserInputWhenEnabled(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()
	ctx := i18n.WithParamEcho(context.Background(), true)
	for name, tc := range paramEchoTypedErrors() {
		t.Run(name, func(t *testing.T) {
			if got := commandErrorText(ctx, tc.err, tc.path, tc.typed); !strings.Contains(got, paramEchoSecret) {
				t.Fatalf("reply with echo lost the input: %q", got)
			}
		})
	}
}

// Untyped errors (raw parser, upstream or internal text) never reach users,
// with or without echo; the old prefix redaction table is no longer needed.
func TestCommandErrorTextNeverEchoesUntypedErrors(t *testing.T) {
	for _, raw := range []error{
		errors.New(`活动查询参数错误: "super-secret"`),
		errors.New(`无效的参数："super-secret"`),
		errors.New(`invalid token "super-secret"`),
		errors.New(`failed to resolve deck music selection "super-secret"`),
		errors.New(`CN服找不到特定的歌: super-secret`),
		errors.New("handler returned nil\nsuper-secret"),
	} {
		for _, echo := range []bool{false, true} {
			ctx := i18n.WithParamEcho(context.Background(), echo)
			got := commandErrorText(ctx, raw, "event", "/查活动 super-secret")
			if strings.Contains(got, paramEchoSecret) {
				t.Fatalf("raw error %q (echo %v) reply = %q", raw, echo, got)
			}
		}
	}
}

// The route guidance for unrecognized arguments never quotes the arguments,
// and the help pointer names the registered command, not the user's text.
func TestCommandErrorTextGuidanceDoesNotEchoArguments(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()
	for _, echo := range []bool{false, true} {
		ctx := i18n.WithParamEcho(context.Background(), echo)
		got := commandErrorText(ctx, usererror.Unrecognized(), "deck/event", "/活动组卡 "+paramEchoSecret)
		if want := i18n.WithUsage(i18n.M("guidance.deck_event"), "/活动组卡").String(); got != want {
			t.Fatalf("echo %v: guidance reply = %q, want %q", echo, got, want)
		}
	}
	// An unregistered command word is user input.
	if got := commandErrorText(context.Background(), usererror.Unrecognized(), "event", "/"+paramEchoSecret); strings.Contains(got, paramEchoSecret) {
		t.Fatalf("help pointer echoed an unregistered command: %q", got)
	}
}
