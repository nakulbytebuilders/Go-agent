//go:build !windows && !linux && !darwin

package input

// isScreenLocked is unsupported here, so the screen is never reported locked.
func isScreenLocked() bool {
	return false
}
