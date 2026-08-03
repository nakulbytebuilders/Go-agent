package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	agentPid := flag.Int("pid", 0, "Target Agent process PID to monitor")
	flag.Parse()

	fmt.Println("==================================================")
	fmt.Println("             Watchdog Process                     ")
	fmt.Println("==================================================")
	fmt.Printf("Monitoring target agent (PID: %d)\n", *agentPid)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Println("Watchdog stopping...")
			return
		case <-ticker.C:
			// Heartbeat checking logic will be implemented in Watchdog phase
		}
	}
}
