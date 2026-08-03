package queue

import (
	"context"
	"io/ioutil"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/database"
)

func TestQueueService(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "queue_test_*")
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
	svc := NewQueueService(db, logger)

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Failed to start QueueService: %v", err)
	}

	id, err := svc.Enqueue(ctx, "app", `{"app_name": "test.exe"}`)
	if err != nil || id <= 0 {
		t.Fatalf("Failed to enqueue item: %v", err)
	}

	pending, err := svc.GetPendingQueue(ctx, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("Expected 1 pending queue item, got %d (err: %v)", len(pending), err)
	}

	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop QueueService: %v", err)
	}
}
