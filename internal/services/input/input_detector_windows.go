//go:build windows

package input

import (
	"math"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetAsyncKeyState   = user32.NewProc("GetAsyncKeyState")
	procGetLastInputInfo    = user32.NewProc("GetLastInputInfo")
	procGetTickCount64      = kernel32.NewProc("GetTickCount64")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
)

const (
	WH_KEYBOARD_LL = 13
	WH_MOUSE_LL    = 14

	WM_KEYDOWN    = 0x0100
	WM_SYSKEYDOWN = 0x0104

	WM_LBUTTONDOWN = 0x0201
	WM_RBUTTONDOWN = 0x0204
	WM_MBUTTONDOWN = 0x0207
	WM_XBUTTONDOWN = 0x020B
)

type POINT struct {
	X int32
	Y int32
}

type LASTINPUTINFO struct {
	CbSize uint32
	DwTime uint32
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type InputSnapshot struct {
	Keypresses    int64
	MouseClicks   int64
	MouseMoveDist float64
	IdleTimeSec   int64
}

type NativeInputTracker struct {
	lastPos      POINT
	hasLastPos   bool
	keyCounter   int64
	clickCounter int64
	kbdHook      uintptr
	mouseHook    uintptr
	prevKeyState [256]bool
}

var globalTracker *NativeInputTracker

func lowLevelKeyboardProc(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		if wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN {
			if globalTracker != nil {
				atomic.AddInt64(&globalTracker.keyCounter, 1)
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func lowLevelMouseProc(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		if wParam == WM_LBUTTONDOWN || wParam == WM_RBUTTONDOWN || wParam == WM_MBUTTONDOWN || wParam == WM_XBUTTONDOWN {
			if globalTracker != nil {
				atomic.AddInt64(&globalTracker.clickCounter, 1)
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func newNativeInputTracker() *NativeInputTracker {
	t := &NativeInputTracker{}
	globalTracker = t

	go t.startHooksLoop()

	return t
}

func (t *NativeInputTracker) startHooksLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	kbdCallback := syscall.NewCallback(lowLevelKeyboardProc)
	mouseCallback := syscall.NewCallback(lowLevelMouseProc)

	hKbd, _, _ := procSetWindowsHookExW.Call(WH_KEYBOARD_LL, kbdCallback, 0, 0)
	hMouse, _, _ := procSetWindowsHookExW.Call(WH_MOUSE_LL, mouseCallback, 0, 0)

	t.kbdHook = hKbd
	t.mouseHook = hMouse

	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
	}

	if t.kbdHook != 0 {
		procUnhookWindowsHookEx.Call(t.kbdHook)
	}
	if t.mouseHook != 0 {
		procUnhookWindowsHookEx.Call(t.mouseHook)
	}
}

func (t *NativeInputTracker) Sample() InputSnapshot {
	var snap InputSnapshot

	// 1. Mouse movement distance
	var pos POINT
	ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pos)))
	if ret != 0 {
		if t.hasLastPos {
			dx := float64(pos.X - t.lastPos.X)
			dy := float64(pos.Y - t.lastPos.Y)
			dist := math.Sqrt(dx*dx + dy*dy)
			snap.MouseMoveDist = dist
		}
		t.lastPos = pos
		t.hasLastPos = true
	}

	// 2. Consume atomic counters from Low-Level Hooks
	hookKeys := atomic.SwapInt64(&t.keyCounter, 0)
	hookClicks := atomic.SwapInt64(&t.clickCounter, 0)

	snap.Keypresses = hookKeys
	snap.MouseClicks = hookClicks

	// 3. Fallback VK polling if hooks produced 0
	if hookKeys == 0 && hookClicks == 0 {
		mouseVKs := map[int]bool{0x01: true, 0x02: true, 0x04: true, 0x05: true, 0x06: true}
		for vk := 1; vk < 256; vk++ {
			r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
			val := uint16(r)
			isPressed := (val & 0x8000) != 0
			if isPressed && !t.prevKeyState[vk] {
				if mouseVKs[vk] {
					snap.MouseClicks++
				} else {
					snap.Keypresses++
				}
			}
			t.prevKeyState[vk] = isPressed
		}
	}

	// 4. Idle time calculation
	var lii LASTINPUTINFO
	lii.CbSize = uint32(unsafe.Sizeof(lii))
	rLII, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii)))
	if rLII != 0 {
		tick, _, _ := procGetTickCount64.Call()
		currentTick := uint32(tick)
		if currentTick >= lii.DwTime {
			snap.IdleTimeSec = int64((currentTick - lii.DwTime) / 1000)
		}
	}

	return snap
}
