package masterdata

type Card struct {
	ID                              int
	CharacterID                     int
	CardRarityType                  string
	Attr                            string
	Prefix                          string
	AssetBundleName                 string
	ReleaseAt                       int64
	SkillID                         int
	CardSkillName                   string
	SupportUnit                     string
	CardParameters                  []CardParameter
	SpecialTrainingPower1BonusFixed int
	SpecialTrainingPower2BonusFixed int
	SpecialTrainingPower3BonusFixed int
	SpecialTrainingSkillID          int
	SpecialTrainingSkillName        string
	CardSupplyID                    int
}

type CardParameter struct {
	ID                int    `json:"id"`
	CardID            int    `json:"cardId"`
	CardParameterType string `json:"cardParameterType"`
	Power             int    `json:"power"`
}

type Skill struct {
	ID                    int           `json:"id"`
	ShortDescription      string        `json:"shortDescription"`
	Description           string        `json:"description"`
	DescriptionSpriteName string        `json:"descriptionSpriteName"`
	SkillEffects          []SkillEffect `json:"skillEffects"`
}

type SkillEffect struct {
	ID                        int                 `json:"id"`
	SkillEffectType           string              `json:"skillEffectType"`
	ActivateEffectDuration    int                 `json:"activateEffectDuration"`
	ActivateEffectValueType   string              `json:"activateEffectValueType"`
	ActivateEffectValue       float64             `json:"activateEffectValue"`
	SkillEffectDetails        []SkillEffectDetail `json:"skillEffectDetails"`
	SkillEnhance              SkillEnhance        `json:"skillEnhance"`
	ConditionType             string              `json:"conditionType"`
	ActivateNotesJudgmentType string              `json:"activateNotesJudgmentType"`
	ActivateUnitCount         int                 `json:"activateUnitCount"`
	ActivateCharacterRank     int                 `json:"activateCharacterRank"`
}

type SkillEffectDetail struct {
	ID                      int     `json:"id"`
	ActivateEffectDuration  float64 `json:"activateEffectDuration"`
	ActivateEffectValueType string  `json:"activateEffectValueType"`
	ActivateEffectValue     int     `json:"activateEffectValue"`
	ActivateEffectValue2    *int    `json:"activateEffectValue2,omitempty"`
}

type SkillEnhance struct {
	ActivateEffectValue int `json:"activateEffectValue"`
}

type Character struct {
	ID        int
	FirstName string
	GivenName string
	Unit      string
}

type Event struct {
	ID              int
	EventType       string
	Name            string
	AssetBundleName string
	StartAt         int64
	AggregateAt     int64
	ClosedAt        int64
}

type EventDeckBonus struct {
	ID                  int
	EventID             int
	GameCharacterUnitID int
	GameCharacterID     int
	CardAttr            string
	BonusRate           float64
}

type GameCharacterUnit struct {
	ID              int
	GameCharacterID int
	Unit            string
	ColorCode       string
}

type WorldBloom struct {
	ID              int
	EventID         int
	GameCharacterID *int
	ChapterNo       int
	ChapterStartAt  int64
	AggregateAt     int64
	ChapterEndAt    int64
	IsSupplemental  bool
	ChapterType     string
}

type Gacha struct {
	ID                     int                   `json:"id"`
	GachaType              string                `json:"gachaType"`
	Name                   string                `json:"name"`
	Seq                    int                   `json:"seq"`
	AssetBundleName        string                `json:"assetbundleName"`
	StartAt                int64                 `json:"startAt"`
	EndAt                  int64                 `json:"endAt"`
	IsShowPeriod           bool                  `json:"isShowPeriod"`
	GachaCeilItemID        *int                  `json:"gachaCeilItemId"`
	WishSelectCount        int                   `json:"wishSelectCount"`
	WishFixedSelectCount   int                   `json:"wishFixedSelectCount"`
	WishLimitedSelectCount int                   `json:"wishLimitedSelectCount"`
	GachaCardRarityRates   []GachaCardRarityRate `json:"gachaCardRarityRates"`
	GachaPickups           []GachaPickup         `json:"gachaPickups"`
	GachaDetails           []GachaDetail         `json:"gachaDetails"`
	GachaBehaviors         []GachaBehavior       `json:"gachaBehaviors"`
	GachaInformation       GachaInformation      `json:"gachaInformation"`
}

