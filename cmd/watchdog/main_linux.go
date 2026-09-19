//go:build linux

package main

import (
	"flag"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// isProcessRunning scans /proc/<pid>/comm for a matching process name
// (Linux equivalent of the Windows Toolhelp32 snapshot walk).
func isProcessRunning(processName string) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue
		}
		if trimComm(string(comm)) == processName {
			return true
		}
	}
	return false
}

func trimComm(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func main() {
	configPathFlag := flag.String("config", "", "Path to YAML configuration file")
	agentPathFlag := flag.String("agent", "", "Path to agent executable")
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
		agentPath = filepath.Join(installDir, "agent")
	}
	agentProcessName := filepath.Base(agentPath)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			return
		case <-ticker.C:
			if !isProcessRunning(agentProcessName) {
				if _, err := os.Stat(agentPath); err == nil {
					cmd := exec.Command(agentPath, "-config", configPath)
					cmd.Dir = installDir
					cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
					_ = cmd.Start()
				}
			}
		}
	}
}
