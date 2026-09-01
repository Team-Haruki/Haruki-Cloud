package logger

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestGlobalLoggerWrappersAndGroups(t *testing.T) {
	previous := DefaultLogger
	defer SetDefaultLogger(previous)
	var output bytes.Buffer
	configured := NewLogger("global-wrapper-test", "DEBUG", &output)
	SetDefaultLogger(configured)
	SetDefaultLogger(nil)
	if DefaultLogger != configured {
		t.Fatal("SetDefaultLogger(nil) replaced the configured logger")
	}

	Debugf("debug %d", 1)
	Infof("info %d", 2)
	Warnf("warn %d", 3)
	Errorf("error %d", 4)
	Criticalf("critical %d", 5)
	configured.Exceptionf("exception %d", 6)
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")
	DebugContext(context.Background(), "debug context")
	InfoContext(context.Background(), "info context")
	WarnContext(context.Background(), "warn context")
	ErrorContext(context.Background(), "error context")
	configured.Slog().WithGroup("scope").Info("grouped", "value", 7)

	for _, expected := range []string{
		"debug 1", "info 2", "warn 3", "error 4", "critical 5", "exception 6",
		"debug message", "info message", "warn message", "error message",
		"debug context", "info context", "warn context", "error context", "grouped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("log output does not contain %q: %s", expected, output.String())
		}
	}
}
