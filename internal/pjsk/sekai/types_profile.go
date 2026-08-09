package sekai

// GetAnotherProfileResponse is the top-level response returned by GetUserProfile
// (maps to GetAnotherProfileResponse / GetUserProfileAPI in the game client).
type GetAnotherProfileResponse struct {
	User                          AnotherUser                            `json:"user"`
	UserProfile                   UserProfile                            `json:"userProfile"`
	UserDeck                      UserDeck                               `json:"userDeck"`
	UserCards                     []AnotherUserCard                      `json:"userCards"`
	UserCharacters                []AnotherUserCharacter                 `json:"userCharacters"`
	UserChallengeLiveSoloResult   UserChallengeLiveSoloResult            `json:"userChallengeLiveSoloResult"`
	UserChallengeLiveSoloStages   []AnotherUserChallengeLiveSoloStage    `json:"userChallengeLiveSoloStages"`
	UserMusicDifficultyClearCount []AnotherUserMusicDifficultyClearCount `json:"userMusicDifficultyClearCount"`
	UserCustomProfileCards        []UserCustomProfileCard                `json:"userCustomProfileCards"`
	UserProfileHonors             []UserProfileHonor                     `json:"userProfileHonors"`
	UserHonors                    []UserHonor                            `json:"userHonors"`
	UserBondsHonors               []UserBondsHonor                       `json:"userBondsHonors"`
	UserStoryFavorites            []UserStoryFavorite                    `json:"userStoryFavorites"`
	UserConfig                    AnotherUserConfig                      `json:"userConfig"`
	UserMultiLiveTopScoreCount    AnotherUserMultiLiveTopScoreCount      `json:"userMultiLiveTopScoreCount"`
	TotalPower                    AnotherTotalPower                      `json:"totalPower"`
	UserHonorMissions             []UserHonorMission                     `json:"userHonorMissions"`
	IsMysekaiOwnerAcceptVisit     bool                                   `json:"isMysekaiOwnerAcceptVisit"`
}

// AnotherUser contains the basic public info of a player as seen by others.
type AnotherUser struct {
	UserID int64  `json:"userId"`
	Name   string `json:"name"`
	Rank   int    `json:"rank"`
}

// AnotherUserCard is a card owned by the player (public-facing subset of fields).
type AnotherUserCard struct {
	CardID                int    `json:"cardId"`
	Level                 int    `json:"level"`
	MasterRank            int    `json:"masterRank"`
	SpecialTrainingStatus string `json:"specialTrainingStatus"`
	DefaultImage          string `json:"defaultImage"`
}

// AnotherUserCharacter contains another player's bond rank for one character.
type AnotherUserCharacter struct {
	CharacterID   int `json:"characterId"`
	CharacterRank int `json:"characterRank"`
}

// AnotherUserChallengeLiveSoloStage contains another player's Challenge Live stage rank.
type AnotherUserChallengeLiveSoloStage struct {
	CharacterID int `json:"characterId"`
	Rank        int `json:"rank"`
}

// AnotherUserMusicDifficultyClearCount holds a player's aggregate clear counts per difficulty.
type AnotherUserMusicDifficultyClearCount struct {
	MusicDifficultyType MusicDifficultyType `json:"musicDifficultyType"`
	LiveClear           int                 `json:"liveClear"`
	FullCombo           int                 `json:"fullCombo"`
	AllPerfect          int                 `json:"allPerfect"`
}

// AnotherUserConfig holds the public-facing account settings of another player.
type AnotherUserConfig struct {
	FriendRequestScope string `json:"friendRequestScope"`
}

// AnotherUserMultiLiveTopScoreCount contains a player's multi-live top-score counts.
type AnotherUserMultiLiveTopScoreCount struct {
	MVP       int `json:"mvp"`
	SuperStar int `json:"superStar"`
}

