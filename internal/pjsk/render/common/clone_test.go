package common

import (
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestCloneHelpersPreserveValuesAndOwnership(t *testing.T) {
	card := &masterdata.Card{ID: 1, CardParameters: []masterdata.CardParameter{{ID: 2}}}
	cardClone := CloneCard(card)
	cardClone.CardParameters[0].ID = 3
	if cardClone == card || card.CardParameters[0].ID != 2 {
		t.Fatal("CloneCard did not deep-copy card parameters")
	}

	music := &masterdata.Music{ID: 4, Categories: []string{"mv"}}
	musicClone := CloneMusic(music)
	musicClone.Categories[0] = "image"
	if musicClone == music || music.Categories[0] != "mv" {
		t.Fatal("CloneMusic did not deep-copy categories")
	}
	musicList := CloneMusicList([]*masterdata.Music{music, nil})
	if len(musicList) != 2 || musicList[0] == music || musicList[1] != nil {
		t.Fatalf("CloneMusicList result = %#v", musicList)
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
	if skillClone == skill || skill.SkillEffects[0].SkillEffectDetails[0].ID != 7 {
		t.Fatal("CloneSkill did not deep-copy nested effect details")
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
	if gacha.GachaCardRarityRates[0].ID != 11 || gacha.GachaPickups[0].ID != 12 ||
		gacha.GachaDetails[0].ID != 13 || gacha.GachaBehaviors[0].ID != 14 {
		t.Fatal("CloneGacha retained a nested slice")
	}
	if list := CloneGachaList([]*masterdata.Gacha{gacha, nil}); len(list) != 2 || list[0] == gacha || list[1] != nil {
		t.Fatalf("CloneGachaList result = %#v", list)
	}

	honor := &masterdata.Honor{ID: 30, Levels: []masterdata.HonorLevel{{Level: 1}}}
	honorClone := CloneHonor(honor)
	honorClone.Levels[0].Level = 2
	if honor.Levels[0].Level != 1 {
		t.Fatal("CloneHonor retained the levels slice")
	}

	costume := &masterdata.Costume3d{ID: 40}
	costumes := CloneCostumes([]*masterdata.Costume3d{nil, costume})
	if len(costumes) != 1 || costumes[0] == costume || costumes[0].ID != 40 {
		t.Fatalf("CloneCostumes result = %#v", costumes)
	}
	if clone := CloneCostume(costume); clone == costume || clone.ID != costume.ID {
		t.Fatalf("CloneCostume result = %#v", clone)
	}
}

func TestCloneHelpersNilAndEmptyInputs(t *testing.T) {
	if CloneCard(nil) != nil || CloneEvent(nil) != nil || CloneCharacter(nil) != nil ||
		CloneGameCharacterUnit(nil) != nil || CloneMusic(nil) != nil ||
		CloneMusicDifficulty(nil) != nil || CloneSkill(nil) != nil || CloneGacha(nil) != nil ||
		CloneHonor(nil) != nil || CloneHonorGroup(nil) != nil || CloneBondsHonor(nil) != nil ||
		CloneCostume(nil) != nil {
		t.Fatal("a clone helper returned non-nil for nil input")
	}
	if CloneMusicDifficulties(nil) != nil || CloneGachaList(nil) != nil || CloneCostumes(nil) != nil {
		t.Fatal("a slice clone helper returned non-nil for an empty input")
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
		if !independent {
			t.Errorf("%s clone reused the source pointer", name)
		}
	}
	if got := CloneMusicDifficulties([]*masterdata.MusicDifficulty{difficulty, nil}); len(got) != 2 || got[0] == difficulty || got[1] != nil {
		t.Fatalf("CloneMusicDifficulties result = %#v", got)
	}
}
