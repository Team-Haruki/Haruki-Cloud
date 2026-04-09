package deck

import (
	"fmt"
	"time"
)

type RecommendConfig struct {
	Enabled        bool
	ServiceBaseURL string
	MasterdataDir  string
	Timeout        time.Duration
	DefaultAlgs    []string
}

type MusicMetaSource interface {
	Get(region string) []byte
}

type DeckRecommender interface {
	Enabled() bool
	ExpandAlgorithms(option map[string]interface{}) []map[string]interface{}
	Recommend(req RecommendRequest) (*RecommendResult, error)
	Close()
}

type RecommendRequest struct {
	Region            string
	UserData          []byte
	UserDataFilePath  string
	MusicMeta         []byte
	MusicMetaFilePath string
	BatchOption       []map[string]interface{}
}

type RecommendResult struct {
	Decks     []RecommendDeck `json:"decks"`
	CostTimes map[string]float64
	WaitTimes map[string]float64
	DeckAlgs  []string
}

type RecommendDeck struct {
	Cards                []RecommendCard `json:"cards"`
	Score                int             `json:"score"`
	LiveScore            int             `json:"live_score"`
	MysekaiEventPoint    int             `json:"mysekai_event_point"`
	TotalPower           int             `json:"total_power"`
	EventBonusRate       float64         `json:"event_bonus_rate"`
	SupportDeckBonusRate float64         `json:"support_deck_bonus_rate"`
	MultiLiveScoreUp     float64         `json:"multi_live_score_up"`
	ChallengeScoreDelta  int             `json:"challenge_score_delta"`
	Algs                 []string        `json:"-"`
}

type RecommendCard struct {
	CardID          int     `json:"card_id"`
	Level           int     `json:"level"`
	MasterRank      int     `json:"master_rank"`
	DefaultImage    string  `json:"default_image"`
	SkillLevel      int     `json:"skill_level"`
	SkillRate       float64 `json:"skill_rate"`
	EventBonusRate  float64 `json:"event_bonus_rate"`
	IsBeforeStory   bool    `json:"is_before_story"`
	IsAfterStory    bool    `json:"is_after_story"`
	IsAfterTraining bool    `json:"is_after_training"`
	HasCanvasBonus  bool    `json:"has_canvas_bonus"`
}

type engineProvider interface {
	Get(region string) (DeckRecommender, error)
}

func deckHash(deck RecommendDeck) string {
	first := 0
	if len(deck.Cards) > 0 {
		first = deck.Cards[0].CardID
	}
	return fmt.Sprintf("%d_%d_%d", deck.Score, deck.TotalPower, first)
}
