package common

import (
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/testutil"
)

func TestCloneHelpersPreserveValuesAndOwnership(t *testing.T) {
	card := &masterdata.Card{ID: 1, CardParameters: []masterdata.CardParameter{{ID: 2}}}
	cardClone := CloneCard(card)
	cardClone.CardParameters[0].ID = 3
	{
		testutil.RequireArgs(t, !(cardClone == card), "CloneCard did not deep-copy card parameters")
		testutil.RequireArgs(t, !(card.CardParameters[0].ID != 2), "CloneCard did not deep-copy card parameters")
	}

	music := &masterdata.Music{ID: 4, Categories: []string{"mv"}}
	musicClone := CloneMusic(music)
	musicClone.Categories[0] = "image"
	{
		testutil.RequireArgs(t, !(musicClone == music), "CloneMusic did not deep-copy categories")
		testutil.RequireArgs(t, !(music.Categories[0] != "mv"), "CloneMusic did not deep-copy categories")
	}

	musicList := CloneMusicList([]*masterdata.Music{music, nil})
	{
		testutil.Require(t, !(len(musicList) != 2), "CloneMusicList result = %#v", musicList)
		testutil.Require(t, !(musicList[0] == music), "CloneMusicList result = %#v", musicList)
		testutil.Require(t, !(musicList[1] != nil), "CloneMusicList result = %#v", musicList)
	}

	detailValue := 9
	skill := &masterdata.Skill{ID: 5, SkillEffects: []masterdata.SkillEffect{{
		ID: 6,
		SkillEffectDetails: []masterdata.SkillEffectDetail{{
			ID:                   7,
			ActivateEffectValue2: &detailValue,
		}},
	}}}
	skillClone := CloneSkill(skill)
	skillClone.SkillEffects[0].SkillEffectDetails[0].ID = 8
	{
		testutil.RequireArgs(t, !(skillClone == skill), "CloneSkill did not deep-copy nested effect details")
		testutil.RequireArgs(t, !(skill.SkillEffects[0].SkillEffectDetails[0].ID != 7), "CloneSkill did not deep-copy nested effect details")
	}

	gacha := &masterdata.Gacha{
		ID:                   10,
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{{ID: 11}},
		GachaPickups:         []masterdata.GachaPickup{{ID: 12}},
		GachaDetails:         []masterdata.GachaDetail{{ID: 13}},
		GachaBehaviors:       []masterdata.GachaBehavior{{ID: 14}},
	}
	gachaClone := CloneGacha(gacha)
	gachaClone.GachaCardRarityRates[0].ID = 21
	gachaClone.GachaPickups[0].ID = 22
	gachaClone.GachaDetails[0].ID = 23
	gachaClone.GachaBehaviors[0].ID = 24
	{
		testutil.RequireArgs(t, !(gacha.GachaCardRarityRates[0].ID != 11), "CloneGacha retained a nested slice")
		testutil.RequireArgs(t, !(gacha.GachaPickups[0].ID != 12), "CloneGacha retained a nested slice")
		testutil.RequireArgs(t, !(gacha.GachaDetails[0].ID != 13), "CloneGacha retained a nested slice")
		testutil.RequireArgs(t, !(gacha.GachaBehaviors[0].ID != 14), "CloneGacha retained a nested slice")
	}
	{

		list := CloneGachaList([]*masterdata.Gacha{gacha, nil})
		{
			testutil.Require(t, !(len(list) != 2), "CloneGachaList result = %#v", list)
			testutil.Require(t, !(list[0] == gacha), "CloneGachaList result = %#v", list)
			testutil.Require(t, !(list[1] != nil), "CloneGachaList result = %#v", list)
		}
	}

	honor := &masterdata.Honor{ID: 30, Levels: []masterdata.HonorLevel{{Level: 1}}}
	honorClone := CloneHonor(honor)
	honorClone.Levels[0].Level = 2
	testutil.RequireArgs(t, !(honor.Levels[0].Level != 1), "CloneHonor retained the levels slice")

	costume := &masterdata.Costume3d{ID: 40}
	costumes := CloneCostumes([]*masterdata.Costume3d{nil, costume})
	{
		testutil.Require(t, !(len(costumes) != 1), "CloneCostumes result = %#v", costumes)
		testutil.Require(t, !(costumes[0] == costume), "CloneCostumes result = %#v", costumes)
		testutil.Require(t, !(costumes[0].ID != 40), "CloneCostumes result = %#v", costumes)
	}
	{

		clone := CloneCostume(costume)
		{
			testutil.Require(t, !(clone == costume), "CloneCostume result = %#v", clone)
			testutil.Require(t, !(clone.ID != costume.ID), "CloneCostume result = %#v", clone)
		}
	}

}

func TestCloneHelpersNilAndEmptyInputs(t *testing.T) {
	{
		testutil.RequireArgs(t, !(CloneCard(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneEvent(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneCharacter(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneGameCharacterUnit(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneMusic(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneMusicDifficulty(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneSkill(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneGacha(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneHonor(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneHonorGroup(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneBondsHonor(nil) != nil), "a clone helper returned non-nil for nil input")
		testutil.RequireArgs(t, !(CloneCostume(nil) != nil), "a clone helper returned non-nil for nil input")
	}
	{
		testutil.RequireArgs(t, !(CloneMusicDifficulties(nil) != nil), "a slice clone helper returned non-nil for an empty input")
		testutil.RequireArgs(t, !(CloneGachaList(nil) != nil), "a slice clone helper returned non-nil for an empty input")
		testutil.RequireArgs(t, !(CloneCostumes(nil) != nil), "a slice clone helper returned non-nil for an empty input")
	}

	event := &masterdata.Event{ID: 1}
	character := &masterdata.Character{ID: 2}
	unit := &masterdata.GameCharacterUnit{ID: 3}
	difficulty := &masterdata.MusicDifficulty{ID: 4}
	group := &masterdata.HonorGroup{ID: 5}
	bonds := &masterdata.BondsHonor{ID: 6}
	for name, independent := range map[string]bool{
		"event":      CloneEvent(event) != event,
		"character":  CloneCharacter(character) != character,
		"unit":       CloneGameCharacterUnit(unit) != unit,
		"difficulty": CloneMusicDifficulty(difficulty) != difficulty,
		"group":      CloneHonorGroup(group) != group,
		"bonds":      CloneBondsHonor(bonds) != bonds,
	} {
		testutil.Check(t, independent, "%s clone reused the source pointer", name)

	}
	{
		got := CloneMusicDifficulties([]*masterdata.MusicDifficulty{difficulty, nil})
		{
			testutil.Require(t, !(len(got) != 2), "CloneMusicDifficulties result = %#v", got)
			testutil.Require(t, !(got[0] == difficulty), "CloneMusicDifficulties result = %#v", got)
			testutil.Require(t, !(got[1] != nil), "CloneMusicDifficulties result = %#v", got)
		}
	}

}