// AnotherTotalPower holds the breakdown of a player's total team power.
type AnotherTotalPower struct {
	TotalPower                                  int `json:"totalPower"`
	BasicCardTotalPower                         int `json:"basicCardTotalPower"`
	AreaItemBonus                               int `json:"areaItemBonus"`
	CharacterRankBonus                          int `json:"characterRankBonus"`
	HonorBonus                                  int `json:"honorBonus"`
	MysekaiFixtureGameCharacterPerformanceBonus int `json:"mysekaiFixtureGameCharacterPerformanceBonus"`
	MysekaiGateLevelBonus                       int `json:"mysekaiGateLevelBonus"`
}

// UserHonorMission tracks progress toward a specific honor mission type.
type UserHonorMission struct {
	UserID             int64  `json:"userId"`
	HonorMissionType   string `json:"honorMissionType"`
	Progress           int    `json:"progress"`
	AchievedMissionIDs []int  `json:"achievedMissionIds"`
}

// UserProfile contains public-facing profile settings.
type UserProfile struct {
	Word             string `json:"word"`
	TwitterID        string `json:"twitterId"`
	ProfileImageType string `json:"profileImageType"`
	ProfileImageID   int    `json:"profileImageId"`
}

// UserDeck describes one of the player's decks.
type UserDeck struct {
	UserID    int64  `json:"userId"`
	DeckID    int    `json:"deckId"`
	Name      string `json:"name"`
	Leader    int    `json:"leader"`
	SubLeader int    `json:"subLeader"`
	Member1   int    `json:"member1"`
	Member2   int    `json:"member2"`
	Member3   int    `json:"member3"`
	Member4   int    `json:"member4"`
	Member5   int    `json:"member5"`
}

// UserChallengeLiveSoloResult holds the player's high-score for a character's Challenge Live.
type UserChallengeLiveSoloResult struct {
	CharacterID int `json:"characterId"`
	HighScore   int `json:"highScore"`
}

// UserHonor records an honor (title) owned by the player.
type UserHonor struct {
	HonorID    int   `json:"honorId"`
	Level      int   `json:"level"`
	ObtainedAt int64 `json:"obtainedAt"`
}

// UserStoryFavorite is a story the player has bookmarked on their profile.
type UserStoryFavorite struct {
	ShareNo   int    `json:"shareNo"`
	StoryType string `json:"storyType"`
	StoryID   int    `json:"storyId"`
	Comment   string `json:"comment"`
	IsSpoiler bool   `json:"isSpoiler"`
}

// UserCustomProfileCard is one custom-profile card slot.
type UserCustomProfileCard struct {
	CustomProfileID     int             `json:"customProfileId"`
	CustomProfileCardID int             `json:"customProfileCardId"`
	ThumbnailPath       string          `json:"thumbnailPath"`
	CustomProfileCard   ProfileCardData `json:"customProfileCard"`
	Seq                 int             `json:"seq"`
}

// ProfileCardData is the layout data for a custom profile card.
type ProfileCardData struct {
	Version            int              `json:"version"`
	Generals           []GeneralData    `json:"generals"`
	GeneralBackgrounds []ImageData      `json:"generalBackgrounds"`
	StoryBackgrounds   []ImageData      `json:"storyBackgrounds"`
	StandMembers       []ImageData      `json:"standMembers"`
	CardMembers        []CardData       `json:"cardMembers"`
	Honors             []HonorData      `json:"honors"`
	BondsHonors        []BondsHonorData `json:"bondsHonors"`
	Texts              []TextData       `json:"texts"`
	Collections        []CollectionData `json:"collections"`
	Others             []ImageData      `json:"others"`
	Shapes             []ShapeData      `json:"shapes"`
	Stamps             []ImageData      `json:"stamps"`
	CharacterIcons     []ImageData      `json:"characterIcons"`
	Materials          []ImageData      `json:"materials"`
	UserInterfaceIcons []ImageData      `json:"userInterfaceIcons"`
}

