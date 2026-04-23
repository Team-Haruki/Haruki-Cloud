package snapshot

import "encoding/json"

type RawUserData struct {
	Now                                               int64                             `json:"now"`
	UserGamedata                                      RawUserGamedata                   `json:"userGamedata"`
	UserProfile                                       RawUserProfile                    `json:"userProfile"`
	UserDecks                                         []RawUserDeck                     `json:"userDecks,omitempty"`
	UserCards                                         []RawUserCard                     `json:"userCards,omitempty"`
	UserBonds                                         []RawUserBond                     `json:"userBonds,omitempty"`
	UserMusicStats                                    []RawMusicResult                  `json:"userMusicResults,omitempty"`
	CompactUserMusicResults                           json.RawMessage                   `json:"compactUserMusicResults,omitempty"`
	UserMusics                                        []RawUserMusic                    `json:"userMusics,omitempty"`
	UserChallengeLiveSoloDecks                        []RawChallengeLiveDeck            `json:"userChallengeLiveSoloDecks,omitempty"`
	UserChallengeLiveSoloResults                      []RawChallengeLiveResult          `json:"userChallengeLiveSoloResults,omitempty"`
	UserChallengeLiveSoloStages                       []RawChallengeLiveStage           `json:"userChallengeLiveSoloStages,omitempty"`
	UserChallengeLiveSoloHighScoreRewards             []RawChallengeLiveReward          `json:"userChallengeLiveSoloHighScoreRewards,omitempty"`
	UserMultiLiveTopScoreCount                        RawUserMultiLiveTopScoreCount     `json:"userMultiLiveTopScoreCount"`
	UserCharacters                                    []RawUserCharacter                `json:"userCharacters,omitempty"`
	UserCharacterMissionV2s                           []RawUserCharacterMissionV2       `json:"userCharacterMissionV2s,omitempty"`
	UserCharacterLiveUsageCounts                      []RawUserCharacterLiveUsageCount  `json:"userCharacterLiveUsageCounts,omitempty"`
	UserCharacterMissionV2Statuses                    []RawUserCharacterMissionV2Status `json:"userCharacterMissionV2Statuses,omitempty"`
	CompactUserCharacterMissionV2Statuses             json.RawMessage                   `json:"compactUserCharacterMissionV2Statuses,omitempty"`
	UserAreas                                         []RawUserArea                     `json:"userAreas,omitempty"`
	UserMaterials                                     []RawUserMaterial                 `json:"userMaterials,omitempty"`
	UserMysekaiCanvases                               []RawUserMysekaiCanvas            `json:"userMysekaiCanvases,omitempty"`
	UserMysekaiGates                                  []RawUserMysekaiGate              `json:"userMysekaiGates,omitempty"`
	UserMysekaiFixtureGameCharacterPerformanceBonuses []RawUserFixtureBonus             `json:"userMysekaiFixtureGameCharacterPerformanceBonuses,omitempty"`
	UserMusicClear                                    []RawMusicClear                   `json:"userMusicDifficultyClearCounts,omitempty"`
	UserHonors                                        []RawUserHonor                    `json:"userHonors,omitempty"`
	UserProfileHonors                                 []RawUserProfileHonor             `json:"userProfileHonors,omitempty"`
	UserFrames                                        []RawUserFrame                    `json:"userPlayerFrames,omitempty"`
	UserEvents                                        []RawUserEvent                    `json:"userEvents,omitempty"`
	UserEventResults                                  []RawUserEventResult              `json:"userEventResults,omitempty"`
	UserWorldBlooms                                   []RawUserWorldBloom               `json:"userWorldBlooms,omitempty"`
}

type RawUserGamedata struct {
	UserID int64  `json:"userId"`
	Name   string `json:"name"`
	Deck   int    `json:"deck"`
	Rank   int    `json:"rank"`
	Coin   int    `json:"coin"`
}

type RawUserProfile struct {
	ProfileImageType string `json:"profileImageType"`
	ProfileImageID   int    `json:"profileImageId"`
	Word             string `json:"word"`
	TwitterID        string `json:"twitterId"`
}

type RawUserDeck struct {
	DeckID    int `json:"deckId"`
	Leader    int `json:"leader"`
	SubLeader int `json:"subLeader"`
	Member1   int `json:"member1"`
	Member2   int `json:"member2"`
	Member3   int `json:"member3"`
	Member4   int `json:"member4"`
	Member5   int `json:"member5"`
}

type RawUserBond struct {
	BondsGroupID int `json:"bondsGroupId"`
	Rank         int `json:"rank"`
	Exp          int `json:"exp"`
}

type RawUserCardEpisode struct {
	CardEpisodeID  int    `json:"cardEpisodeId"`
	ScenarioStatus string `json:"scenarioStatus"`
}

type RawUserCard struct {
	CardID                int                  `json:"cardId"`
	Level                 int                  `json:"level"`
	SkillLevel            int                  `json:"skillLevel,omitempty"`
	MasterRank            int                  `json:"masterRank"`
	SpecialTrainingStatus string               `json:"specialTrainingStatus"`
	DefaultImage          string               `json:"defaultImage"`
	Episodes              []RawUserCardEpisode `json:"episodes,omitempty"`
}

type RawMusicResult struct {
	MusicID             int    `json:"musicId"`
	MusicDifficulty     string `json:"musicDifficulty"`
	MusicDifficultyType string `json:"musicDifficultyType"`
	PlayResult          string `json:"playResult"`
	FullComboFlg        bool   `json:"fullComboFlg"`
	FullPerfectFlg      bool   `json:"fullPerfectFlg"`
}

