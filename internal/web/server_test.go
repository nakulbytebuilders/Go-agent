package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/controller"
)

func TestWebServerEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent_web_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_agent.db")
	cfgPath := filepath.Join(tmpDir, "agent.yaml")

	cfg := config.DefaultConfig()
	cfg.Database.Path = dbPath
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	ctl, err := controller.NewAgentController(cfgPath)
	if err != nil {
		t.Fatalf("failed to initialize AgentController: %v", err)
	}
	defer ctl.Shutdown()

	webCfg := config.WebServerConfig{
		Enabled:  true,
		Host:     "127.0.0.1",
		Port:     9898,
		AutoOpen: false,
	}

	ws := NewWebServer(ctl, webCfg, nil)

	// Test GET /api/services/status
	req := httptest.NewRequest(http.MethodGet, "/api/services/status", nil)
	w := httptest.NewRecorder()
	ws.handleServicesStatus(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %d", resp.StatusCode)
	}

	var statuses map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		t.Errorf("failed to decode JSON response: %v", err)
	}

	// Test GET /api/screenshots
	reqSS := httptest.NewRequest(http.MethodGet, "/api/screenshots?limit=5", nil)
	wSS := httptest.NewRecorder()
	ws.handleGetScreenshots(wSS, reqSS)

	respSS := wSS.Result()
	if respSS.StatusCode != http.StatusOK {
		t.Errorf("expected status OK for screenshots, got %d", respSS.StatusCode)
	}

	// Test GET /api/screenshots/interval
	reqInt := httptest.NewRequest(http.MethodGet, "/api/screenshots/interval", nil)
	wInt := httptest.NewRecorder()
	ws.handleScreenshotInterval(wInt, reqInt)

	respInt := wInt.Result()
	if respInt.StatusCode != http.StatusOK {
		t.Errorf("expected status OK for screenshot interval, got %d", respInt.StatusCode)
	}

	// Test Server Start and Stop
	if err := ws.Start(); err != nil {
		t.Fatalf("failed to start web server: %v", err)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ws.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop web server: %v", err)
	}
}
