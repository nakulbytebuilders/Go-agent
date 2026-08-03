//go:build windows

package apptracker

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW             = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

type ActiveWindowInfo struct {
	AppName     string
	WindowTitle string
	PID         int32
}

func getActiveWindowInfo() (ActiveWindowInfo, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ActiveWindowInfo{AppName: "System", WindowTitle: "Desktop / Idle", PID: 0}, nil
	}

	// 1. Get Window Title
	buf := make([]uint16, 512)
	ret, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	title := ""
	if ret > 0 {
		title = syscall.UTF16ToString(buf[:ret])
	}

	// 2. Get PID
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))

	// 3. Get Executable Name from PID
	appName := "System"
	if pid > 0 {
		hProcess, _, _ := procOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
		if hProcess != 0 {
			defer procCloseHandle.Call(hProcess)

			pathBuf := make([]uint16, 1024)
			size := uint32(len(pathBuf))
			r, _, _ := procQueryFullProcessImageNameW.Call(hProcess, 0, uintptr(unsafe.Pointer(&pathBuf[0])), uintptr(unsafe.Pointer(&size)))
			if r != 0 {
				fullPath := syscall.UTF16ToString(pathBuf[:size])
				appName = filepath.Base(fullPath)
			}
		}
	}

	if title == "" {
		title = appName
	}

	return ActiveWindowInfo{
		AppName:     appName,
		WindowTitle: title,
		PID:         int32(pid),
	}, nil
}
