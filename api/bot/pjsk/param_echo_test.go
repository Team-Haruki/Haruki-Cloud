package pjsk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
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

// paramEchoNumber is a number "typed" by the user in paramEchoNumericErrors.
const paramEchoNumber = 987654321

// paramEchoNumericErrors are typed errors that carry a number parsed from the
// user's command (IDs, ranks, counts, WL turns, BPM, QQ numbers, pages).
// Numbers are user input too.
func paramEchoNumericErrors() map[string]struct {
	err         error
	path, typed string
} {
	n := i18n.UserNumber(paramEchoNumber)
	typed := fmt.Sprint(paramEchoNumber)
	region := i18n.RegionLabel("jp")
	event := i18n.M("common.event_label", i18n.Data{"Region": region, "UserID": n})
	return map[string]struct {
		err         error
		path, typed string
	}{
		"event id":         {usererror.New(usererror.CodeNotFound, i18n.M("event.not_found_in_region", i18n.Data{"Region": region, "UserID": n})), "event", "/查活动 event" + typed},
		"event label":      {usererror.Invalid(i18n.M("sk.wl.not_wl_event", i18n.Data{"Event": event})), "sk", "/sk event" + typed},
		"wl chapter":       {usererror.Invalid(i18n.M("sk.wl.no_chapter_no", i18n.Data{"Event": event, "UserChapter": n})), "sk", "/sk wl" + typed},
		"planner rank":     {usererror.New(usererror.CodeNotFound, i18n.M("event.planner.no_line", i18n.Data{"UserRank": n})), "event/planner", "/活动规划 t" + typed},
		"trace rank":       {usererror.New(usererror.CodeNotFound, i18n.M("sk.trace.no_rank_data", i18n.Data{"UserRank": n})), "sk/player-trace", "/玩家追踪 " + typed},
		"wl turn":          {usererror.Misuse(i18n.M("deck.wl.turn_needs_character", i18n.Data{"UserTurn": n})), "deck/event", "/活动组卡 wl" + typed},
		"wl future turn":   {usererror.Invalid(i18n.M("deck.wl.future_turn_unit", i18n.Data{"Available": 3, "UserTurn": n})), "deck/event", "/活动组卡 wl" + typed},
		"fixed card":       {usererror.Invalid(i18n.M("deck.fixed.card_not_in_region", i18n.Data{"Region": region, "UserCardID": n})), "deck/event", "/活动组卡 #" + typed},
		"note count":       {usererror.New(usererror.CodeNotFound, i18n.M("music.note_count.no_chart", i18n.Data{"UserCount": n})), "music", "/物量 " + typed},
		"bpm":              {usererror.New(usererror.CodeNotFound, i18n.M("music.bpm.no_chart", i18n.Data{"UserBPM": n})), "music", "/bpm " + typed},
		"alias song id":    {usererror.New(usererror.CodeNotFound, i18n.M("alias.id_not_found.music", i18n.Data{"UserID": n})), "alias/music/add", "/添加歌曲别名"},
		"review ids":       {usererror.New(usererror.CodeNotFound, i18n.M("alias.review_not_found", i18n.Data{"UserIDs": n})), "alias/approve", "/同意别名 " + typed},
		"housing id":       {usererror.New(usererror.CodeNotFound, i18n.M("mysekai.housing.not_found", i18n.Data{"UserID": n})), "mysekai/housing", "/百景 " + typed},
		"custom card page": {usererror.New(usererror.CodeNotFound, i18n.M("profile.custom_card.not_found_page", i18n.Data{"UserPage": n, "Total": 2})), "profile/custom-profile-card", "/自定义资料卡 " + typed},
		"costume":          {usererror.Invalid(i18n.M("costume.not_for_character", i18n.Data{"Part": i18n.M("costume.part.outfit"), "UserID": n, "UserCharacter": n})), "costume", "/查服装 " + typed},
		"bound uid":        {usererror.New(usererror.CodeNotFound, i18n.M("binding.selector_uid_not_bound", i18n.Data{"UserUID": n})), "profile", "/个人信息 " + typed},
	}
}

func TestCommandErrorTextHidesNumbersWithoutParamEcho(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()
	secret := fmt.Sprint(paramEchoNumber)
	for name, tc := range paramEchoNumericErrors() {
		t.Run(name, func(t *testing.T) {
			got := commandErrorText(context.Background(), tc.err, tc.path, tc.typed)
			if strings.Contains(got, secret) {
				t.Fatalf("reply echoed the number: %q", got)
			}
			if got == i18n.RequestFailed().String() {
				t.Fatalf("echo-free reply fell back to the generic reply")
			}
			echo := commandErrorText(i18n.WithParamEcho(context.Background(), true), tc.err, tc.path, tc.typed)
			if !strings.Contains(echo, secret) {
				t.Fatalf("reply with echo lost the number: %q", echo)
			}
		})
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

// A success reply that repeats unreviewed alias text (onebot11.LocalizedText)
// carries the reply without the text and, separately, the echo reply.
func TestSucceededSharedBotCommandKeepsBothVariants(t *testing.T) {
	submitted := i18n.M("alias.add.done", i18n.Data{
		"Count": 1,
		"Kind":  i18n.M("alias.kind.music"),
		"Records": []i18n.Message{i18n.M("alias.record.pending", i18n.Data{
			"ReviewID": 12, "Kind": i18n.M("alias.kind.music"), "Name": "Tell Your World", "EntityID": 74,
			"UserAlias": i18n.UserText(paramEchoSecret),
		})},
	})
	result := succeededSharedBotCommand(context.Background(), onebot11.Message{onebot11.LocalizedText(submitted)}, sharedCommandMetadata{Outcome: "ok"}, false)
	for _, body := range [][]byte{result.Response.JSONBody, result.Response.MsgPackBody} {
		if strings.Contains(string(body), paramEchoSecret) {
			t.Fatalf("default reply repeats the unreviewed alias: %s", body)
		}
	}
	if !strings.Contains(string(result.Response.JSONBody), "待审核别名 #12") {
		t.Fatalf("default reply lost the review ID: %s", result.Response.JSONBody)
	}
	for _, body := range [][]byte{result.EchoResponse.JSONBody, result.EchoResponse.MsgPackBody} {
		if !strings.Contains(string(body), paramEchoSecret) {
			t.Fatalf("echo reply lost the alias: %s", body)
		}
	}
	if echo := result.responseFor(BotCommandRequest{EnableParamEcho: true}); !strings.Contains(string(echo.JSONBody), paramEchoSecret) {
		t.Fatalf("echo client got %s", echo.JSONBody)
	}

	plain := succeededSharedBotCommand(context.Background(), onebot11.Message{onebot11.Text("已绑定")}, sharedCommandMetadata{Outcome: "ok"}, false)
	if len(plain.EchoResponse.JSONBody) != 0 {
		t.Fatalf("a reply without user input needs no echo variant: %s", plain.EchoResponse.JSONBody)
	}
}
