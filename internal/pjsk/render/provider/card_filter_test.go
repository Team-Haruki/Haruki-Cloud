package provider

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestCardMatchesUnitFilterSupportsExplicitMainAndSupportUnit(t *testing.T) {
	tests := []struct {
		name        string
		filter      *CardFilter
		mainUnit    string
		supportUnit string
		want        bool
	}{
		{
			name:        "oc unit matches exact main unit",
			filter:      &CardFilter{MainUnit: "idol", SupportUnit: "none"},
			mainUnit:    "idol",
			supportUnit: "",
			want:        true,
		},
		{
			name:        "oc unit does not match vs card of same support",
			filter:      &CardFilter{MainUnit: "idol", SupportUnit: "none"},
			mainUnit:    "piapro",
			supportUnit: "idol",
			want:        false,
		},
		{
			name:        "pure vs matches piapro with no support",
			filter:      &CardFilter{MainUnit: "piapro", SupportUnit: "none"},
			mainUnit:    "piapro",
			supportUnit: "",
			want:        true,
		},
		{
			name:        "pure vs does not match oc card",
			filter:      &CardFilter{MainUnit: "piapro", SupportUnit: "none"},
			mainUnit:    "street",
			supportUnit: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cardMatchesUnitFilter(tt.filter, tt.mainUnit, tt.supportUnit); got != tt.want {
				t.Fatalf("cardMatchesUnitFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocalCardProviderFilterHonorsMainUnitConstraint(t *testing.T) {
	charMap := map[int]*masterdata.Character{
		1: {ID: 1, Unit: "idol"},
		2: {ID: 2, Unit: "piapro"},
		3: {ID: 3, Unit: "piapro"},
	}
	characters := &localCharacterProvider{}
	characters.chars.init(func() (map[int]*masterdata.Character, error) { return charMap, nil })

	cardData := cardIndex{
		all: []*masterdata.Card{
			{ID: 101, CharacterID: 1, SupportUnit: ""},
			{ID: 102, CharacterID: 2, SupportUnit: "idol"},
			{ID: 103, CharacterID: 3, SupportUnit: ""},
		},
	}
	provider := &localCardProvider{characters: characters}
	provider.cards.init(func() (cardIndex, error) { return cardData, nil })

	results, err := provider.Filter(context.Background(), &CardFilter{MainUnit: "idol", SupportUnit: "none"})
	if err != nil {
		t.Fatalf("Filter(oc) error = %v", err)
	}
	if len(results) != 1 || results[0].ID != 101 {
		t.Fatalf("unexpected oc filter results: %+v", results)
	}

	results, err = provider.Filter(context.Background(), &CardFilter{MainUnit: "piapro", SupportUnit: "none"})
	if err != nil {
		t.Fatalf("Filter(pure vs) error = %v", err)
	}
	if len(results) != 1 || results[0].ID != 103 {
		t.Fatalf("unexpected pure-vs filter results: %+v", results)
	}
}

func TestCardSkillTypesMatchSupportsLegacyJudgmentAlias(t *testing.T) {
	tests := []struct {
		name       string
		filterType string
		actualType string
		want       bool
	}{
		{name: "exact score", filterType: "score_up", actualType: "score_up", want: true},
		{name: "exact heal", filterType: "life_recovery", actualType: "life_recovery", want: true},
		{name: "legacy judgment alias", filterType: "judgment_up", actualType: "judgment_accuracy_up", want: true},
		{name: "reverse legacy judgment alias", filterType: "judgment_accuracy_up", actualType: "judgment_up", want: true},
		{name: "different types", filterType: "life_recovery", actualType: "score_up", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cardSkillTypesMatch(tt.filterType, tt.actualType); got != tt.want {
				t.Fatalf("cardSkillTypesMatch(%q, %q) = %v, want %v", tt.filterType, tt.actualType, got, tt.want)
			}
		})
	}
}

func TestLocalCardProviderFilterHonorsDetailedSkillIDs(t *testing.T) {
	cardData := cardIndex{
		all: []*masterdata.Card{
			{ID: 101, SkillID: 12},
			{ID: 102, SkillID: 15},
			{ID: 103, SkillID: 1},
		},
	}
	provider := &localCardProvider{}
	provider.cards.init(func() (cardIndex, error) { return cardData, nil })

	results, err := provider.Filter(context.Background(), &CardFilter{SkillIDs: []int{12}})
	if err != nil {
		t.Fatalf("Filter(blood score) error = %v", err)
	}
	if len(results) != 1 || results[0].ID != 101 {
		t.Fatalf("unexpected blood-score filter results: %+v", results)
	}

	results, err = provider.Filter(context.Background(), &CardFilter{SkillIDs: []int{15, 16, 17, 18, 19}})
	if err != nil {
		t.Fatalf("Filter(group score) error = %v", err)
	}
	if len(results) != 1 || results[0].ID != 102 {
		t.Fatalf("unexpected group-score filter results: %+v", results)
	}
}
