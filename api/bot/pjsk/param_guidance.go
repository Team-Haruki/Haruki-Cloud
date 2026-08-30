package pjsk

import "strings"

type parameterGuidance struct {
	name        string
	expectation string
}

type parameterErrorPattern struct {
	prefix      string
	parameter   string
	expectation string
	generic     bool
}

var commandParameterGuidance = map[string]parameterGuidance{
	"arrest":                      {"查水表参数", "使用 @用户、游戏 UID 或 u序号"},
	"card/box":                    {"查卡箱参数", cardFilterGuidance},
	"card/detail":                 {"卡牌详情参数", "使用卡牌 ID、角色名或可识别的卡牌条件"},
	"card/image":                  {"卡面参数", "使用卡牌 ID 或可识别的卡牌条件"},
	"card/list":                   {"卡牌列表参数", cardFilterGuidance},
	"costume/combo":               {"3D 组合参数", "按帮助中的标签填写角色、服装、饰品、发型及其 ID"},
	"costume/detail":              {"服装详情参数", "使用组件 ID，或角色加服装、饰品、发型名称"},
	"costume/list":                {"服装列表参数", "使用角色、部件类型或帮助中列出的筛选条件"},
	"deck/event":                  {"活动组卡参数", "检查活动、歌曲、火数、目标、算法、固定卡/角色等参数"},
	"deck/challenge":              {"挑战组卡参数", "先提供挑战角色；歌曲、难度及组卡筛选项按帮助填写"},
	"deck/no-event":               {"长草组卡参数", "检查歌曲、难度、目标、算法、固定卡/角色等参数"},
	"deck/bonus":                  {"加成组卡参数", "可先写 event123，再填写一个或多个正整数目标加成"},
	"deck/mysekai":                {"烤森组卡参数", "检查活动、WL角色、固定卡/角色和培养条件；不要填写普通歌曲、火数或队友参数"},
	"deck/score-up":               {"实效计算参数", "必须依次提供 5 个非负技能数值：队长、技能2、技能3、技能4、技能5"},
	"education/area":              {"区域道具参数", "使用团名、角色名或道具分类；full 需与分类一起使用"},
	"education/bonds":             {"绊等级参数", "使用两个角色名或帮助中列出的筛选条件"},
	"education/challenge":         {"挑战等级参数", "使用角色名或角色 ID"},
	"education/character-mission": {"角色任务参数", "使用角色名或角色 ID，可按帮助追加任务分类"},
	"education/leader":            {"队长次数参数", "使用角色名或角色 ID"},
	"education/power":             {"综合力加成参数", "使用团名、角色名、属性或帮助中列出的分类"},
	"event":                       {"活动查询参数", "使用活动 ID、event123、活动名称或帮助中列出的筛选条件"},
	"event/list":                  {"活动列表参数", "使用活动类型、团体、属性、角色或时间范围等筛选条件"},
	"event/planner":               {"活动规划参数", "先提供目标 pt 或目标排名，再按帮助填写当前 pt、活动、歌曲、火数及组卡条件"},
	"event/record":                {"活动记录参数", "使用活动 ID、event123 或可识别的活动条件"},
	"gacha":                       {"卡池查询参数", "使用卡池 ID、卡池名称或帮助中列出的筛选条件"},
	"inventory/list":              {"库存查询参数", "使用资源分类、名称或帮助中列出的筛选条件"},
	"misc/birthday":               {"生日查询参数", "使用角色名、月份、日期或帮助中列出的筛选条件"},
	"music":                       {"歌曲查询参数", musicDifficultyGuidance},
	"music/b30":                   {"B30 参数", "使用支持的难度、FC/AP 等筛选条件"},
	"music/bpm":                   {"BPM 查询参数", "使用歌曲 ID、歌曲名或别名"},
	"music/bpm-search":            {"BPM 搜索参数", "使用 BPM 数值或范围"},
	"music/chart":                 {"谱面参数", "使用歌曲 ID、歌曲名或别名，并指定支持的难度"},
	"music/cover":                 {"翻唱版本参数", "使用歌曲 ID、歌曲名或别名"},
	"music/list":                  {"歌曲列表参数", "使用难度、等级、团体、标签或帮助中列出的筛选条件"},
	"music/note-count":            {"物量查询参数", musicDifficultyGuidance},
	"music/progress":              {"歌曲进度参数", "使用支持的难度及 FC/AP 等筛选条件"},
	"music/rewards":               {"歌曲奖励参数", musicDifficultyGuidance},
	"mysekai/blueprint":           {"烤森蓝图参数", "使用蓝图分类、名称、角色或帮助中列出的筛选条件"},
	"mysekai/door-upgrade":        {"烤森大门升级参数", "使用支持的大门等级或目标等级"},
	"mysekai/fixture-detail":      {"烤森家具参数", "使用一个或多个家具 ID，或 full 加分类/关键词"},
	"mysekai/fixture-list":        {"烤森家具列表参数", "使用家具分类、来源或名称关键词"},
	"mysekai/housing-sk":          {"百景查询参数", "使用榜单页码、投稿编号或帮助中列出的筛选条件"},
	"mysekai/map":                 {"烤森地图参数", "使用 1 到 4 的地图编号"},
	"mysekai/music-record":        {"烤森唱片参数", "使用唱片名称、歌曲名或帮助中列出的筛选条件"},
	"mysekai/overview":            {"烤森总览参数", "仅使用帮助中支持的账号选择参数"},
	"mysekai/photo":               {"烤森照片参数", "使用有效的照片序号"},
	"mysekai/resource":            {"烤森资源参数", "使用资源分类、地图编号或帮助中列出的筛选条件"},
	"mysekai/talk-list":           {"烤森对话参数", "使用角色名或帮助中列出的筛选条件"},
	"profile":                     {"个人信息参数", "使用自己的绑定账号、游戏 UID 或 u序号"},
	"profile/arrest-difficulty":   {"抓捕难度参数", "使用支持的难度名，并填写开启或关闭"},
	"profile/bg/adjust":           {"个人信息背景参数", "使用横屏/竖屏、模糊 0~10、透明 0~100"},
	"profile/bind":                {"绑定参数", "使用纯数字游戏 UID，可按帮助指定区服"},
	"profile/bind/swap":           {"绑定顺序参数", "使用两个账号序号，例如 u1 u2"},
	"profile/chart-style":         {"谱面样式参数", "使用帮助中列出的样式名称"},
	"profile/custom-profile-card": {"自定义个人信息参数", "按帮助填写卡片布局、字段或开关"},
	"profile/default":             {"默认账号参数", "使用账号 ID 或 u序号"},
	"profile/timezone":            {"时区参数", "使用 UTC 偏移、IANA 时区名或可识别的城市名"},
	"profile/uid":                 {"UID 查询参数", "仅查询自己的绑定账号，可使用 u序号"},
	"profile/unbind":              {"解绑参数", "使用账号 ID 或 u序号"},
	"score":                       {"控分参数", "按帮助填写目标分数、技能值和房间条件"},
	"score/custom-room":           {"自定义房间控分参数", "按帮助填写目标分数、队伍技能和房间条件"},
	"score/music-board":           {"歌曲排行榜参数", "先提供歌曲名或歌曲 ID，再填写难度、模式及榜单计算参数"},
	"score/music-meta":            {"歌曲计算参数", "先提供歌曲名或歌曲 ID，再填写难度及计算条件"},
	"sk/check-room":               {"查房参数", "使用有效的房间号"},
	"sk/csb":                      {"CSB 参数", "使用活动、排名或帮助中列出的查询条件"},
	"sk/daily-speed":              {"日速参数", "使用排名、UID 或帮助中列出的时间范围"},
	"sk/line":                     {"分数线参数", "使用正整数排名；多个排名用空格分隔，范围使用“起始-结束”"},
	"sk/player-trace":             {"玩家追踪参数", "使用纯数字游戏 UID"},
	"sk/predict":                  {"预测参数", "使用正整数排名及帮助中列出的预测条件"},
	"sk/query":                    {"查榜参数", "使用正整数排名、排名范围或纯数字游戏 UID"},
	"sk/rank-trace":               {"排名追踪参数", "使用正整数排名"},
	"sk/speed":                    {"时速参数", "使用正整数排名、排名范围或纯数字游戏 UID"},
	"sk/winrate":                  {"胜率参数", "使用帮助中列出的活动及对比条件"},
	"stamp":                       {"表情查询参数", "使用表情 ID、角色名或关键词"},
	"vlive":                       {"虚拟 Live 参数", "使用 Live ID、名称或帮助中列出的筛选条件"},
}

