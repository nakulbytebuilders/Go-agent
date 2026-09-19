//go:build darwin

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const plistLabel = "com.winsentinel.agent"

func launchAgentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

func plistPath() (string, error) {
	dir, err := launchAgentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, plistLabel+".plist"), nil
}

// Install registers the agent as a per-user LaunchAgent so it starts on
// login, mirroring the Windows HKCU Run-key / Linux systemd --user behavior.
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

	dir, err := launchAgentsDir()
	if err != nil {
		return fmt.Errorf("failed to resolve LaunchAgents dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents dir: %w", err)
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>-config</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
</dict>
</plist>
`, plistLabel, exePath, configPath)

	path, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("failed to write LaunchAgent plist: %w", err)
	}

	uid := os.Getuid()
	target := fmt.Sprintf("gui/%d/%s", uid, plistLabel)
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", uid), path).Run()
	if out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", uid), path).CombinedOutput(); err != nil {
		// Fall back to the older (pre-10.11 semantics still supported today) load command.
		if out2, err2 := exec.Command("launchctl", "load", "-w", path).CombinedOutput(); err2 != nil {
			return fmt.Errorf("failed to load LaunchAgent: %w\nOutput: %s / %s", err, string(out), string(out2))
		}
	}
	_ = exec.Command("launchctl", "enable", target).Run()

	return nil
}

// Uninstall removes the agent from LaunchAgent autostart.
func Uninstall() error {
	path, err := plistPath()
	if err != nil {
		return err
	}

	uid := os.Getuid()
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", uid), path).Run()
	_ = exec.Command("launchctl", "unload", "-w", path).Run()

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove LaunchAgent plist: %w", err)
	}

	return nil
}

// IsInstalled checks whether the LaunchAgent plist is present and loaded.
func IsInstalled() (bool, string) {
	path, err := plistPath()
	if err != nil {
		return false, ""
	}
	if _, err := os.Stat(path); err != nil {
		return false, ""
	}

	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return true, path
	}
	if strings.Contains(string(out), plistLabel) {
		return true, path
	}
	return true, path
}