type GachaPickup struct {
	ID              int    `json:"id"`
	GachaID         int    `json:"gachaId"`
	CardID          int    `json:"cardId"`
	GachaPickupType string `json:"gachaPickupType"`
}

type GachaDetail struct {
	ID      int  `json:"id"`
	GachaID int  `json:"gachaId"`
	CardID  int  `json:"cardId"`
	Weight  int  `json:"weight"`
	IsWish  bool `json:"isWish"`
}

type GachaCardRarityRate struct {
	ID             int     `json:"id"`
	GroupID        int     `json:"groupId"`
	CardRarityType string  `json:"cardRarityType"`
	LotteryType    string  `json:"lotteryType"`
	Rate           float64 `json:"rate"`
}

type GachaBehavior struct {
	ID                   int    `json:"id"`
	GachaID              int    `json:"gachaId"`
	GachaBehaviorType    string `json:"gachaBehaviorType"`
	CostResourceType     string `json:"costResourceType"`
	CostResourceQuantity int    `json:"costResourceQuantity"`
	SpinCount            int    `json:"spinCount"`
	ExecuteLimit         *int   `json:"executeLimit"`
	GroupID              int    `json:"groupId"`
	Priority             int    `json:"priority"`
	ResourceCategory     string `json:"resourceCategory"`
	GachaSpinnableType   string `json:"gachaSpinnableType"`
}

type GachaInformation struct {
	GachaID     int    `json:"gachaId"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}

type Music struct {
	ID                 int
	Seq                int
	ReleaseConditionID int
	Categories         []string
	Title              string
	Pronunciation      string
	Lyricist           string
	Composer           string
	Arranger           string
	DancerCount        int
	SelfDancerCount    int
	AssetBundleName    string
	PublishedAt        int64
	DigitizedAt        int64
	IsFullLength       bool
}

type MusicDifficulty struct {
	ID              int    `json:"id"`
	MusicID         int    `json:"musicId"`
	MusicDifficulty string `json:"musicDifficulty"`
	PlayLevel       int    `json:"playLevel"`
	TotalNoteCount  int    `json:"totalNoteCount"`
}

type MusicVocal struct {
	ID              int
	MusicID         int
	MusicVocalType  string
	Caption         string
	Characters      []MusicVocalCharacter
	AssetBundleName string
}

type MusicVocalCharacter struct {
	ID            int
	MusicID       int
	MusicVocalID  int
	CharacterType string
	CharacterID   int
}

type LimitedTimeMusic struct {
	ID      int
	MusicID int
	StartAt int64
	EndAt   int64
}

type Costume3d struct {
	ID              int    `json:"id"`
	CharacterID     int    `json:"characterId"`
	AssetBundleName string `json:"assetbundleName"`
	Description     string `json:"description"`
}

type Stamp struct {
	ID              int
	AssetBundleName string
}

type Honor struct {
	ID              int
	GroupID         int
	HonorType       string
	HonorRarity     string
	Name            string
	Description     string
	AssetBundleName string
	Levels          []HonorLevel
}

type HonorLevel struct {
	Level           int
	HonorRarity     string
	Description     string
	AssetBundleName string
}

type HonorGroup struct {
	ID                        int
	HonorType                 string
	Name                      string
	Description               string
	BackgroundAssetBundleName *string
	FrameName                 *string
}

type BondsHonor struct {
	ID                   int
	GameCharacterUnitID1 int
	GameCharacterUnitID2 int
	HonorRarity          string
	Name                 string
	Description          string
	BondsGroupID         int
}
