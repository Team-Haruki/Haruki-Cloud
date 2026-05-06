package handler

import (
	"testing"

	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestNormalizeSekaiAPIFetchErrorTranslatesAuthError(t *testing.T) {
	err := normalizeSekaiAPIFetchError(&sekaiapi.APIError{StatusCode: 401, Message: "Invalid token"})
	assertReplayErrorText(t, err, "SekaiAPI 拉取失败：访问令牌无效，请检查 Cloud 与 SekaiAPI 配置")
}

func TestNormalizeToolboxDataFetchErrorTranslatesPermissionDetail(t *testing.T) {
	err := normalizeToolboxDataFetchError(
		&sekaiapi.ToolboxAPIError{StatusCode: 403, Message: "forbidden: invalid platform or platform_user_id for this user"},
		"mysekai",
		nil,
	)
	assertReplayErrorText(t, err, "当前QQ号未在工具箱完成绑定，或无权访问该mysekai数据，请前往工具箱绑定当前QQ号后重试\n"+ErrMsgToolboxURL)
}

func TestNormalizeSKUserFacingErrorTranslatesTrackerRateLimit(t *testing.T) {
	err := normalizeSKUserFacingError(&sekaiapi.TrackerAPIError{StatusCode: 429, Message: "rate limited by tracker"})
	assertReplayErrorText(t, err, "查榜请求过于频繁，请稍后再试")
}

func TestNormalizeDrawingUserFacingErrorTranslatesContentSize(t *testing.T) {
	err := normalizeDrawingUserFacingError(errString(`api request failed with status: 500, body: {"detail":"Content size is too large with (140, 80) > (96, 80)"}`))
	assertReplayErrorText(t, err, "渲染内容过大，请减少查询内容后重试")
}

func TestNormalizeDeckServiceUserFacingErrorTranslatesFixedConflict(t *testing.T) {
	err := normalizeDeckServiceUserFacingError(errString("fixed_characters and fixed_cards cannot be used together"))
	assertReplayErrorText(t, err, "组卡服务版本过旧，暂不支持同时固定角色和卡牌，请更新组卡服务后重试")
}

func TestWrapDomainErrorTranslatesSekaiProfileFetchError(t *testing.T) {
	err := WrapDomainError(errString(`sekaiapi profile fetch failed: sekai api error: status 401, message: "Invalid token"`))
	assertReplayErrorText(t, err, "SekaiAPI 拉取失败：访问令牌无效，请检查 Cloud 与 SekaiAPI 配置")
}

func TestWrapDomainErrorTranslatesDrawingAPIError(t *testing.T) {
	err := WrapDomainError(errString(`api request failed with status: 500, body: {"detail":"Canvas size is too large with (4000, 2000) > (3000, 1800)"}`))
	assertReplayErrorText(t, err, "渲染画布过大，请减少查询内容后重试")
}

func TestWrapDomainErrorTranslatesDeckServiceCacheMiss(t *testing.T) {
	err := WrapDomainError(errString(`deck-service returned HTTP 500: {"error":"User data not found for userdata_hash: missing-userdata-hash"}`))
	assertReplayErrorText(t, err, "组卡缓存数据失效，请稍后重试")
}