var parameterErrorPatterns = []parameterErrorPattern{
	{prefix: "无效的游戏ID", parameter: "游戏 UID", expectation: "应为至少 10 位纯数字"},
	{prefix: "无效的用户ID", parameter: "用户 ID", expectation: "应为纯数字"},
	{prefix: "无效的UID", parameter: "游戏 UID", expectation: "应为纯数字"},
	{prefix: "无法解析的排名参数", parameter: "排名", expectation: "应为正整数；多个排名用空格分隔，范围使用“起始-结束”"},
	{prefix: "起始排名不能大于结束排名", parameter: "排名范围", expectation: "起始排名必须小于或等于结束排名"},
	{prefix: "无法解析要比较的歌曲", parameter: "歌曲比较", expectation: musicAliasGuidance},
	{prefix: "failed to resolve compare music selection", parameter: "歌曲比较", expectation: musicAliasGuidance},
	{prefix: "unable to parse music query", parameter: "歌曲", expectation: musicAliasGuidance},
	{prefix: "failed to resolve deck music selection", parameter: "歌曲", expectation: "使用歌曲 ID、歌曲名或可识别的别名，可追加支持的难度"},
	{prefix: "failed to resolve music meta query", parameter: "歌曲", expectation: musicAliasGuidance},
	{prefix: "找不到歌曲或参数错误", parameter: "歌曲/谱面", expectation: "检查歌曲 ID、名称、难度和榜单计算参数"},
	{prefix: "找不到特定的歌", parameter: "歌曲", expectation: "使用歌曲 ID、歌曲名或更明确的别名"},
	{prefix: "music not found", parameter: "歌曲", expectation: "使用歌曲 ID、歌曲名或更明确的别名"},
	{prefix: "failed to search card list", parameter: "卡牌筛选", expectation: cardFilterGuidance},
	{prefix: "failed to search card box", parameter: "卡箱筛选", expectation: cardFilterGuidance},
	{prefix: "failed to search card", parameter: "卡牌", expectation: cardQueryGuidance},
	{prefix: "card not found (filter)", parameter: "卡牌", expectation: cardQueryGuidance},
	{prefix: "no cards found for filter", parameter: "卡牌筛选", expectation: "减少筛选条件，或使用角色、属性、稀有度等支持项"},
	{prefix: "gacha not found", parameter: "卡池", expectation: "使用卡池 ID、名称或更明确的筛选条件"},
	{prefix: "解析综合力失败", parameter: "综合力", expectation: "使用“综合数字”，例如“综合20w”"},
	{prefix: "解析活动加成失败", parameter: "活动加成", expectation: "使用“加成数字”或“加成数字%”"},
	{prefix: "解析游玩间隔失败", parameter: "游玩间隔", expectation: "使用“间隔数字”，可带“秒”"},
	{prefix: "无法解析指定的队友综合力", parameter: "队友综合力", expectation: "使用“队友综合数字”，例如“队友综合25w”"},
	{prefix: "无法解析指定的队友实效", parameter: "队友实效", expectation: "使用“队友实效数字”，例如“队友实效200”"},
	{prefix: "无法解析指定的实效下限", parameter: "实效下限", expectation: "使用“实效数字”，例如“实效200”"},
	{prefix: "格式错误，#后面请填写卡牌ID或角色", parameter: "固定卡牌/角色", expectation: "在 # 后填写正整数卡牌 ID 或可识别的角色名"},
	{prefix: "固定卡牌ID必须为正整数", parameter: "固定卡牌 ID", expectation: "应为正整数"},
	{prefix: "固定卡牌或固定角色不能为空", parameter: "固定卡牌/角色", expectation: "在 # 后填写卡牌 ID 或角色名"},
	{prefix: "固定卡牌和固定角色总数不能超过5个", parameter: "固定卡牌/角色数量", expectation: "固定卡牌和固定角色合计最多 5 个"},
	{prefix: "固定角色最多只能指定5个", parameter: "固定角色数量", expectation: "最多指定 5 个角色"},
	{prefix: "固定角色ID必须为正整数", parameter: "固定角色 ID", expectation: "应为正整数"},
	{prefix: "固定角色ID不能重复", parameter: "固定角色 ID", expectation: "同一角色只能指定一次"},
	{prefix: "无效的 WL 角色ID", parameter: "WL 角色", expectation: "使用可识别的角色名或有效角色 ID"},
	{prefix: "无效的 WL 活动序号", parameter: "WL 活动", expectation: "使用有效的 wl序号 或 event活动ID"},
	{prefix: "不支持的服务器", parameter: "区服", expectation: "仅支持 jp、cn、tw、kr、en"},
	{prefix: "不支持的区服", parameter: "区服", expectation: "仅支持 jp、cn、tw、kr、en"},
	{prefix: "无法识别的个人信息背景参数", parameter: "背景设置", expectation: "使用横屏/竖屏、模糊 0~10、透明 0~100"},
	{prefix: "数值超出范围", parameter: "数值范围", expectation: "使用提示范围内的整数"},
	{prefix: "请提供正确的数值", parameter: "数值", expectation: "应为提示范围内的整数"},
	{prefix: "找不到符合的时区", parameter: "时区", expectation: "使用 UTC 偏移、IANA 时区名或可识别的城市名"},
	{prefix: "不支持的难度", parameter: "难度", expectation: "使用 easy、normal、hard、expert、master 或 append"},
	{prefix: "未识别到有效的烤森地图编号", parameter: "地图编号", expectation: "应为 1 到 4"},
	{prefix: "请输入正确的家具ID", parameter: "家具 ID", expectation: "应为正整数，可一次填写多个"},
	{prefix: "照片编号超出范围", parameter: "照片序号", expectation: "使用当前照片列表范围内的序号"},
	{prefix: "mysekai talk list invalid character query", parameter: "角色", expectation: characterQueryGuidance},
	{prefix: "mysekai fixture detail invalid query", parameter: "家具", expectation: "使用家具 ID、分类或名称关键词"},
	{prefix: "未识别到角色", parameter: "角色", expectation: characterQueryGuidance},
	{prefix: "未找到角色", parameter: "角色", expectation: characterQueryGuidance},
	{prefix: "匹配到多个角色", parameter: "角色", expectation: "使用更完整的角色名或明确的角色 ID"},
	{prefix: "未找到家具类别", parameter: "家具分类", expectation: "使用帮助中列出的家具分类或来源"},
	{prefix: "找不到特定的卡牌", parameter: "卡牌", expectation: cardQueryGuidance},
	{prefix: "找不到特定的卡池", parameter: "卡池", expectation: "使用卡池 ID、名称或更明确的筛选条件"},
	{prefix: "活动查询参数错误", generic: true},
	{prefix: "活动查询参数格式不正确", generic: true},
	{prefix: "查卡池参数错误", generic: true},
	{prefix: "卡池查询参数格式不正确", generic: true},
	{prefix: "无效的参数", generic: true},
	{prefix: "参数格式不正确", generic: true},
	{prefix: "无法解析的指令", generic: true},
	{prefix: "无法解析的列表查询指令", generic: true},
	{prefix: "无法解析的活动指令", generic: true},
	{prefix: "无法识别的指令格式", generic: true},
	{prefix: "未知的查询模式", generic: true},
	{prefix: "无法识别查询参数", generic: true},
	{prefix: "无法识别组合参数", generic: true},
	{prefix: "invalid score control request", generic: true},
	{prefix: "invalid custom-room score request", generic: true},
	{prefix: "invalid birthday request", generic: true},
	{prefix: "invalid event gacha query", generic: true},
	{prefix: "invalid gacha id", generic: true},
	{prefix: "music board request has no items", generic: true},
}

