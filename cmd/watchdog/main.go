package main

import (
	"flag"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isProcessRunning(processName string) bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(name, processName) {
			return true
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	return false
}

func main() {
	configPathFlag := flag.String("config", "", "Path to YAML configuration file")
	agentPathFlag := flag.String("agent", "", "Path to agent.exe executable")
	flag.Parse()

	exePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}

	installDir := filepath.Dir(exePath)
	configPath := *configPathFlag
	if configPath == "" {
		configPath = filepath.Join(installDir, "configs", "agent.yaml")
	}

	agentPath := *agentPathFlag
	if agentPath == "" {
		agentPath = filepath.Join(installDir, "agent.exe")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			return
		case <-ticker.C:
			// Check if agent.exe process is alive
			if !isProcessRunning("agent.exe") {
				if _, err := os.Stat(agentPath); err == nil {
					cmd := exec.Command(agentPath, "-config", configPath)
					cmd.Dir = installDir
					cmd.SysProcAttr = &syscall.SysProcAttr{
						CreationFlags: 0x08000000 | 0x00000008, // CREATE_NO_WINDOW | DETACHED_PROCESS
					}
					_ = cmd.Start()
				}
			}
		}
	}
}
