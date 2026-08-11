package handler

import (
	"context"
	"errors"
	"testing"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
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
		name      string
		input     error
		dataLabel string
		wantErr   string
	}{
		{
			name:      "account binding not found",
			input:     sekaiapi.ErrAccountBindingNotFound,
			dataLabel: "suite",
			wantErr:   "你还没有在工具箱绑定游戏账号，无法获取suite数据，请前往工具箱绑定游戏账号并上传数据后重试\n" + ErrMsgToolboxURL,
		},
		{
			name:      "game data not found suite",
			input:     sekaiapi.ErrGameDataNotFound,
			dataLabel: "suite",
			wantErr:   buildPrivateDataNotFoundMessage("suite", binding),
		},
		{
			name:      "game data not found mysekai",
			input:     sekaiapi.ErrGameDataNotFound,
			dataLabel: "mysekai",
			wantErr:   buildPrivateDataNotFoundMessage("mysekai", binding),
		},
		{
			name:      "invalid platform user",
			input:     sekaiapi.ErrInvalidPlatformUser,
			dataLabel: "mysekai",
			wantErr:   buildToolboxAccessDeniedMessage("mysekai", binding),
		},
		{
			name:      "account owner banned",
			input:     sekaiapi.ErrAccountOwnerBanned,
			dataLabel: "suite",
			wantErr:   "工具箱账号已被封禁，无法获取suite数据",
		},
		{
			name:      "service unavailable",
			input:     &sekaiapi.ToolboxAPIError{StatusCode: 503, Message: "toolbox service unavailable"},
			dataLabel: "suite",
			wantErr:   "工具箱服务暂时不可用，请稍后再试",
		},
		{
			name:      "generic forbidden detail is hidden",
			input:     &sekaiapi.ToolboxAPIError{StatusCode: 403, Message: "forbidden: some internal detail"},
			dataLabel: "suite",
			wantErr:   "工具箱拒绝了当前suite数据请求",
		},
		{
			name:      "generic not found detail is hidden",
			input:     &sekaiapi.ToolboxAPIError{StatusCode: 404, Message: "unexpected missing payload detail"},
			dataLabel: "suite",
			wantErr:   "工具箱未找到当前suite数据",
		},
		{
			name:      "generic upstream detail is hidden",
			input:     &sekaiapi.ToolboxAPIError{StatusCode: 500, Message: "raw upstream detail"},
			dataLabel: "suite",
			wantErr:   "工具箱请求失败（状态 500）",
		},
		{
			name:      "authentication failure",
			input:     &sekaiapi.ToolboxAPIError{StatusCode: 401, Message: "unauthorized"},
			dataLabel: "suite",
			wantErr:   "工具箱请求失败（状态 401）",
		},
		{
			name:      "network timeout",
			input:     errString("toolbox: request failed after retries: context deadline exceeded"),
			dataLabel: "suite",
			wantErr:   "连接工具箱超时或网络异常，请稍后再试",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := normalizeToolboxDataFetchError(tc.input, tc.dataLabel, binding)
			assertReplayErrorText(t, err, tc.wantErr)
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

	assertReplayErrorText(
		t,
		newSuiteDataNotFoundReplayErrorForBinding(binding),
		"你已自行隐藏 CN服7558747506658564903 的 suite 抓包信息，请先发送“/展示抓包”恢复展示后再重试",
	)
	assertReplayErrorText(
		t,
		newMySekaiDataNotFoundReplayErrorForBinding(binding),
		"你已自行隐藏 CN服7558747506658564903 的 mysekai 抓包信息，请先发送“/展示烤森抓包”恢复展示后再重试",
	)
}

func TestTempProfileUsesTemporaryBindingNotice(t *testing.T) {
	prev := harukiConfig.Cfg
	harukiConfig.Cfg = harukiConfig.Config{Profile: harukiConfig.ProfileTemp}
	t.Cleanup(func() { harukiConfig.Cfg = prev })

	testCases := []struct {
		name string
		err  error
	}{
		{name: "local binding missing", err: WrapDomainError(accountdata.ErrNoBinding)},
		{name: "binding service unavailable", err: WrapDomainError(accountdata.ErrBindingServiceUnavailable)},
		{name: "toolbox account binding missing", err: normalizeToolboxDataFetchError(sekaiapi.ErrAccountBindingNotFound, "suite", nil)},
		{
			name: "toolbox invalid platform detail",
			err: normalizeToolboxDataFetchError(
				&sekaiapi.ToolboxAPIError{StatusCode: 403, Message: "forbidden: invalid platform or platform_user_id for this user"},
				"mysekai",
				nil,
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assertReplayErrorText(t, tc.err, ErrMsgTempBindingUnavailable)
		})
	}
}

func TestWrapDomainErrorMapsToolboxServiceErrors(t *testing.T) {
	err := WrapDomainError(&sekaiapi.ToolboxAPIError{StatusCode: 503, Message: "toolbox service unavailable"})
	assertReplayErrorText(t, err, "工具箱服务暂时不可用，请稍后再试")

	err = WrapDomainError(errString("toolbox: request failed after retries: context deadline exceeded"))
	assertReplayErrorText(t, err, "连接工具箱超时或网络异常，请稍后再试")
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
	if err == nil {
		t.Fatal("expected error")
	}
	var replyErr onebot11.ReplayError
	if !errors.As(err, &replyErr) {
		t.Fatalf("expected ReplayError, got %T (%v)", err, err)
	}
	if string(replyErr) != buildToolboxAccessDeniedMessage("suite", &accountdata.ResolvedBinding{
		Server:     "jp",
		PJSKUserID: "12345678901234",
		Visible:    false,
	}) {
		t.Fatalf("unexpected replay error: %q", replyErr)
	}
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
			err: errString("toolbox: request failed after retries: context deadline exceeded"),
		},
	})

	_, err := requireCardCatalogDetailedProfile(rc)
	assertReplayErrorText(t, err, "连接工具箱超时或网络异常，请稍后再试")
}

func TestCardCatalogSnapshotErrorTitleDistinguishesUpstreamFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "missing suite",
			err:  sekaiapi.ErrGameDataNotFound,
			want: CardCatalogTitleNoSuite,
		},
		{
			name: "authentication failure",
			err:  &sekaiapi.ToolboxAPIError{StatusCode: 401, Message: "unauthorized"},
			want: "工具箱请求失败（状态 401）；当前显示全服卡牌",
		},
		{
			name: "network timeout",
			err:  errString("toolbox: request failed after retries: context deadline exceeded"),
			want: "连接工具箱超时或网络异常，请稍后再试；当前显示全服卡牌",
		},
		{
			name: "unknown internal failure",
			err:  errString("internal detail must not be exposed"),
			want: CardCatalogTitleSuiteUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cardCatalogSnapshotErrorTitle(tt.err, nil); got != tt.want {
				t.Fatalf("cardCatalogSnapshotErrorTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
