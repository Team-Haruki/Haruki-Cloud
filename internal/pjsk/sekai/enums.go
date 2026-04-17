package sekai

type MusicDifficultyType string

const (
	MusicDifficultyEasy   MusicDifficultyType = "easy"
	MusicDifficultyNormal MusicDifficultyType = "normal"
	MusicDifficultyHard   MusicDifficultyType = "hard"
	MusicDifficultyExpert MusicDifficultyType = "expert"
	MusicDifficultyMaster MusicDifficultyType = "master"
	MusicDifficultyAppend MusicDifficultyType = "append"
)

// AllMusicDifficulties is the canonical ordered list of all playable difficulties.
var AllMusicDifficulties = []MusicDifficultyType{
	MusicDifficultyEasy,
	MusicDifficultyNormal,
	MusicDifficultyHard,
	MusicDifficultyExpert,
	MusicDifficultyMaster,
	MusicDifficultyAppend,
}

type ServerRegion string

const (
	ServerRegionJP ServerRegion = "jp"
	ServerRegionEN ServerRegion = "en"
	ServerRegionTW ServerRegion = "tw"
	ServerRegionKR ServerRegion = "kr"
	ServerRegionCN ServerRegion = "cn"
)

type EventType string

const (
	EventTypeMarathon         EventType = "marathon"          // 马拉松 (普活)
	EventTypeCheerfulCarnival EventType = "cheerful_carnival" // 欢乐嘉年华 (5v5)
	EventTypeWorldBloom       EventType = "world_bloom"       // 世界连接 (World Link)
)

type WorldBloomType string

const (
	WorldBloomTypeGameCharacter WorldBloomType = "game_character"
	WorldBloomTypeFinale        WorldBloomType = "finale"
)

type EventStatus string

const (
	EventStatusNotStarted  EventStatus = "not_started" // 还没开始
	EventStatusOngoing     EventStatus = "ongoing"     // 正在进行
	EventStatusAggregating EventStatus = "aggregating" // 集算中
	EventStatusEnded       EventStatus = "ended"       // 已结束
)

type EventSpeedType string

const (
	EventSpeedTypeHourly    EventSpeedType = "hourly"
	EventSpeedTypeSemiDaily EventSpeedType = "semi_daily"
	EventSpeedTypeDaily     EventSpeedType = "daily"
)

type Unit string

const (
	UnitNone                Unit = "none"
	UnitLeoneed             Unit = "light_sound"
	UnitMoreMoreJump        Unit = "idol"
	UnitVividBadSquad       Unit = "street"
	UnitWonderlandsShowtime Unit = "theme_park"
	UnitNightcord           Unit = "school_refusal"
)

var EventRankingLinesNormal = []int{
	10, 20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 1500, 2000, 2500, 3000, 4000, 5000,
	10000, 20000, 30000, 40000, 50000,
	100000, 200000, 300000,
}

// ToolboxDataType identifies the kind of private snapshot served by the Toolbox API.
// suite   → user game-data snapshot (replaces local user.json; stored in SnapshotStore)
// mysekai → MySekai world snapshot  (replaces local mysekai.json; stored in MySekaiStore)
type ToolboxDataType string

const (
	ToolboxDataTypeSuite   ToolboxDataType = "suite"
	ToolboxDataTypeMySekai ToolboxDataType = "mysekai"
)

var EventRankingLinesWorldBloom = []int{
	10, 20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 2000, 3000, 4000, 5000, 7000,
	10000, 20000, 30000, 40000, 50000, 70000,
	100000,
}
