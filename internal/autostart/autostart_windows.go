package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	registryKeyPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	registryValue   = "MonitoringAgent"
)

// Install registers the agent.exe to auto-start on Windows login.
// It writes to HKCU (current user) so no admin privileges are needed.
func Install(configPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks and get the absolute path
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Resolve config path to absolute
	if !filepath.IsAbs(configPath) {
		exeDir := filepath.Dir(exePath)
		configPath = filepath.Join(exeDir, configPath)
	}
	configPath, _ = filepath.Abs(configPath)

	// Build the command that will run on startup
	// e.g.: "C:\path\to\agent.exe" -config "C:\path\to\configs\agent.yaml"
	startupCmd := fmt.Sprintf(`"%s" -config "%s"`, exePath, configPath)

	// Use reg.exe to add the auto-start entry
	cmd := exec.Command("reg", "add", registryKeyPath,
		"/v", registryValue,
		"/t", "REG_SZ",
		"/d", startupCmd,
		"/f",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set registry value: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// Uninstall removes the agent from Windows auto-start.
func Uninstall() error {
	cmd := exec.Command("reg", "delete", registryKeyPath,
		"/v", registryValue,
		"/f",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If the value doesn't exist, that's fine
		if strings.Contains(string(output), "unable to find") ||
			strings.Contains(string(output), "could not find") {
			return nil
		}
		return fmt.Errorf("failed to delete registry value: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// IsInstalled checks if the agent is currently registered for auto-start.
func IsInstalled() (bool, string) {
	cmd := exec.Command("reg", "query", registryKeyPath,
		"/v", registryValue,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, ""
	}

	// Parse the output to get the value
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, registryValue) {
			// The reg query output format: "    MonitoringAgent    REG_SZ    <value>"
			parts := strings.SplitN(line, "REG_SZ", 2)
			if len(parts) == 2 {
				return true, strings.TrimSpace(parts[1])
			}
			return true, line
		}
	}

	return false, ""
}
