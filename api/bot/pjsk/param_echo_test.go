package pjsk

import (
	"strings"
	"testing"
)

func TestClientErrorTextRedactsParamEcho(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		trigger string
		path    string
		want    string
	}{
		{
			name:    "quoted event args",
			in:      "活动查询参数错误: \"super-secret\"\n【查单个活动格式】",
			trigger: "/查活动",
			path:    "event",
			want:    "参数解析失败：活动查询参数\n要求：使用活动 ID、event123、活动名称或帮助中列出的筛选条件\n查看完整用法请发送：/查活动 -help",
		},
		{
			name: "replay error args",
			in:   "无效的参数：\"super-secret\"\n使用方式：/cmd",
			want: "参数解析失败：命令参数\n要求：检查参数数量、顺序和格式\n查看完整用法请发送：/cmd -help",
		},
		{
			name: "english token",
			in:   "invalid token \"super-secret\"",
			want: "参数解析失败：命令参数\n要求：检查参数数量、顺序和格式",
		},
		{
			name: "wrapped card query",
			in:   "failed to search card: query card 662: sekai: card not found",
			want: "参数解析失败：卡牌\n要求：使用卡牌 ID、角色名或更明确的筛选条件",
		},
		{
			name: "music not found",
			in:   "CN服找不到特定的歌: super-secret\n如果需要查其他服务器歌曲请加区服前缀",
			want: "CN服找不到特定的歌\n如果需要查其他服务器歌曲请加区服前缀",
		},
		{
			name: "card not found",
			in:   "找不到特定的卡牌: super-secret",
			want: "参数解析失败：卡牌\n要求：使用卡牌 ID、角色名或更明确的筛选条件",
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
			if got := clientErrorTextForCommand(tt.in, false, tt.trigger, tt.path); got != tt.want {
				t.Fatalf("clientErrorText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientErrorTextStillRedactsParamEchoWhenEnabled(t *testing.T) {
	in := "活动查询参数错误: \"super-secret\"\n【查单个活动格式】"
	want := "参数解析失败：活动查询参数\n要求：使用活动 ID、event123、活动名称或帮助中列出的筛选条件\n查看完整用法请发送：/查活动 -help"
	if got := clientErrorTextForCommand(in, true, "/查活动", "event"); got != want {
		t.Fatalf("clientErrorText() = %q, want %q", got, want)
	}
}

func TestClientErrorTextUsesBranchSpecificParameterGuidance(t *testing.T) {
	tests := []struct {
		name    string
		message string
		path    string
		trigger string
		want    string
	}{
		{
			name:    "event deck music",
			message: `failed to resolve deck music selection "do-not-echo"`,
			path:    "deck/event",
			trigger: "/活动组卡",
			want:    "参数解析失败：活动组卡参数 · 歌曲\n要求：使用歌曲 ID、歌曲名或可识别的别名，可追加支持的难度\n查看完整用法请发送：/活动组卡 -help",
		},
		{
			name:    "no event deck usage",
			message: "使用方式:\n/长草组卡 [歌曲/组卡参数...]",
			path:    "deck/no-event",
			trigger: "/长草组卡",
			want:    "参数解析失败：长草组卡参数\n要求：检查歌曲、难度、目标、算法、固定卡/角色等参数\n查看完整用法请发送：/长草组卡 -help",
		},
		{
			name:    "bonus deck generic",
			message: `无效的参数："do-not-echo"`,
			path:    "deck/bonus",
			trigger: "/加成组卡",
			want:    "参数解析失败：加成组卡参数\n要求：可先写 event123，再填写一个或多个正整数目标加成\n查看完整用法请发送：/加成组卡 -help",
		},
		{
			name:    "challenge deck branch",
			message: `无法识别的指令格式: "do-not-echo"`,
			path:    "deck/challenge",
			trigger: "/挑战组卡",
			want:    "参数解析失败：挑战组卡参数\n要求：先提供挑战角色；歌曲、难度及组卡筛选项按帮助填写\n查看完整用法请发送：/挑战组卡 -help",
		},
		{
			name:    "mysekai deck branch",
			message: `无效的参数："do-not-echo"`,
			path:    "deck/mysekai",
			trigger: "/烤森组卡",
			want:    "参数解析失败：烤森组卡参数\n要求：检查活动、WL角色、固定卡/角色和培养条件；不要填写普通歌曲、火数或队友参数\n查看完整用法请发送：/烤森组卡 -help",
		},
		{
			name:    "score up deck branch",
			message: "使用方式: /实效 队长技能 技能2 技能3 技能4 技能5",
			path:    "deck/score-up",
			trigger: "/实效",
			want:    "参数解析失败：实效计算参数\n要求：必须依次提供 5 个非负技能数值：队长、技能2、技能3、技能4、技能5\n查看完整用法请发送：/实效 -help",
		},
		{
			name:    "score board bonus",
			message: `解析活动加成失败: "do-not-echo"`,
			path:    "score/music-board",
			trigger: "/歌曲榜",
			want:    "参数解析失败：歌曲排行榜参数 · 活动加成\n要求：使用“加成数字”或“加成数字%”\n查看完整用法请发送：/歌曲榜 -help",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clientErrorTextForCommand(tt.message, false, tt.trigger, tt.path)
			if got != tt.want {
				t.Fatalf("clientErrorTextForCommand() = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "do-not-echo") {
				t.Fatalf("response echoed original parameter: %q", got)
			}
		})
	}
}

func TestClientErrorTextCompactsStandaloneUsage(t *testing.T) {
	in := "使用方式:\n/区域道具 团名\n/区域道具 角色名\n/区域道具 full"
	want := "参数格式不正确\n查看完整用法请发送：/区域道具 -help"
	if got := clientErrorText(in, false); got != want {
		t.Fatalf("clientErrorText() = %q, want %q", got, want)
	}
}
