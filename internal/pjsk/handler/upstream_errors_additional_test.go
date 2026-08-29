package handler

import (
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestNormalizeTrackerUserFacingErrorMatrix(t *testing.T) {
	if normalizeTrackerUserFacingError(nil) != nil {
		t.Fatal("nil error should remain nil")
	}
	replay := onebot11.NewReplayError("already normalized")
	if got := normalizeTrackerUserFacingError(replay); got.Error() != replay.Error() {
		t.Fatalf("replay error changed: %v", got)
	}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "ranking missing", err: sekaiapi.ErrRankingNotFound, want: "当前榜单没有找到对应的排行榜数据"},
		{name: "maintenance", err: sekaiapi.ErrServerMaintenance, want: "当前游戏服务器维护中，请稍后再试"},
		{name: "not configured", err: errors.New("tracker client is not configured"), want: "查榜服务未就绪，请稍后再试"},
		{name: "empty base URL", err: errors.New("tracker: base_url is empty"), want: "查榜服务未就绪，请稍后再试"},
		{name: "retry timeout", err: errors.New("tracker: request failed after retries"), want: "连接查榜服务超时或网络异常，请稍后再试"},
		{name: "deadline", err: errors.New("context deadline exceeded"), want: "连接查榜服务超时或网络异常，请稍后再试"},
		{name: "client timeout", err: errors.New("Client.Timeout exceeded"), want: "连接查榜服务超时或网络异常，请稍后再试"},
		{name: "io timeout", err: errors.New("i/o timeout"), want: "连接查榜服务超时或网络异常，请稍后再试"},
		{name: "refused", err: errors.New("connection refused"), want: "查榜服务暂时不可用，请稍后再试"},
		{name: "dns", err: errors.New("no such host"), want: "查榜服务暂时不可用，请稍后再试"},
		{name: "decode", err: errors.New("tracker: failed to unmarshal response"), want: "查榜服务返回数据解析失败，请稍后再试"},
		{name: "typed translated", err: &sekaiapi.TrackerAPIError{StatusCode: 429, Message: "rate limited by tracker"}, want: "查榜请求过于频繁，请稍后再试"},
		{name: "typed empty", err: &sekaiapi.TrackerAPIError{StatusCode: 500}, want: "查榜请求失败（状态 500）"},
		{name: "typed unknown", err: &sekaiapi.TrackerAPIError{StatusCode: 418, Message: "teapot"}, want: "查榜请求失败（状态 418）"},
		{name: "string translated", err: errors.New(`tracker api error: status 429, message: "rate limited by tracker"`), want: "查榜请求过于频繁，请稍后再试"},
		{name: "string unknown", err: errors.New(`tracker api error: status 500, message: "unknown"`), want: "查榜请求失败，请稍后再试"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReplayErrorText(t, normalizeTrackerUserFacingError(tt.err), tt.want)
		})
	}

	original := errors.New("unclassified")
	if got := normalizeTrackerUserFacingError(original); got != original {
		t.Fatalf("unclassified error changed: %v", got)
	}
}

func TestNormalizeDrawingUserFacingErrorMatrix(t *testing.T) {
	if normalizeDrawingUserFacingError(nil) != nil {
		t.Fatal("nil error should remain nil")
	}
	replay := onebot11.NewReplayError("already normalized")
	if got := normalizeDrawingUserFacingError(replay); got.Error() != replay.Error() {
		t.Fatalf("replay error changed: %v", got)
	}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "client missing", err: errors.New("drawing client is not configured"), want: "渲染服务未就绪，请稍后再试"},
		{name: "upstream missing", err: errors.New("drawing upstream is unavailable"), want: "渲染服务未就绪，请稍后再试"},
		{name: "base URL", err: errors.New("drawing client base_url is empty"), want: "渲染服务未就绪，请稍后再试"},
		{name: "storage", err: errors.New("image storage is not configured"), want: "图片服务未就绪，请稍后再试"},
		{name: "asset path", err: errors.New("asset path is empty"), want: "图片资源不可用"},
		{name: "timeout", err: errors.New("context deadline exceeded"), want: "连接渲染服务超时或网络异常，请稍后再试"},
		{name: "refused", err: errors.New("connection refused"), want: "渲染服务暂时不可用，请稍后再试"},
		{name: "eof", err: errors.New("EOF"), want: "渲染服务暂时不可用，请稍后再试"},
		{name: "translated detail", err: errors.New(`api request failed with status: 500 body: {"detail":"canvas size is too large"}`), want: "渲染画布过大，请减少查询内容后重试"},
		{name: "404", err: errors.New(`api request failed with status: 404 body: unknown`), want: "图片资源缺失，暂时无法渲染"},
		{name: "500", err: errors.New(`api request failed with status: 500 body: unknown`), want: "渲染服务内部错误，请稍后再试"},
		{name: "400", err: errors.New(`api request failed with status: 400 body: unknown`), want: "渲染请求失败（状态 400）"},
		{name: "direct translation", err: errors.New("failed to open image"), want: "图片资源损坏，暂时无法渲染"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReplayErrorText(t, normalizeDrawingUserFacingError(tt.err), tt.want)
		})
	}
	original := errors.New("unclassified drawing failure")
	if got := normalizeDrawingUserFacingError(original); got != original {
		t.Fatalf("unclassified error changed: %v", got)
	}
}

