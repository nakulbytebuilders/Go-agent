//go:build darwin

package input

import (
	"os/exec"
	"strings"
	"sync"
	"time"
)

// lockPollThrottle limits how often ioreg is spawned; the caller asks every
// second.
const lockPollThrottle = 5 * time.Second

var lockCache struct {
	mu        sync.Mutex
	checkedAt time.Time
	locked    bool
}

// isScreenLocked reads the console session's CGSSessionScreenIsLocked flag from
// the IORegistry. If ioreg fails, the screen is reported as not locked.
func isScreenLocked() bool {
	lockCache.mu.Lock()
	defer lockCache.mu.Unlock()

	if !lockCache.checkedAt.IsZero() && time.Since(lockCache.checkedAt) < lockPollThrottle {
		return lockCache.locked
	}
	lockCache.checkedAt = time.Now()
	lockCache.locked = queryScreenIsLocked()
	return lockCache.locked
}

func queryScreenIsLocked() bool {
	out, err := exec.Command("ioreg", "-w0", "-n", "Root", "-d1").Output()
	if err != nil {
		return false
	}
	// The key only appears (as Yes) while the screen is locked. Spacing around
	// "=" differs between macOS versions, so ignore it.
	compact := strings.ReplaceAll(string(out), " ", "")
	return strings.Contains(compact, `"CGSSessionScreenIsLocked"=Yes`)
}
