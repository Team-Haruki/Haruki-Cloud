package sekai

import (
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"regexp"
)

// mysekaiDeckCombinedParams wraps deck params with user query params for mysekai deck.
type mysekaiDeckCombinedParams struct {
	Deck  deckAutoQueryParams `json:"deck"`
	Query UserQueryParams     `json:"query"`
}

type deckAutoQueryParams struct {
	EventID                      *int                               `json:"event_id,omitempty"`
	TargetBonuses                []int                              `json:"target_bonuses,omitempty"`
	Args                         string                             `json:"args,omitempty"`
	Algorithm                    string                             `json:"algorithm,omitempty"`
	LiveType                     string                             `json:"live_type,omitempty"`
	Target                       string                             `json:"target,omitempty"`
	MusicQuery                   string                             `json:"music_query,omitempty"`
	MusicID                      *int                               `json:"music_id,omitempty"`
	MusicDiff                    string                             `json:"music_diff,omitempty"`
	EventAttr                    string                             `json:"event_attr,omitempty"`
	EventUnit                    string                             `json:"event_unit,omitempty"`
	WorldBloomCharacterID        *int                               `json:"world_bloom_character_id,omitempty"`
	WorldBloomCharacterQuery     string                             `json:"world_bloom_character_query,omitempty"`
	WorldBloomEventTurn          *int                               `json:"world_bloom_event_turn,omitempty"`
	ChallengeLiveCharacterID     *int                               `json:"challenge_live_character_id,omitempty"`
	ChallengeLiveCharacterQuery  string                             `json:"challenge_live_character_query,omitempty"`
	FixedCards                   []int                              `json:"fixed_cards,omitempty"`
	FixedCharacters              []int                              `json:"fixed_characters,omitempty"`
	FixedCharacterQueries        []string                           `json:"fixed_character_queries,omitempty"`
	Rarity1Config                *renderdeck.CardConfigPatch        `json:"rarity_1_config,omitempty"`
	Rarity2Config                *renderdeck.CardConfigPatch        `json:"rarity_2_config,omitempty"`
	Rarity3Config                *renderdeck.CardConfigPatch        `json:"rarity_3_config,omitempty"`
	Rarity4Config                *renderdeck.CardConfigPatch        `json:"rarity_4_config,omitempty"`
	RarityBirthdayConfig         *renderdeck.CardConfigPatch        `json:"rarity_birthday_config,omitempty"`
	SingleCardConfigs            []renderdeck.SingleCardConfigPatch `json:"single_card_configs,omitempty"`
	MultiLiveTeammatePower       *int                               `json:"multi_live_teammate_power,omitempty"`
	MultiLiveTeammateScoreUp     *int                               `json:"multi_live_teammate_score_up,omitempty"`
	MultiLiveScoreUpLowerBound   *float64                           `json:"multi_live_score_up_lower_bound,omitempty"`
	SkillOrderChooseStrategy     string                             `json:"skill_order_choose_strategy,omitempty"`
	SkillReferenceChooseStrategy string                             `json:"skill_reference_choose_strategy,omitempty"`
	KeepAfterTrainingState       bool                               `json:"keep_after_training_state,omitempty"`
}

type deckCommonConfig struct {
	allowLiveType      bool
	allowMultiLive     bool
	allowTarget        bool
	allowAlgorithm     bool
	allowRandom        bool
	allowFixed         bool
	allowCardConfig    bool
	defaultNoChange    bool
	defaultArgsTrigger string
}

type deckAliasRule struct {
	re   *regexp.Regexp
	unit string
}

var (
	deckEventIDRegex = regexp.MustCompile(`(?i)(活动|event)\s*(\d+)`)
	deckWlTurnRegex  = regexp.MustCompile(`(?i)\bwl([12])\b`)
)

var deckPowerTargetKeywords = []string{"综合力", "综合", "总合力", "总和", "power"}
var deckSkillTargetKeywords = []string{"倍率", "实效", "skill", "时效"}
var deckBoostKeywords = []string{"boost", "火", "体力", "体"}
var deckSkillMaxKeywords = []string{"满技能", "满技", "skillmax", "技能满级", "slv4"}
var deckMasterMaxKeywords = []string{"满突破", "满破", "rankmax", "mastermax", "5破", "五破"}
var deckEpisodeReadKeywords = []string{"剧情已读", "满剧情", "前后篇已读", "前后篇", "已读"}
var deckCanvasKeywords = []string{"满画布", "全画布", "画布", "满画板", "全画板", "画板"}
var deckDisableKeywords = []string{"禁用", "disable"}
var deckKeepAfterTrainingKeywords = []string{"bfes不变", "bf不变"}
var deckTeammatePowerKeywords = []string{"队友综合力", "队友总合力", "队友综合", "队友总和"}
var deckTeammateScoreUpKeywords = []string{"队友实效", "队友技能", "队友时效"}
var deckSkillOrderKeywords = []string{"技能顺序", "技能排列"}
var deckSkillReferenceKeywords = []string{"技能抽取", "技能吸取"}
var deckMaxKeywords = []string{"最高", "最大", "最优", "最强", "最佳"}
var deckMinKeywords = []string{"最低", "最小", "最差", "最弱", "最烂"}
var deckAverageKeywords = []string{"平均", "均值", "期望"}
var deckInlineDifficultySuffixes = []string{
	"append", "expert", "master", "normal", "easy", "hard",
	"粉谱", "红谱", "紫谱", "蓝谱", "绿谱", "黄谱",
	"apd", "app", "exp", "mas", "nm", "ez", "hd", "ex", "ma",
}

var deckRarityPrefixes = []struct {
	prefix string
	apply  func(*deckAutoQueryParams, renderdeck.CardConfigPatch)
}{
	{"生日", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.RarityBirthdayConfig, patch)
	}},
	{"一星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity1Config, patch)
	}},
	{"1星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity1Config, patch)
	}},
	{"二星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity2Config, patch)
	}},
	{"2星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity2Config, patch)
	}},
	{"三星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity3Config, patch)
	}},
	{"3星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity3Config, patch)
	}},
	{"四星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity4Config, patch)
	}},
	{"4星", func(params *deckAutoQueryParams, patch renderdeck.CardConfigPatch) {
		mergeDeckCardConfigPatch(&params.Rarity4Config, patch)
	}},
}

var deckUnitAliasRules = newDeckUnitAliasRules()
