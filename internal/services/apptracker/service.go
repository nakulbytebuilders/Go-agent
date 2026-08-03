package apptracker

import (
	"context"
	"encoding/json"
	"fmt"

	"log/slog"
	"sync"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/database"
	"github.com/monitoring-agent/agent/internal/models"
	"github.com/monitoring-agent/agent/internal/services"
)

type AppTrackerService struct {
	mu           sync.RWMutex
	db           *database.DatabaseManager
	cfg          config.AppTrackerConfig
	log          *slog.Logger
	state        string
	cancel       context.CancelFunc

	// Active session tracking
	currentApp   ActiveWindowInfo
	sessionStart time.Time
}

func NewAppTrackerService(db *database.DatabaseManager, cfg config.AppTrackerConfig, log *slog.Logger) *AppTrackerService {
	return &AppTrackerService{
		db:    db,
		cfg:   cfg,
		log:   log,
		state: services.StateStopped,
	}
}

func (s *AppTrackerService) Name() string {
	return "appTracker"
}

func (s *AppTrackerService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state == services.StateRunning {
		s.mu.Unlock()
		return nil
	}
	s.state = services.StateStarting
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.state = services.StateRunning

	// Reset active tracking session
	info, _ := getActiveWindowInfo()
	s.currentApp = info
	s.sessionStart = time.Now()

	s.mu.Unlock()

	s.log.Info("AppTracker service starting", "poll_interval_sec", s.cfg.PollIntervalSec)
	go s.runLoop(runCtx)
	return nil
}

func (s *AppTrackerService) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.state == services.StateStopped {
		s.mu.Unlock()
		return nil
	}
	s.state = services.StateStopping
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	// Flush last active window record to DB before stopping
	if s.currentApp.AppName != "" && !s.sessionStart.IsZero() {
		s.flushCurrentActivityLocked(ctx)
	}

	s.state = services.StateStopped
	s.mu.Unlock()

	s.log.Info("AppTracker service stopped")
	return nil
}

func (s *AppTrackerService) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	return s.Start(ctx)
}

func (s *AppTrackerService) GetCurrentActiveApp() models.AppActivity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	duration := int64(now.Sub(s.sessionStart).Seconds())
	if duration < 0 {
		duration = 0
	}

	return models.AppActivity{
		AppName:     s.currentApp.AppName,
		WindowTitle: s.currentApp.WindowTitle,
		PID:         s.currentApp.PID,
		DurationSec: duration,
		StartTime:   s.sessionStart,
		EndTime:     now,
	}
}

func (s *AppTrackerService) Status() models.ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return models.ServiceStatus{
		Name:      s.Name(),
		State:     s.state,
		Message:   fmt.Sprintf("App tracker running: active app '%s'", s.currentApp.AppName),
		LastCheck: time.Now(),
	}
}

func (s *AppTrackerService) Health() models.HealthReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	isHealthy := s.state == services.StateRunning || s.state == services.StateStopped
	return models.HealthReport{
		IsHealthy: isHealthy,
		Details: map[string]string{
			"state":      s.state,
			"active_app": s.currentApp.AppName,
		},
	}
}

func (s *AppTrackerService) runLoop(ctx context.Context) {
	interval := s.cfg.PollIntervalSec
	if interval <= 0 {
		interval = 2
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollActiveWindow(ctx)
		}
	}
}

func (s *AppTrackerService) pollActiveWindow(ctx context.Context) {
	info, err := getActiveWindowInfo()
	if err != nil {
		s.log.Debug("Failed to get active window info", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	// Check if active app or window title changed, OR if 10 seconds elapsed in current session
	if info.AppName != s.currentApp.AppName || info.WindowTitle != s.currentApp.WindowTitle || now.Sub(s.sessionStart) >= 10*time.Second {
		if s.currentApp.AppName != "" {
			s.flushCurrentActivityLocked(ctx)
		}

		// Start new tracking session segment
		s.currentApp = info
		s.sessionStart = now
		s.log.Debug("Active application session segment recorded", "app", info.AppName, "title", info.WindowTitle, "pid", info.PID)
	}
}

func (s *AppTrackerService) flushCurrentActivityLocked(ctx context.Context) {
	now := time.Now()
	durationSec := int64(now.Sub(s.sessionStart).Seconds())

	// Only record activities with non-zero duration
	if durationSec > 0 && s.currentApp.AppName != "" && s.db != nil {
		act := &models.AppActivity{
			AppName:     s.currentApp.AppName,
			WindowTitle: s.currentApp.WindowTitle,
			PID:         s.currentApp.PID,
			DurationSec: durationSec,
			StartTime:   s.sessionStart,
			EndTime:     now,
		}
		if id, err := s.db.InsertActivity(ctx, act); err != nil {
			s.log.Error("Failed to record application activity", "error", err)
		} else {
			act.ID = id
			s.log.Debug("Recorded application activity", "app", act.AppName, "duration_sec", durationSec)
			payload, err := json.Marshal(act)
			if err == nil {
				_, _ = s.db.InsertQueue(ctx, &models.SyncQueueItem{
					PayloadType: "app",
					PayloadJSON: string(payload),
					Status:      "pending",
				})
			}
		}

	}
}
