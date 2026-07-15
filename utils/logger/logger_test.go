package logger

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

var errWriterFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriterFailed
}

func TestMultiWriterContinuesAfterWriterFailure(t *testing.T) {
	var healthy bytes.Buffer
	writer := NewMultiWriter(failingWriter{}, &healthy)

	n, err := writer.Write([]byte("still delivered"))
	if !errors.Is(err, errWriterFailed) {
		t.Fatal("expected the first writer error")
	}
	if n != len("still delivered") {
		t.Fatalf("written = %d", n)
	}
	if healthy.String() != "still delivered" {
		t.Fatalf("healthy writer did not receive data: %q", healthy.String())
	}
}

func TestNewLoggerFromGlobalUsesLatestGlobalConfig(t *testing.T) {
	previousLevel := GetGlobalLogLevel()
	previousWriter := getGlobalFileWriter()
	t.Cleanup(func() {
		SetGlobalLogLevel(previousLevel)
		SetGlobalFileWriter(previousWriter)
	})

	var before bytes.Buffer
	SetGlobalLogLevel("INFO")
	SetGlobalFileWriter(&before)

	logger := NewLoggerFromGlobal("global-test")

	var after bytes.Buffer
	SetGlobalLogLevel("DEBUG")
	SetGlobalFileWriter(&after)

	logger.Debugf("debug line")

	if before.Len() != 0 {
		t.Fatalf("expected original writer to stay unused, got %q", before.String())
	}
	if !strings.Contains(after.String(), "debug line") {
		t.Fatalf("expected updated writer to receive log line, got %q", after.String())
	}
}

func TestNewLoggerFromGlobalDoesNotDuplicateWrites(t *testing.T) {
	previousLevel := GetGlobalLogLevel()
	previousWriter := getGlobalFileWriter()
	t.Cleanup(func() {
		SetGlobalLogLevel(previousLevel)
		SetGlobalFileWriter(previousWriter)
	})

	var buf bytes.Buffer
	SetGlobalLogLevel("INFO")
	SetGlobalFileWriter(&buf)

	logger := NewLoggerFromGlobal("global-test")
	logger.Infof("single line")

	if count := strings.Count(buf.String(), "single line"); count != 1 {
		t.Fatalf("expected one log line, got %d in %q", count, buf.String())
	}
}

func TestLoggerUsesCanonicalStructuredFormat(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger("render.card", "INFO", &buf)
	ctx := WithContextAttrs(context.Background(), slog.String("request_id", "req-1"))

	log.InfoContext(ctx, "card box completed", "cards", 722, "duration_ms", 12.5)

	line := buf.String()
	for _, want := range []string{
		"level=INFO",
		`msg="card box completed"`,
		"component=render.card",
		"cards=722",
		"duration_ms=12.5",
		"request_id=req-1",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("canonical log line %q does not contain %q", line, want)
		}
	}
	if !strings.HasSuffix(line, "\n") {
		t.Fatalf("log line must end in a newline: %q", line)
	}
}

func TestTelemetryLoggerIgnoresStricterGlobalLevel(t *testing.T) {
	previousLevel := GetGlobalLogLevel()
	previousWriter := getGlobalFileWriter()
	t.Cleanup(func() {
		SetGlobalLogLevel(previousLevel)
		SetGlobalFileWriter(previousWriter)
	})

	var buf bytes.Buffer
	SetGlobalLogLevel("ERROR")
	SetGlobalFileWriter(&buf)

	NewLoggerWithGlobalWriter("Command", "INFO").Info("command completed", "duration_ms", 1)

	if !strings.Contains(buf.String(), "command completed") {
		t.Fatalf("expected fixed-level telemetry log, got %q", buf.String())
	}
}

func TestCommandLoggerUsesDedicatedWriter(t *testing.T) {
	previousLevel := GetGlobalLogLevel()
	previousWriter := getGlobalFileWriter()
	commandWriterMu.RLock()
	previousCommandWriter := commandFileWriter
	commandWriterMu.RUnlock()
	t.Cleanup(func() {
		SetGlobalLogLevel(previousLevel)
		SetGlobalFileWriter(previousWriter)
		SetCommandWriter(previousCommandWriter)
	})

	var ordinary bytes.Buffer
	var command bytes.Buffer
	SetGlobalLogLevel("INFO")
	SetGlobalFileWriter(&ordinary)
	SetCommandWriter(&command)

	NewLoggerWithCommandWriter("Command", "INFO").Info("bot command completed", "event", "bot_command")

	if ordinary.Len() != 0 {
		t.Fatalf("command summary leaked into ordinary sink: %q", ordinary.String())
	}
	if got := command.String(); !strings.Contains(got, "event=bot_command") {
		t.Fatalf("dedicated command sink missing summary: %q", got)
	}
}

func TestInstallStandardHandlersUnifiesSlogAndStandardLog(t *testing.T) {
	previousLevel := GetGlobalLogLevel()
	previousWriter := getGlobalFileWriter()
	previousSlog := slog.Default()
	previousStandardWriter := stdlog.Writer()
	previousStandardFlags := stdlog.Flags()
	previousStandardPrefix := stdlog.Prefix()
	t.Cleanup(func() {
		SetGlobalLogLevel(previousLevel)
		SetGlobalFileWriter(previousWriter)
		slog.SetDefault(previousSlog)
		stdlog.SetOutput(previousStandardWriter)
		stdlog.SetFlags(previousStandardFlags)
		stdlog.SetPrefix(previousStandardPrefix)
	})

	var buf bytes.Buffer
	SetGlobalLogLevel("INFO")
	SetGlobalFileWriter(&buf)
	InstallStandardHandlers()

	stdlog.Print("legacy log")
	slog.Info("slog log")

	output := buf.String()
	if strings.Count(output, "level=INFO") != 2 {
		t.Fatalf("expected two canonical lines, got %q", output)
	}
	for _, want := range []string{"component=Haruki", `msg="legacy log"`, `msg="slog log"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("unified output %q does not contain %q", output, want)
		}
	}
}

func TestGlobalWriterSerializesIndependentLoggers(t *testing.T) {
	previousWriter := getGlobalFileWriter()
	t.Cleanup(func() { SetGlobalFileWriter(previousWriter) })

	var output bytes.Buffer
	SetGlobalFileWriter(&output)
	first := NewLoggerFromGlobal("first")
	second := NewLoggerFromGlobal("second")
	const writes = 100
	var group sync.WaitGroup
	for i := range writes {
		group.Add(2)
		go func() {
			defer group.Done()
			first.Info("line", "index", i)
		}()
		go func() {
			defer group.Done()
			second.Info("line", "index", i)
		}()
	}
	group.Wait()

	if lines := strings.Count(output.String(), "\n"); lines != writes*2 {
		t.Fatalf("log lines = %d, want %d", lines, writes*2)
	}
}
