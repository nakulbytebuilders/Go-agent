//go:build !windows

package input

type InputSnapshot struct {
	Keypresses    int64
	MouseClicks   int64
	MouseMoveDist float64
	IdleTimeSec   int64
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
