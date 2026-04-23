package common

import (
	json "github.com/bytedance/sonic"
	"fmt"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ConvertCardEntity converts a database Card entity to the masterdata Card
// model, including full CardParameters deserialization.
func ConvertCardEntity(entity *sekaiDB.Card) (*masterdata.Card, error) {
	if entity == nil {
		return nil, fmt.Errorf("card entity is nil")
	}

	var parameters []masterdata.CardParameter
	if len(entity.CardParameters) > 0 {
		var err error
		parameters, err = masterdata.DecodeCardParameters(entity.CardParameters)
		if err != nil {
			return nil, fmt.Errorf("decode card parameters: %w", err)
		}
		for idx := range parameters {
			if parameters[idx].CardID == 0 {
				parameters[idx].CardID = int(entity.GameID)
			}
		}
	}

	return &masterdata.Card{
		ID:                              int(entity.GameID),
		CharacterID:                     int(entity.CharacterID),
		CardRarityType:                  entity.CardRarityType,
		Attr:                            entity.Attr,
		Prefix:                          entity.Prefix,
		AssetBundleName:                 entity.AssetbundleName,
		ReleaseAt:                       entity.ReleaseAt,
		SkillID:                         int(entity.SkillID),
		CardSkillName:                   entity.CardSkillName,
		SupportUnit:                     entity.SupportUnit,
		CardParameters:                  parameters,
		SpecialTrainingPower1BonusFixed: int(entity.SpecialTrainingPower1BonusFixed),
		SpecialTrainingPower2BonusFixed: int(entity.SpecialTrainingPower2BonusFixed),
		SpecialTrainingPower3BonusFixed: int(entity.SpecialTrainingPower3BonusFixed),
		SpecialTrainingSkillID:          int(entity.SpecialTrainingSkillID),
		SpecialTrainingSkillName:        entity.SpecialTrainingSkillName,
		CardSupplyID:                    int(entity.CardSupplyID),
		InitialSpecialTrainingStatus:    entity.InitialSpecialTrainingStatus,
	}, nil
}

// ConvertEventEntity converts a database Event entity to the masterdata Event model.
func ConvertEventEntity(entity *sekaiDB.Event) *masterdata.Event {
	if entity == nil {
		return nil
	}
	return &masterdata.Event{
		ID:              int(entity.GameID),
		EventType:       entity.EventType,
		Unit:            entity.Unit,
		Name:            entity.Name,
		AssetBundleName: entity.AssetbundleName,
		StartAt:         entity.StartAt,
		AggregateAt:     entity.AggregateAt,
		ClosedAt:        entity.ClosedAt,
		VirtualLiveID:   int(entity.VirtualLiveID),
	}
}

// ConvertMusicEntity converts a database Music entity to the masterdata Music model.
func ConvertMusicEntity(entity *sekaiDB.Music) *masterdata.Music {
	return &masterdata.Music{
		ID:                 int(entity.GameID),
		Seq:                int(entity.Seq),
		ReleaseConditionID: int(entity.ReleaseConditionID),
		Categories:         ToStringSliceFromRaw(entity.Categories),
		Title:              entity.Title,
		Pronunciation:      entity.Pronunciation,
		Lyricist:           entity.Lyricist,
		Composer:           entity.Composer,
		Arranger:           entity.Arranger,
		DancerCount:        int(entity.DancerCount),
		SelfDancerCount:    int(entity.SelfDancerPosition),
		AssetBundleName:    entity.AssetbundleName,
		PublishedAt:        entity.PublishedAt,
		DigitizedAt:        entity.ReleasedAt,
		IsFullLength:       entity.IsFullLength,
	}
}

// ConvertGachaEntity converts a database Gacha entity to the masterdata Gacha model.
func ConvertGachaEntity(entity *sekaiDB.Gacha) (*masterdata.Gacha, error) {
	if entity == nil {
		return nil, fmt.Errorf("gacha entity is nil")
	}
	rarityRates, err := DecodeSlice[masterdata.GachaCardRarityRate](entity.GachaCardRarityRates)
	if err != nil {
		return nil, fmt.Errorf("decode gacha rarity rates: %w", err)
	}
	details, err := DecodeSlice[masterdata.GachaDetail](entity.GachaDetails)
	if err != nil {
		return nil, fmt.Errorf("decode gacha details: %w", err)
	}
	behaviors, err := DecodeSlice[masterdata.GachaBehavior](entity.GachaBehaviors)
	if err != nil {
		return nil, fmt.Errorf("decode gacha behaviors: %w", err)
	}
	pickups, err := DecodeSlice[masterdata.GachaPickup](entity.GachaPickups)
	if err != nil {
		return nil, fmt.Errorf("decode gacha pickups: %w", err)
	}
	information, err := DecodeMap[masterdata.GachaInformation](entity.GachaInformation)
	if err != nil {
		return nil, fmt.Errorf("decode gacha information: %w", err)
	}

	var ceilItemID *int
	if entity.GachaCeilItemID != 0 {
		ceilItemID = new(int(entity.GachaCeilItemID))
	}

	return &masterdata.Gacha{
		ID:                     int(entity.GameID),
		GachaType:              entity.GachaType,
		Name:                   entity.Name,
		Seq:                    int(entity.Seq),
		AssetBundleName:        entity.AssetbundleName,
		StartAt:                entity.StartAt,
		EndAt:                  entity.EndAt,
		IsShowPeriod:           entity.IsShowPeriod,
		GachaCeilItemID:        ceilItemID,
		WishSelectCount:        int(entity.WishSelectCount),
		WishFixedSelectCount:   int(entity.WishFixedSelectCount),
		WishLimitedSelectCount: int(entity.WishLimitedSelectCount),
		GachaCardRarityRates:   rarityRates,
		GachaDetails:           details,
		GachaBehaviors:         behaviors,
		GachaPickups:           pickups,
		GachaInformation:       information,
	}, nil
}

// ConvertSkillEntity converts a database Skill entity to the masterdata Skill model.
func ConvertSkillEntity(entity *sekaiDB.Skill) (*masterdata.Skill, error) {
	if entity == nil {
		return nil, fmt.Errorf("skill entity is nil")
	}
	var effects []masterdata.SkillEffect
	if len(entity.SkillEffects) > 0 {
		if err := json.Unmarshal(entity.SkillEffects, &effects); err != nil {
			return nil, fmt.Errorf("decode skill effects: %w", err)
		}
	}
	return &masterdata.Skill{
		ID:                    int(entity.GameID),
		ShortDescription:      entity.ShortDescription,
		Description:           entity.Description,
		DescriptionSpriteName: entity.DescriptionSpriteName,
		SkillEffects:          effects,
	}, nil
}
