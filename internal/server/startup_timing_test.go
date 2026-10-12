package server

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestStartupTimerLogsPhasesAndTotal(t *testing.T) {
	var output bytes.Buffer
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	timer := newStartupTimerAt(startupTestLogger(&output), clock)

	now = now.Add(120 * time.Millisecond)
	timer.phase("redis")
	now = now.Add(30 * time.Millisecond)
	timer.phase("users_db")
	now = now.Add(50 * time.Millisecond)
	timer.done()

	logs := output.String()
	for _, want := range []string{
		"phase=redis phase_ms=120 elapsed_ms=120",
		"phase=users_db phase_ms=30 elapsed_ms=150",
		"startup complete",
		"elapsed_ms=200",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("startup timer log missing %q:\n%s", want, logs)
		}
	}

	var nilTimer *startupTimer
	nilTimer.phase("noop")
	nilTimer.done()
}
