package card

import "testing"

func TestExtractSupplyKeywords(t *testing.T) {
	extractor := NewExtractor(nil)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "limited", input: "限定", want: SupplyLimited},
		{name: "term limited", input: "期间限定", want: SupplyLimited},
		{name: "festival", input: "fes", want: SupplyFes},
		{name: "cfes", input: "cfes", want: SupplyCFes},
		{name: "bfes", input: "bfes", want: SupplyBFes},
		{name: "collab", input: "联动限定", want: SupplyCollab},
		{name: "normal short", input: "非限", want: SupplyNormal},
		{name: "normal full", input: "非限定", want: SupplyNormal},
		{name: "birthday", input: "生日", want: SupplyBirthday},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractor.ExtractSupply(tc.input)
			if !result.Found {
				t.Fatalf("ExtractSupply(%q) did not match", tc.input)
			}
			if result.Value != tc.want {
				t.Fatalf("ExtractSupply(%q) = %q, want %q", tc.input, result.Value, tc.want)
			}
		})
	}
}

func TestMatchesRawSupplyFilter(t *testing.T) {
	cases := []struct {
		name   string
		filter string
		raw    string
		want   bool
	}{
		{name: "limited matches term", filter: SupplyLimited, raw: "term_limited", want: true},
		{name: "limited matches wl", filter: SupplyLimited, raw: "unit_event_limited", want: true},
		{name: "limited matches collab", filter: SupplyLimited, raw: "collaboration_limited", want: true},
		{name: "limited matches cfes", filter: SupplyLimited, raw: "colorful_festival_limited", want: true},
		{name: "limited matches bfes", filter: SupplyLimited, raw: "bloom_festival_limited", want: true},
		{name: "fes matches cfes", filter: SupplyFes, raw: "colorful_festival_limited", want: true},
		{name: "fes matches bfes", filter: SupplyFes, raw: "bloom_festival_limited", want: true},
		{name: "cfes matches cfes", filter: SupplyCFes, raw: "colorful_festival_limited", want: true},
		{name: "cfes excludes bfes", filter: SupplyCFes, raw: "bloom_festival_limited", want: false},
		{name: "bfes matches bfes", filter: SupplyBFes, raw: "bloom_festival_limited", want: true},
		{name: "bfes excludes cfes", filter: SupplyBFes, raw: "colorful_festival_limited", want: false},
		{name: "collab matches collab", filter: SupplyCollab, raw: "collaboration_limited", want: true},
		{name: "collab excludes term", filter: SupplyCollab, raw: "term_limited", want: false},
		{name: "fes excludes term", filter: SupplyFes, raw: "term_limited", want: false},
		{name: "normal matches normal", filter: SupplyNormal, raw: "normal", want: true},
		{name: "normal excludes limited", filter: SupplyNormal, raw: "term_limited", want: false},
		{name: "birthday matches birthday", filter: SupplyBirthday, raw: "birthday", want: true},
		{name: "birthday excludes normal", filter: SupplyBirthday, raw: "normal", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesRawSupplyFilter(tc.filter, tc.raw)
			if got != tc.want {
				t.Fatalf("matchesRawSupplyFilter(%q, %q) = %t, want %t", tc.filter, tc.raw, got, tc.want)
			}
		})
	}
}
