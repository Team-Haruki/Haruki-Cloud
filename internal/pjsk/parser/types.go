package parser

import (
	"encoding/json"
	"regexp"
)

// ── Command types ───────────────────────────────────────────────────────────

// CommandType 定义指令类型
type CommandType int

const (
	CmdTypeUnknown CommandType = iota

	// 查询类 (SK)
	CmdTypeEventQuerySelf      // 查自己 (sk)
	CmdTypeEventQueryAt        // 查别人 (sk @123)
	CmdTypeEventQueryUID       // 查指定UID (sk 350...)
	CmdTypeEventQueryRank      // 查指定排名 (sk 100)
	CmdTypeEventQueryRankRange // 查排名范围 (sk 100-200)
	CmdTypeEventQueryMultiRank // 查多个排名 (sk 1 2 3)

	// 操作类 (Bind)
	CmdTypeBind   // 绑定 (bind 350...)
	CmdTypeUnbind // 解绑 (unbind)
)

// EventCommand 是提供给数据库开发者的接口结构体
type EventCommand struct {
	Type      CommandType
	TargetID  string // QQ ID (@12345) 或 Game UID (350...)
	Param1    int    // Rank Start, or Single Rank
	Param2    int    // Rank End
	MultiArgs []int  // Multiple Ranks
	Original  string // 原始指令
}

// ── Event query types ───────────────────────────────────────────────────────

// EventQueryType 定义活动查询类型
type EventQueryType int

const (
	QueryTypeEventUnknown EventQueryType = iota
	QueryTypeEventID                     // 指定 ID: event123
	QueryTypeEventSeq                    // 索引: -1, 10, next, prev
	QueryTypeEventBan                    // Ban主: mnr1
	QueryTypeEventFilter                 // 筛选: 25h, wl
)

// EventFilter 活动筛选条件
type EventFilter struct {
	Unit         string // 25h, vbs, etc.
	EventType    string // marathon, cheerful_carnival, world_bloom
	Year         int    // 2024
	CharacterID  int    // 筛选单个角色的活动
	CharacterIDs []int  // 筛选多个角色的活动
	BannerCharID int    // 箱活ban主
	Blend        bool   // 混活
	Attr         string // cute, cool, etc.
}

// EventQueryInfo 解析后的活动查询信息
type EventQueryInfo struct {
	Type       EventQueryType
	EventID    int
	Index      int         // 正数或负数索引
	Keyword    string      // "next", "prev", "current"
	BanCharID  int         // Ban主角色ID
	BanSeq     int         // Ban主第几次箱活
	Filter     EventFilter // 筛选条件
	Original   string
	IsDetailed bool // 是否需要详细信息
}

// EventParser 活动查询解析器
type EventParser struct {
	nicknames        map[string]int
	orderedNicknames []string
}

// ── Extractor types ─────────────────────────────────────────────────────────

type dictRule struct {
	re  *regexp.Regexp
	val string
}

// Extractor 通用特征提取器
type Extractor struct {
	nicknames map[string]int // 昵称 -> CharacterID
}

// ExtractResult 提取结果
type ExtractResult[T any] struct {
	Value     T
	Remaining string
	Found     bool
}

// ── Supply constants ────────────────────────────────────────────────────────

const (
	SupplyNormal   = "normal"
	SupplyLimited  = "limited"
	SupplyFes      = "festival"
	SupplyBirthday = "birthday"
)

// ── Global resolver types ───────────────────────────────────────────────────

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
	ModuleVLive
	ModuleArrest
	ModuleRegTime
	ModuleCheckData
	ModuleAlias
)

// ResolvedCommand stores normalized command parsing result.
type ResolvedCommand struct {
	Module            TargetModule
	Mode              string
	Query             string
	Region            string
	RegionExplicit    bool // true when the region was set by a prefix (/jp…) or -r flag
	Params            json.RawMessage
	IsHelp            bool
	IsVerbose         bool
	IsPreview         bool
	RequesterPlatform string
	RequesterUserID   string
	RequesterGroupID  string
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

// CommandParser 负责解析数据库相关指令
type CommandParser struct{}
