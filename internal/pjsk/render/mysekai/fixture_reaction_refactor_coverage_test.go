package mysekai

import (
	"reflect"
	"testing"
)

func TestFixtureReactionRefactorResidualBranches(t *testing.T) {
	grouped := groupFixtureReactionCharacters([]any{
		"invalid",
		map[string]any{"FixtureId": 2},
		map[string]any{
			"FixtureId": 1,
			"ReactionCharacter": []any{
				"invalid",
				map[string]any{"CharacterUnitIds": []any{0, 10, 20}},
			},
		},
	}, 1)
	if got, want := grouped[2], [][]int{{10, 20}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fixture reaction groups = %#v, want %#v", got, want)
	}
}
