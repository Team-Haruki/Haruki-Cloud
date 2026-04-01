package main

import (
	"io"
	"os"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"
)

func setupLogging() io.Writer {
	harukiConfig.LoadConfig("haruki-db-configs.yaml")
	loggerWriter := io.Writer(os.Stdout)

	if harukiConfig.Cfg.Backend.MainLogFile != "" {
		logFile, err := harukiLogger.OpenLogFile(harukiConfig.Cfg.Backend.MainLogFile)
		if err != nil {
			tmpLogger := harukiLogger.NewLogger("Main", harukiConfig.Cfg.Backend.LogLevel, os.Stdout)
			tmpLogger.Errorf("failed to open main log file: %v", err)
			os.Exit(1)
		}
		loggerWriter = harukiLogger.NewMultiWriter(os.Stdout, logFile)
	}

	harukiLogger.SetGlobalLogLevel(harukiConfig.Cfg.Backend.LogLevel)
	harukiLogger.SetGlobalFileWriter(loggerWriter)
	return loggerWriter
}

func logStartupInfo(mainLogger *harukiLogger.Logger) {
	mainLogger.Infof("========================= Haruki Database Backend %s =========================", Version)
	mainLogger.Infof("Powered By Haruki Dev Team")
	mainLogger.Infof("Haruki Suite Backend Main Access Log Level: %s", harukiConfig.Cfg.Backend.LogLevel)
	mainLogger.Infof("Haruki Suite Backend Main Access Log Save Path: %s", harukiConfig.Cfg.Backend.MainLogFile)
	mainLogger.Infof("Go Fiber Access Log Save Path: %s", harukiConfig.Cfg.Backend.AccessLogPath)
}
