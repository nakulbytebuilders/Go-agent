//go:build !windows

package wtssession

// HasActiveUserSession is unsupported outside Windows. The boot-service /
// user-session split this package supports is Windows-specific (it exists
// to work around HKCU Run-key-only autostart and WTS session semantics that
// don't apply the same way on Linux/macOS), so this always reports no
// session rather than claim knowledge it doesn't have.
func HasActiveUserSession() bool {
	return false
}
