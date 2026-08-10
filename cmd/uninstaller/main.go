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

func deleteRegistryKey(root registry.Key, path string, viewFlags uint32) error {
	parentPath, keyName := filepath.Split(path)
	parentPath = strings.TrimSuffix(parentPath, `\`)

	// Open parent key with DELETE permission (0x00010000 | SET_VALUE | ENUMERATE_SUB_KEYS)
	parentKey, err := registry.OpenKey(root, parentPath, 0x00010000|registry.SET_VALUE|registry.ENUMERATE_SUB_KEYS|viewFlags)
	if err != nil {
		return err
	}
	defer parentKey.Close()

	return registry.DeleteKey(parentKey, keyName)
}

func deleteRegistryValue(root registry.Key, path string, valueName string, viewFlags uint32) error {
	k, err := registry.OpenKey(root, path, registry.SET_VALUE|viewFlags)
	if err != nil {
		return err
	}
	defer k.Close()

	return k.DeleteValue(valueName)
}

func main() {
	exePath, err := os.Executable()
	if err != nil {
		exePath = ""
	}

	tempDir := os.TempDir()

	isRunner := false
	isQuiet := false
	for _, arg := range os.Args {
		if arg == "--from-temp" {
			isRunner = true
		}
		if arg == "/quiet" || arg == "-quiet" {
			isQuiet = true
		}
	}

	// If uninstaller is running inside AppData/installation dir, copy to TEMP and run SYNCHRONOUSLY
	if !isRunner && exePath != "" && !strings.HasPrefix(strings.ToLower(exePath), strings.ToLower(tempDir)) {
		tempUninstaller := filepath.Join(tempDir, fmt.Sprintf("wsntl_uninstaller_%d.exe", time.Now().UnixNano()))

		if data, err := os.ReadFile(exePath); err == nil {
			if err := os.WriteFile(tempUninstaller, data, 0755); err == nil {
				cmdArgs := []string{"--from-temp"}
				if isQuiet {
					cmdArgs = append(cmdArgs, "/quiet")
				}
				cmd := exec.Command(tempUninstaller, cmdArgs...)
				cmd.SysProcAttr = &syscall.SysProcAttr{
					CreationFlags: 0x08000000, // CREATE_NO_WINDOW
				}
				// SYNCHRONOUS WAIT: Ensures Registry key is deleted BEFORE Windows Settings checks
				_ = cmd.Run()

				// Schedule background cleanup of temporary uninstaller runner
				cleanCmd := fmt.Sprintf("ping 127.0.0.1 -n 3 > nul & del /f /q \"%s\"", tempUninstaller)
				_ = exec.Command("cmd", "/c", cleanCmd).Start()
				os.Exit(0)
			}
		}
	}

	// 1. Stop background processes (Watchdog FIRST to prevent agent resurrection)
	_ = exec.Command("taskkill", "/F", "/T", "/IM", "watchdog.exe").Run()
	_ = exec.Command("taskkill", "/F", "/T", "/IM", "agent.exe").Run()
	_ = exec.Command("taskkill", "/F", "/T", "/IM", "ui.exe").Run()
	time.Sleep(300 * time.Millisecond)

	// 2. Remove scheduled tasks across all known task paths
	_ = exec.Command("schtasks", "/Delete", "/TN", "\\Microsoft\\Windows\\Hotpatch\\Monitoring", "/F").Run()
	_ = exec.Command("schtasks", "/Delete", "/TN", "Monitoring", "/F").Run()
	_ = exec.Command("schtasks", "/Delete", "/TN", "WinSentinelAgent", "/F").Run()

	// 3. Native Registry Cleanup (Run keys across HKCU/HKLM, 64-bit & 32-bit views)
	runKeyPath := `Software\Microsoft\Windows\CurrentVersion\Run`
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{0, 0x0100, 0x0200} {
			_ = deleteRegistryValue(root, runKeyPath, "WinSentinelAgent", view)
			_ = deleteRegistryValue(root, runKeyPath, "WinSentinelWatchdog", view)
			_ = deleteRegistryValue(root, runKeyPath, "MonitoringAgent", view)
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
			for _, view := range []uint32{0, 0x0100, 0x0200} {
				_ = deleteRegistryKey(root, path, view)
			}
		}
	}

	// Direct reg.exe execution for guaranteed removal
	_ = exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`, "/f").Run()
	_ = exec.Command("reg", "delete", `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`, "/f").Run()
	_ = exec.Command("reg", "delete", `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`, "/f", "/reg:64").Run()
	_ = exec.Command("reg", "delete", `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent`, "/f", "/reg:32").Run()

	// 5. Clean AppData installation directory contents
	appDataDir := os.Getenv("APPDATA")
	if appDataDir != "" {
		installDir := filepath.Join(appDataDir, "MonitoringAgent")
		if entries, err := os.ReadDir(installDir); err == nil {
			for _, entry := range entries {
				if entry.Name() != "uninstaller.exe" {
					_ = os.RemoveAll(filepath.Join(installDir, entry.Name()))
				}
			}
		}
		_ = os.RemoveAll(installDir)

		// Schedule background purge of remaining folder after parent uninstaller process terminates
		purgeCmd := fmt.Sprintf("ping 127.0.0.1 -n 3 > nul & rmdir /s /q \"%s\"", installDir)
		_ = exec.Command("cmd", "/c", purgeCmd).Start()
	}

	if !isQuiet && isRunner {
		// Non-blocking notification if interactive
		go showMsgBox("Uninstalled Successfully", "WinSentinel Monitoring Agent has been completely uninstalled from your PC.")
		time.Sleep(100 * time.Millisecond)
	}
}
