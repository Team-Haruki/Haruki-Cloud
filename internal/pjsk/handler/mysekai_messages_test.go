package handler

import "testing"

func TestNormalizeMySekaiUserFacingErrorTalkListUsage(t *testing.T) {
	err := normalizeMySekaiUserFacingError(errString("mysekai talk list requires character query"), "mysekai-talk-list")
	assertReplayErrorText(t, err, "使用方式:\n/烤森对话列表\n/烤森对话列表 角色名\n查看家具详情请使用：/msf 家具ID")
}

func TestNormalizeMySekaiUserFacingErrorServiceUnavailable(t *testing.T) {
	err := normalizeMySekaiUserFacingError(errString("mysekai service unavailable: mysekai controller is not configured"), "mysekai-talk-list")
	assertReplayErrorText(t, err, "烤森服务未就绪，请稍后再试")
}
