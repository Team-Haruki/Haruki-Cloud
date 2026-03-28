//go:build !cgo || !pjsk_deck_cgo

package deck_cgo

import "fmt"

type CardConfig struct {
	Disable     bool `json:"disable"`
	LevelMax    bool `json:"level_max"`
	EpisodeRead bool `json:"episode_read"`
	MasterMax   bool `json:"master_max"`
	SkillMax    bool `json:"skill_max"`
	Canvas      bool `json:"canvas"`
}

type SingleCardConfig struct {
	CardID      int  `json:"card_id"`
	Disable     bool `json:"disable"`
	LevelMax    bool `json:"level_max"`
	EpisodeRead bool `json:"episode_read"`
	MasterMax   bool `json:"master_max"`
	SkillMax    bool `json:"skill_max"`
	Canvas      bool `json:"canvas"`
}

type Options struct {
	Region                       string
	LiveType                     string
	MusicID                      int
	MusicDiff                    string
	EventID                      *int
	EventAttr                    string
	EventUnit                    string
	EventType                    string
	WorldBloomCharacterID        *int
	WorldBloomEventTurn          *int
	ChallengeLiveCharacterID     *int
	Algorithm                    string
	Target                       string
	Limit                        int
	TimeoutMs                    *int
	Rarity1Config                *CardConfig
	Rarity2Config                *CardConfig
	Rarity3Config                *CardConfig
	Rarity4Config                *CardConfig
	RarityBDConfig               *CardConfig
	SingleCardCfgs               []SingleCardConfig
	FixedCards                   []int
	FixedCharacters              []int
	TargetBonusList              []int
	MultiLiveTeammatePower       *int
	MultiLiveTeammateScoreUp     *int
	MultiLiveScoreUpLowerBound   *float64
	SkillOrderChooseStrategy     string
	SkillReferenceChooseStrategy string
	KeepAfterTrainingState       bool
	BestSkillAsLeader            *bool
}

type ResultCard struct {
	CardID            int
	TotalPower        int
	BasePower         int
	EventBonusRate    float64
	MasterRank        int
	Level             int
	SkillLevel        int
	SkillScoreUp      int
	SkillLifeRecovery int
	Episode1Read      bool
	Episode2Read      bool
	AfterTraining     bool
	DefaultImage      string
	HasCanvasBonus    bool
}

type ResultDeck struct {
	Score                int
	LiveScore            int
	MysekaiEventPoint    int
	TotalPower           int
	BasePower            int
	AreaItemBonusPower   int
	CharacterBonusPower  int
	HonorBonusPower      int
	FixtureBonusPower    int
	GateBonusPower       int
	EventBonusRate       float64
	SupportDeckBonusRate float64
	MultiLiveScoreUp     float64
	ChallengeScoreDelta  int
	Cards                []ResultCard
}

type Result struct {
	Decks []ResultDeck `json:"decks"`
}

type Engine struct{}

func NewEngine() (*Engine, error) {
	return nil, fmt.Errorf("deck_cgo is not available in this build")
}

func SetStaticDataDir(string) error {
	return fmt.Errorf("deck_cgo is not available in this build")
}

func (e *Engine) Close() {}

func (e *Engine) UpdateMasterdata(string, string) error {
	return fmt.Errorf("deck_cgo is not available in this build")
}

func (e *Engine) UpdateMasterdataFromStrings(map[string][]byte, string) error {
	return fmt.Errorf("deck_cgo is not available in this build")
}

func (e *Engine) UpdateMusicmetas(string, string) error {
	return fmt.Errorf("deck_cgo is not available in this build")
}

func (e *Engine) UpdateMusicmetasFromBytes([]byte, string) error {
	return fmt.Errorf("deck_cgo is not available in this build")
}

func (e *Engine) Recommend(Options, []byte) (*Result, error) {
	return nil, fmt.Errorf("deck_cgo is not available in this build")
}

type Pool struct{}

func NewPool(string, map[string][]byte, string, []byte, string, int) (*Pool, error) {
	return nil, fmt.Errorf("deck_cgo is not available in this build")
}

func (p *Pool) Acquire() *Engine { return nil }

func (p *Pool) Release(*Engine) {}

func (p *Pool) Do(func(*Engine) error) error {
	return fmt.Errorf("deck_cgo is not available in this build")
}

func (p *Pool) Close() {}

func (p *Pool) Size() int { return 0 }
