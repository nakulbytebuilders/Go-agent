package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monitoring-agent/agent/internal/config"
)

func TestConfigLoadAndSave(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfgPath := filepath.Join(tempDir, "agent.yaml")

	// Test 1: Load missing config creates default
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	if !cfg.Services.AppTracker || !cfg.Services.Screenshot {
		t.Errorf("Expected default services to be enabled")
	}

	// Test 2: Modify and Save
	cfg.Services.Screenshot = false
	cfg.Server.APIURL = "https://custom.api.com"

	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Test 3: Reload modified config
	reloaded, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if reloaded.Services.Screenshot != false {
		t.Errorf("Expected Screenshot service to be false, got true")
	}
	if reloaded.Server.APIURL != "https://custom.api.com" {
		t.Errorf("Expected APIURL 'https://custom.api.com', got '%s'", reloaded.Server.APIURL)
	}
}
