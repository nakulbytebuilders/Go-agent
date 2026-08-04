package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/monitoring-agent/agent/internal/autostart"
	"github.com/monitoring-agent/agent/internal/controller"
	"github.com/monitoring-agent/agent/internal/services/updater"
	"github.com/monitoring-agent/agent/internal/web"
)

func main() {
	// Set working directory to executable's directory
	// Fixes CWD being C:\Windows\System32 when agent is launched via Windows Startup (HKCU Run key)
	if exePath, err := os.Executable(); err == nil {
		if exePathResolved, err := filepath.EvalSymlinks(exePath); err == nil {
			_ = os.Chdir(filepath.Dir(exePathResolved))
		} else {
			_ = os.Chdir(filepath.Dir(exePath))
		}
	}

	configPath := flag.String("config", "configs/agent.yaml", "Path to YAML configuration file")
	install := flag.Bool("install", false, "Register agent to auto-start on Windows login")
	uninstall := flag.Bool("uninstall", false, "Remove agent from Windows auto-start")
	status := flag.Bool("status", false, "Check if agent is registered for auto-start")
	flag.Parse()

	// Handle auto-start commands
	if *install {
		if err := autostart.Install(*configPath); err != nil {
			fmt.Printf("[ERROR] Failed to install auto-start: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[SUCCESS] Agent registered for auto-start!")
		fmt.Println("  The agent will now start automatically when you log in to Windows.")
		fmt.Printf("  Config: %s\n", *configPath)
		if installed, cmd := autostart.IsInstalled(); installed {
			fmt.Printf("  Registry: %s\n", cmd)
		}
		os.Exit(0)
	}

	if *uninstall {
		if err := autostart.Uninstall(); err != nil {
			fmt.Printf("[ERROR] Failed to remove auto-start: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[SUCCESS] Agent removed from auto-start.")
		fmt.Println("  The agent will no longer start automatically on login.")
		os.Exit(0)
	}

	if *status {
		installed, cmd := autostart.IsInstalled()
		if installed {
			fmt.Println("[INFO] Agent IS registered for auto-start.")
			fmt.Printf("  Command: %s\n", cmd)
		} else {
			fmt.Println("[INFO] Agent is NOT registered for auto-start.")
			fmt.Println("  Run: agent.exe --install  to enable auto-start.")
		}
		os.Exit(0)
	}

	fmt.Println("==================================================")
	fmt.Println("   Cross-Platform Monitoring Agent Service       ")
	fmt.Println("==================================================")

	ctl, err := controller.NewAgentController(*configPath)
	if err != nil {
		fmt.Printf("Fatal: failed to initialize AgentController: %v\n", err)
		os.Exit(1)
	}

	// Start all enabled services
	ctl.StartEnabledServices()

	// Start auto-updater service (checks every 1 hour)
	apiURL := ctl.GetConfig().Server.APIURL
	if apiURL == "" {
		apiURL = "http://monitor-cloudd.test/api"
	}
	updaterSvc := updater.NewUpdaterService(apiURL, *configPath)
	go updaterSvc.StartUpdateLoop(context.Background(), 1*time.Hour)

	// Start embedded browser-based web dashboard & REST API server
	webServer := web.NewWebServer(ctl, ctl.GetConfig().WebServer, nil)
	if err := webServer.Start(); err != nil {
		fmt.Printf("Warning: failed to start web server: %v\n", err)
	}

	// Intercept termination signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Println("Agent background service running. Press Ctrl+C to stop.")
	sig := <-sigChan
	fmt.Printf("\nReceived signal '%s'. Initiating graceful shutdown...\n", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = webServer.Stop(shutdownCtx)

	ctl.Shutdown()
	fmt.Println("Agent stopped cleanly.")
}
