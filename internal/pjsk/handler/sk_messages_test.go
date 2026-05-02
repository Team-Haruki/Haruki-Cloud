package handler

import "testing"

func TestNormalizeSKUserFacingErrorTranslatesMissingEventID(t *testing.T) {
	err := normalizeSKUserFacingError(errString("event_id is required when no current event can be inferred"))
	assertReplayErrorText(t, err, "当前没有可推断的活动，请指定活动ID")
}

func TestNormalizeSKUserFacingErrorTranslatesMissingRanks(t *testing.T) {
	err := normalizeSKUserFacingError(errString("tracker ranks/user_id are empty"))
	assertReplayErrorText(t, err, "请指定排名或用户ID")
}

func TestNormalizeSKUserFacingErrorTranslatesServiceError(t *testing.T) {
	err := normalizeSKUserFacingError(errString("tracker client is not configured"))
	assertReplayErrorText(t, err, "查榜服务未就绪，请稍后再试")
}
