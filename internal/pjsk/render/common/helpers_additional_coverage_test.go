package common

import (
	"testing"

	sekaiDB "haruki-cloud/database/sekai"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"
)

func TestJSONNicknamePointerAndSliceHelpers(t *testing.T) {
	var decoded map[string]any
	{
		err := DecodeJSONUseNumber([]byte(`{"id":9007199254740993}`), &decoded)
		testutil.RequireArgs(t, !(err != nil), err)
	}
	{

		_, ok := decoded["id"].(json.Number)
		testutil.Require(t, ok, "number was decoded as %T", decoded["id"])
	}
	{

		err := DecodeJSONUseNumber([]byte(`{`), &decoded)
		testutil.RequireArgs(t, !(err == nil), "invalid JSON unexpectedly decoded")
	}
	{
		testutil.RequireArgs(t, !(JSONString(nil) != ""), "JSONString branches returned unexpected values")
		testutil.RequireArgs(t, !(JSONString(json.RawMessage(`"value"`)) != "value"), "JSONString branches returned unexpected values")
		testutil.RequireArgs(t, !(JSONString(json.RawMessage(`123`)) != "123"), "JSONString branches returned unexpected values")
	}
	{

		got, err := DecodeSlice[int](nil)
		{
			testutil.Require(t, !(err != nil), "empty slice decode = %v,%v", got, err)
			testutil.Require(t, !(got != nil), "empty slice decode = %v,%v", got, err)
		}
	}
	{

		got, err := DecodeSlice[int](json.RawMessage(`[1,2]`))
		{
			testutil.Require(t, !(err != nil), "slice decode = %v,%v", got, err)
			testutil.Require(t, !(len(got) != 2), "slice decode = %v,%v", got, err)
		}
	}
	{

		_, err := DecodeSlice[int](json.RawMessage(`{`))
		testutil.RequireArgs(t, !(err == nil), "invalid slice JSON unexpectedly decoded")
	}
	{

		got, err := DecodeMap[map[string]int](nil)
		{
			testutil.Require(t, !(err != nil), "empty map decode = %v,%v", got, err)
			testutil.Require(t, !(got != nil), "empty map decode = %v,%v", got, err)
		}
	}
	{

		got, err := DecodeMap[map[string]int](json.RawMessage(`{"a":1}`))
		{
			testutil.Require(t, !(err != nil), "map decode = %v,%v", got, err)
			testutil.Require(t, !(got["a"] != 1), "map decode = %v,%v", got, err)
		}
	}
	{

		_, err := DecodeMap[map[string]int](json.RawMessage(`{`))
		testutil.RequireArgs(t, !(err == nil), "invalid map JSON unexpectedly decoded")
	}
	{
		testutil.RequireArgs(t, !(ToStringSliceFromRaw(nil) != nil), "invalid raw string slices should be nil")
		testutil.RequireArgs(t, !(ToStringSliceFromRaw(json.RawMessage(`{`)) != nil), "invalid raw string slices should be nil")
	}
	{

		got := ToStringSliceFromRaw(json.RawMessage(`["a"," ","b"]`))
		{
			testutil.Require(t, !(len(got) != 2), "filtered strings = %v", got)
			testutil.Require(t, !(got[0] != "a"), "filtered strings = %v", got)
			testutil.Require(t, !(got[1] != "b"), "filtered strings = %v", got)
		}
	}

	source := map[string]int{"miku": 21}
	clone := CloneNicknames(source)
	clone["miku"] = 1
	testutil.RequireArgs(t, !(source["miku"] != 21), "nickname clone shared storage")
	{

		got := NormalizeNicknameQuery(" MIKU  LONG ")
		testutil.Require(t, !(got != "mikulong"), "unexpected normalized nickname: %q", got)
	}

	value := "x"
	{
		testutil.RequireArgs(t, !(CloneStringPtr(nil) != nil), "string pointer clone branches failed")
		testutil.RequireArgs(t, !(CloneStringPtr(&value) == &value), "string pointer clone branches failed")
		testutil.RequireArgs(t, !(*CloneStringPtr(&value) != "x"), "string pointer clone branches failed")
	}
	{
		testutil.RequireArgs(t, *BoolPtr(true), "pointer helpers returned unexpected values")
		testutil.RequireArgs(t, !(OptionalString(" ") != nil), "pointer helpers returned unexpected values")
		testutil.RequireArgs(t, !(*OptionalString(" x ") != "x"), "pointer helpers returned unexpected values")
	}
	{
		testutil.RequireArgs(t, ContainsString([]string{"One", "Two"}, "two"), "ContainsString branches failed")
		testutil.RequireArgs(t, !(ContainsString([]string{"One"}, "three")), "ContainsString branches failed")
	}

}

