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
	SpecialTrainingPower1BonusFixed int
	SpecialTrainingPower2BonusFixed int
	SpecialTrainingPower3BonusFixed int
	SpecialTrainingSkillID          int
	SpecialTrainingSkillName        string
	CardSupplyID                    int
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
