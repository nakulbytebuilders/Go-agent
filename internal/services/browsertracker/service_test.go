package browsertracker

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

func TestBrowserTrackerService(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "browsertracker_test_*")
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
	bCfg := config.BrowserTrackerConfig{
		PollIntervalSec: 1,
	}

	svc := NewBrowserTrackerService(db, bCfg, logger)

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Failed to start BrowserTracker service: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	activeBrowser := svc.GetCurrentActiveBrowser()
	t.Logf("Active browser: %s (%s - %s)", activeBrowser.BrowserName, activeBrowser.Domain, activeBrowser.TabTitle)

	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop BrowserTracker service: %v", err)
	}

	records, err := db.GetRecentBrowserActivities(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to query browser activities: %v", err)
	}

	t.Logf("Browser tracker test completed. Logged records count: %d", len(records))
}
