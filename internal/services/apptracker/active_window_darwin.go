//go:build darwin

package apptracker

import (
	"os/exec"
	"strconv"
	"strings"
)

type ActiveWindowInfo struct {
	AppName     string
	WindowTitle string
	PID         int32
}

// activeWindowScript asks System Events for the frontmost app, its PID and
// (best-effort) its front window title, in a single osascript call to avoid
// spawning multiple processes per poll. Reading the window title requires
// the caller to have Accessibility permission granted in System Settings ->
// Privacy & Security -> Accessibility; if not granted, title is left blank
// and we fall back to the app name.
const activeWindowScript = `
tell application "System Events"
	set frontApp to first application process whose frontmost is true
	set appName to name of frontApp
	set appPID to unix id of frontApp
	set winTitle to ""
	try
		set winTitle to name of front window of frontApp
	end try
end tell
return appName & "|||" & appPID & "|||" & winTitle
`

func getActiveWindowInfo() (ActiveWindowInfo, error) {
	out, err := exec.Command("osascript", "-e", activeWindowScript).Output()
	if err != nil {
		return ActiveWindowInfo{AppName: "System", WindowTitle: "Desktop / Idle", PID: 0}, nil
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "|||", 3)
	if len(parts) < 2 {
		return ActiveWindowInfo{AppName: "System", WindowTitle: "Desktop / Idle", PID: 0}, nil
	}

	appName := strings.TrimSpace(parts[0])
	pid, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	title := ""
	if len(parts) == 3 {
		title = strings.TrimSpace(parts[2])
	}
	if title == "" {
		title = appName
	}

	return ActiveWindowInfo{AppName: appName, WindowTitle: title, PID: int32(pid)}, nil
}
