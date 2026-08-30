package masterdata

import (
	"testing"

	json "haruki-cloud/internal/jsonutil"
)

func TestDecodeCardParametersEmptyAndSingleShapes(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, {}, []byte("null"), []byte("{}"), []byte(`{"param1":null,"param2":""}`)} {
		parameters, err := DecodeCardParameters(raw)
		if err != nil || parameters != nil {
			t.Fatalf("DecodeCardParameters(%q) = %+v, %v", raw, parameters, err)
		}
	}
	parameters, err := DecodeCardParameters([]byte(`{"cardParameterType":" PARAM1 ","power":12}`))
	if err != nil || len(parameters) != 1 || parameters[0].CardParameterType != "param1" {
		t.Fatalf("single parameter = %+v, %v", parameters, err)
	}
}

func TestDecodeCardParameterEntryScalarShapes(t *testing.T) {
	testCases := []struct {
		name  string
		raw   string
		power int
	}{
		{"integer", "123", 123},
		{"float", "123.9", 123},
		{"string", `"456"`, 456},
		{"series", `[1,2.8,"3",{"Power":4},{"value":"5"}]`, 5},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			item, ok, err := decodeCardParameterEntry(" PARAM2 ", []byte(testCase.raw))
			if err != nil || !ok || item.Power != testCase.power || item.CardParameterType != "param2" {
				t.Fatalf("decodeCardParameterEntry() = %+v, %v, %v", item, ok, err)
			}
		})
	}
}

func TestDecodeCardParameterEntryRejectsUnsupportedShapes(t *testing.T) {
	for _, raw := range []string{`"not-a-number"`, `true`, `["not-a-number"]`} {
		if _, _, err := decodeCardParameterEntry("param1", []byte(raw)); err == nil {
			t.Fatalf("decodeCardParameterEntry(%s) should fail", raw)
		}
	}
	if _, err := DecodeCardParameters([]byte("{")); err == nil {
		t.Fatal("invalid parameter JSON should fail")
	}
}

func TestDecodeCardParameterPowerValueShapes(t *testing.T) {
	testCases := []struct {
		raw   string
		power int
		ok    bool
	}{
		{"", 0, false},
		{"null", 0, false},
		{"8", 8, true},
		{"8.9", 8, true},
		{`" 9 "`, 9, true},
		{`""`, 0, false},
		{`{"power":10}`, 10, true},
		{`{"Value":"11"}`, 11, true},
		{`{"other":12}`, 0, false},
		{"true", 0, false},
	}
	for _, testCase := range testCases {
		power, ok, err := decodeCardParameterPowerValue([]byte(testCase.raw))
		if err != nil || power != testCase.power || ok != testCase.ok {
			t.Fatalf("decodeCardParameterPowerValue(%q) = %d, %v, %v", testCase.raw, power, ok, err)
		}
	}
	if _, _, err := decodeCardParameterPowerValue([]byte(`"bad"`)); err == nil {
		t.Fatal("invalid string power should fail")
	}
}

func TestCardParameterOrderingAndNormalization(t *testing.T) {
	if compareCardParameterKeys("param2", "param10") >= 0 {
		t.Fatal("param2 should sort before param10")
	}
	if compareCardParameterKeys("param2", "power") >= 0 {
		t.Fatal("indexed parameters should sort before named parameters")
	}
	if compareCardParameterKeys("zeta", "alpha") <= 0 || compareCardParameterKeys("same", "same") != 0 {
		t.Fatal("named parameter ordering is incorrect")
	}
	items := []CardParameter{{CardParameterType: " PARAM1 "}, {CardID: 9}}
	normalizeCardParameters(items, 7)
	if items[0].CardParameterType != "param1" || items[0].CardID != 7 || items[1].CardID != 9 {
		t.Fatalf("normalizeCardParameters() = %+v", items)
	}
	if cardParameterMeaningful(CardParameter{}) || !cardParameterMeaningful(CardParameter{Power: 1}) {
		t.Fatal("cardParameterMeaningful() returned unexpected result")
	}
}

func TestCardUnmarshalJSONSnakeCaseAndErrors(t *testing.T) {
	var card Card
	if err := json.Unmarshal([]byte(`{"id":44,"card_parameters":{"param1":12}}`), &card); err != nil {
		t.Fatalf("snake-case card parameters: %v", err)
	}
	if len(card.CardParameters) != 1 || card.CardParameters[0].CardID != 44 {
		t.Fatalf("snake-case parameters = %+v", card.CardParameters)
	}
	if err := json.Unmarshal([]byte(`{"cardParameters":{"param1":true}}`), &card); err == nil {
		t.Fatal("unsupported card parameter should fail")
	}
	if err := card.UnmarshalJSON([]byte("{")); err == nil {
		t.Fatal("invalid card JSON should fail")
	}
}