func TestEntityConverterErrorAndSuccessBranches(t *testing.T) {
	{
		_, err := ConvertCardEntity(nil)
		testutil.RequireArgs(t, !(err == nil), "nil card entity unexpectedly converted")
	}
	{

		_, err := ConvertCardEntity(&sekaiDB.Card{CardParameters: json.RawMessage(`{`)})
		testutil.RequireArgs(t, !(err == nil), "invalid card parameters unexpectedly converted")
	}
	{
		testutil.RequireArgs(t, !(ConvertEventEntity(nil) != nil), "nil simple entities unexpectedly converted")
		testutil.RequireArgs(t, !(ConvertCostumeEntity(nil) != nil), "nil simple entities unexpectedly converted")
	}

	event := ConvertEventEntity(&sekaiDB.Event{GameID: 1, Name: "Event"})
	{
		testutil.Require(t, !(event.ID != 1), "unexpected event conversion: %+v", event)
		testutil.Require(t, !(event.Name != "Event"), "unexpected event conversion: %+v", event)
	}

	costume := ConvertCostumeEntity(&sekaiDB.Costume3D{GameID: 2, Name: "Costume"})
	{
		testutil.Require(t, !(costume.ID != 2), "unexpected costume conversion: %+v", costume)
		testutil.Require(t, !(costume.Description != "Costume"), "unexpected costume conversion: %+v", costume)
	}

	music := ConvertMusicEntity(&sekaiDB.Music{GameID: 3, Categories: json.RawMessage(`["mv"]`)})
	{
		testutil.Require(t, !(music.ID != 3), "unexpected music conversion: %+v", music)
		testutil.Require(t, !(len(music.Categories) != 1), "unexpected music conversion: %+v", music)
	}
	{

		_, err := ConvertGachaEntity(nil)
		testutil.RequireArgs(t, !(err == nil), "nil gacha entity unexpectedly converted")
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
		{
			_, err := ConvertGachaEntity(entity)
			testutil.Require(t, !(err == nil), "invalid gacha case %d unexpectedly converted", i)
		}

	}
	gacha, err := ConvertGachaEntity(&sekaiDB.Gacha{GameID: 4, GachaCeilItemID: 9})
	{
		testutil.Require(t, !(err != nil), "unexpected gacha conversion: %+v,%v", gacha, err)
		testutil.Require(t, !(gacha.ID != 4), "unexpected gacha conversion: %+v,%v", gacha, err)
		testutil.Require(t, !(gacha.GachaCeilItemID == nil), "unexpected gacha conversion: %+v,%v", gacha, err)
		testutil.Require(t, !(*gacha.GachaCeilItemID != 9), "unexpected gacha conversion: %+v,%v", gacha, err)
	}
	{

		_, err := ConvertSkillEntity(nil)
		testutil.RequireArgs(t, !(err == nil), "nil skill entity unexpectedly converted")
	}
	{

		_, err := ConvertSkillEntity(&sekaiDB.Skill{SkillEffects: json.RawMessage(`{`)})
		testutil.RequireArgs(t, !(err == nil), "invalid skill effects unexpectedly converted")
	}

	skill, err := ConvertSkillEntity(&sekaiDB.Skill{GameID: 5, SkillEffects: json.RawMessage(`[{}]`)})
	{
		testutil.Require(t, !(err != nil), "unexpected skill conversion: %+v,%v", skill, err)
		testutil.Require(t, !(skill.ID != 5), "unexpected skill conversion: %+v,%v", skill, err)
		testutil.Require(t, !(len(skill.SkillEffects) != 1), "unexpected skill conversion: %+v,%v", skill, err)
	}

}
