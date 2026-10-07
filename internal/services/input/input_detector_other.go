//go:build !windows && !linux && !darwin

package input

import "time"

type InputSnapshot struct {
	Keypresses    int64
	MouseClicks   int64
	MouseMoveDist float64
	IdleTimeSec   int64
	// LastInput stays zero: there is no input signal on this platform, so idle
	// time is never reported.
	LastInput time.Time
}

type NativeInputTracker struct{}

func newNativeInputTracker() *NativeInputTracker {
	return &NativeInputTracker{}
}

func (t *NativeInputTracker) Sample() InputSnapshot {
	return InputSnapshot{
		Keypresses:    0,
		MouseClicks:   0,
		MouseMoveDist: 0.0,
		IdleTimeSec:   0,
	}
}
