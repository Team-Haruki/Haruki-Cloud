package displaytime

import (
	"testing"
	"time"
)

func TestNormalizeUnixMillisBoundaries(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want int64
	}{
		{name: "negative", in: -1, want: 0},
		{name: "zero", in: 0, want: 0},
		{name: "seconds", in: 1_700_000_000, want: 1_700_000_000_000},
		{name: "millisecond boundary", in: 1_000_000_000_000, want: 1_000_000_000_000},
		{name: "milliseconds", in: 1_700_000_000_123, want: 1_700_000_000_123},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeUnixMillis(test.in); got != test.want {
				t.Fatalf("NormalizeUnixMillis(%d) = %d, want %d", test.in, got, test.want)
			}
		})
	}
}

func TestTimeFromUnixMillisNormalizesSecondsAndLocation(t *testing.T) {
	if got := TimeFromUnixMillis(0, "UTC"); !got.IsZero() {
		t.Fatalf("TimeFromUnixMillis(0) = %v, want zero time", got)
	}

	want := time.Unix(1_700_000_000, 0)
	seconds := TimeFromUnixMillis(1_700_000_000, "UTC")
	if !seconds.Equal(want) || seconds.Location().String() != "UTC" {
		t.Fatalf("seconds conversion = %v in %s", seconds, seconds.Location())
	}

	millis := TimeFromUnixMillis(1_700_000_000_123, DefaultTimeZone)
	if millis.UnixMilli() != 1_700_000_000_123 || millis.Location().String() != DefaultTimeZone {
		t.Fatalf("milliseconds conversion = %v in %s", millis, millis.Location())
	}
}
