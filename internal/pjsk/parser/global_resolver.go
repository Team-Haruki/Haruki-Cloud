package parser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// TargetModule identifies target module resolved from command.
type TargetModule int

const (
	ModuleUnknown TargetModule = iota
	ModuleCard
	ModuleGacha
	ModuleMusic
	ModuleEvent
	ModuleDeck
	ModuleSK
	ModuleMysekai
	ModuleProfile
	ModuleHelp
	ModuleEducation
	ModuleScore
	ModuleStamp
	ModuleMisc
)

// ResolvedCommand stores normalized command parsing result.
type ResolvedCommand struct {
	Module    TargetModule
	Mode      string
	Query     string
	Region    string
	Params    json.RawMessage
	IsHelp    bool
	IsVerbose bool
	IsPreview bool
}

// GlobalCommandResolver provides unified command parsing.
type GlobalCommandResolver struct {
	extractor *Extractor
	routes    []route
}

type route struct {
	pattern *regexp.Regexp
	module  TargetModule
	mode    string
}

var globalRoutes []route

func init() {
	globalRoutes = []route{
		// ── Card ─────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(card-detail|卡面|详情)\s*(.*)`), ModuleCard, "card-detail"},
		{regexp.MustCompile(`(?i)^/(查卡|查牌|查卡牌|卡牌列表|card|cards|pjsk card|pjsk member)\s*(.*)`), ModuleCard, "card-list"},
		{regexp.MustCompile(`(?i)^/(查箱|查框|卡牌一览|卡面一览|卡一览|box|card-box|pjsk box)\s*(.*)`), ModuleCard, "card-box"},

		// ── Gacha ────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(卡池|查卡池|卡池列表|卡池一览|抽卡|gacha|gacha-list|pjsk gacha)\s*(.*)`), ModuleGacha, "gacha"},

		// ── Music ────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(歌曲列表|歌曲一览|乐曲列表|乐曲一览|难度排行|定数表|歌曲定数|查乐曲|music-list)\s*(.*)`), ModuleMusic, "music-list"},
		{regexp.MustCompile(`(?i)^/(打歌进度|歌曲进度|打歌信息|pjsk进度|progress|music-progress)\s*(.*)`), ModuleMusic, "music-progress"},
		{regexp.MustCompile(`(?i)^/(谱面预览|查谱面|查谱|谱面|查谱图|chart|music-chart)\s*(.*)`), ModuleMusic, "music-chart"},
		{regexp.MustCompile(`(?i)^/(曲目奖励|歌曲奖励|music rewards|music-rewards|pjsk music rewards)\s*(.*)`), ModuleMusic, "music-rewards"},
		{regexp.MustCompile(`(?i)^/(查曲|查歌|查乐|查音乐|查询乐曲|查歌曲|歌曲|乐曲|song|music)\s*(.*)`), ModuleMusic, "music-detail"},

		// ── Deck ─────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(活动组卡|活动组队|活动卡组|活动配队|组卡|组队|配队|指定属性组卡|指定属性组队|指定属性卡组|指定属性配队|模拟组卡|模拟配队|模拟组队|模拟卡组|pjsk event card|pjsk event deck|pjsk deck)\s*(.*)`), ModuleDeck, "deck-event"},
		{regexp.MustCompile(`(?i)^/(挑战组卡|挑战组队|挑战卡组|挑战配队|pjsk challenge card|pjsk challenge deck)\s*(.*)`), ModuleDeck, "deck-challenge"},
		{regexp.MustCompile(`(?i)^/(长草组卡|长草组队|长草卡组|长草配队|最强卡组|最强组卡|最强组队|最强配队|pjsk no event deck|pjsk best deck)\s*(.*)`), ModuleDeck, "deck-no-event"},
		{regexp.MustCompile(`(?i)^/(加成组卡|加成组队|加成卡组|加成配队|控分组卡|控分组队|控分卡组|控分配队|pjsk bonus deck|pjsk bonus card)\s*(.*)`), ModuleDeck, "deck-bonus"},
		{regexp.MustCompile(`(?i)^/(烤森组卡|烤森组队|烤森卡组|烤森配队|ms组卡|ms组队|ms卡组|ms配队|mysekai deck|pjsk mysekai deck)\s*(.*)`), ModuleDeck, "deck-mysekai"},

		// ── Event ────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(活动列表|查活动列表|活动一览|events|event-list)\s*(.*)`), ModuleEvent, "event-list"},
		{regexp.MustCompile(`(?i)^/(活动|查活动|event)\s*(.*)`), ModuleEvent, "event-detail"},

		// ── Education ────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(挑战赛|挑战信息|挑战赛信息|挑战一览|每日挑战|pjsk challenge info|challenge info)\s*(.*)`), ModuleEducation, "education-challenge"},
		{regexp.MustCompile(`(?i)^/(加成信息|角色加成|加成一览|pjsk power bonus info|power bonus)\s*(.*)`), ModuleEducation, "education-power"},
		{regexp.MustCompile(`(?i)^/(区域道具|区域道具材料|道具升级|pjsk area item|area item)\s*(.*)`), ModuleEducation, "education-area"},
		{regexp.MustCompile(`(?i)^/(羁绊|角色羁绊|羁绊等级|羁绊信息|牵绊|牵绊等级|pjsk bonds|pjsk bond)\s*(.*)`), ModuleEducation, "education-bonds"},
		{regexp.MustCompile(`(?i)^/(加成统计|领队统计|角色领队|领队统计|pjsk leader count)\s*(.*)`), ModuleEducation, "education-leader"},

		// ── Score ────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(分数|查分数|pjsk score|score control)\s*(.*)`), ModuleScore, "score-control"},
		{regexp.MustCompile(`(?i)^/(自定义房间分数|自定义分数|custom room score|pjsk custom room score)\s*(.*)`), ModuleScore, "score-custom-room"},
		{regexp.MustCompile(`(?i)^/(曲目meta|music meta|pjsk music meta)\s*(.*)`), ModuleScore, "score-music-meta"},
		{regexp.MustCompile(`(?i)^/(曲目榜|歌曲比较|music board|pjsk music board)\s*(.*)`), ModuleScore, "score-music-board"},

		// ── Stamp ────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(贴纸|查贴纸|pjsk贴纸|pjsk stamp|pjsk bq|stamp)\s*(.*)`), ModuleStamp, "stamp-list"},

		// ── Misc ─────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(角色生日|生日贺图|pjsk chara birthday|chara birthday)\s*(.*)`), ModuleMisc, "misc-birthday"},

		// ── SK ───────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(sk-line|sk线|榜线|pjsk sk line|pjsk board line|skl)\s*(.*)`), ModuleSK, "sk-line"},
		{regexp.MustCompile(`(?i)^/(sk-query|sk查询|sk查分|pjsk sk board|pjsk board)\s*(.*)`), ModuleSK, "sk-query"},
		{regexp.MustCompile(`(?i)^/(sk-check-room|sk查房|查房|cf|pjsk查房|csb|冲水板|pjsk冲水板)\s*(.*)`), ModuleSK, "sk-check-room"},
		{regexp.MustCompile(`(?i)^/(sk-speed|sk时速|时速线|pjsk sk speed|pjsk board speed|sks|skv|sktime)\s*(.*)`), ModuleSK, "sk-speed"},
		{regexp.MustCompile(`(?i)^/(sk-player-trace|sk玩家轨迹|玩家轨迹|ptr|pjsk玩家追踪|pjsk ptr)\s*(.*)`), ModuleSK, "sk-player-trace"},
		{regexp.MustCompile(`(?i)^/(sk-rank-trace|sk档线轨迹|档线轨迹|rtr|skt|sklt|sktl|pjsk追踪|pjsk sk追踪)\s*(.*)`), ModuleSK, "sk-rank-trace"},
		{regexp.MustCompile(`(?i)^/(sk-winrate|sk胜率|胜率预测|pjsk winrate predict|5v5预测|5v5胜率)\s*(.*)`), ModuleSK, "sk-winrate"},

		// ── Mysekai ──────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(mysekai-resource|mysekai资源|烤森资源|msr|msmap|msa)\s*(.*)`), ModuleMysekai, "mysekai-resource"},
		{regexp.MustCompile(`(?i)^/(mysekai-fixture-list|mysekai家具列表|烤森家具列表|msf)\s*(.*)`), ModuleMysekai, "mysekai-fixture-list"},
		{regexp.MustCompile(`(?i)^/(mysekai-fixture-detail|mysekai家具详情|烤森家具详情)\s*(.*)`), ModuleMysekai, "mysekai-fixture-detail"},
		{regexp.MustCompile(`(?i)^/(mysekai-door-upgrade|mysekai大门升级|烤森大门升级|msg|msgate)\s*(.*)`), ModuleMysekai, "mysekai-door-upgrade"},
		{regexp.MustCompile(`(?i)^/(mysekai-music-record|mysekai唱片|烤森唱片|msm|mss)\s*(.*)`), ModuleMysekai, "mysekai-music-record"},
		{regexp.MustCompile(`(?i)^/(mysekai-talk-list|mysekai对话列表|烤森对话列表)\s*(.*)`), ModuleMysekai, "mysekai-talk-list"},

		// ── Help ─────────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/(help|帮助)\s*(.*)`), ModuleHelp, "help"},

		// ── Profile ──────────────────────────────────────────────────────────────
		{regexp.MustCompile(`(?i)^/sk\s*(.*)`), ModuleProfile, "profile"},
		{regexp.MustCompile(`(?i)^/(个人中心|个人信息|名片|pjsk profile|profile)\s*(.*)`), ModuleProfile, "profile"},
	}
}

