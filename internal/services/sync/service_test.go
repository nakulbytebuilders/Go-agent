package sync

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/models"
)

func TestSendBatchToCloudRoutesIdlePeriods(t *testing.T) {
	var body map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agents/agent-1/batch-sync" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("invalid JSON body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewSyncService(nil, config.SyncConfig{}, config.ServerConfig{
		APIURL:  server.URL + "/api",
		AgentID: "agent-1",
		APIKey:  "key",
	}, slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	items := []models.SyncQueueItem{
		{PayloadType: "idle", PayloadJSON: `{"name":"idle","start_time":"2026-09-19T10:00:00+05:30","end_time":"2026-09-19T10:07:00+05:30"}`},
		{PayloadType: "idle", PayloadJSON: `{"name":"locked","start_time":"2026-09-19T12:00:00+05:30","end_time":"2026-09-19T12:30:00+05:30"}`},
		{PayloadType: "idle", PayloadJSON: `{"name":"idle","start_time":"2026-09-19T13:00:00+05:30"}`}, // no end_time: dropped
		{PayloadType: "input", PayloadJSON: `{"keyboard_count":4}`},
	}
	if err := svc.sendBatchToCloud(context.Background(), items); err != nil {
		t.Fatalf("sendBatchToCloud failed: %v", err)
	}

	var idle []map[string]string
	if err := json.Unmarshal(body["idle_activities"], &idle); err != nil {
		t.Fatalf("idle_activities missing or malformed: %v", err)
	}
	if len(idle) != 2 || idle[0]["name"] != "idle" || idle[1]["name"] != "locked" {
		t.Fatalf("expected the two well-formed periods, got %+v", idle)
	}
	if idle[0]["start_time"] != "2026-09-19T10:00:00+05:30" || idle[0]["end_time"] != "2026-09-19T10:07:00+05:30" {
		t.Fatalf("times must be passed through untouched, got %+v", idle[0])
	}

	var metrics []json.RawMessage
	if err := json.Unmarshal(body["metrics"], &metrics); err != nil || len(metrics) != 1 {
		t.Fatalf("idle periods must not leak into metrics, got %s (err=%v)", body["metrics"], err)
	}
}

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
