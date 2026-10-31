package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	registryhandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/pjsk/accountdata"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/usererror"

	json "haruki-cloud/internal/jsonutil"
)

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
}

func TestGlobalModerationErrorsAreTyped(t *testing.T) {
	_, err := executeGlobalModeration(nil)
	requireUserError(t, err, usererror.CodeMisconfigured, "common.misconfigured")
	if usererror.IsExpected(err) || errors.Unwrap(err) == nil {
		t.Fatalf("misconfiguration must be logged with its cause: %v", err)
	}

	service := &accountdata.BanService{}
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{BanChecker: service}, Cmd: &CommandRequest{Mode: modeGlobalBack, Params: json.RawMessage(`{`)}}
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

func TestGlobalKillCommandRemoved(t *testing.T) {
	EnsureCommandHandlersRegistered()
	if _, ok := registryhandler.LookupCommandHandler("/kill"); ok {
		t.Fatal("removed kill command is still registered")
	}
	if _, ok := registryhandler.LookupCommandHandler("/back"); !ok {
		t.Fatal("back command is missing")
	}
	_, err := executeGlobalModeration(&RequestContext{
		Ctx: context.Background(),
		App: &renderapp.App{BanChecker: &accountdata.BanService{}},
		Cmd: &CommandRequest{Mode: "global-kill", Params: json.RawMessage(`{}`)},
	})
	if err == nil || !strings.Contains(usererror.LogText(err), "unsupported global moderation mode") {
		t.Fatalf("removed kill mode should be unsupported, got %v", err)
	}
}