func TestDrawingDataInsufficientClassification(t *testing.T) {
	if normalizeSKPlayerTraceDrawingError(nil) != nil {
		t.Fatal("nil should remain nil")
	}
	replay := onebot11.NewReplayError("kept")
	if got := normalizeSKPlayerTraceDrawingError(replay); got.Error() != replay.Error() {
		t.Fatalf("replay changed: %v", got)
	}

	positive := []error{
		drawing.ErrDrawingDataInsufficient,
		errors.New("insufficient data"),
		errors.New("not enough data"),
		errors.New("数据不足"),
		errors.New("single positional indexer is out-of-bounds"),
		errors.New("index out of range"),
		errors.New(`{"detail":"data insufficient"}`),
	}
	for _, err := range positive {
		if !isDrawingDataInsufficientError(err) {
			t.Errorf("expected insufficient classification for %q", err)
		}
		assertReplayErrorText(t, normalizeSKPlayerTraceDrawingError(err), "玩家轨迹数据不足，暂时无法渲染")
	}
	if isDrawingDataInsufficientError(nil) || isDrawingDataInsufficientError(errors.New("")) || isDrawingDataInsufficientError(errors.New("other")) {
		t.Fatal("unexpected insufficient classification")
	}
	original := errors.New("other")
	if got := normalizeSKPlayerTraceDrawingError(original); got != original {
		t.Fatalf("unclassified trace error changed: %v", got)
	}
}

func TestNormalizeDeckServiceUserFacingErrorMatrix(t *testing.T) {
	if normalizeDeckServiceUserFacingError(nil) != nil {
		t.Fatal("nil should remain nil")
	}
	replay := onebot11.NewReplayError("kept")
	if got := normalizeDeckServiceUserFacingError(replay); got.Error() != replay.Error() {
		t.Fatalf("replay changed: %v", got)
	}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "unavailable", err: errors.New("deck-service unavailable"), want: "组卡服务未就绪，请稍后再试"},
		{name: "state", err: errors.New("deck-service target state is not initialized"), want: "组卡服务未就绪，请稍后再试"},
		{name: "breaker", err: errors.New("circuit breaker open"), want: "组卡服务未就绪，请稍后再试"},
		{name: "timeout", err: errors.New("context deadline exceeded"), want: "获取组卡所需数据超时，请稍后重试"},
		{name: "refused", err: errors.New("connection refused"), want: "组卡服务暂时不可用，请稍后再试"},
		{name: "translated", err: errors.New("music metas not found"), want: "组卡服务找不到该区服的歌曲元数据，请更新 masterdata 后重试"},
		{name: "http translated", err: errors.New(`deck-service returned HTTP 500: {"error":"returned empty response"}`), want: "组卡服务返回空结果，请稍后再试"},
		{name: "http server", err: errors.New(`deck-service returned HTTP 503: unknown`), want: "组卡服务内部错误，请稍后再试"},
		{name: "http client", err: errors.New(`deck-service returned HTTP 422: unknown`), want: "组卡请求失败（状态 422）"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReplayErrorText(t, normalizeDeckServiceUserFacingError(tt.err), tt.want)
		})
	}
	original := errors.New("unclassified deck failure")
	if got := normalizeDeckServiceUserFacingError(original); got != original {
		t.Fatalf("unclassified error changed: %v", got)
	}
}

