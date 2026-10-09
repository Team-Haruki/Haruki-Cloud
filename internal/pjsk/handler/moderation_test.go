package handler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/usererror"

	json "haruki-cloud/internal/jsonutil"
)

func TestGlobalKillHandleParsesPermanentBan(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/kill",
		ArgText:    "123456789 恶意滥用",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Module != parser.ModuleAdmin {
		t.Fatalf("unexpected module: %v", result.Module)
	}
	var params globalKillParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.QQID != "123456789" || params.Reason != "恶意滥用" || params.Days != nil {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestGlobalKillHandleParsesLastIntegerAsDays(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/kill",
		ArgText:    "00123456789 频繁攻击服务 30",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var params globalKillParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.QQID != "123456789" || params.Reason != "频繁攻击服务" || params.Days == nil || *params.Days != 30 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestGlobalKillHandleRejectsSelfBan(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "123456789",
		TriggerCmd: "/kill",
		ArgText:    "123456789 测试",
	})
	requireUserError(t, err, usererror.CodeForbidden, "moderation.kill.self")
}

func TestGlobalKillHandleKeepsNumericReasonWhenDurationIsOmitted(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/kill",
		ArgText:    "123456789 404",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var params globalKillParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Reason != "404" || params.Days != nil {
		t.Fatalf("unexpected numeric reason params: %+v", params)
	}
}

func TestGlobalKillHandleRejectsMalformedDuration(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/kill",
		ArgText:    "123456789 滥用 999999999999999999999999999999",
	})
	requireUserError(t, err, usererror.CodeBadParam, "common.bad_param")
	if typed, _ := usererror.As(err); typed.Message.Data["Reason"].(i18n.Message).ID != "moderation.kill.days_invalid" {
		t.Fatalf("bad duration reason = %+v", typed.Message)
	}
}

func TestGlobalBackHandleRequiresOneQQID(t *testing.T) {
	h := sekaiHandlers{}.GlobalBackHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/back",
		ArgText:    "123456789",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var params globalBackParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.QQID != "123456789" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

// requireUserError asserts that err is a typed user error with code and
// message ID. Tests check codes and IDs; the catalog owns the wording.
func requireUserError(t *testing.T, err error, code usererror.Code, id string) {
	t.Helper()
	typed, ok := usererror.As(err)
	if !ok {
		t.Fatalf("error = %v (%T), want typed user error %s/%s", err, err, code, id)
	}
	if typed.Code != code || typed.Message.ID != id {
		t.Fatalf("user error = %s/%s, want %s/%s", typed.Code, typed.Message.ID, code, id)
	}
}

func TestModerationUsageErrorsPointToHelp(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"kill without reason", func() error { _, _, _, err := parseGlobalKillArgs("123"); return err }()},
		{"kill with extra args", func() error { _, _, _, err := parseGlobalKillArgs("1 a 2 3"); return err }()},
		{"back without qq", func() error { _, err := parseQQIDArg(""); return err }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireUserError(t, tc.err, usererror.CodeUsage, "common.with_usage")
			if !usererror.IsInput(tc.err) || !usererror.IsExpected(tc.err) {
				t.Fatalf("usage error must count as user input: %v", tc.err)
			}
		})
	}
	_, err := parseQQIDArg("abc")
	requireUserError(t, err, usererror.CodeBadParam, "common.bad_param")
	_, _, _, err = parseGlobalKillArgs("1 " + strings.Repeat("理", accountdata.BanReasonMaxRunes+1))
	requireUserError(t, err, usererror.CodeInput, "moderation.kill.reason_too_long")
}

func TestGlobalModerationErrorsAreTyped(t *testing.T) {
	_, err := executeGlobalModeration(nil)
	requireUserError(t, err, usererror.CodeMisconfigured, "common.misconfigured")
	if usererror.IsExpected(err) || errors.Unwrap(err) == nil {
		t.Fatalf("misconfiguration must be logged with its cause: %v", err)
	}

	service := &accountdata.BanService{}
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{BanChecker: service}, Cmd: &CommandRequest{Mode: modeGlobalKill, Params: json.RawMessage(`{`)}}
	_, err = executeGlobalModeration(rc)
	requireUserError(t, err, usererror.CodeInternal, "common.request_failed")

	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"10001","qq_id":"12","reason":"spam"}`)
	_, err = executeGlobalModeration(rc)
	requireUserError(t, err, usererror.CodeForbidden, "moderation.not_admin")

	service.SetAdminQQIDs([]string{"10001"})
	_, err = executeGlobalModeration(rc)
	requireUserError(t, err, usererror.CodeMisconfigured, "common.misconfigured")

	rc.Cmd = &CommandRequest{Mode: "wrong"}
	_, err = executeGlobalModeration(rc)
	requireUserError(t, err, usererror.CodeInternal, "common.request_failed")
}

func TestModerationFailureHidesUntypedCauses(t *testing.T) {
	cause := errors.New("pq: connection reset")
	err := moderationFailure(cause)
	requireUserError(t, err, usererror.CodeInternal, "common.request_failed")
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "pq") {
		t.Fatalf("moderationFailure() = %q", err)
	}
	typed := usererror.ReadOnly()
	if moderationFailure(typed) != typed {
		t.Fatal("typed errors must pass through unchanged")
	}
}

func TestGlobalKillReplyUsesRequesterTimeZone(t *testing.T) {
	expiresAt := time.Date(2026, 10, 9, 6, 5, 0, 0, time.UTC)
	ctx := displaytime.WithRequestTimeZone(context.Background(), "Asia/Tokyo")
	got := i18n.FormatUserTime(expiresAt, displaytime.RequestLocation(ctx)).String()
	if got != "2026-10-09 15:05 (UTC+9)" {
		t.Fatalf("FormatUserTime() = %q", got)
	}
}
