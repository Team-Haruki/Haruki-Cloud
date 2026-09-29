package cachefill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
)

type sqlStateError string

func (e sqlStateError) Error() string    { return "db error" }
func (e sqlStateError) SQLState() string { return string(e) }

func TestIsMissingTable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New(`pq: relation "honorwords" does not exist`), true},
		{errors.New(`ERROR: relation "honorwords" does not exist (SQLSTATE 42P01)`), true},
		{fmt.Errorf("wrap: %w", sqlStateError("42P01")), true},
		{sqlStateError("42703"), false},
		{errors.New("Error 1146 (42S02): Table 'sekai.honorwords' doesn't exist"), true},
		{errors.New("no such table: honorwords"), true},
		{errors.New(`pq: column "name2" does not exist`), false},
		{errors.New(`pq: column "x" of relation "y" does not exist`), false},
		{errors.New("connection refused"), false},
	}
	for _, tc := range cases {
		if got := IsMissingTable(tc.err); got != tc.want {
			t.Errorf("IsMissingTable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestMissingTableFillIsQuietButNotCached(t *testing.T) {
	var logs bytes.Buffer
	group := Group{Cache: "mysekai", Logger: slog.New(slog.NewTextHandler(&logs, nil))}
	missing := errors.New(`pq: relation "honorwords" does not exist`)
	err := group.Do(context.Background(), "honorWords.json", func(context.Context) error { return missing })
	if !errors.Is(err, missing) {
		t.Fatalf("Do() error = %v", err)
	}
	if !group.Failing("honorWords.json") {
		t.Fatal("missing table fill must back off like any failure")
	}
	if logs.Len() != 0 {
		t.Fatalf("missing table logged at warn level: %s", logs.String())
	}
}
