package input

import (
	"context"
	"io/ioutil"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/database"
)

func TestInputService(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "input_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	dbCfg := config.DatabaseConfig{
		Path:         dbPath,
		MaxOpenConns: 5,
		MaxIdleConns: 2,
	}

	db, err := database.NewDatabaseManager(dbCfg)
	if err != nil {
		t.Fatalf("Failed to initialize database manager: %v", err)
	}
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.InputTrackerConfig{
		PollIntervalSec: 1,
	}

	svc := NewInputService(db, cfg, logger)

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Failed to start InputService: %v", err)
	}

	time.Sleep(1200 * time.Millisecond)

	cur := svc.GetCurrentInputMetrics()
	t.Logf("Live input state: keys=%d, clicks=%d, idle=%ds", cur.KeyboardCount, cur.MouseClicks, cur.IdleTimeSec)

	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop InputService: %v", err)
	}
}
