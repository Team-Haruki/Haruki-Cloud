package masterdata

import (
	"encoding/json"
	"testing"
)

func TestDecodeCardParametersAcceptsArrayShape(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":1,"cardId":1001,"cardParameterType":"param1","power":123},
		{"id":2,"cardId":1001,"cardParameterType":"param2","power":234}
	]`)

	got, err := DecodeCardParameters(raw)
	if err != nil {
		t.Fatalf("DecodeCardParameters(array) error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(got))
	}
	if got[0].CardParameterType != "param1" || got[0].Power != 123 {
		t.Fatalf("unexpected first parameter: %+v", got[0])
	}
	if got[1].CardParameterType != "param2" || got[1].Power != 234 {
		t.Fatalf("unexpected second parameter: %+v", got[1])
	}
}

func TestDecodeCardParametersAcceptsObjectShape(t *testing.T) {
	raw := json.RawMessage(`{
		"param3": 300,
		"param1": {"power": 100},
		"param2": {"id": 2, "power": 200}
	}`)

	got, err := DecodeCardParameters(raw)
	if err != nil {
		t.Fatalf("DecodeCardParameters(object) error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(got))
	}
	if got[0].CardParameterType != "param1" || got[0].Power != 100 {
		t.Fatalf("unexpected param1: %+v", got[0])
	}
	if got[1].CardParameterType != "param2" || got[1].Power != 200 || got[1].ID != 2 {
		t.Fatalf("unexpected param2: %+v", got[1])
	}
	if got[2].CardParameterType != "param3" || got[2].Power != 300 {
		t.Fatalf("unexpected param3: %+v", got[2])
	}
}

func TestDecodeCardParametersAcceptsPowerSeriesShape(t *testing.T) {
	raw := json.RawMessage(`{
		"param1": [3790, 3867, 3944, 11370],
		"param2": [3306, 3373, 9918],
		"param3": [3185, 3250, 9556]
	}`)

	got, err := DecodeCardParameters(raw)
	if err != nil {
		t.Fatalf("DecodeCardParameters(series) error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(got))
	}
	if got[0].CardParameterType != "param1" || got[0].Power != 11370 {
		t.Fatalf("unexpected param1: %+v", got[0])
	}
	if got[1].CardParameterType != "param2" || got[1].Power != 9918 {
		t.Fatalf("unexpected param2: %+v", got[1])
	}
	if got[2].CardParameterType != "param3" || got[2].Power != 9556 {
		t.Fatalf("unexpected param3: %+v", got[2])
	}
}

func TestCardUnmarshalJSONAcceptsObjectCardParameters(t *testing.T) {
	raw := []byte(`{
		"id": 1001,
		"characterId": 21,
		"cardParameters": {
			"param2": 220,
			"param1": 110,
			"param3": 330
		}
	}`)

	var card Card
	if err := json.Unmarshal(raw, &card); err != nil {
		t.Fatalf("unmarshal card: %v", err)
	}
	if len(card.CardParameters) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(card.CardParameters))
	}
	for _, parameter := range card.CardParameters {
		if parameter.CardID != 1001 {
			t.Fatalf("expected card id 1001, got %+v", parameter)
		}
	}
	if card.CardParameters[0].CardParameterType != "param1" || card.CardParameters[0].Power != 110 {
		t.Fatalf("unexpected first parameter: %+v", card.CardParameters[0])
	}
	if card.CardParameters[1].CardParameterType != "param2" || card.CardParameters[1].Power != 220 {
		t.Fatalf("unexpected second parameter: %+v", card.CardParameters[1])
	}
	if card.CardParameters[2].CardParameterType != "param3" || card.CardParameters[2].Power != 330 {
		t.Fatalf("unexpected third parameter: %+v", card.CardParameters[2])
	}
}

func TestCardUnmarshalJSONAcceptsSeriesCardParameters(t *testing.T) {
	raw := []byte(`{
		"id": 721,
		"characterId": 20,
		"cardParameters": {
			"param1": [3790, 3867, 3944, 11370],
			"param2": [3306, 3373, 9918],
			"param3": [3185, 3250, 9556]
		}
	}`)

	var card Card
	if err := json.Unmarshal(raw, &card); err != nil {
		t.Fatalf("unmarshal card: %v", err)
	}
	if len(card.CardParameters) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(card.CardParameters))
	}
	if card.CardParameters[0].CardID != 721 || card.CardParameters[0].Power != 11370 {
		t.Fatalf("unexpected first parameter: %+v", card.CardParameters[0])
	}
	if card.CardParameters[1].CardID != 721 || card.CardParameters[1].Power != 9918 {
		t.Fatalf("unexpected second parameter: %+v", card.CardParameters[1])
	}
	if card.CardParameters[2].CardID != 721 || card.CardParameters[2].Power != 9556 {
		t.Fatalf("unexpected third parameter: %+v", card.CardParameters[2])
	}
}
