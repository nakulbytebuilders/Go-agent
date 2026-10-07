//go:build windows

// Command svc is the boot-time half of the agent: a real Windows Service
// (LocalSystem, Automatic start) that comes up when the machine boots,
// regardless of whether anyone has logged in. It only enrolls the device
// and polls server policy as a heartbeat — see internal/bootsvc for why it
// never touches the activity database. The per-user agent.exe (started via
// the HKCU Run key / scheduled task on logon) is unchanged and still does
// all the actual activity tracking.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/logger"
)

const (
	serviceName        = "WinSentinelBootSvc"
	serviceDisplayName = "WinSentinel Agent (Boot Service)"
	serviceDescription = "Keeps the WinSentinel monitoring agent enrolled and reachable from the moment this machine starts, before anyone logs in. Collects no activity data itself; see the WinSentinel Agent service/process for that."
)

func main() {
	configPath := flag.String("config", "configs/agent.yaml", "Path to YAML configuration file")
	install := flag.Bool("install", false, "Install as a Windows service (Automatic start, runs at boot) and start it")
	uninstall := flag.Bool("uninstall", false, "Remove the Windows service")
	start := flag.Bool("start", false, "Start the installed Windows service")
	stop := flag.Bool("stop", false, "Stop the installed Windows service")
	flag.Parse()

	// Match agent.exe: run relative to the executable's own directory, not
	// whatever directory the SCM happens to launch services from.
	if exePath, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
			_ = os.Chdir(filepath.Dir(resolved))
		} else {
			_ = os.Chdir(filepath.Dir(exePath))
		}
	}
	if abs, err := filepath.Abs(*configPath); err == nil {
		*configPath = abs
	}

	switch {
	case *install:
		exePath, err := os.Executable()
		if err != nil {
			fmt.Printf("[ERROR] failed to resolve executable path: %v\n", err)
			os.Exit(1)
		}
		if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
			exePath = resolved
		}
		if err := installService(exePath, *configPath); err != nil {
			fmt.Printf("[ERROR] failed to install service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[SUCCESS] WinSentinelBootSvc installed and started.")
		return
	case *uninstall:
		if err := removeService(); err != nil {
			fmt.Printf("[ERROR] failed to remove service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[SUCCESS] WinSentinelBootSvc removed.")
		return
	case *start:
		if err := startService(); err != nil {
			fmt.Printf("[ERROR] %v\n", err)
			os.Exit(1)
		}
		return
	case *stop:
		if err := stopService(); err != nil {
			fmt.Printf("[ERROR] %v\n", err)
			os.Exit(1)
		}
		return
	}

	isService, err := svc.IsWindowsService()
	if err != nil {
		fmt.Printf("[ERROR] failed to determine how this process was started: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("[ERROR] failed to load configuration: %v\n", err)
		os.Exit(1)
	}
	lm, err := logger.Init(cfg.Logger)
	if err != nil {
		fmt.Printf("[ERROR] failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	if !isService {
		// Launched directly from a console (e.g. for debugging), not by the
		// SCM: run the same loop in the foreground instead of calling
		// svc.Run, which would otherwise just fail with "not a service".
		runForeground(cfg, *configPath, lm)
		return
	}

	h := &serviceHandler{cfg: cfg, configPath: *configPath, log: lm.BootLogger}
	if err := svc.Run(serviceName, h); err != nil {
		lm.BootLogger.Error("Service failed", "error", err)
		os.Exit(1)
	}
}
