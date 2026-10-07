//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/monitoring-agent/agent/internal/bootsvc"
	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/logger"
)

// serviceHandler implements svc.Handler, bridging the Windows Service
// Control Manager's start/stop/session-change protocol to a plain
// bootsvc.Runner.
type serviceHandler struct {
	cfg        *config.Config
	configPath string
	log        *slog.Logger
}

func (h *serviceHandler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptSessionChange

	s <- svc.Status{State: svc.StartPending}

	runner := bootsvc.New(h.cfg, h.configPath, h.log)
	ctx, cancel := context.WithCancel(context.Background())
	// Guarantees the runner is told to stop even if the change-request
	// channel ends some other way than a Stop/Shutdown command (e.g. the
	// SCM tearing it down abnormally) — without this, the loop below could
	// fall through to <-runnerDone with cancel never called, and block
	// forever since the runner only exits once its context is cancelled.
	defer cancel()
	runnerDone := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(runnerDone)
	}()

	s <- svc.Status{State: svc.Running, Accepts: accepted}
	h.log.Info("WinSentinelBootSvc running")

loop:
	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			s <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			h.log.Info("WinSentinelBootSvc received stop/shutdown request")
			s <- svc.Status{State: svc.StopPending}
			cancel()
			break loop
		case svc.SessionChange:
			h.log.Info("Windows session change observed", "event", sessionChangeEventName(req.EventType))
		}
	}

	<-runnerDone
	s <- svc.Status{State: svc.Stopped}
	return false, 0
}

// sessionChangeEventName turns a WTS_SESSION_* code from a SessionChange
// ChangeRequest into a readable label, purely for diagnostics: this is the
// exact moment Windows itself says a user logged on/off/locked/unlocked, so
// comparing it against agent.exe's own start time is the fastest way to
// confirm whether a reported "offline" gap was really just nobody logged in
// yet, versus something else going wrong after logon.
func sessionChangeEventName(eventType uint32) string {
	switch eventType {
	case 0x1:
		return "console_connect"
	case 0x2:
		return "console_disconnect"
	case 0x3:
		return "remote_connect"
	case 0x4:
		return "remote_disconnect"
	case 0x5:
		return "session_logon"
	case 0x6:
		return "session_logoff"
	case 0x7:
		return "session_lock"
	case 0x8:
		return "session_unlock"
	default:
		return fmt.Sprintf("unknown(0x%x)", eventType)
	}
}

// runForeground runs the same loop svc.Run would drive, but directly under
// Ctrl+C — used when this binary is launched from a console instead of by
// the SCM (e.g. `svc.exe -config ...` while debugging).
func runForeground(cfg *config.Config, configPath string, lm *logger.LoggerManager) {
	fmt.Println("Running WinSentinelBootSvc in the foreground (not installed as a service). Press Ctrl+C to stop.")

	runner := bootsvc.New(cfg, configPath, lm.BootLogger)
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	runner.Run(ctx)
}

func installService(exePath, configPath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows service control manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()

	if existing, err := m.OpenService(serviceName); err == nil {
		_ = existing.Close()
		return fmt.Errorf("service %s is already installed; uninstall it first", serviceName)
	}

	s, err := m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName:      serviceDisplayName,
		Description:      serviceDescription,
		StartType:        mgr.StartAutomatic,
		ErrorControl:     mgr.ErrorNormal,
		DelayedAutoStart: false,
	}, "-config", configPath)
	if err != nil {
		return err
	}
	defer s.Close()

	// Restart on crash natively via the SCM, independent of watchdog.exe
	// (which itself only runs in the user's session and can't resurrect a
	// LocalSystem service). Reset the failure count daily.
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 86400)

	if err := s.Start(); err != nil {
		return fmt.Errorf("service created but failed to start: %w", err)
	}
	return nil
}

func removeService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows service control manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		// Not installed: nothing to do.
		return nil
	}
	defer s.Close()

	_, _ = s.Control(svc.Stop)
	for i := 0; i < 40; i++ {
		st, err := s.Query()
		if err != nil || st.State == svc.Stopped {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	return s.Delete()
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows service control manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()

	return s.Start()
}

func stopService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows service control manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()

	_, err = s.Control(svc.Stop)
	return err
}
