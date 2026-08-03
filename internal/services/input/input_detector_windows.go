//go:build windows

package input

import (
	"math"
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	procGetLastInputInfo = user32.NewProc("GetLastInputInfo")
	procGetTickCount64   = kernel32.NewProc("GetTickCount64")
)


type POINT struct {
	X int32
	Y int32
}

type LASTINPUTINFO struct {
	CbSize uint32
	DwTime uint32
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
	prevKeyState [256]bool
}

func newNativeInputTracker() *NativeInputTracker {
	return &NativeInputTracker{}
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

	// 2. Mouse clicks & Keypresses via VK polling
	mouseVKs := map[int]bool{0x01: true, 0x02: true, 0x04: true, 0x05: true, 0x06: true}

	for vk := 1; vk < 256; vk++ {
		r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
		isPressed := (r & 0x8000) != 0
		if isPressed && !t.prevKeyState[vk] {
			if mouseVKs[vk] {
				snap.MouseClicks++
			} else {
				snap.Keypresses++
			}
		}
		t.prevKeyState[vk] = isPressed
	}

	// 3. Idle time calculation
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
