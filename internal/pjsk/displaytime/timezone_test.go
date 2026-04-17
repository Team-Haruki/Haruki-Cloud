package displaytime

import (
	"slices"
	"testing"
)

func TestResolveUserTimeZoneInputDirectAndAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "direct", input: "Asia/Shanghai", want: "Asia/Shanghai"},
		{name: "alias", input: "CST", want: "Asia/Shanghai"},
		{name: "utc alias", input: "UST", want: "UTC"},
		{name: "offset unique", input: "+5:45", want: "Asia/Kathmandu"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, candidates, err := ResolveUserTimeZoneInput(tt.input)
			if err != nil {
				t.Fatalf("ResolveUserTimeZoneInput(%q) error = %v", tt.input, err)
			}
			if len(candidates) != 0 {
				t.Fatalf("ResolveUserTimeZoneInput(%q) returned candidates: %v", tt.input, candidates)
			}
			if got != tt.want {
				t.Fatalf("ResolveUserTimeZoneInput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveUserTimeZoneInputOffsetAmbiguous(t *testing.T) {
	t.Parallel()

	got, candidates, err := ResolveUserTimeZoneInput("+28800")
	if err != nil {
		t.Fatalf("ResolveUserTimeZoneInput(+28800) error = %v", err)
	}
	if got != "" {
		t.Fatalf("ResolveUserTimeZoneInput(+28800) got resolved timezone %q, want ambiguity", got)
	}
	if len(candidates) < 2 {
		t.Fatalf("ResolveUserTimeZoneInput(+28800) candidates = %v, want multiple matches", candidates)
	}
	if !slices.Contains(candidates, "Asia/Shanghai") {
		t.Fatalf("ResolveUserTimeZoneInput(+28800) candidates = %v, want Asia/Shanghai included", candidates)
	}
}

func TestResolveUserTimeZoneInputInvalid(t *testing.T) {
	t.Parallel()

	if _, _, err := ResolveUserTimeZoneInput("Not/A_Timezone"); err == nil {
		t.Fatal("ResolveUserTimeZoneInput(Not/A_Timezone) expected error, got nil")
	}
}
