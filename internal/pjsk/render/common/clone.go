package common

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	"slices"
)

// CloneCard returns a deep copy of a Card, including CardParameters.
func CloneCard(item *masterdata.Card) *masterdata.Card {
	if item == nil {
		return nil
	}
	c := *item
	if len(item.CardParameters) > 0 {
		c.CardParameters = slices.Clone(item.CardParameters)
	}
	return &c
}

// CloneEvent returns a shallow copy of an Event.
func CloneEvent(item *masterdata.Event) *masterdata.Event {
	if item == nil {
		return nil
	}
	return new(*item)
}

// CloneCharacter returns a shallow copy of a Character.
func CloneCharacter(item *masterdata.Character) *masterdata.Character {
	if item == nil {
		return nil
	}
	return new(*item)
}

// CloneGameCharacterUnit returns a shallow copy of a GameCharacterUnit.
func CloneGameCharacterUnit(item *masterdata.GameCharacterUnit) *masterdata.GameCharacterUnit {
	if item == nil {
		return nil
	}
	return new(*item)
}

// CloneMusic returns a deep copy of a Music, including the Categories slice.
func CloneMusic(item *masterdata.Music) *masterdata.Music {
	if item == nil {
		return nil
	}
	c := *item
	if item.Categories != nil {
		c.Categories = slices.Clone(item.Categories)
	}
	return &c
}

// CloneMusicList returns a deep-copied slice of Music pointers.
func CloneMusicList(items []*masterdata.Music) []*masterdata.Music {
	result := make([]*masterdata.Music, 0, len(items))
	for _, item := range items {
		result = append(result, CloneMusic(item))
	}
	return result
}

// CloneSkill returns a deep copy of a Skill, including SkillEffects and
// nested SkillEffectDetails.
func CloneSkill(item *masterdata.Skill) *masterdata.Skill {
	if item == nil {
		return nil
	}
	c := *item
	if len(item.SkillEffects) > 0 {
		c.SkillEffects = make([]masterdata.SkillEffect, len(item.SkillEffects))
		for idx := range item.SkillEffects {
			c.SkillEffects[idx] = item.SkillEffects[idx]
			if len(item.SkillEffects[idx].SkillEffectDetails) > 0 {
				c.SkillEffects[idx].SkillEffectDetails = slices.Clone(item.SkillEffects[idx].SkillEffectDetails)
			}
		}
	}
	return &c
}

// CloneGacha returns a deep copy of a Gacha, including all nested slices.
func CloneGacha(item *masterdata.Gacha) *masterdata.Gacha {
	if item == nil {
		return nil
	}
	c := *item
	if len(item.GachaCardRarityRates) > 0 {
		c.GachaCardRarityRates = slices.Clone(item.GachaCardRarityRates)
	}
	if len(item.GachaPickups) > 0 {
		c.GachaPickups = slices.Clone(item.GachaPickups)
	}
	if len(item.GachaDetails) > 0 {
		c.GachaDetails = slices.Clone(item.GachaDetails)
	}
	if len(item.GachaBehaviors) > 0 {
		c.GachaBehaviors = slices.Clone(item.GachaBehaviors)
	}
	return &c
}

// CloneGachaList returns a deep-copied slice of Gacha pointers.
func CloneGachaList(items []*masterdata.Gacha) []*masterdata.Gacha {
	if len(items) == 0 {
		return nil
	}
	result := make([]*masterdata.Gacha, 0, len(items))
	for _, item := range items {
		result = append(result, CloneGacha(item))
	}
	return result
}

// CloneHonor returns a deep copy of an Honor, including the Levels slice.
func CloneHonor(src *masterdata.Honor) *masterdata.Honor {
	if src == nil {
		return nil
	}
	c := *src
	if src.Levels != nil {
		c.Levels = slices.Clone(src.Levels)
	}
	return &c
}

// CloneHonorGroup returns a shallow copy of an HonorGroup.
func CloneHonorGroup(src *masterdata.HonorGroup) *masterdata.HonorGroup {
	if src == nil {
		return nil
	}
	return new(*src)
}

// CloneBondsHonor returns a shallow copy of a BondsHonor.
func CloneBondsHonor(src *masterdata.BondsHonor) *masterdata.BondsHonor {
	if src == nil {
		return nil
	}
	return new(*src)
}

// CloneCostumes returns a deep-copied slice of Costume3d pointers, skipping nils.
func CloneCostumes(items []*masterdata.Costume3d) []*masterdata.Costume3d {
	if len(items) == 0 {
		return nil
	}
	result := make([]*masterdata.Costume3d, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, new(*item))
	}
	return result
}
