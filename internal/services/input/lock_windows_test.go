//go:build windows

package input

import "testing"

// A real lock can't be provoked from a test, so this only checks the syscalls
// run cleanly and reports what they saw.
func TestIsScreenLockedRuns(t *testing.T) {
	t.Logf("isScreenLocked() = %v", isScreenLocked())
}
