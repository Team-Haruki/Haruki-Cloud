package card

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestCalculatePowerUnchangedByStrongestCardParameters(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 9))
	types := []string{"param1", "param2", "param3", "param4", ""}
	for trial := range 200 {
		params := make([]masterdata.CardParameter, rng.IntN(160))
		for i := range params {
			params[i] = masterdata.CardParameter{ID: i, CardID: trial, CardParameterType: types[rng.IntN(len(types))], Power: rng.IntN(3000) - 100}
		}
		original := slices.Clone(params)
		full := &masterdata.Card{ID: trial, CardParameters: params, SpecialTrainingPower1BonusFixed: 100, SpecialTrainingPower3BonusFixed: 50}
		compact := *full
		compact.CardParameters = masterdata.StrongestCardParameters(params)
		if len(compact.CardParameters) > len(types) {
			t.Fatalf("kept %d parameters", len(compact.CardParameters))
		}
		if !reflect.DeepEqual(params, original) {
			t.Fatal("StrongestCardParameters modified its input")
		}
		var b Builder
		if got, want := b.calculatePower(&compact), b.calculatePower(full); !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d: compact power %+v, full power %+v", trial, got, want)
		}
	}
	if got := masterdata.StrongestCardParameters(nil); got != nil {
		t.Fatalf("nil input = %v", got)
	}
	tie := []masterdata.CardParameter{{ID: 1, CardParameterType: "param1", Power: 5}, {ID: 2, CardParameterType: "param1", Power: 5}}
	if got := masterdata.StrongestCardParameters(tie); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("tie kept %+v, want the first entry", got)
	}
}
