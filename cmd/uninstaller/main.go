package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

func showMsgBox(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	procMessageBoxW := user32.NewProc("MessageBoxW")
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), 0)
}

func deleteRegistryKey(root registry.Key, path string, viewFlags uint32) {
	k, err := registry.OpenKey(root, path, registry.ALL_ACCESS|viewFlags)
	if err != nil {
		return
	}
	_ = k.Close()

	parentPath, keyName := filepath.Split(path)
	parentPath = strings.TrimSuffix(parentPath, `\`)

	parentKey, err := registry.OpenKey(root, parentPath, registry.ALL_ACCESS|viewFlags)
	if err == nil {
		_ = registry.DeleteKey(parentKey, keyName)
		_ = parentKey.Close()
	}
}

func deleteRegistryValue(root registry.Key, path string, valueName string, viewFlags uint32) {
	k, err := registry.OpenKey(root, path, registry.SET_VALUE|viewFlags)
	if err == nil {
		_ = k.DeleteValue(valueName)
		_ = k.Close()
	}
}

func main() {
	exePath, err := os.Executable()
	if err != nil {
		exePath = ""
	}

	tempDir := os.TempDir()

	// Self-relocation to %TEMP% to prevent AppData folder lock
	if exePath != "" && !strings.HasPrefix(strings.ToLower(exePath), strings.ToLower(tempDir)) {
		tempUninstaller := filepath.Join(tempDir, "wsntl_uninstaller_runner.exe")

		if data, err := os.ReadFile(exePath); err == nil {
			_ = os.WriteFile(tempUninstaller, data, 0755)

			cmd := exec.Command(tempUninstaller, "--from-temp")
			cmd.SysProcAttr = &syscall.SysProcAttr{
				CreationFlags: 0x08000000, // CREATE_NO_WINDOW
			}
			_ = cmd.Start()
			os.Exit(0)
		}
	}

	// Give previous process handle time to terminate completely
	time.Sleep(500 * time.Millisecond)

	// 1. Kill any running watchdog & agent processes (Watchdog FIRST to prevent resurrection)
	_ = exec.Command("taskkill", "/F", "/T", "/IM", "watchdog.exe").Run()
	_ = exec.Command("taskkill", "/F", "/T", "/IM", "agent.exe").Run()
	_ = exec.Command("taskkill", "/F", "/T", "/IM", "ui.exe").Run()
	time.Sleep(500 * time.Millisecond)

	// 2. Remove scheduled tasks across all known task paths
	_ = exec.Command("schtasks", "/Delete", "/TN", "\\Microsoft\\Windows\\Hotpatch\\Monitoring", "/F").Run()
	_ = exec.Command("schtasks", "/Delete", "/TN", "Monitoring", "/F").Run()
	_ = exec.Command("schtasks", "/Delete", "/TN", "WinSentinelAgent", "/F").Run()

	// 3. Native Registry Cleanup (Run keys across HKCU/HKLM, 64-bit & 32-bit views)
	runKeyPath := `Software\Microsoft\Windows\CurrentVersion\Run`
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{0x0100, 0x0200} { // 0x0100: KEY_WOW64_64KEY, 0x0200: KEY_WOW64_32KEY
			deleteRegistryValue(root, runKeyPath, "WinSentinelAgent", view)
			deleteRegistryValue(root, runKeyPath, "WinSentinelWatchdog", view)
			deleteRegistryValue(root, runKeyPath, "MonitoringAgent", view)
		}
	}

	// 4. Native Uninstall Registry Key Removal across HKCU & HKLM (64-bit & 32-bit views)
	uninstallPaths := []string{
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinel`,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\MonitoringAgent`,
	}
	for _, path := range uninstallPaths {
		for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
			for _, view := range []uint32{0x0100, 0x0200} {
				deleteRegistryKey(root, path, view)
			}
		}
	}

	// Fallback via reg.exe commands
	_ = exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`, "/f").Run()
	_ = exec.Command("reg", "delete", `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`, "/f", "/reg:64").Run()
	_ = exec.Command("reg", "delete", `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`, "/f", "/reg:32").Run()

	// 5. Remove AppData installation directory with retry loop
	appDataDir := os.Getenv("APPDATA")
	if appDataDir != "" {
		installDir := filepath.Join(appDataDir, "MonitoringAgent")
		for i := 0; i < 5; i++ {
			if _, err := os.Stat(installDir); err == nil {
				err = os.RemoveAll(installDir)
				if err == nil {
					break
				}
				time.Sleep(300 * time.Millisecond)
			} else {
				break
			}
		}
	}

	msg := "WinSentinel Monitoring Agent has been completely uninstalled from your PC.\n\n- Background tracking stopped.\n- Removed from Windows Installed Apps & Startup.\n- All local logs & database deleted."
	fmt.Println(msg)
	showMsgBox("Uninstalled Successfully", msg)
}


