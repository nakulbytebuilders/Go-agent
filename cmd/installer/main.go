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
)

//go:embed agent.exe
var embeddedAgentBytes []byte

//go:embed uninstaller.exe
var embeddedUninstallerBytes []byte

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

	data, err := os.ReadFile(exePath)
	if err != nil {
		return nil, err
	}

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

	// 1. Auto-start registry key (registers under both WinSentinelAgent and MonitoringAgent)
	startCmdStr := fmt.Sprintf(`"%s" -config "%s"`, targetAgentPath, configFilePath)
	_ = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "WinSentinelAgent", "/t", "REG_SZ", "/d", startCmdStr, "/f").Run()
	_ = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "MonitoringAgent", "/t", "REG_SZ", "/d", startCmdStr, "/f").Run()

	// 2. Windows Installed Apps (Add or Remove Programs) registry key
	uninstallRegKey := `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`
	_ = exec.Command("reg", "add", uninstallRegKey, "/v", "DisplayName", "/t", "REG_SZ", "/d", "WinSentinel Monitoring Agent", "/f").Run()
	_ = exec.Command("reg", "add", uninstallRegKey, "/v", "DisplayVersion", "/t", "REG_SZ", "/d", "1.0.0", "/f").Run()
	_ = exec.Command("reg", "add", uninstallRegKey, "/v", "Publisher", "/t", "REG_SZ", "/d", "WinSentinel", "/f").Run()
	_ = exec.Command("reg", "add", uninstallRegKey, "/v", "UninstallString", "/t", "REG_SZ", "/d", fmt.Sprintf(`"%s"`, targetUninstallerPath), "/f").Run()
	_ = exec.Command("reg", "add", uninstallRegKey, "/v", "DisplayIcon", "/t", "REG_SZ", "/d", targetAgentPath, "/f").Run()
	_ = exec.Command("reg", "add", uninstallRegKey, "/v", "InstallLocation", "/t", "REG_SZ", "/d", installDir, "/f").Run()
	_ = exec.Command("reg", "add", uninstallRegKey, "/v", "NoModify", "/t", "REG_DWORD", "/d", "1", "/f").Run()
	_ = exec.Command("reg", "add", uninstallRegKey, "/v", "NoRepair", "/t", "REG_DWORD", "/d", "1", "/f").Run()

	_ = exec.Command("taskkill", "/F", "/IM", "agent.exe").Run()

	cmd := exec.Command(targetAgentPath, "-config", configFilePath)
	cmd.Dir = installDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000008,
	}
	_ = cmd.Start()

	msg := fmt.Sprintf("WinSentinel Monitoring Agent installed and connected successfully!\n\nUser: %s\nConnection Key: %s\nMachine: %s\n\nRunning 100%% silently in background.", empName, empKey, machName)
	showMsgBox("Installation Successful", msg)
}
