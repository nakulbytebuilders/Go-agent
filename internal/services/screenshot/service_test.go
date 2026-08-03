package screenshot_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/services/screenshot"
)

func TestTakeScreenshot(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.ScreenshotConfig{
		IntervalSec: 60,
		Quality:     80,
	}

	svc := screenshot.NewScreenshotService(nil, cfg, logger)
	rec, err := svc.TakeScreenshot(context.Background())
	if err != nil {
		t.Fatalf("TakeScreenshot failed: %v", err)
	}

	if rec == nil {
		t.Fatal("Expected non-nil record")
	}

	t.Logf("Screenshot created: %s, size: %d, %dx%d", rec.FilePath, rec.FileSize, rec.Width, rec.Height)

	if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
		t.Fatalf("File does not exist: %s", rec.FilePath)
	}
}

func TestScreenshotIntervalChange(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.ScreenshotConfig{
		IntervalSec: 60,
		Quality:     80,
	}

	svc := screenshot.NewScreenshotService(nil, cfg, logger)
	if svc.GetInterval() != 60 {
		t.Fatalf("Expected default interval 60, got %d", svc.GetInterval())
	}

	svc.SetInterval(300)
	if svc.GetInterval() != 300 {
		t.Fatalf("Expected updated interval 300, got %d", svc.GetInterval())
	}

	svc.SetInterval(600)
	if svc.GetInterval() != 600 {
		t.Fatalf("Expected updated interval 600, got %d", svc.GetInterval())
	}
}

