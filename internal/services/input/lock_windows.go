//go:build windows

package input

import (
	"syscall"
	"unsafe"
)

var (
	wtsapi32                       = syscall.NewLazyDLL("wtsapi32.dll")
	procWTSQuerySessionInformation = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory              = wtsapi32.NewProc("WTSFreeMemory")
)

const (
	wtsCurrentServerHandle = 0
	wtsCurrentSession      = 0xFFFFFFFF
	wtsSessionInfoEx       = 25 // WTS_INFO_CLASS: WTSSessionInfoEx

	wtsSessionStateLock = 0 // WTSINFOEX_LEVEL1_W.SessionFlags: WTS_SESSIONSTATE_LOCK
)

// wtsInfoExLevel1 mirrors WTSINFOEX_LEVEL1_W from wtsapi32.h (Level 1 payload
// of WTSINFOEXW). Only SessionFlags is used; the rest exists to keep field
// offsets correct.
type wtsInfoExLevel1 struct {
	SessionID      uint32
	SessionState   uint32
	SessionFlags   int32
	WinStationName [32]uint16
	UserName       [21]uint16
	DomainName     [17]uint16
	LogonTime      int64
	ConnectTime    int64
	DisconnectTime int64
	LastInputTime  int64
	CurrentTime    int64
}

// wtsInfoEx mirrors WTSINFOEXW, which is a Level field followed by a union
// that only ever holds Level 1 data in practice.
type wtsInfoEx struct {
	Level uint32
	_pad  uint32
	Data  wtsInfoExLevel1
}

// isScreenLocked reports whether the current session is locked, via the same
// WTS session-lock state Task Manager and fast user switching use. Unlike an
// OpenInputDesktop check, this is true for the entire locked duration —
// including the lock-screen curtain shown before the PIN/password prompt is
// reached — not just the moment credentials are being entered.
func isScreenLocked() bool {
	var buf uintptr
	var bytesReturned uint32
	ret, _, _ := procWTSQuerySessionInformation.Call(
		uintptr(wtsCurrentServerHandle),
		uintptr(wtsCurrentSession),
		uintptr(wtsSessionInfoEx),
		uintptr(unsafe.Pointer(&buf)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if ret == 0 || buf == 0 {
		return false
	}
	defer procWTSFreeMemory.Call(buf)

	info := (*wtsInfoEx)(unsafe.Pointer(buf))
	if info.Level != 1 {
		return false
	}
	return info.Data.SessionFlags == wtsSessionStateLock
}
