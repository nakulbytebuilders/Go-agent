//go:build darwin

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
)

// isProcessRunning shells out to `pgrep -x` (preinstalled on macOS) - the
// Linux/Windows variants use /proc and Toolhelp32 respectively, which have
// no macOS equivalent without cgo.
func isProcessRunning(processName string) bool {
	out, err := exec.Command("pgrep", "-x", processName).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
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
