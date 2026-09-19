//go:build darwin

package input

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// InputSnapshot mirrors the Windows snapshot shape. macOS has no
// unprivileged, dependency-free API for global keypress/click counts or
// cursor position without cgo + an Accessibility/Input Monitoring grant, so
// (like the Linux implementation) Keypresses/MouseClicks/MouseMoveDist stay
// at 0 - only IdleTimeSec (a real signal from IOHIDSystem) is populated.
type InputSnapshot struct {
	Keypresses    int64
	MouseClicks   int64
	MouseMoveDist float64
	IdleTimeSec   int64
}

// NativeInputTracker shells out to `ioreg` for idle time. That's an
// external process, so real sampling is throttled (the caller ticks every
// 100ms; spawning a process that often would be wasteful).
type NativeInputTracker struct {
	mu          sync.Mutex
	lastSample  time.Time
	lastIdleSec int64
}

const darwinSampleThrottle = 500 * time.Millisecond

func newNativeInputTracker() *NativeInputTracker {
	return &NativeInputTracker{}
}

func (t *NativeInputTracker) Sample() InputSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	if !t.lastSample.IsZero() && now.Sub(t.lastSample) < darwinSampleThrottle {
		return InputSnapshot{IdleTimeSec: t.lastIdleSec}
	}
	t.lastSample = now
	t.lastIdleSec = idleTimeSec()

	return InputSnapshot{IdleTimeSec: t.lastIdleSec}
}

// idleTimeSec reads HIDIdleTime (nanoseconds since last user input, reset by
// both keyboard and mouse activity) from the IOHIDSystem service via ioreg.
func idleTimeSec() int64 {
	out, err := exec.Command("ioreg", "-c", "IOHIDSystem").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		idx := strings.Index(line, "\"HIDIdleTime\" = ")
		if idx == -1 {
			continue
		}
		valStr := strings.TrimSpace(line[idx+len("\"HIDIdleTime\" = "):])
		nanos, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			return 0
		}
		return int64(nanos / 1_000_000_000)
	}
	return 0
}
