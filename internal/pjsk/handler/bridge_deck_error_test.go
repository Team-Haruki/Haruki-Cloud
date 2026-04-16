package handler

import (
	"testing"

	onebot11 "haruki-cloud/internal/pjsk/onebot11"
)

func TestNormalizeDeckUserFacingError(t *testing.T) {
	testCases := []struct {
		name    string
		input   error
		wantErr string
	}{
		{
			name:    "music not found",
			input:   errString("failed to search music by title or alias: music not found: 虾ex"),
			wantErr: "未找到歌曲：虾ex",
		},
		{
			name:    "snapshot required",
			input:   errString("local user snapshot is not configured"),
			wantErr: "组卡需要用户卡牌持有数据，请先绑定账号并上传 Suite 抓包或本地快照",
		},
		{
			name:    "binding missing",
			input:   errString("解析绑定账号失败：record not found"),
			wantErr: "组卡需要先绑定账号；如果已绑定，请确认该账号已上传 Suite 抓包数据",
		},
		{
			name:    "upstream timeout",
			input:   errString("toolbox: request failed after retries: context deadline exceeded"),
			wantErr: "获取组卡所需数据超时，请稍后重试",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := normalizeDeckUserFacingError(tc.input)
			replyErr, ok := err.(onebot11.ReplayError)
			if !ok {
				t.Fatalf("expected ReplayError, got %T (%v)", err, err)
			}
			if string(replyErr) != tc.wantErr {
				t.Fatalf("unexpected replay error: %q", replyErr)
			}
		})
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}