func TestUpstreamDetailTranslationMatrices(t *testing.T) {
	binding := &accountdata.ResolvedBinding{Server: "jp", PJSKUserID: "12345678901234", Visible: true}
	for _, message := range []string{
		"invalid platform or platform_user_id",
		"account owner is banned",
		"account binding not found",
		"game data not found",
		"toolbox service unavailable",
		"missing token",
		"invalid token",
	} {
		if translated, ok := translateToolboxAPIDetail("suite", message, binding); !ok || strings.TrimSpace(translated) == "" {
			t.Errorf("toolbox translation failed for %q", message)
		}
	}
	if translated, ok := translateToolboxAPIDetail("suite", "", binding); ok || translated != "" {
		t.Fatal("empty toolbox message should not translate")
	}
	if _, ok := translateToolboxAPIDetail("suite", "unknown", binding); ok {
		t.Fatal("unknown toolbox message translated")
	}

	assertTranslations := func(name string, fn func(string) (string, bool), messages []string) {
		t.Helper()
		for _, message := range messages {
			if translated, ok := fn(message); !ok || strings.TrimSpace(translated) == "" {
				t.Errorf("%s translation failed for %q", name, message)
			}
		}
		if translated, ok := fn(""); ok || translated != "" {
			t.Errorf("%s empty message translated", name)
		}
		if _, ok := fn("unknown"); ok {
			t.Errorf("%s unknown message translated", name)
		}
	}
	assertTranslations("sekai", translateSekaiAPIDetail, []string{
		"missing token", "invalid token", "not authorized for this server", "invalid api type", "internal server error", "upstream unavailable",
	})
	assertTranslations("tracker", translateTrackerAPIDetail, []string{
		"rate limited by tracker", "no heartbeat found", "invalid server", "ranking record not found", "not found",
	})
	assertTranslations("drawing", translateDrawingAPIDetail, []string{
		"content size is too large", "canvas size is too large", "target file not found", "cannot identify image file", "download failed", "connection refused", "context deadline exceeded",
	})
	assertTranslations("deck", translateDeckServiceDetail, []string{
		"fixed_characters and fixed_cards cannot be used together",
		"event not found for eventId: 1",
		"music metas not found",
		"userdata_hash is required",
		"unsupported media type",
		"invalid recommend payload",
		"no user data bytes available",
		"returned empty response",
		"connection refused",
		"context deadline exceeded",
	})
}

func TestUpstreamErrorPayloadParsing(t *testing.T) {
	tests := []struct {
		message string
		prefix  string
		status  int
		detail  string
		ok      bool
	}{
		{message: `api failed 404 body: {"detail":"missing"}`, prefix: "api failed", status: 404, detail: "missing", ok: true},
		{message: `api failed 503 payload: {"error":"offline"}`, prefix: "api failed", status: 503, detail: "offline", ok: true},
		{message: "api failed nope", prefix: "api failed", ok: false},
		{message: "other 500", prefix: "api failed", ok: false},
	}
	for _, tt := range tests {
		status, detail, ok := extractStatusAndPayload(tt.message, tt.prefix)
		if status != tt.status || detail != tt.detail || ok != tt.ok {
			t.Errorf("extractStatusAndPayload(%q) = %d %q %v", tt.message, status, detail, ok)
		}
	}

	if status, tail, ok := consumeLeadingInt(" 418 rest"); !ok || status != 418 || tail != " rest" {
		t.Fatalf("consumeLeadingInt = %d %q %v", status, tail, ok)
	}
	for _, value := range []string{"", "none"} {
		if _, _, ok := consumeLeadingInt(value); ok {
			t.Errorf("consumeLeadingInt(%q) unexpectedly succeeded", value)
		}
	}

	for _, tt := range []struct {
		raw  string
		want string
	}{
		{raw: "", want: ""},
		{raw: `"quoted"`, want: "quoted"},
		{raw: `{"detail":"detail text"}`, want: "detail text"},
		{raw: `{"error":"error text"}`, want: "error text"},
		{raw: `{"message":"message text"}`, want: "message text"},
		{raw: `{"detail":""}`, want: `{"detail":""}`},
		{raw: "plain", want: "plain"},
	} {
		if got := parseEmbeddedErrorText(tt.raw); got != tt.want {
			t.Errorf("parseEmbeddedErrorText(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
	if got := extractQuotedMessage(`prefix "hello" suffix`); got != "hello" {
		t.Fatalf("quoted message = %q", got)
	}
	if got := extractQuotedMessage("plain"); got != "plain" {
		t.Fatalf("plain message = %q", got)
	}
}
