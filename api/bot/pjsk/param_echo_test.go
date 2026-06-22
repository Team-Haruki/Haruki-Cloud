package pjsk

import "testing"

func TestClientErrorTextRedactsParamEcho(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		helpTrigger string
		want        string
	}{
		{
			name: "quoted event args",
			in:   "活动查询参数错误: \"super-secret\"\n【查单个活动格式】",
			want: "活动查询参数错误\n【查单个活动格式】",
		},
		{
			name:        "quoted event args with command help fallback",
			in:          "活动查询参数错误: \"super-secret\"\n【查单个活动格式】",
			helpTrigger: "/查活动",
			want:        "活动查询参数错误\n查看完整用法请发送：/查活动 -help",
		},
		{
			name: "replay error args",
			in:   "无效的参数：\"super-secret\"\n使用方式：/cmd",
			want: "无效的参数\n查看完整用法请发送：/cmd -help",
		},
		{
			name: "standalone usage help",
			in:   "使用方式:\n/区域道具 团名\n/区域道具 角色名\n/区域道具 full",
			want: "参数格式不正确\n查看完整用法请发送：/区域道具 -help",
		},
		{
			name: "english token",
			in:   "invalid token \"super-secret\"",
			want: "无效的参数",
		},
		{
			name: "wrapped card query",
			in:   "failed to search card: query card 662: sekai: card not found",
			want: "找不到特定的卡牌",
		},
		{
			name: "music not found",
			in:   "CN服找不到特定的歌: super-secret\n如果需要查其他服务器歌曲请加区服前缀",
			want: "CN服找不到特定的歌\n如果需要查其他服务器歌曲请加区服前缀",
		},
		{
			name: "card not found",
			in:   "找不到特定的卡牌: super-secret",
			want: "找不到特定的卡牌",
		},
		{
			name: "non echo error",
			in:   "event_id is required",
			want: "当前没有可推断的活动，请指定活动ID",
		},
		{
			name: "service error",
			in:   "misc birthday service unavailable: sekai client not configured",
			want: "生日服务未就绪，请稍后再试",
		},
		{
			name: "deck fixed conflict",
			in:   "fixed_characters and fixed_cards cannot be used together",
			want: "组卡服务版本过旧，暂不支持同时固定角色和卡牌，请更新组卡服务后重试",
		},
		{
			name: "drawing api error",
			in:   "api request failed with status: 500, body: {\"detail\":\"Content size is too large\"}",
			want: "渲染请求失败，请稍后再试",
		},
		{
			name: "drawing timeout",
			in:   `Post "http://haruki-drawing:8000/api/pjsk/misc/chara-birthday": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`,
			want: "连接渲染服务超时或网络异常，请稍后再试",
		},
		{
			name: "sekai api error",
			in:   "sekai api error: status 401, message: \"Invalid token\"",
			want: "SekaiAPI 拉取失败，请稍后再试",
		},
		{
			name: "custom chart upstream url",
			in:   `获取自制谱面 JSON 失败: sekai api error: status 502, message: "Fetch failed from https://production-game-api.sekai.colorfulpalette.org/image/blob/custom-music-score/full/a/b"`,
			want: "获取自制谱面数据失败，请稍后再试",
		},
		{
			name: "unknown private url",
			in:   "upstream failed: https://production-game-api.sekai.colorfulpalette.org/api/jp/user/123/profile",
			want: "请求处理失败，请稍后再试",
		},
		{
			name: "public toolbox url",
			in:   "工具箱地址：https://haruki.seiunx.com/",
			want: "工具箱地址：https://haruki.seiunx.com/",
		},
		{
			name: "unknown english error",
			in:   "handler returned nil\nsuper-secret",
			want: "请求处理失败，请稍后再试",
		},
		{
			name: "mixed chinese latin error",
			in:   "BPM 必须大于 0",
			want: "BPM 必须大于 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clientErrorTextForCommand(tt.in, false, tt.helpTrigger)
			if got != tt.want {
				t.Fatalf("clientErrorText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientErrorTextStillRedactsParamEchoWhenEnabled(t *testing.T) {
	in := "活动查询参数错误: \"super-secret\"\n【查单个活动格式】"
	want := "活动查询参数错误\n查看完整用法请发送：/查活动 -help"
	if got := clientErrorTextForCommand(in, true, "/查活动"); got != want {
		t.Fatalf("clientErrorText() = %q, want %q", got, want)
	}
}
