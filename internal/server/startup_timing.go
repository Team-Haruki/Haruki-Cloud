package server

import (
	"time"

	harukiLogger "haruki-cloud/utils/logger"
)

// startupTimer logs how long each bootstrap phase took, so a slow start can
// be attributed from the main log alone. One Info line per phase and one
// summary; no allocation beyond the log record.
type startupTimer struct {
	logger  *harukiLogger.Logger
	now     func() time.Time
	started time.Time
	last    time.Time
}

func newStartupTimer(logger *harukiLogger.Logger) *startupTimer {
	return newStartupTimerAt(logger, time.Now)
}

func newStartupTimerAt(logger *harukiLogger.Logger, now func() time.Time) *startupTimer {
	start := now()
	return &startupTimer{logger: logger, now: now, started: start, last: start}
}

// phase closes the phase named name: it logs the time since the previous
// phase ended and the time since startup began.
func (t *startupTimer) phase(name string) {
	if t == nil {
		return
	}
	now := t.now()
	t.logger.Info("startup phase complete", "phase", name,
		"phase_ms", now.Sub(t.last).Milliseconds(), "elapsed_ms", now.Sub(t.started).Milliseconds())
	t.last = now
}

// done logs the total bootstrap time; the caller logs it right before the
// listener starts.
func (t *startupTimer) done() {
	if t == nil {
		return
	}
	t.logger.Info("startup complete", "elapsed_ms", t.now().Sub(t.started).Milliseconds())
}
