package server

import (
	"io"
	"os"
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/version"
	harukiLogger "haruki-cloud/utils/logger"
)

const defaultConfigPath = "haruki-cloud.yaml"

var mainLogFileHandle *os.File

func resolveConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("HARUKI_CONFIG_PATH")); path != "" {
		return path
	}
	return defaultConfigPath
}

func setupLogging() io.Writer {
	harukiConfig.LoadConfig(resolveConfigPath())
	loggerWriter := io.Writer(os.Stdout)
	mainLogFileHandle = nil

	if harukiConfig.Cfg.Backend.MainLogFile != "" {
		logFile, err := harukiLogger.OpenLogFile(harukiConfig.Cfg.Backend.MainLogFile)
		if err != nil {
			tmpLogger := harukiLogger.NewLogger("Main", harukiConfig.Cfg.Backend.LogLevel, os.Stdout)
			tmpLogger.Errorf("failed to open main log file: %v", err)
			os.Exit(1)
		}
		mainLogFileHandle = logFile
		loggerWriter = harukiLogger.NewMultiWriter(os.Stdout, logFile)
	}

	harukiLogger.SetGlobalLogLevel(harukiConfig.Cfg.Backend.LogLevel)
	harukiLogger.SetGlobalFileWriter(loggerWriter)
	return loggerWriter
}

func closeMainLogFile(mainLogger *harukiLogger.Logger) {
	if mainLogFileHandle == nil {
		return
	}
	if err := mainLogFileHandle.Close(); err != nil && mainLogger != nil {
		mainLogger.Warnf("failed to close main log file: %v", err)
	}
	mainLogFileHandle = nil
}

func logStartupInfo(mainLogger *harukiLogger.Logger) {
	mainLogger.Infof("========================= Haruki Cloud %s =========================", version.Get())
	mainLogger.Infof("Powered By Haruki Dev Team")
	mainLogger.Infof("Profile: %s", harukiConfig.Cfg.Profile)
	mainLogger.Infof("Log Level: %s", harukiConfig.Cfg.Backend.LogLevel)
	mainLogger.Infof("Main Log Path: %s", harukiConfig.Cfg.Backend.MainLogFile)
	mainLogger.Infof("Access Log Path: %s", harukiConfig.Cfg.Backend.AccessLogPath)
}
