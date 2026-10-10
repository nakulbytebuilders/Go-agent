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
	BootLogger     *slog.Logger
}

var globalManager *LoggerManager

// bestEffortWriter forwards to w and never reports a failure.
type bestEffortWriter struct{ w io.Writer }

func (b bestEffortWriter) Write(p []byte) (int, error) {
	_, _ = b.w.Write(p)
	return len(p), nil
}

// newTeeWriter writes to the log file and, when there is one, to the console.
//
// agent.exe is built as a Windows GUI program, so when the Run key or the
// watchdog starts it there is no console and every write to os.Stdout fails.
// io.MultiWriter stops at the first writer that fails, so with stdout first the
// file never received a line: agent.log stayed empty after every reboot. The
// file comes first, and a console that is missing cannot fail the write.
func newTeeWriter(file io.Writer, console io.Writer) io.Writer {
	return io.MultiWriter(file, bestEffortWriter{console})
}

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

		handlerOpts := &slog.HandlerOptions{
			Level: level,
		}
		handler := slog.NewJSONHandler(newTeeWriter(rotator, os.Stdout), handlerOpts)
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
		// BootLogger is file-only (no stdout multiwriter): the boot service
		// runs headless under the SCM with no console to write to.
		BootLogger: createFileOnlyLogger("boot.log"),
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

func GetBootLogger() *slog.Logger {
	if globalManager != nil {
		return globalManager.BootLogger
	}
	return slog.Default()
}
