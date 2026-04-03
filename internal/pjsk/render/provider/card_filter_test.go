package provider

import (
	"sync"
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
	characters := &localCharacterProvider{
		charByID: map[int]*masterdata.Character{
			1: {ID: 1, Unit: "idol"},
			2: {ID: 2, Unit: "piapro"},
			3: {ID: 3, Unit: "piapro"},
		},
	}
	characters.charOnce.Do(func() {})

	provider := &localCardProvider{
		characters: characters,
		cardAll: []*masterdata.Card{
			{ID: 101, CharacterID: 1, SupportUnit: ""},
			{ID: 102, CharacterID: 2, SupportUnit: "idol"},
			{ID: 103, CharacterID: 3, SupportUnit: ""},
		},
	}
	provider.cardsOnce = sync.Once{}
	provider.cardsOnce.Do(func() {})

	results, err := provider.Filter(&CardFilter{MainUnit: "idol", SupportUnit: "none"})
	if err != nil {
		t.Fatalf("Filter(oc) error = %v", err)
	}
	if len(results) != 1 || results[0].ID != 101 {
		t.Fatalf("unexpected oc filter results: %+v", results)
	}

	results, err = provider.Filter(&CardFilter{MainUnit: "piapro", SupportUnit: "none"})
	if err != nil {
		t.Fatalf("Filter(pure vs) error = %v", err)
	}
	if len(results) != 1 || results[0].ID != 103 {
		t.Fatalf("unexpected pure-vs filter results: %+v", results)
	}
}