// GeneralData is a general info element on a custom profile card.
type GeneralData struct {
	ObjectData           ObjectData `json:"objectData"`
	PlayerInfoResourceID int        `json:"type"`
}

// ImageData is any image element on a custom profile card.
type ImageData struct {
	ObjectData ObjectData `json:"objectData"`
	ID         int        `json:"id"`
}

// CardData is a card member element on a custom profile card.
type CardData struct {
	ObjectData              ObjectData `json:"objectData"`
	ID                      int        `json:"id"`
	Type                    int        `json:"type"`
	ShowMasterRank          bool       `json:"showMasterRank"`
	UseAfterSpecialTraining bool       `json:"useAfterSpecialTraining"`
}

// HonorData is an honor element on a custom profile card.
type HonorData struct {
	ObjectData ObjectData `json:"objectData"`
	ID         int        `json:"id"`
	Rarity     int        `json:"rarity"`
	FullSize   bool       `json:"fullSize"`
}

// BondsHonorData is a bonds-honor element on a custom profile card.
type BondsHonorData struct {
	ObjectData           ObjectData `json:"objectData"`
	ID                   int        `json:"id"`
	FullSize             bool       `json:"fullSize"`
	WordID               int        `json:"wordId"`
	Inverse              bool       `json:"inverse"`
	UseUnitVirtualSinger bool       `json:"useUnitVirtualSinger"`
}

// TextData is a text element on a custom profile card.
type TextData struct {
	ObjectData     ObjectData `json:"objectData"`
	Text           string     `json:"text"`
	FontID         int        `json:"fontId"`
	Type           int        `json:"type"`
	ColorID        int        `json:"colorId"`
	Size           float32    `json:"size"`
	OutlineColorID int        `json:"outlineColorId"`
	OutlineSize    float32    `json:"outlineSize"`
	LineSpacing    float32    `json:"lineSpacing"`
}

// CollectionData is a collection element on a custom profile card.
type CollectionData struct {
	ObjectData ObjectData `json:"objectData"`
	ID         int        `json:"id"`
	TargetID   int        `json:"targetId"`
}

// ShapeData is a shape element on a custom profile card.
type ShapeData struct {
	ObjectData     ObjectData `json:"objectData"`
	ID             int        `json:"id"`
	ColorID        int        `json:"colorId"`
	OutlineColorID int        `json:"outlineColorId"`
	Alpha          float32    `json:"alpha"`
	OutlineAlpha   float32    `json:"outlineAlpha"`
	OutlineSize    float32    `json:"outlineSize"`
}

// ObjectData contains the transform of a custom profile card element.
type ObjectData struct {
	Position Float3 `json:"position"`
	Scale    Float3 `json:"scale"`
	Rotation Float4 `json:"rotation"`
	Layer    int    `json:"layer"`
	IsLock   bool   `json:"lock"`
	Visible  bool   `json:"visible"`
}

// Float3 is a 3-component float vector (maps to Unity's Vector3).
type Float3 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

// Float4 is a 4-component float vector (maps to Unity's Quaternion).
type Float4 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
	W float32 `json:"w"`
}

// UserProfileHonor is one of the three honor slots displayed on a player's profile.
type UserProfileHonor struct {
	Seq                int    `json:"seq"`
	ProfileHonorType   string `json:"profileHonorType"`
	HonorID            int    `json:"honorId"`
	BondsHonorViewType string `json:"bondsHonorViewType"`
	BondsHonorWordID   int    `json:"bondsHonorWordId"`
	HonorLevel         int    `json:"honorLevel"`
}

// UserBondsHonor is a bonds honor (unit-pair badge) owned by the player.
type UserBondsHonor struct {
	BondsHonorID int    `json:"bondsHonorId"`
	Level        int    `json:"level"`
	ObtainedAt   int64  `json:"obtainedAt"`
	Description  string `json:"description"`
}
