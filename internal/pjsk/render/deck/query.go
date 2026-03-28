package deck

import "haruki-cloud/utils/drawing"

type CardConfigPatch struct {
	Disable     bool `json:"disable,omitempty"`
	LevelMax    bool `json:"level_max,omitempty"`
	EpisodeRead bool `json:"episode_read,omitempty"`
	MasterMax   bool `json:"master_max,omitempty"`
	SkillMax    bool `json:"skill_max,omitempty"`
	Canvas      bool `json:"canvas,omitempty"`
}

type SingleCardConfigPatch struct {
	CardID      int  `json:"card_id"`
	Disable     bool `json:"disable,omitempty"`
	LevelMax    bool `json:"level_max,omitempty"`
	EpisodeRead bool `json:"episode_read,omitempty"`
	MasterMax   bool `json:"master_max,omitempty"`
	SkillMax    bool `json:"skill_max,omitempty"`
	Canvas      bool `json:"canvas,omitempty"`
}

type AutoQuery struct {
	Region                       string                              `json:"region"`
	RecommendType                string                              `json:"recommend_type"`
	EventID                      *int                                `json:"event_id,omitempty"`
	Limit                        int                                 `json:"limit,omitempty"`
	TargetBonuses                []int                               `json:"target_bonuses,omitempty"`
	Args                         string                              `json:"args,omitempty"`
	Algorithm                    string                              `json:"algorithm,omitempty"`
	LiveType                     string                              `json:"live_type,omitempty"`
	Target                       string                              `json:"target,omitempty"`
	MusicQuery                   string                              `json:"music_query,omitempty"`
	MusicID                      *int                                `json:"music_id,omitempty"`
	MusicDiff                    string                              `json:"music_diff,omitempty"`
	MusicTitle                   string                              `json:"music_title,omitempty"`
	MusicCoverPath               string                              `json:"music_cover_path,omitempty"`
	EventAttr                    string                              `json:"event_attr,omitempty"`
	EventUnit                    string                              `json:"event_unit,omitempty"`
	WorldBloomCharacterID        *int                                `json:"world_bloom_character_id,omitempty"`
	WorldBloomEventTurn          *int                                `json:"world_bloom_event_turn,omitempty"`
	ChallengeLiveCharacterID     *int                                `json:"challenge_live_character_id,omitempty"`
	FixedCards                   []int                               `json:"fixed_cards,omitempty"`
	FixedCharacters              []int                               `json:"fixed_characters,omitempty"`
	Rarity1Config                *CardConfigPatch                    `json:"rarity_1_config,omitempty"`
	Rarity2Config                *CardConfigPatch                    `json:"rarity_2_config,omitempty"`
	Rarity3Config                *CardConfigPatch                    `json:"rarity_3_config,omitempty"`
	Rarity4Config                *CardConfigPatch                    `json:"rarity_4_config,omitempty"`
	RarityBirthdayConfig         *CardConfigPatch                    `json:"rarity_birthday_config,omitempty"`
	SingleCardConfigs            []SingleCardConfigPatch             `json:"single_card_configs,omitempty"`
	MultiLiveTeammatePower       *int                                `json:"multi_live_teammate_power,omitempty"`
	MultiLiveTeammateScoreUp     *int                                `json:"multi_live_teammate_score_up,omitempty"`
	MultiLiveScoreUpLowerBound   *float64                            `json:"multi_live_score_up_lower_bound,omitempty"`
	SkillOrderChooseStrategy     string                              `json:"skill_order_choose_strategy,omitempty"`
	SkillReferenceChooseStrategy string                              `json:"skill_reference_choose_strategy,omitempty"`
	KeepAfterTrainingState       bool                                `json:"keep_after_training_state,omitempty"`
	Profile                      *drawing.DetailedProfileCardRequest `json:"-"`
}
