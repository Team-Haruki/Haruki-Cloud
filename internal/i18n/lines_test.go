package i18n

import "testing"

func TestLinesTextSkipsZeroMessages(t *testing.T) {
	got := LinesText([]Message{Verbatim("a"), {}, Verbatim("b")})
	if got != "a\nb" {
		t.Fatalf("LinesText() = %q", got)
	}
	if LinesText(nil) != "" {
		t.Fatal("LinesText(nil) should be empty")
	}
}

func TestDecimal(t *testing.T) {
	for _, tt := range []struct {
		value    float64
		decimals int
		want     string
	}{{2.88, 3, "2.88"}, {5, 1, "5"}, {3.6849, 2, "3.68"}, {-0.001, 1, "0"}} {
		if got := Decimal(tt.value, tt.decimals); got != tt.want {
			t.Errorf("Decimal(%v, %d) = %q, want %q", tt.value, tt.decimals, got, tt.want)
		}
	}
}
