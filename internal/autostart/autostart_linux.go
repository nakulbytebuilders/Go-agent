//go:build linux

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const serviceName = "winsentinel-agent.service"

func systemdUserDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func serviceFilePath() (string, error) {
	dir, err := systemdUserDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, serviceName), nil
}

// Install registers the agent as a systemd --user service so it starts on
// login, mirroring the Windows HKCU Run-key behavior (per-user, no root).
func Install(configPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(filepath.Dir(exePath), configPath)
	}
	configPath, _ = filepath.Abs(configPath)

	dir, err := systemdUserDir()
	if err != nil {
		return fmt.Errorf("failed to resolve systemd user dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create systemd user dir: %w", err)
	}

	unit := fmt.Sprintf(`[Unit]
Description=WinSentinel Monitoring Agent
After=graphical-session.target

[Service]
Type=simple
ExecStart="%s" -config "%s"
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, exePath, configPath)

	svcPath, err := serviceFilePath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(svcPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("failed to write systemd unit: %w", err)
	}

	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload systemd user daemon: %w\nOutput: %s", err, string(out))
	}
	if out, err := exec.Command("systemctl", "--user", "enable", serviceName).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable systemd service: %w\nOutput: %s", err, string(out))
	}

	return nil
}

// Uninstall removes the agent from systemd --user autostart.
func Uninstall() error {
	_ = exec.Command("systemctl", "--user", "disable", "--now", serviceName).Run()

	svcPath, err := serviceFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(svcPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove systemd unit: %w", err)
	}

	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()

	return nil
}

// IsInstalled checks whether the systemd --user unit is enabled.
func IsInstalled() (bool, string) {
	out, err := exec.Command("systemctl", "--user", "is-enabled", serviceName).CombinedOutput()
	if err != nil {
		return false, ""
	}
	state := strings.TrimSpace(string(out))
	if state == "" || strings.Contains(state, "disabled") {
		return false, ""
	}
	return true, serviceName
}
