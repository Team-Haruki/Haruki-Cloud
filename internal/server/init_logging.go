package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"
	"haruki-cloud/version"
)

const defaultConfigPath = "haruki-cloud.yaml"

const (
	logQueueCapacity        = 4096
	commandLogQueueCapacity = 4096
	logFlushTimeout         = 5 * time.Second
)

var mainLogFileHandle *os.File
var mainLogAsyncWriter *harukiLogger.AsyncWriter
var commandLogAsyncWriter *harukiLogger.AsyncWriter
var reliableLogOverflowWriter = harukiLogger.NewSerializedWriter(os.Stderr)

func resolveConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("HARUKI_CONFIG_PATH")); path != "" {
		return path
	}
	return defaultConfigPath
}

func setupLogging() io.Writer {
	// Install the bridge before loading config so startup/configuration failures
	// use the same parseable format as the running service.
	harukiLogger.SetGlobalFileWriter(os.Stdout)
	harukiLogger.SetCommandWriter(nil)
	harukiLogger.SetGlobalLogLevel(harukiConfig.LogLevelInfo)
	harukiLogger.InstallStandardHandlers()
	harukiConfig.LoadConfig(resolveConfigPath())
	loggerDestination := io.Writer(os.Stdout)
	mainLogFileHandle = nil
	mainLogAsyncWriter = nil
	commandLogAsyncWriter = nil

	if harukiConfig.Cfg.Backend.MainLogFile != "" {
		logFile, err := harukiLogger.OpenLogFile(harukiConfig.Cfg.Backend.MainLogFile)
		if err != nil {
			fatalStartup(nil, "failed to open main log file", "error_type", fmt.Sprintf("%T", err))
		}
		mainLogFileHandle = logFile
		loggerDestination = harukiLogger.NewMultiWriter(os.Stdout, logFile)
	}

	serializedDestination := harukiLogger.NewSerializedWriter(loggerDestination)
	mainLogAsyncWriter = harukiLogger.NewReliableAsyncWriter(serializedDestination, logQueueCapacity, reliableLogOverflowWriter)
	commandLogAsyncWriter = harukiLogger.NewReliableAsyncWriter(
		serializedDestination,
		commandLogQueueCapacity,
		reliableLogOverflowWriter,
	)
	harukiLogger.SetGlobalLogLevel(harukiConfig.Cfg.Backend.LogLevel)
	harukiLogger.SetGlobalFileWriter(mainLogAsyncWriter)
	harukiLogger.SetCommandWriter(commandLogAsyncWriter)
	return mainLogAsyncWriter
}

func closeMainLogFile(mainLogger *harukiLogger.Logger) {
	if !flushMainLogWriters(mainLogger) {
		return
	}
	closeMainLogFileHandle()
}

func flushMainLogWriters(mainLogger *harukiLogger.Logger) bool {
	flushComplete := true
	ctx, cancel := context.WithTimeout(context.Background(), logFlushTimeout)
	defer cancel()
	type flushResult struct {
		sink string
		err  error
	}
	flushResults := make(chan flushResult, 2)
	flushCount := 0
	if commandLogAsyncWriter != nil {
		writer := commandLogAsyncWriter
		commandLogAsyncWriter = nil
		harukiLogger.SetCommandWriter(nil)
		flushCount++
		go func() { flushResults <- flushResult{sink: "command", err: writer.CloseContext(ctx)} }()
	}
	if mainLogAsyncWriter != nil {
		if dropped := mainLogAsyncWriter.Dropped(); dropped > 0 && mainLogger != nil {
			mainLogger.Warn("log queue dropped records", "event", "log_queue_drop", "records", dropped)
		}
		writer := mainLogAsyncWriter
		flushCount++
		go func() { flushResults <- flushResult{sink: "main", err: writer.CloseContext(ctx)} }()
		mainLogAsyncWriter = nil
	}
	for range flushCount {
		result := <-flushResults
		if result.err == nil {
			continue
		}
		flushComplete = false
		_ = harukiLogger.WriteEmergency(os.Stderr, "Main", result.sink+" log flush failed",
			"event", result.sink+"_log_flush",
			"error_type", fmt.Sprintf("%T", result.err),
		)
	}
	return flushComplete
}

func closeMainLogFileHandle() {
	if mainLogFileHandle == nil {
		return
	}
	if err := mainLogFileHandle.Close(); err != nil {
		_ = harukiLogger.WriteEmergency(os.Stderr, "Main", "failed to close main log file",
			"event", "main_log_close",
			"error_type", fmt.Sprintf("%T", err),
		)
	}
	mainLogFileHandle = nil
}

// fatalStartup guarantees one canonical terminal record without traversing an
// asynchronous sink, then gives the main/access files a bounded best-effort
// flush before terminating. The direct stderr path is intentionally first so
// a blocked file writer cannot hide the startup failure.
func fatalStartup(mainLogger *harukiLogger.Logger, msg string, args ...any) {
	emergencyAttrs := make([]any, 0, len(args)+2)
	emergencyAttrs = append(emergencyAttrs, "event", "startup_fatal")
	emergencyAttrs = append(emergencyAttrs, args...)
	if err := harukiLogger.WriteEmergency(os.Stderr, "Main", msg, emergencyAttrs...); err != nil {
		_ = harukiLogger.WriteEmergency(os.Stdout, "Main", msg, emergencyAttrs...)
	}
	closeAccessLogFile(mainLogger)
	if flushMainLogWriters(mainLogger) {
		// Both asynchronous workers have stopped, so this file-only write cannot
		// interleave with queued terminal/file records or wait on their lock.
		if mainLogFileHandle != nil {
			_ = harukiLogger.WriteEmergency(mainLogFileHandle, "Main", msg, emergencyAttrs...)
		}
		closeMainLogFileHandle()
	}
	os.Exit(1)
}

func logStartupInfo(mainLogger *harukiLogger.Logger) {
	mainLogger.Info("service starting",
		"version", version.Get(),
		"profile", harukiConfig.Cfg.Profile,
		"log_level", harukiConfig.Cfg.Backend.LogLevel,
		"main_log_path", harukiConfig.Cfg.Backend.MainLogFile,
		"access_log_path", harukiConfig.Cfg.Backend.AccessLogPath,
	)
}
