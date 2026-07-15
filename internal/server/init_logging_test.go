package server

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	harukiLogger "haruki-cloud/utils/logger"
)

const fatalStartupChildEnv = "HARUKI_TEST_FATAL_STARTUP_CHILD"

func TestFatalStartupWritesTerminalAndFlushesFileBeforeExit(t *testing.T) {
	if path := os.Getenv(fatalStartupChildEnv); path != "" {
		runFatalStartupChild(path)
		return
	}

	logPath := t.TempDir() + "/fatal.log"
	cmd := exec.Command(os.Args[0], "-test.run=^TestFatalStartupWritesTerminalAndFlushesFileBeforeExit$")
	cmd.Env = append(os.Environ(), fatalStartupChildEnv+"="+logPath)
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("child exit error = %v, want exit code 1", err)
	}
	if got := stderr.String(); strings.Count(got, `msg="child startup failed"`) != 1 || !strings.Contains(got, "level=CRITICAL") || !strings.Contains(got, "event=startup_fatal") {
		t.Fatalf("terminal emergency record missing: %q", got)
	}
	if got := stdout.String(); strings.Contains(got, "child startup failed") {
		t.Fatalf("fatal record was duplicated to stdout: %q", got)
	}
	fileRecord, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read flushed fatal log: %v", err)
	}
	if got := string(fileRecord); strings.Count(got, `msg="child startup failed"`) != 1 || !strings.Contains(got, "level=CRITICAL") {
		t.Fatalf("file fatal record missing: %q", got)
	}
}

func runFatalStartupChild(logPath string) {
	logFile, err := harukiLogger.OpenLogFile(logPath)
	if err != nil {
		os.Exit(2)
	}
	mainLogFileHandle = logFile
	destination := harukiLogger.NewSerializedWriter(harukiLogger.NewMultiWriter(os.Stdout, logFile))
	mainLogAsyncWriter = harukiLogger.NewAsyncWriter(destination, 1)
	commandLogAsyncWriter = harukiLogger.NewPriorityAsyncWriter(destination, 1, time.Millisecond, os.Stderr)
	harukiLogger.SetGlobalFileWriter(mainLogAsyncWriter)
	harukiLogger.SetCommandWriter(commandLogAsyncWriter)
	mainLogger := harukiLogger.NewLogger("Main", "INFO", mainLogAsyncWriter)
	fatalStartup(mainLogger, "child startup failed", "reason", "test")
}
