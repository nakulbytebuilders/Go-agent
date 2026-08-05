package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

func showMsgBox(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	procMessageBoxW := user32.NewProc("MessageBoxW")
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), 0)
}

func main() {
	// 1. Kill any running agent & watchdog process
	_ = exec.Command("taskkill", "/F", "/IM", "watchdog.exe").Run()
	_ = exec.Command("taskkill", "/F", "/IM", "agent.exe").Run()
	time.Sleep(200 * time.Millisecond)

	// 2. Remove registry autostart & Windows Installed Apps entry
	regCmd1 := `reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "WinSentinelAgent" /f`
	_ = exec.Command("cmd", "/c", regCmd1).Run()

	regCmdWatchdog := `reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "WinSentinelWatchdog" /f`
	_ = exec.Command("cmd", "/c", regCmdWatchdog).Run()

	regCmd2 := `reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "MonitoringAgent" /f`
	_ = exec.Command("cmd", "/c", regCmd2).Run()

	regUninstall := `reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent" /f`
	_ = exec.Command("cmd", "/c", regUninstall).Run()

	// 3. Remove AppData directory
	appDataDir := os.Getenv("APPDATA")
	if appDataDir != "" {
		installDir := filepath.Join(appDataDir, "MonitoringAgent")
		if _, err := os.Stat(installDir); err == nil {
			_ = os.RemoveAll(installDir)
		}
	}

	msg := "WinSentinel Monitoring Agent has been completely uninstalled from your PC.\n\n- Background tracking stopped.\n- Removed from Windows Installed Apps & Startup.\n- All local logs & database deleted."
	fmt.Println(msg)
	showMsgBox("Uninstalled Successfully", msg)
}