func parameterGuidanceForError(line, commandPath string) (parameterGuidance, bool) {
	line = strings.TrimSpace(line)
	branch, hasBranch := parameterGuidanceForPath(commandPath)
	for _, pattern := range parameterErrorPatterns {
		if !hasGuidancePrefix(line, pattern.prefix) {
			continue
		}
		if pattern.generic {
			if hasBranch {
				return branch, true
			}
			return parameterGuidance{name: "命令参数", expectation: "检查参数数量、顺序和格式"}, true
		}
		name := pattern.parameter
		if hasBranch && !strings.Contains(branch.name, pattern.parameter) {
			name = branch.name + " · " + pattern.parameter
		}
		return parameterGuidance{name: name, expectation: pattern.expectation}, true
	}
	return parameterGuidance{}, false
}

func parameterGuidanceForPath(commandPath string) (parameterGuidance, bool) {
	commandPath = strings.Trim(strings.TrimSpace(commandPath), "/")
	guidance, ok := commandParameterGuidance[commandPath]
	return guidance, ok
}

func hasGuidancePrefix(line, prefix string) bool {
	line = strings.ToLower(strings.TrimSpace(line))
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	return line == prefix || strings.HasPrefix(line, prefix+":") ||
		strings.HasPrefix(line, prefix+"：") || strings.HasPrefix(line, prefix+" ") ||
		strings.HasPrefix(line, prefix+"（")
}

func formatParameterGuidance(guidance parameterGuidance, helpTrigger string) string {
	name := strings.TrimSpace(guidance.name)
	if name == "" {
		name = "命令参数"
	}
	lines := []string{"参数解析失败：" + name}
	if expectation := strings.TrimSpace(guidance.expectation); expectation != "" {
		lines = append(lines, "要求："+expectation)
	}
	if trigger := normalizeUsageHelpTrigger(helpTrigger); trigger != "" {
		lines = append(lines, "查看完整用法请发送："+trigger+" -help")
	}
	return strings.Join(lines, "\n")
}
