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

func TestSkipTLSVerifyIsOnlyTheDefaultForDevelopmentHosts(t *testing.T) {
	yes, no := true, false

	cases := []struct {
		name string
		cfg  config.ServerConfig
		want bool
	}{
		{"public host is verified", config.ServerConfig{APIURL: "https://monitor.example.com/api"}, false},
		{"public host with a port is verified", config.ServerConfig{APIURL: "https://monitor.example.com:8443/api"}, false},
		{"LAN address is verified", config.ServerConfig{APIURL: "https://192.168.1.20/api"}, false},
		{"a name merely containing 'test' is verified", config.ServerConfig{APIURL: "https://attest.example.com/api"}, false},
		{"empty URL is verified", config.ServerConfig{}, false},
		{"localhost is a dev host", config.ServerConfig{APIURL: "https://localhost:8000/api"}, true},
		{"loopback address is a dev host", config.ServerConfig{APIURL: "http://127.0.0.1:8000/api"}, true},
		{"ipv6 loopback is a dev host", config.ServerConfig{APIURL: "https://[::1]/api"}, true},
		{".test is a dev host", config.ServerConfig{APIURL: "https://monitor-cloudd.test/api"}, true},
		{".local is a dev host", config.ServerConfig{APIURL: "https://monitor.local/api"}, true},
		{"explicit true wins on a public host", config.ServerConfig{APIURL: "https://monitor.example.com/api", InsecureSkipVerify: &yes}, true},
		{"explicit false wins on a dev host", config.ServerConfig{APIURL: "https://monitor-cloudd.test/api", InsecureSkipVerify: &no}, false},
	}

	for _, c := range cases {
		if got := c.cfg.SkipTLSVerify(); got != c.want {
			t.Errorf("%s: SkipTLSVerify() = %v, want %v", c.name, got, c.want)
		}
	}
}
