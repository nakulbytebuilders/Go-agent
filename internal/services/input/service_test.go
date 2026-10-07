package input

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/database"
	"github.com/monitoring-agent/agent/internal/models"
	"github.com/monitoring-agent/agent/internal/services"
)

func TestPresencePeriodsAreQueuedForSync(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := database.NewDatabaseManager(config.DatabaseConfig{
		Path:         filepath.Join(tmpDir, "test.db"),
		MaxOpenConns: 5,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("Failed to initialize database manager: %v", err)
	}
	defer db.Close()

	svc := NewInputService(db, config.InputTrackerConfig{PollIntervalSec: 1}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	// Drive the tracker by hand instead of starting the loop, so the test
	// controls the clock.
	svc.state = services.StateRunning
	svc.presence = newPresenceTracker(IdleThreshold, t0)

	ctx := context.Background()
	svc.mu.Lock()
	svc.observePresenceLocked(ctx, t0.Add(time.Second), true)
	svc.observePresenceLocked(ctx, t0.Add(5*time.Minute), false)
	svc.mu.Unlock()

	items, err := db.FetchPendingQueue(ctx, 10)
	if err != nil {
		t.Fatalf("FetchPendingQueue failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected both the immediate lock event and the final close to be queued, got %+v", items)
	}

	var first models.IdlePeriod
	if err := json.Unmarshal([]byte(items[0].PayloadJSON), &first); err != nil {
		t.Fatalf("payload is not an IdlePeriod: %v", err)
	}
	if first.Name != PeriodLocked || !first.StartTime.Equal(t0.Add(time.Second)) || !first.EndTime.Equal(t0.Add(time.Second)) {
		t.Fatalf("unexpected first queued period: %+v", first)
	}

	var last models.IdlePeriod
	if err := json.Unmarshal([]byte(items[1].PayloadJSON), &last); err != nil {
		t.Fatalf("payload is not an IdlePeriod: %v", err)
	}
	if last.Name != PeriodLocked || !last.StartTime.Equal(t0.Add(time.Second)) || !last.EndTime.Equal(t0.Add(5*time.Minute)) {
		t.Fatalf("unexpected closing queued period: %+v", last)
	}
}

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
