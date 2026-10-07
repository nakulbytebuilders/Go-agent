//go:build windows

// Package wtssession answers one narrow question for the boot service: is
// anyone actually logged in right now? It exists so the dashboard can tell
// "machine is on, nobody has logged in yet" apart from "machine is on, the
// per-user agent should be tracking" instead of lumping both under
// "offline".
package wtssession

import (
	"syscall"
	"unsafe"
)

var (
	wtsapi32                  = syscall.NewLazyDLL("wtsapi32.dll")
	procWTSEnumerateSessionsW = wtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSFreeMemory         = wtsapi32.NewProc("WTSFreeMemory")
)

const (
	wtsCurrentServerHandle = 0

	// WTS_CONNECTSTATE_CLASS: WTSActive. Session 0 (services) and any
	// disconnected/listening session never report this state, so checking
	// for it is enough to tell a real, logged-on desktop session apart from
	// the machine simply being powered on.
	wtsActive = 0
)

// wtsSessionInfo mirrors WTS_SESSION_INFOW from wtsapi32.h.
type wtsSessionInfo struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

// HasActiveUserSession reports whether any session on this machine is an
// active, logged-on desktop session. Fast user switching aside, there is
// normally at most one; a locked session still counts (the user is present,
// just locked — that distinction belongs to the idle/locked tracker, not
// here).
func HasActiveUserSession() bool {
	var sessionInfo uintptr
	var count uint32

	ret, _, _ := procWTSEnumerateSessionsW.Call(
		uintptr(wtsCurrentServerHandle),
		0,
		1,
		uintptr(unsafe.Pointer(&sessionInfo)),
		uintptr(unsafe.Pointer(&count)),
	)
	if ret == 0 || sessionInfo == 0 {
		return false
	}
	defer procWTSFreeMemory.Call(sessionInfo)

	const entrySize = unsafe.Sizeof(wtsSessionInfo{})
	for i := uint32(0); i < count; i++ {
		entry := (*wtsSessionInfo)(unsafe.Pointer(sessionInfo + uintptr(i)*entrySize))
		// Session 0 is the services session and is never a real desktop
		// login, but it is excluded by State anyway since it never reports
		// WTSActive.
		if entry.State == wtsActive {
			return true
		}
	}
	return false
}
