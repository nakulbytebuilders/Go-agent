package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/monitoring-agent/agent/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

type LoggerManager struct {
	AgentLogger    *slog.Logger
	WatchdogLogger *slog.Logger
	SyncLogger     *slog.Logger
	ErrorLogger    *slog.Logger
}

var globalManager *LoggerManager

func Init(cfg config.LoggerConfig) (*LoggerManager, error) {
	logDir := cfg.Dir
	if logDir == "" {
		logDir = "logs"
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	createLogger := func(filename string) *slog.Logger {
		rotator := &lumberjack.Logger{
			Filename:   filepath.Join(logDir, filename),
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		}

		// Also write to stdout in debug mode or default console output
		multiWriter := io.MultiWriter(os.Stdout, rotator)

		handlerOpts := &slog.HandlerOptions{
			Level: level,
		}
		handler := slog.NewJSONHandler(multiWriter, handlerOpts)
		return slog.New(handler)
	}

	createFileOnlyLogger := func(filename string) *slog.Logger {
		rotator := &lumberjack.Logger{
			Filename:   filepath.Join(logDir, filename),
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		}
		handlerOpts := &slog.HandlerOptions{
			Level: level,
		}
		handler := slog.NewJSONHandler(rotator, handlerOpts)
		return slog.New(handler)
	}

	lm := &LoggerManager{
		AgentLogger:    createLogger("agent.log"),
		WatchdogLogger: createFileOnlyLogger("watchdog.log"),
		SyncLogger:     createFileOnlyLogger("sync.log"),
		ErrorLogger:    createFileOnlyLogger("error.log"),
	}

	globalManager = lm
	slog.SetDefault(lm.AgentLogger)

	return lm, nil
}

func GetAgentLogger() *slog.Logger {
	if globalManager != nil {
		return globalManager.AgentLogger
	}
	return slog.Default()
}

func GetWatchdogLogger() *slog.Logger {
	if globalManager != nil {
		return globalManager.WatchdogLogger
	}
	return slog.Default()
}

func GetSyncLogger() *slog.Logger {
	if globalManager != nil {
		return globalManager.SyncLogger
	}
	return slog.Default()
}

func GetErrorLogger() *slog.Logger {
	if globalManager != nil {
		return globalManager.ErrorLogger
	}
	return slog.Default()
}
