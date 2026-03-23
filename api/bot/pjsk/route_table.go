package pjsk

import "haruki-cloud/internal/pjsk/parser"

// botModeEntry describes one bot API endpoint: which parser module/mode it maps to,
// its URL path (relative to /internal/pjsk/bot/), and human-readable command prefixes
// for the manifest that Bot downloads to pre-match commands to endpoints.
type botModeEntry struct {
	module   parser.TargetModule
	mode     string
	path     string   // relative to /internal/pjsk/bot/, e.g. "card/detail"
	prefixes []string // representative command prefixes for the manifest
}

// botModeTable is the single source of truth for all 41 render-mode endpoints.
// The manifest endpoint and route registration both derive from this table.
var botModeTable = []botModeEntry{
	// ── Card ──────────────────────────────────────────────────────────────────
	{parser.ModuleCard, "card-detail", "card/detail",
		[]string{"/卡面", "/详情", "/card-detail"}},
	{parser.ModuleCard, "card-list", "card/list",
		[]string{"/查卡", "/查牌", "/card", "/cards", "/pjsk card"}},
	{parser.ModuleCard, "card-box", "card/box",
		[]string{"/查箱", "/查框", "/box", "/card-box", "/pjsk box"}},

	// ── Gacha ─────────────────────────────────────────────────────────────────
	{parser.ModuleGacha, "gacha", "gacha",
		[]string{"/卡池", "/查卡池", "/gacha", "/gacha-list", "/pjsk gacha"}},

	// ── Music ─────────────────────────────────────────────────────────────────
	{parser.ModuleMusic, "music-detail", "music",
		[]string{"/查曲", "/查歌", "/歌曲", "/乐曲", "/song", "/music"}},
	{parser.ModuleMusic, "music-list", "music/list",
		[]string{"/歌曲列表", "/难度排行", "/定数表", "/music-list"}},
	{parser.ModuleMusic, "music-chart", "music/chart",
		[]string{"/谱面", "/查谱面", "/谱面预览", "/chart", "/music-chart"}},
	{parser.ModuleMusic, "music-progress", "music/progress",
		[]string{"/打歌进度", "/歌曲进度", "/progress", "/music-progress"}},
	{parser.ModuleMusic, "music-rewards", "music/rewards",
		[]string{"/曲目奖励", "/歌曲奖励", "/music-rewards"}},

	// ── Deck ──────────────────────────────────────────────────────────────────
	{parser.ModuleDeck, "deck-event", "deck/event",
		[]string{"/活动组卡", "/组卡", "/配队", "/pjsk deck"}},
	{parser.ModuleDeck, "deck-challenge", "deck/challenge",
		[]string{"/挑战组卡", "/挑战配队", "/pjsk challenge deck"}},
	{parser.ModuleDeck, "deck-no-event", "deck/no-event",
		[]string{"/长草组卡", "/最强卡组", "/pjsk best deck"}},
	{parser.ModuleDeck, "deck-bonus", "deck/bonus",
		[]string{"/加成组卡", "/控分组卡", "/pjsk bonus deck"}},
	{parser.ModuleDeck, "deck-mysekai", "deck/mysekai",
		[]string{"/烤森组卡", "/ms组卡", "/mysekai deck"}},

	// ── Event ─────────────────────────────────────────────────────────────────
	{parser.ModuleEvent, "event-detail", "event",
		[]string{"/活动", "/查活动", "/event"}},
	{parser.ModuleEvent, "event-list", "event/list",
		[]string{"/活动列表", "/活动一览", "/events", "/event-list"}},

	// ── Education ─────────────────────────────────────────────────────────────
	{parser.ModuleEducation, "education-challenge", "education/challenge",
		[]string{"/挑战赛", "/挑战信息", "/挑战一览", "/challenge info"}},
	{parser.ModuleEducation, "education-power", "education/power",
		[]string{"/加成信息", "/角色加成", "/power bonus"}},
	{parser.ModuleEducation, "education-area", "education/area",
		[]string{"/区域道具", "/道具升级", "/area item"}},
	{parser.ModuleEducation, "education-bonds", "education/bonds",
		[]string{"/羁绊", "/牵绊", "/羁绊等级", "/pjsk bonds"}},
	{parser.ModuleEducation, "education-leader", "education/leader",
		[]string{"/加成统计", "/领队统计", "/pjsk leader count"}},

	// ── Score ─────────────────────────────────────────────────────────────────
	{parser.ModuleScore, "score-control", "score",
		[]string{"/分数", "/查分数", "/pjsk score"}},
	{parser.ModuleScore, "score-custom-room", "score/custom-room",
		[]string{"/自定义房间分数", "/custom room score"}},
	{parser.ModuleScore, "score-music-meta", "score/music-meta",
		[]string{"/曲目meta", "/music meta", "/pjsk music meta"}},
	{parser.ModuleScore, "score-music-board", "score/music-board",
		[]string{"/曲目榜", "/music board", "/pjsk music board"}},

	// ── Stamp ─────────────────────────────────────────────────────────────────
	{parser.ModuleStamp, "stamp-list", "stamp",
		[]string{"/贴纸", "/查贴纸", "/pjsk stamp", "/stamp"}},

	// ── Misc ──────────────────────────────────────────────────────────────────
	{parser.ModuleMisc, "misc-birthday", "misc/birthday",
		[]string{"/角色生日", "/生日贺图", "/chara birthday"}},

	// ── SK ────────────────────────────────────────────────────────────────────
	{parser.ModuleSK, "sk-line", "sk/line",
		[]string{"/sk线", "/榜线", "/skl", "/sk-line"}},
	{parser.ModuleSK, "sk-query", "sk/query",
		[]string{"/sk查询", "/sk查分", "/pjsk board", "/sk-query"}},
	{parser.ModuleSK, "sk-check-room", "sk/check-room",
		[]string{"/查房", "/cf", "/csb", "/冲水板", "/sk-check-room"}},
	{parser.ModuleSK, "sk-speed", "sk/speed",
		[]string{"/sk时速", "/时速线", "/sks", "/skv", "/sk-speed"}},
	{parser.ModuleSK, "sk-player-trace", "sk/player-trace",
		[]string{"/sk玩家轨迹", "/玩家轨迹", "/ptr", "/sk-player-trace"}},
	{parser.ModuleSK, "sk-rank-trace", "sk/rank-trace",
		[]string{"/sk档线轨迹", "/档线轨迹", "/rtr", "/skt", "/sk-rank-trace"}},
	{parser.ModuleSK, "sk-winrate", "sk/winrate",
		[]string{"/sk胜率", "/胜率预测", "/5v5预测", "/sk-winrate"}},

	// ── MySekai ───────────────────────────────────────────────────────────────
	{parser.ModuleMysekai, "mysekai-resource", "mysekai/resource",
		[]string{"/mysekai资源", "/烤森资源", "/msr", "/msmap"}},
	{parser.ModuleMysekai, "mysekai-fixture-list", "mysekai/fixture-list",
		[]string{"/mysekai家具列表", "/烤森家具列表", "/msf"}},
	{parser.ModuleMysekai, "mysekai-fixture-detail", "mysekai/fixture-detail",
		[]string{"/mysekai家具详情", "/烤森家具详情", "/mysekai-fixture-detail"}},
	{parser.ModuleMysekai, "mysekai-door-upgrade", "mysekai/door-upgrade",
		[]string{"/mysekai大门升级", "/烤森大门升级", "/msg", "/msgate"}},
	{parser.ModuleMysekai, "mysekai-music-record", "mysekai/music-record",
		[]string{"/mysekai唱片", "/烤森唱片", "/msm", "/mss"}},
	{parser.ModuleMysekai, "mysekai-talk-list", "mysekai/talk-list",
		[]string{"/mysekai对话列表", "/烤森对话列表", "/mysekai-talk-list"}},

	// ── Profile ───────────────────────────────────────────────────────────────
	{parser.ModuleProfile, "profile", "profile",
		[]string{"/sk", "/个人中心", "/名片", "/profile", "/pjsk profile"}},
}

// moduleNameStr converts a TargetModule to its string name for the manifest.
func moduleNameStr(m parser.TargetModule) string {
	switch m {
	case parser.ModuleCard:
		return "card"
	case parser.ModuleGacha:
		return "gacha"
	case parser.ModuleMusic:
		return "music"
	case parser.ModuleDeck:
		return "deck"
	case parser.ModuleEvent:
		return "event"
	case parser.ModuleEducation:
		return "education"
	case parser.ModuleScore:
		return "score"
	case parser.ModuleStamp:
		return "stamp"
	case parser.ModuleMisc:
		return "misc"
	case parser.ModuleSK:
		return "sk"
	case parser.ModuleMysekai:
		return "mysekai"
	case parser.ModuleProfile:
		return "profile"
	case parser.ModuleHelp:
		return "help"
	default:
		return "unknown"
	}
}
