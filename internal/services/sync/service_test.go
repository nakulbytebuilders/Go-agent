package sync

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
)

func TestSyncService(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.SyncConfig{
		IntervalSec: 1,
		BatchSize:   10,
	}
	serverCfg := config.ServerConfig{
		APIURL: "http://127.0.0.1:8000/api",
	}

	svc := NewSyncService(nil, cfg, serverCfg, logger)
	if svc.Name() != "sync" {
		t.Errorf("expected name sync, got %s", svc.Name())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("failed to start sync service: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("failed to stop sync service: %v", err)
	}
}
