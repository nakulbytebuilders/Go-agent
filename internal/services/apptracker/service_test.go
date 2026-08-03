package apptracker

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

func TestAppTrackerService(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "apptracker_test_*")
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
	appCfg := config.AppTrackerConfig{
		PollIntervalSec: 1,
	}

	svc := NewAppTrackerService(db, appCfg, logger)

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Failed to start AppTracker service: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	activeApp := svc.GetCurrentActiveApp()
	if activeApp.AppName == "" {
		t.Errorf("Expected active app name, got empty string")
	}

	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop AppTracker service: %v", err)
	}

	records, err := db.GetRecentAppActivities(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to query app activities: %v", err)
	}

	t.Logf("App tracker test completed. Logged records count: %d, Active app: %s (%s)", len(records), activeApp.AppName, activeApp.WindowTitle)
}
