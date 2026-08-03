package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server         ServerConfig         `yaml:"server"`
	WebServer      WebServerConfig      `yaml:"web_server"`
	Database       DatabaseConfig       `yaml:"database"`
	Logger         LoggerConfig         `yaml:"logger"`
	Services       ServicesToggleConfig `yaml:"services"`
	AppTracker     AppTrackerConfig     `yaml:"app_tracker"`
	BrowserTracker BrowserTrackerConfig `yaml:"browser_tracker"`
	Screenshot     ScreenshotConfig     `yaml:"screenshot"`
	Input          InputTrackerConfig   `yaml:"input"`
	Sync           SyncConfig           `yaml:"sync"`
}

type WebServerConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	AutoOpen bool   `yaml:"auto_open"`
}

type ServerConfig struct {
	APIURL               string `yaml:"api_url"`
	HeartbeatIntervalSec int    `yaml:"heartbeat_interval_sec"`
	AgentID              string `yaml:"agent_id"`
	APIKey               string `yaml:"api_key"`
	EmployeeID           string `yaml:"employee_id"`
	MachineName          string `yaml:"machine_name"`
}

type DatabaseConfig struct {
	Path         string `yaml:"path"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

type LoggerConfig struct {
	Dir        string `yaml:"dir"`
	Level      string `yaml:"level"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAgeDays int    `yaml:"max_age_days"`
	Compress   bool   `yaml:"compress"`
}

type ServicesToggleConfig struct {
	AppTracker     bool `yaml:"appTracker"`
	BrowserTracker bool `yaml:"browserTracker"`
	Screenshot     bool `yaml:"screenshot"`
	Input          bool `yaml:"input"`
	Sync           bool `yaml:"sync"`
	Queue          bool `yaml:"queue"`
}

type AppTrackerConfig struct {
	PollIntervalSec int `yaml:"poll_interval_sec"`
}

type BrowserTrackerConfig struct {
	PollIntervalSec int `yaml:"poll_interval_sec"`
}

type ScreenshotConfig struct {
	IntervalSec int `yaml:"interval_sec"`
	Quality     int `yaml:"quality"`
}

type InputTrackerConfig struct {
	PollIntervalSec int `yaml:"poll_interval_sec"`
}

type SyncConfig struct {
	IntervalSec int `yaml:"interval_sec"`
	BatchSize   int `yaml:"batch_size"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			APIURL:               "http://localhost:8000/api",
			HeartbeatIntervalSec: 15,
			EmployeeID:           "ss_agent_default",
			MachineName:          "Windows-Desktop",
		},
		WebServer: WebServerConfig{
			Enabled:  true,
			Host:     "0.0.0.0",
			Port:     8080,
			AutoOpen: true,
		},
		Database: DatabaseConfig{
			Path:         "data/agent.db",
			MaxOpenConns: 1,
			MaxIdleConns: 1,
		},
		Logger: LoggerConfig{
			Dir:        "logs",
			Level:      "info",
			MaxSizeMB:  10,
			MaxBackups: 5,
			MaxAgeDays: 30,
			Compress:   true,
		},
		Services: ServicesToggleConfig{
			AppTracker:     true,
			BrowserTracker: true,
			Screenshot:     true,
			Input:          true,
			Sync:           true,
			Queue:          true,
		},
		AppTracker: AppTrackerConfig{
			PollIntervalSec: 2,
		},
		BrowserTracker: BrowserTrackerConfig{
			PollIntervalSec: 3,
		},
		Screenshot: ScreenshotConfig{
			IntervalSec: 60,
			Quality:     80,
		},
		Input: InputTrackerConfig{
			PollIntervalSec: 5,
		},
		Sync: SyncConfig{
			IntervalSec: 30,
			BatchSize:   50,
		},
	}
}

func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath == "" {
		configPath = "configs/agent.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Save default config if missing
			dir := filepath.Dir(configPath)
			if dir != "" && dir != "." {
				_ = os.MkdirAll(dir, 0755)
			}
			if saveErr := SaveConfig(configPath, cfg); saveErr != nil {
				return nil, fmt.Errorf("failed to create default config file at %s: %w", configPath, saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file at %s: %w", configPath, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	return cfg, nil
}

func SaveConfig(configPath string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to yaml: %w", err)
	}

	dir := filepath.Dir(configPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for config: %w", err)
		}
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
