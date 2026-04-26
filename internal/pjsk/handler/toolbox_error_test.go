package handler

import (
	"context"
	"errors"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestNormalizeToolboxDataFetchError(t *testing.T) {
	binding := &accountdata.ResolvedBinding{
		Server:     "jp",
		PJSKUserID: "12345678901234",
		Visible:    false,
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
			wantErr:   "当前QQ号未在工具箱完成绑定，或无权访问该mysekai数据，请前往工具箱绑定当前QQ号后重试\n" + ErrMsgToolboxURL,
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
	if string(replyErr) != "当前QQ号未在工具箱完成绑定，或无权访问该suite数据，请前往工具箱绑定当前QQ号后重试\n"+ErrMsgToolboxURL {
		t.Fatalf("unexpected replay error: %q", replyErr)
	}
}
