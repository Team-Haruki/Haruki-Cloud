package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/accountdata"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

func TestNormalizeToolboxDataFetchError(t *testing.T) {
	binding := &accountdata.ResolvedBinding{
		Server:         "jp",
		PJSKUserID:     "12345678901234",
		Visible:        false,
		SuiteVisible:   true,
		MySekaiVisible: true,
	}

	testCases := []struct {
		name  string
		input error
		kind  string
		code  usererror.Code
		id    string
	}{
		{"account binding not found", sekaiapi.ErrAccountBindingNotFound, "suite", usererror.CodeSetup, "binding.toolbox.not_bound"},
		{"game data not found suite", sekaiapi.ErrGameDataNotFound, "suite", usererror.CodeSetup, "binding.data.not_found_account"},
		{"game data not found mysekai", sekaiapi.ErrGameDataNotFound, "mysekai", usererror.CodeSetup, "binding.data.not_found_account"},
		{"invalid platform user", sekaiapi.ErrInvalidPlatformUser, "mysekai", usererror.CodeSetup, "binding.toolbox.access_denied_account"},
		{"account owner banned", sekaiapi.ErrAccountOwnerBanned, "suite", usererror.CodeForbidden, "binding.toolbox.owner_banned"},
		{"service unavailable", &sekaiapi.ToolboxAPIError{StatusCode: 503, Message: "toolbox service unavailable"}, "suite", usererror.CodeUnavailable, "common.unavailable"},
		{"generic forbidden detail is hidden", &sekaiapi.ToolboxAPIError{StatusCode: 403, Message: "forbidden: some internal detail"}, "suite", usererror.CodeSetup, "binding.toolbox.access_denied_account"},
		{"generic not found detail is hidden", &sekaiapi.ToolboxAPIError{StatusCode: 404, Message: "unexpected missing payload detail"}, "suite", usererror.CodeSetup, "binding.data.not_found_account"},
		{"generic upstream detail is hidden", &sekaiapi.ToolboxAPIError{StatusCode: 500, Message: "raw upstream detail"}, "suite", usererror.CodeUnavailable, "upstream.failed"},
		{"authentication failure needs the bot owner", &sekaiapi.ToolboxAPIError{StatusCode: 401, Message: "unauthorized"}, "suite", usererror.CodeMisconfigured, "common.misconfigured"},
		{"network timeout", upstreamerr.Transport(upstreamerr.ServiceToolbox, "toolbox: request failed after retries", context.DeadlineExceeded), "suite", usererror.CodeTimeout, "common.timeout"},
		{"bare request deadline", fmt.Errorf("fetch: %w", context.DeadlineExceeded), "suite", usererror.CodeTimeout, "common.timeout"},
		{"unclassified failure", errors.New("plain failure"), "suite", usererror.CodeUnavailable, "common.unavailable"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := normalizeToolboxDataFetchError(tc.input, tc.kind, binding)
			typed := testutil.RequireUserError(t, err, tc.code, tc.id)
			if typed.Cause == nil && tc.id != "binding.data.not_found_account" {
				t.Fatalf("the upstream error must stay the logged cause: %+v", typed)
			}
		})
	}
}

func TestPrivateDataNotFoundErrorsExplainHiddenBindings(t *testing.T) {
	binding := &accountdata.ResolvedBinding{
		Server:         "cn",
		PJSKUserID:     "7558747506658564903",
		Visible:        true,
		SuiteVisible:   false,
		MySekaiVisible: false,
	}

	suite := testutil.RequireUserError(t, suiteDataNotFoundError(binding), usererror.CodeSetup, "binding.data.hidden_suite")
	if suite.Message.Data["Account"].(i18n.Message).String() != i18n.AccountLabel("cn", "7558747506658564903", true).String() {
		t.Fatalf("hidden suite account = %+v", suite.Message.Data)
	}
	testutil.RequireUserError(t, mysekaiDataNotFoundError(binding), usererror.CodeSetup, "binding.data.hidden_mysekai")
	testutil.RequireUserError(t, suiteDataNotFoundError(nil), usererror.CodeSetup, "binding.data.not_found")
	hidden := testutil.RequireUserError(t, mysekaiDataNotFoundError(&accountdata.ResolvedBinding{MySekaiVisible: false}), usererror.CodeSetup, "binding.data.hidden_mysekai")
	if hidden.Message.Data["Account"].(i18n.Message).ID != "binding.current_account" {
		t.Fatalf("a binding without UID must name the current account: %+v", hidden.Message.Data)
	}
}

func TestTempProfileUsesTemporaryBindingNotice(t *testing.T) {
	prev := harukiConfig.Cfg
	harukiConfig.Cfg = harukiConfig.Config{Profile: harukiConfig.ProfileTemp}
	t.Cleanup(func() { harukiConfig.Cfg = prev })

	for name, err := range map[string]error{
		"local binding missing":           WrapDomainError(accountdata.ErrNoBinding),
		"binding service unavailable":     WrapDomainError(accountdata.ErrBindingServiceUnavailable),
		"toolbox account binding missing": normalizeToolboxDataFetchError(sekaiapi.ErrAccountBindingNotFound, "suite", nil),
		"toolbox invalid platform detail": normalizeToolboxDataFetchError(&sekaiapi.ToolboxAPIError{StatusCode: 403, Message: "forbidden: invalid platform or platform_user_id for this user"}, "mysekai", nil),
	} {
		t.Run(name, func(t *testing.T) {
			testutil.RequireUserError(t, err, usererror.CodeSetup, "binding.temp_environment")
		})
	}
}

