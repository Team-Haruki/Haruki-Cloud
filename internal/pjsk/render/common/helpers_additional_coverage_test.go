package common

import (
	"testing"

	sekaiDB "haruki-cloud/database/sekai"
	json "haruki-cloud/internal/jsonutil"
)

func TestJSONNicknamePointerAndSliceHelpers(t *testing.T) {
	var decoded map[string]any
	if err := DecodeJSONUseNumber([]byte(`{"id":9007199254740993}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["id"].(json.Number); !ok {
		t.Fatalf("number was decoded as %T", decoded["id"])
	}
	if err := DecodeJSONUseNumber([]byte(`{`), &decoded); err == nil {
		t.Fatal("invalid JSON unexpectedly decoded")
	}
	if JSONString(nil) != "" || JSONString(json.RawMessage(`"value"`)) != "value" || JSONString(json.RawMessage(`123`)) != "123" {
		t.Fatal("JSONString branches returned unexpected values")
	}
	if got, err := DecodeSlice[int](nil); err != nil || got != nil {
		t.Fatalf("empty slice decode = %v,%v", got, err)
	}
	if got, err := DecodeSlice[int](json.RawMessage(`[1,2]`)); err != nil || len(got) != 2 {
		t.Fatalf("slice decode = %v,%v", got, err)
	}
	if _, err := DecodeSlice[int](json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid slice JSON unexpectedly decoded")
	}
	if got, err := DecodeMap[map[string]int](nil); err != nil || got != nil {
		t.Fatalf("empty map decode = %v,%v", got, err)
	}
	if got, err := DecodeMap[map[string]int](json.RawMessage(`{"a":1}`)); err != nil || got["a"] != 1 {
		t.Fatalf("map decode = %v,%v", got, err)
	}
	if _, err := DecodeMap[map[string]int](json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid map JSON unexpectedly decoded")
	}
	if ToStringSliceFromRaw(nil) != nil || ToStringSliceFromRaw(json.RawMessage(`{`)) != nil {
		t.Fatal("invalid raw string slices should be nil")
	}
	if got := ToStringSliceFromRaw(json.RawMessage(`["a"," ","b"]`)); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("filtered strings = %v", got)
	}

	source := map[string]int{"miku": 21}
	clone := CloneNicknames(source)
	clone["miku"] = 1
	if source["miku"] != 21 {
		t.Fatal("nickname clone shared storage")
	}
	if got := NormalizeNicknameQuery(" MIKU  LONG "); got != "mikulong" {
		t.Fatalf("unexpected normalized nickname: %q", got)
	}
	value := "x"
	if CloneStringPtr(nil) != nil || CloneStringPtr(&value) == &value || *CloneStringPtr(&value) != "x" {
		t.Fatal("string pointer clone branches failed")
	}
	if !*BoolPtr(true) || OptionalString(" ") != nil || *OptionalString(" x ") != "x" {
		t.Fatal("pointer helpers returned unexpected values")
	}
	if !ContainsString([]string{"One", "Two"}, "two") || ContainsString([]string{"One"}, "three") {
		t.Fatal("ContainsString branches failed")
	}
}

func TestEntityConverterErrorAndSuccessBranches(t *testing.T) {
	if _, err := ConvertCardEntity(nil); err == nil {
		t.Fatal("nil card entity unexpectedly converted")
	}
	if _, err := ConvertCardEntity(&sekaiDB.Card{CardParameters: json.RawMessage(`{`)}); err == nil {
		t.Fatal("invalid card parameters unexpectedly converted")
	}
	if ConvertEventEntity(nil) != nil || ConvertCostumeEntity(nil) != nil {
		t.Fatal("nil simple entities unexpectedly converted")
	}
	event := ConvertEventEntity(&sekaiDB.Event{GameID: 1, Name: "Event"})
	if event.ID != 1 || event.Name != "Event" {
		t.Fatalf("unexpected event conversion: %+v", event)
	}
	costume := ConvertCostumeEntity(&sekaiDB.Costume3D{GameID: 2, Name: "Costume"})
	if costume.ID != 2 || costume.Description != "Costume" {
		t.Fatalf("unexpected costume conversion: %+v", costume)
	}
	music := ConvertMusicEntity(&sekaiDB.Music{GameID: 3, Categories: json.RawMessage(`["mv"]`)})
	if music.ID != 3 || len(music.Categories) != 1 {
		t.Fatalf("unexpected music conversion: %+v", music)
	}

	if _, err := ConvertGachaEntity(nil); err == nil {
		t.Fatal("nil gacha entity unexpectedly converted")
	}
	valid := json.RawMessage(`[]`)
	invalidCases := []*sekaiDB.Gacha{
		{GachaCardRarityRates: json.RawMessage(`{`)},
		{GachaCardRarityRates: valid, GachaDetails: json.RawMessage(`{`)},
		{GachaCardRarityRates: valid, GachaDetails: valid, GachaBehaviors: json.RawMessage(`{`)},
		{GachaCardRarityRates: valid, GachaDetails: valid, GachaBehaviors: valid, GachaPickups: json.RawMessage(`{`)},
		{GachaCardRarityRates: valid, GachaDetails: valid, GachaBehaviors: valid, GachaPickups: valid, GachaInformation: json.RawMessage(`{`)},
	}
	for i, entity := range invalidCases {
		if _, err := ConvertGachaEntity(entity); err == nil {
			t.Fatalf("invalid gacha case %d unexpectedly converted", i)
		}
	}
	gacha, err := ConvertGachaEntity(&sekaiDB.Gacha{GameID: 4, GachaCeilItemID: 9})
	if err != nil || gacha.ID != 4 || gacha.GachaCeilItemID == nil || *gacha.GachaCeilItemID != 9 {
		t.Fatalf("unexpected gacha conversion: %+v,%v", gacha, err)
	}

	if _, err := ConvertSkillEntity(nil); err == nil {
		t.Fatal("nil skill entity unexpectedly converted")
	}
	if _, err := ConvertSkillEntity(&sekaiDB.Skill{SkillEffects: json.RawMessage(`{`)}); err == nil {
		t.Fatal("invalid skill effects unexpectedly converted")
	}
	skill, err := ConvertSkillEntity(&sekaiDB.Skill{GameID: 5, SkillEffects: json.RawMessage(`[{}]`)})
	if err != nil || skill.ID != 5 || len(skill.SkillEffects) != 1 {
		t.Fatalf("unexpected skill conversion: %+v,%v", skill, err)
	}
}