type RawUserMusic struct {
	MusicID                     int                            `json:"musicId"`
	UserMusicDifficultyStatuses []RawUserMusicDifficultyStatus `json:"userMusicDifficultyStatuses,omitempty"`
}

type RawUserMusicDifficultyStatus struct {
	MusicDifficulty     string           `json:"musicDifficulty"`
	MusicDifficultyType string           `json:"musicDifficultyType"`
	UserMusicResults    []RawMusicResult `json:"userMusicResults,omitempty"`
}

type RawChallengeLiveResult struct {
	CharacterID int `json:"characterId"`
	HighScore   int `json:"highScore"`
}

type RawChallengeLiveDeck struct {
	CharacterID int `json:"characterId"`
	Leader      int `json:"leader"`
	Support1    int `json:"support1"`
	Support2    int `json:"support2"`
	Support3    int `json:"support3"`
	Support4    int `json:"support4"`
}

type RawChallengeLiveStage struct {
	CharacterID int `json:"characterId"`
	Rank        int `json:"rank"`
}

type RawUserMultiLiveTopScoreCount struct {
	MVP       int `json:"mvp"`
	SuperStar int `json:"superStar"`
}

type RawChallengeLiveReward struct {
	ChallengeLiveHighScoreRewardID int `json:"challengeLiveHighScoreRewardId"`
	CharacterID                    int `json:"characterId"`
}

type RawUserCharacter struct {
	CharacterID   int `json:"characterId"`
	CharacterRank int `json:"characterRank"`
}

type RawUserCharacterMissionV2 struct {
	CharacterMissionType string `json:"characterMissionType"`
	CharacterID          int    `json:"characterId"`
	Progress             int    `json:"progress"`
}

type RawUserCharacterLiveUsageCount struct {
	CharacterID            int    `json:"characterId"`
	CharacterLiveUsageType string `json:"characterLiveUsageType"`
	UsageCount             int    `json:"usageCount"`
}

type RawUserCharacterMissionV2Status struct {
	ParameterGroupID int `json:"parameterGroupId"`
	Seq              int `json:"seq"`
	CharacterID      int `json:"characterId"`
}

type RawUserArea struct {
	AreaItems []RawUserAreaItem `json:"areaItems,omitempty"`
}

type RawUserAreaItem struct {
	AreaItemID int `json:"areaItemId"`
	Level      int `json:"level"`
}

type RawUserMaterial struct {
	MaterialID int `json:"materialId"`
	Quantity   int `json:"quantity"`
}

type RawUserMysekaiCanvas struct {
	CardID   int `json:"cardId"`
	Quantity int `json:"quantity"`
}

type RawUserMysekaiGate struct {
	MysekaiGateID    int `json:"mysekaiGateId"`
	MysekaiGateLevel int `json:"mysekaiGateLevel"`
}

type RawUserFixtureBonus struct {
	GameCharacterID int     `json:"gameCharacterId"`
	TotalBonusRate  float64 `json:"totalBonusRate"`
}

type RawMusicClear struct {
	MusicDifficultyType string `json:"musicDifficultyType"`
	LiveClear           int    `json:"liveClear"`
	FullCombo           int    `json:"fullCombo"`
	AllPerfect          int    `json:"allPerfect"`
}

// RawUserEvent supports both the current suite payload and the older/manual
// editing shape used for event-record imports:
// {"eventId":156,"eventPoint":10050827,"rank":671,"rankingRewardReceivedAt":1739066808312}
type RawUserEvent struct {
	EventID                 int   `json:"eventId"`
	EventPoint              int   `json:"eventPoint"`
	Rank                    int   `json:"rank,omitempty"`
	RankingRewardReceivedAt int64 `json:"rankingRewardReceivedAt,omitempty"`
}

type RawUserEventResult struct {
	EventID int `json:"eventId"`
	Rank    int `json:"rank"`
}

// RawUserWorldBloom matches the manual World Link single-board import shape:
// {"eventId":170,"gameCharacterId":20,"worldBloomChapterPoint":68271005,"rank":363}
type RawUserWorldBloom struct {
	EventID                int `json:"eventId"`
	GameCharacterID        int `json:"gameCharacterId"`
	WorldBloomChapterPoint int `json:"worldBloomChapterPoint"`
	Rank                   int `json:"rank"`
}

type RawUserHonor struct {
	Seq           int    `json:"seq"`
	HonorID       int    `json:"honorId"`
	HonorLevel    int    `json:"level"`
	ProfilePlayer bool   `json:"profilePlayer"`
	HonorRarity   string `json:"honorRarity"`
}

type RawUserFrame struct {
	PlayerFrameID           int    `json:"playerFrameId"`
	PlayerFrameAttachStatus string `json:"playerFrameAttachStatus"`
}

type RawUserProfileHonor struct {
	Seq                int    `json:"seq"`
	ProfileHonorType   string `json:"profileHonorType"`
	HonorID            int    `json:"honorId"`
	HonorLevel         int    `json:"honorLevel"`
	HonorId2           int    `json:"honorId2"`
	BondsHonorViewType string `json:"bondsHonorViewType"`
	BondsHonorWordId   int    `json:"bondsHonorWordId"`
}

type ChallengeLiveData struct {
	Results []ChallengeLiveResult
	Stages  []ChallengeLiveStage
	Rewards []ChallengeLiveReward
}

type ChallengeLiveResult struct {
	CharacterID int
	HighScore   int
}

type ChallengeLiveStage struct {
	CharacterID int
	Rank        int
}

type ChallengeLiveReward struct {
	RewardID    int
	CharacterID int
}
