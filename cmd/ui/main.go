package main

import (
	"flag"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/monitoring-agent/agent/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/agent.yaml", "Path to YAML configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	port := 8080
	host := "127.0.0.1"
	if err == nil {
		if cfg.WebServer.Port > 0 {
			port = cfg.WebServer.Port
		}
		if cfg.WebServer.Host != "" {
			host = cfg.WebServer.Host
		}
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	if host != "127.0.0.1" && host != "localhost" {
		url = fmt.Sprintf("http://%s:%d", host, port)
	}

	fmt.Printf("Opening Agent Browser Dashboard at: %s ...\n", url)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Error opening browser: %v\n", err)
		fmt.Printf("Please open your browser manually and visit: %s\n", url)
	}
}
