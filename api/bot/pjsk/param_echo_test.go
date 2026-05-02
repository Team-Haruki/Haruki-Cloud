package pjsk

import "testing"

func TestClientErrorTextRedactsParamEcho(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "quoted event args",
			in:   "活动查询参数错误: \"super-secret\"\n【查单个活动格式】",
			want: "活动查询参数错误\n【查单个活动格式】",
		},
		{
			name: "replay error args",
			in:   "无效的参数：\"super-secret\"\n使用方式：/cmd",
			want: "无效的参数\n使用方式：/cmd",
		},
		{
			name: "english token",
			in:   "invalid token \"super-secret\"",
			want: "invalid token",
		},
		{
			name: "music not found",
			in:   "CN服找不到特定的歌: super-secret\n如果需要查其他区服的歌曲请加区服前缀",
			want: "CN服找不到特定的歌\n如果需要查其他区服的歌曲请加区服前缀",
		},
		{
			name: "non echo error",
			in:   "event_id is required",
			want: "event_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientErrorText(tt.in, false); got != tt.want {
				t.Fatalf("clientErrorText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientErrorTextKeepsParamEchoWhenEnabled(t *testing.T) {
	in := "活动查询参数错误: \"super-secret\"\n【查单个活动格式】"
	if got := clientErrorText(in, true); got != in {
		t.Fatalf("clientErrorText() = %q, want %q", got, in)
	}
}