func TestWrapDomainErrorClassifiesByType(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		code usererror.Code
		id   string
	}{
		{"no binding", accountdata.ErrNoBinding, usererror.CodeSetup, "binding.required"},
		{"binding storage down", accountdata.ErrBindingServiceUnavailable, usererror.CodeUnavailable, "common.unavailable"},
		{"no suite snapshot", rendersnapshot.ErrNotConfigured, usererror.CodeSetup, "binding.data.not_found"},
		{"no mysekai snapshot", rendersnapshot.ErrMySekaiUnavailable, usererror.CodeSetup, "binding.data.not_found"},
		{"toolbox 503", &sekaiapi.ToolboxAPIError{StatusCode: 503, Message: "toolbox service unavailable"}, usererror.CodeUnavailable, "common.unavailable"},
		{"toolbox timeout", upstreamerr.Transport(upstreamerr.ServiceToolbox, "toolbox: request failed after retries", context.DeadlineExceeded), usererror.CodeTimeout, "common.timeout"},
		{"game server maintenance", sekaiapi.ErrServerMaintenance, usererror.CodeUnavailable, "upstream.maintenance"},
		{"player not found", sekaiapi.ErrUserNotFound, usererror.CodeNotFound, "upstream.game_data.player_not_found"},
		{"client not configured", sekaiapi.ErrClientNotConfigured, usererror.CodeMisconfigured, "common.misconfigured"},
		{"ranking rate limit", &sekaiapi.TrackerAPIError{StatusCode: 429, Message: "rate limited by tracker"}, usererror.CodeUnavailable, "upstream.rate_limited"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.RequireUserError(t, WrapDomainError(tc.err), tc.code, tc.id)
		})
	}
	plain := errors.New("connection refused by something")
	if WrapDomainError(plain) != plain {
		t.Fatal("an unclassified error must pass through (the reply layer gives the generic reply)")
	}
	typed := usererror.ReadOnly()
	if WrapDomainError(typed) != typed {
		t.Fatal("a typed error must pass through unchanged")
	}
}

func TestRequireVisibleSuiteSnapshotPropagatesToolboxTypedError(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings: service,
		Snapshots: &runtimeSnapshotProviderStub{
			err: sekaiapi.ErrInvalidPlatformUser,
		},
	})

	_, _, err := rc.requireVisibleSuiteSnapshot()
	testutil.RequireUserError(t, err, usererror.CodeSetup, "binding.toolbox.access_denied_account")
}

func TestResolveTargetSnapshotWithErrorPreservesToolboxFailure(t *testing.T) {
	provider := &runtimeSnapshotProviderStub{
		err: &sekaiapi.ToolboxAPIError{StatusCode: 503, Message: "toolbox service unavailable"},
	}
	originalFactory := snapshotProviderFactory
	snapshotProviderFactory = func(*renderapp.App) rendersnapshot.HarukiSnapshotProvider {
		return provider
	}
	t.Cleanup(func() { snapshotProviderFactory = originalFactory })

	snap, err := resolveTargetSnapshotWithError(
		context.Background(),
		&renderapp.App{},
		"jp",
		"qq",
		"42",
		"12345678901234",
		false,
	)
	if snap != nil {
		t.Fatalf("expected nil snapshot, got %T", snap)
	}
	var apiErr *sekaiapi.ToolboxAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 503 {
		t.Fatalf("expected preserved Toolbox 503 error, got %T (%v)", err, err)
	}
}

func TestRequireCardCatalogDetailedProfilePropagatesToolboxFailure(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	rc := NewRequestContext(ctx, &CommandRequest{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings: service,
		Snapshots: &runtimeSnapshotProviderStub{
			err: upstreamerr.Transport(upstreamerr.ServiceToolbox, "toolbox: request failed after retries", context.DeadlineExceeded),
		},
	})

	_, err := requireCardCatalogDetailedProfile(rc)
	testutil.RequireUserError(t, err, usererror.CodeTimeout, "common.timeout")
}

func TestCardCatalogSnapshotErrorTitleDistinguishesUpstreamFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"missing suite", sekaiapi.ErrGameDataNotFound, i18n.T("card.catalog_notice.no_suite")},
		{"access denied names the reason", sekaiapi.ErrInvalidPlatformUser, i18n.T("card.catalog_notice.with_reason", i18n.Data{"Reason": firstLine(i18n.T("binding.toolbox.access_denied", i18n.Data{"Data": i18n.M("binding.data_kind.suite"), "ToolboxLink": i18n.M("binding.toolbox_link")}))})},
		{"authentication failure", &sekaiapi.ToolboxAPIError{StatusCode: 401, Message: "unauthorized"}, i18n.T("card.catalog_notice.suite_unavailable")},
		{"network timeout", upstreamerr.Transport(upstreamerr.ServiceToolbox, "toolbox", context.DeadlineExceeded), i18n.T("card.catalog_notice.suite_unavailable")},
		{"unknown internal failure", errString("internal detail must not be exposed"), i18n.T("card.catalog_notice.suite_unavailable")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cardCatalogSnapshotErrorTitle(tt.err, nil); got != tt.want {
				t.Fatalf("cardCatalogSnapshotErrorTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

func TestNormalizeSekaiAPIFetchErrorMapsRequestDeadlineToTimeout(t *testing.T) {
	err := normalizeSekaiAPIFetchError(fmt.Errorf("profile: %w", context.DeadlineExceeded))
	testutil.RequireUserError(t, err, usererror.CodeTimeout, "common.timeout")
}