func NewGlobalCommandResolver(nicknames map[string]int) *GlobalCommandResolver {
	r := &GlobalCommandResolver{
		extractor: NewExtractor(nicknames),
		routes:    globalRoutes,
	}
	return r
}

// Resolve parses raw command text.
func (r *GlobalCommandResolver) Resolve(input string) (*ResolvedCommand, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return &ResolvedCommand{Module: ModuleHelp, IsHelp: true}, nil
	}

	res := &ResolvedCommand{}

	prefixRes := r.extractor.ExtractRegionPrefix(input)
	if prefixRes.Found && prefixRes.Value != "" {
		res.Region = prefixRes.Value
		input = prefixRes.Remaining
	}

	regRes := r.extractor.ExtractRegion(input)
	if regRes.Value != "" {
		res.Region = regRes.Value
	}
	input = regRes.Remaining

	verbRes := r.extractor.ExtractVerbose(input)
	res.IsVerbose = verbRes.Value
	input = verbRes.Remaining

	preRes := r.extractor.ExtractPreview(input)
	res.IsPreview = preRes.Value
	input = preRes.Remaining

	helpRes := r.extractor.ExtractHelp(input)
	res.IsHelp = helpRes.Value
	input = helpRes.Remaining

	if res.IsHelp {
		res.Module = ModuleHelp
		return res, nil
	}

	if res.Region == "" {
		res.Region = "jp"
	}

	for _, rt := range r.routes {
		if matches := rt.pattern.FindStringSubmatch(input); len(matches) > 1 {
			res.Module = rt.module
			res.Mode = rt.mode
			if res.Module == ModuleHelp {
				res.IsHelp = true
			}
			if len(matches) > 2 {
				res.Query = strings.TrimSpace(matches[2])
			}
			return res, nil
		}
	}

	return nil, fmt.Errorf("无法识别指令格式，请发送 /help 查看说明")
}
