//go:build linux

package input

import (
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// InputSnapshot mirrors the Windows snapshot shape. Linux (X11) has no
// unprivileged API for global keypress/click counts (that needs root or
// membership of the `input` group to read /dev/input/event*), so
// Keypresses/MouseClicks are left at 0 here rather than faking numbers.
// Idle time (via xprintidle) and mouse movement distance are real signals.
type InputSnapshot struct {
	Keypresses    int64
	MouseClicks   int64
	MouseMoveDist float64
	IdleTimeSec   int64
}

// NativeInputTracker polls xdotool/xprintidle. Both are external processes,
// so real sampling is throttled internally (the caller ticks every 100ms;
// spawning a process that often would be wasteful) while still satisfying
// the same Sample() contract as the Windows implementation.
type NativeInputTracker struct {
	mu           sync.Mutex
	lastX, lastY float64
	hasLastPos   bool
	lastSample   time.Time
	lastIdleSec  int64
}

const linuxSampleThrottle = 500 * time.Millisecond

func newNativeInputTracker() *NativeInputTracker {
	return &NativeInputTracker{}
}

func (t *NativeInputTracker) Sample() InputSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	if !t.lastSample.IsZero() && now.Sub(t.lastSample) < linuxSampleThrottle {
		// Too soon since the last real sample: report no new movement but
		// keep returning the last known idle time so callers still see it.
		return InputSnapshot{IdleTimeSec: t.lastIdleSec}
	}
	t.lastSample = now

	var moveDist float64
	if x, y, ok := mouseLocation(); ok {
		if t.hasLastPos {
			dx := x - t.lastX
			dy := y - t.lastY
			moveDist = math.Sqrt(dx*dx + dy*dy)
		}
		t.lastX, t.lastY = x, y
		t.hasLastPos = true
	}

	t.lastIdleSec = idleTimeSec()

	return InputSnapshot{
		Keypresses:    0,
		MouseClicks:   0,
		MouseMoveDist: moveDist,
		IdleTimeSec:   t.lastIdleSec,
	}
}

// mouseLocation shells out to `xdotool getmouselocation`. Requires xdotool
// to be installed; returns ok=false (no movement recorded) if unavailable.
func mouseLocation() (x, y float64, ok bool) {
	out, err := exec.Command("xdotool", "getmouselocation", "--shell").Output()
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if v, found := strings.CutPrefix(line, "X="); found {
			x, _ = strconv.ParseFloat(v, 64)
		} else if v, found := strings.CutPrefix(line, "Y="); found {
			y, _ = strconv.ParseFloat(v, 64)
		}
	}
	return x, y, true
}

// idleTimeSec shells out to `xprintidle` (returns idle milliseconds).
// Returns 0 (never idle) if the tool isn't installed rather than failing.
func idleTimeSec() int64 {
	out, err := exec.Command("xprintidle").Output()
	if err != nil {
		return 0
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return ms / 1000
}
