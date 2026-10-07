//go:build linux

package input

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// lockPollThrottle limits how often loginctl is spawned; the caller asks every
// second.
const lockPollThrottle = 5 * time.Second

var lockCache struct {
	mu        sync.Mutex
	checkedAt time.Time
	locked    bool
}

// isScreenLocked asks systemd-logind whether the current session is locked.
// Desktop screen lockers (GNOME, KDE, ...) set the session's LockedHint. If
// loginctl is missing or fails, the screen is reported as not locked.
func isScreenLocked() bool {
	lockCache.mu.Lock()
	defer lockCache.mu.Unlock()

	if !lockCache.checkedAt.IsZero() && time.Since(lockCache.checkedAt) < lockPollThrottle {
		return lockCache.locked
	}
	lockCache.checkedAt = time.Now()
	lockCache.locked = querySessionLockedHint()
	return lockCache.locked
}

func querySessionLockedHint() bool {
	session := os.Getenv("XDG_SESSION_ID")
	if session == "" {
		session = "self"
	}
	out, err := exec.Command("loginctl", "show-session", session, "-p", "LockedHint", "--value").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "yes"
}
