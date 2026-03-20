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
