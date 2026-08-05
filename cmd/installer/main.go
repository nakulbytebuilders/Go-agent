package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

//go:embed agent.exe
var embeddedAgentBytes []byte

//go:embed uninstaller.exe
var embeddedUninstallerBytes []byte

//go:embed watchdog.exe
var embeddedWatchdogBytes []byte

type OverlayConfig struct {
	ServerURL    string `json:"server_url"`
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
}

func showMsgBox(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	procMessageBoxW := user32.NewProc("MessageBoxW")
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), 0)
}

func parseOverlayConfig() (*OverlayConfig, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	// 1. Check for adjacent installer_config.json in same folder
	exeDir := filepath.Dir(exePath)
	cfgFile := filepath.Join(exeDir, "installer_config.json")
	if cfgData, err := os.ReadFile(cfgFile); err == nil {
		var cfg OverlayConfig
		if err := json.Unmarshal(cfgData, &cfg); err == nil && cfg.EmployeeID != "" {
			return &cfg, nil
		}
	}

	// 2. Check for binary overlay footer bytes
	data, err := os.ReadFile(exePath)
	if err == nil {
		startMarker := []byte("WSNTL_CFG_START")
		endMarker := []byte("WSNTL_CFG_END!")

		startIndex := bytes.Index(data, startMarker)
		endIndex := bytes.Index(data, endMarker)

		if startIndex != -1 && endIndex != -1 && endIndex > startIndex {
			jsonBytes := data[startIndex+len(startMarker) : endIndex]
			var cfg OverlayConfig
			if err := json.Unmarshal(jsonBytes, &cfg); err == nil {
				return &cfg, nil
			}
		}
	}

	// 3. Fallback UUID match in filename
	filename := filepath.Base(exePath)
	uuidRegex := regexp.MustCompile(`([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)
	match := uuidRegex.FindString(filename)
	if match != "" {
		return &OverlayConfig{
			ServerURL:  "http://monitor-cloudd.test/api",
			EmployeeID: match,
		}, nil
	}

	return nil, fmt.Errorf("no overlay config found")
}

func main() {
	overlay, err := parseOverlayConfig()

	var serverURL, empKey, empName string

	if err == nil && overlay != nil {
		serverURL = overlay.ServerURL
		empKey = overlay.EmployeeID
		empName = overlay.EmployeeName
	} else {
		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Enter Server API URL (default: http://monitor-cloudd.test/api): ")
		serverURL, _ = reader.ReadString('\n')
		serverURL = strings.TrimSpace(serverURL)
		if serverURL == "" {
			serverURL = "http://monitor-cloudd.test/api"
		}

		fmt.Print("Enter Employee Key / Connection ID: ")
		empKey, _ = reader.ReadString('\n')
		empKey = strings.TrimSpace(empKey)

		if empKey == "" {
			empKey = "03d06c36-3882-4976-905c-864b2975c065"
		}
	}

	machName, _ := os.Hostname()
	if machName == "" {
		machName = "BYTE_BUILDERS"
	}

	appDataDir := os.Getenv("APPDATA")
	if appDataDir == "" {
		appDataDir = "C:\\Users\\Public"
	}

	installDir := filepath.Join(appDataDir, "MonitoringAgent")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		showMsgBox("Installation Error", fmt.Sprintf("Failed to create installation directory: %v", err))
		os.Exit(1)
	}

	targetAgentPath := filepath.Join(installDir, "agent.exe")
	if err := os.WriteFile(targetAgentPath, embeddedAgentBytes, 0755); err != nil {
		showMsgBox("Installation Error", fmt.Sprintf("Failed to write agent executable: %v", err))
		os.Exit(1)
	}

	configDir := filepath.Join(installDir, "configs")
	_ = os.MkdirAll(configDir, 0755)

	configFilePath := filepath.Join(configDir, "agent.yaml")

	yamlContent := fmt.Sprintf(`server:
  api_url: "%s"
  heartbeat_interval_sec: 15
  employee_id: "%s"
  machine_name: "%s"

web_server:
  enabled: true
  host: "0.0.0.0"
  port: 8080
  auto_open: false

database:
  path: "data/agent.db"
  max_open_conns: 1
  max_idle_conns: 1

logger:
  dir: "logs"
  level: "info"
  max_size_mb: 10
  max_backups: 5
  max_age_days: 30
  compress: true

app_tracker:
  enabled: true
  poll_interval_sec: 1

browser_tracker:
  enabled: true
  poll_interval_sec: 1

screenshot:
  enabled: true
  interval_sec: 30
  quality: 80
  storage_dir: "data/screenshots"

input:
  enabled: true
  poll_interval_sec: 1
  idle_threshold_sec: 60

sync:
  enabled: true
  interval_sec: 5
  batch_size: 50
`, serverURL, empKey, machName)

	if err := os.WriteFile(configFilePath, []byte(yamlContent), 0644); err != nil {
		showMsgBox("Installation Error", fmt.Sprintf("Failed to write agent configuration: %v", err))
		os.Exit(1)
	}

	targetUninstallerPath := filepath.Join(installDir, "uninstaller.exe")
	if len(embeddedUninstallerBytes) > 0 {
		_ = os.WriteFile(targetUninstallerPath, embeddedUninstallerBytes, 0755)
	}

	targetWatchdogPath := filepath.Join(installDir, "watchdog.exe")
	if len(embeddedWatchdogBytes) > 0 {
		_ = os.WriteFile(targetWatchdogPath, embeddedWatchdogBytes, 0755)
	}

	// 1. Native Windows Registry Auto-Start Keys for Agent and Watchdog
	startCmdStr := fmt.Sprintf(`"%s" -config "%s"`, targetAgentPath, configFilePath)
	watchdogStartCmdStr := fmt.Sprintf(`"%s" -config "%s" -agent "%s"`, targetWatchdogPath, configFilePath, targetAgentPath)

	if runKey, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE); err == nil {
		_ = runKey.SetStringValue("WinSentinelAgent", startCmdStr)
		_ = runKey.SetStringValue("WinSentinelWatchdog", watchdogStartCmdStr)
		_ = runKey.Close()
	}

	// 2. Native Windows Registry Add/Remove Programs Key
	if uninstallKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`, registry.ALL_ACCESS); err == nil {
		_ = uninstallKey.SetStringValue("DisplayName", "WinSentinel Monitoring Agent")
		_ = uninstallKey.SetStringValue("DisplayVersion", "1.0.0")
		_ = uninstallKey.SetStringValue("Publisher", "WinSentinel")
		_ = uninstallKey.SetStringValue("UninstallString", fmt.Sprintf(`"%s"`, targetUninstallerPath))
		_ = uninstallKey.SetStringValue("DisplayIcon", targetAgentPath)
		_ = uninstallKey.SetStringValue("InstallLocation", installDir)
		_ = uninstallKey.SetDWordValue("NoModify", 1)
		_ = uninstallKey.SetDWordValue("NoRepair", 1)
		_ = uninstallKey.Close()
	}

	// Launch Agent Service
	cmdAgent := exec.Command(targetAgentPath, "-config", configFilePath)
	cmdAgent.Dir = installDir
	cmdAgent.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000008,
	}
	_ = cmdAgent.Start()

	// Launch Watchdog Process
	if len(embeddedWatchdogBytes) > 0 {
		cmdWatchdog := exec.Command(targetWatchdogPath, "-config", configFilePath, "-agent", targetAgentPath)
		cmdWatchdog.Dir = installDir
		cmdWatchdog.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: 0x08000000 | 0x00000008,
		}
		_ = cmdWatchdog.Start()
	}

	msg := fmt.Sprintf("WinSentinel Monitoring Agent installed and connected successfully!\n\nUser: %s\nConnection Key: %s\nMachine: %s\n\nRunning 100%% silently in background with active Watchdog protection.", empName, empKey, machName)
	showMsgBox("Installation Successful", msg)
}
