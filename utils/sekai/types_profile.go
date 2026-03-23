package sekai

// GetUserProfileResponse is the top-level response returned by GetUserProfile.
type GetUserProfileResponse struct {
	User                         UserData                      `json:"user"`
	UserProfile                  UserProfile                   `json:"userProfile"`
	UserDecks                    []UserDeck                    `json:"userDecks"`
	UserCards                    []UserCard                    `json:"userCards"`
	UserMusics                   []UserMusic                   `json:"userMusics"`
	UserMusicResults             []UserMusicResult             `json:"userMusicResults"`
	UserCharacters               []UserCharacter               `json:"userCharacters"`
	UserChallengeLiveSoloResults []UserChallengeLiveSoloResult `json:"userChallengeLiveSoloResults"`
	UserChallengeLiveSoloStages  []UserChallengeLiveSoloStage  `json:"userChallengeLiveSoloStages"`
	UserAreaItems                []UserAreaItem                `json:"userAreaItems"`
	UserHonors                   []UserHonor                   `json:"userHonors"`
	UserStoryFavorites           []UserStoryFavorite           `json:"userStoryFavorites"`
	UserCustomProfileCards       []UserCustomProfileCard       `json:"userCustomProfileCards"`
	UserProfileHonors            []UserProfileHonor            `json:"userProfileHonors"`
	UserBondsHonors              []UserBondsHonor              `json:"userBondsHonors"`
	UserConfig                   UserConfig                    `json:"userConfig"`
}

// UserData wraps the user's game data.
type UserData struct {
	UserGamedata UserGamedata `json:"userGamedata"`
}

// UserGamedata contains the player's core account state.
type UserGamedata struct {
	UserID          int64  `json:"userId"`
	Name            string `json:"name"`
	Deck            int    `json:"deck"`
	Rank            int    `json:"rank"`
	Exp             int    `json:"exp"`
	TotalExp        int    `json:"totalExp"`
	Coin            int    `json:"coin"`
	VirtualCoin     int    `json:"virtualCoin"`
	LastLoginAt     int64  `json:"lastLoginAt"`
	CustomProfileID *int   `json:"customProfileId"`
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

// UserCard is a card owned by the player.
type UserCard struct {
	UserID                int64             `json:"userId"`
	CardID                int               `json:"cardId"`
	Level                 int               `json:"level"`
	Exp                   int               `json:"exp"`
	TotalExp              int               `json:"totalExp"`
	SkillLevel            int               `json:"skillLevel"`
	SkillExp              int               `json:"skillExp"`
	TotalSkillExp         int               `json:"totalSkillExp"`
	MasterRank            int               `json:"masterRank"`
	SpecialTrainingStatus string            `json:"specialTrainingStatus"`
	DefaultImage          string            `json:"defaultImage"`
	DuplicateCount        int               `json:"duplicateCount"`
	CreatedAt             int64             `json:"createdAt"`
	Episodes              []UserCardEpisode `json:"episodes"`
}

// UserCardEpisode records the read/clear state of a card story episode.
type UserCardEpisode struct {
	CardEpisodeID         int      `json:"cardEpisodeId"`
	ScenarioStatus        string   `json:"scenarioStatus"`
	ScenarioStatusReasons []string `json:"scenarioStatusReasons"`
	IsNotSkipped          bool     `json:"isNotSkipped"`
}

// UserMusic records which music the player owns.
type UserMusic struct {
	MusicID int `json:"musicId"`
}

// UserMusicResult holds a player's best result for one music×difficulty×play-type combination.
type UserMusicResult struct {
	MusicID             int    `json:"musicId"`
	MusicDifficultyType string `json:"musicDifficultyType"`
	PlayType            string `json:"playType"`
	PlayResult          string `json:"playResult"`
	HighScore           int    `json:"highScore"`
	FullComboFlg        bool   `json:"fullComboFlg"`
	FullPerfectFlg      bool   `json:"fullPerfectFlg"`
	MvpCount            int    `json:"mvpCount"`
	SuperStarCount      int    `json:"superStarCount"`
}

// UserCharacter tracks the player's bond rank with one character.
type UserCharacter struct {
	UserID        int64 `json:"userId"`
	CharacterID   int   `json:"characterId"`
	CharacterRank int   `json:"characterRank"`
	Exp           int   `json:"exp"`
	TotalExp      int   `json:"totalExp"`
}

// UserChallengeLiveSoloResult holds the player's high-score for a character's
// Challenge Live.
type UserChallengeLiveSoloResult struct {
	CharacterID int `json:"characterId"`
	HighScore   int `json:"highScore"`
}

// UserChallengeLiveSoloStage tracks the player's progress in one Challenge Live stage.
type UserChallengeLiveSoloStage struct {
	CharacterID              int    `json:"characterId"`
	ChallengeLiveStageType   string `json:"challengeLiveStageType"`
	Rank                     int    `json:"rank"`
	ChallengeLiveStageID     int    `json:"challengeLiveStageId"`
	ChallengeLiveStageStatus string `json:"challengeLiveStageStatus"`
	Point                    int    `json:"point"`
}

// UserAreaItem records the upgrade level of an area item.
type UserAreaItem struct {
	AreaItemID int `json:"areaItemId"`
	Level      int `json:"level"`
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

// UserConfig holds the player's account-level settings.
type UserConfig struct {
	DefaultMusicType     string `json:"defaultMusicType"`
	IsDisplayLoginStatus bool   `json:"isDisplayLoginStatus"`
	FriendRequestScope   string `json:"friendRequestScope"`
}
