package database_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/database"
	"github.com/monitoring-agent/agent/internal/models"
)

func TestDatabaseManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "db_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test_agent.db")
	dbCfg := config.DatabaseConfig{
		Path:         dbPath,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	}

	dbMgr, err := database.NewDatabaseManager(dbCfg)
	if err != nil {
		t.Fatalf("Failed to initialize DatabaseManager: %v", err)
	}
	defer dbMgr.Close()

	ctx := context.Background()

	// Test InsertActivity
	act := &models.AppActivity{
		AppName:     "code.exe",
		WindowTitle: "main.go - Go-agent",
		PID:         1234,
		DurationSec: 60,
		StartTime:   time.Now().Add(-time.Minute),
		EndTime:     time.Now(),
	}
	id, err := dbMgr.InsertActivity(ctx, act)
	if err != nil {
		t.Fatalf("InsertActivity failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("Expected positive insert ID, got %d", id)
	}

	// Test Queue Operations
	qItem := &models.SyncQueueItem{
		PayloadType: "app",
		PayloadJSON: `{"app_name":"code.exe"}`,
		RetryCount:  0,
		Status:      "pending",
		LastError:   "",
	}
	qID, err := dbMgr.InsertQueue(ctx, qItem)
	if err != nil {
		t.Fatalf("InsertQueue failed: %v", err)
	}

	items, err := dbMgr.FetchPendingQueue(ctx, 10)
	if err != nil {
		t.Fatalf("FetchPendingQueue failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != qID {
		t.Errorf("Expected 1 pending item with ID %d, got %v", qID, items)
	}

	// Test Queue Delete
	if err := dbMgr.DeleteQueueItem(ctx, qID); err != nil {
		t.Fatalf("DeleteQueueItem failed: %v", err)
	}
	remaining, _ := dbMgr.FetchPendingQueue(ctx, 10)
	if len(remaining) != 0 {
		t.Errorf("Expected 0 pending items after deletion, got %d", len(remaining))
	}
}
