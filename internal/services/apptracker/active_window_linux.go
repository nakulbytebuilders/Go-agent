//go:build linux

package apptracker

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type ActiveWindowInfo struct {
	AppName     string
	WindowTitle string
	PID         int32
}

// getActiveWindowInfo reads the active window via xdotool (X11/XWayland).
// Requires the `xdotool` package to be installed on the host; on Wayland-only
// sessions (no XWayland) this will fail gracefully and report "System".
func getActiveWindowInfo() (ActiveWindowInfo, error) {
	winID, err := activeWindowID()
	if err != nil || winID == "" {
		return ActiveWindowInfo{AppName: "System", WindowTitle: "Desktop / Idle", PID: 0}, nil
	}

	title := ""
	if out, err := exec.Command("xdotool", "getwindowname", winID).Output(); err == nil {
		title = strings.TrimSpace(string(out))
	}

	pid := 0
	if out, err := exec.Command("xdotool", "getwindowpid", winID).Output(); err == nil {
		pid, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	}

	appName := processNameFromPID(pid)
	if appName == "" {
		appName = "Unknown"
	}
	if title == "" {
		title = appName
	}

	return ActiveWindowInfo{AppName: appName, WindowTitle: title, PID: int32(pid)}, nil
}

func activeWindowID() (string, error) {
	out, err := exec.Command("xdotool", "getactivewindow").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// processNameFromPID reads the process comm name directly from procfs,
// avoiding an extra external command per poll.
func processNameFromPID(pid int) string {
	if pid <= 0 {
		return ""
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
