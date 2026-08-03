package input

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

type InputService struct {
	mu            sync.RWMutex
	db            *database.DatabaseManager
	cfg           config.InputTrackerConfig
	log           *slog.Logger
	state         string
	cancel        context.CancelFunc

	tracker       *NativeInputTracker
	intervalStart time.Time

	// Current accumulators
	accumKeypresses int64
	accumClicks     int64
	accumDistance   float64
	lastIdleTime    int64
}

func NewInputService(db *database.DatabaseManager, cfg config.InputTrackerConfig, log *slog.Logger) *InputService {
	return &InputService{
		db:      db,
		cfg:     cfg,
		log:     log,
		state:   services.StateStopped,
		tracker: newNativeInputTracker(),
	}
}

func (s *InputService) Name() string {
	return "input"
}

func (s *InputService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state == services.StateRunning {
		s.mu.Unlock()
		return nil
	}
	s.state = services.StateStarting
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.state = services.StateRunning

	s.intervalStart = time.Now()
	s.accumKeypresses = 0
	s.accumClicks = 0
	s.accumDistance = 0
	s.lastIdleTime = 0

	s.mu.Unlock()

	s.log.Info("InputTracker service starting", "poll_interval_sec", s.cfg.PollIntervalSec)
	go s.runLoop(runCtx)
	return nil
}

func (s *InputService) Stop(ctx context.Context) error {
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

	s.flushIntervalLocked(ctx)

	s.state = services.StateStopped
	s.mu.Unlock()

	s.log.Info("InputTracker service stopped")
	return nil
}

func (s *InputService) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	return s.Start(ctx)
}

func (s *InputService) GetCurrentInputMetrics() models.InputActivity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	return models.InputActivity{
		KeyboardCount: s.accumKeypresses,
		MouseClicks:   s.accumClicks,
		MouseMoveDist: s.accumDistance,
		IdleTimeSec:   s.lastIdleTime,
		IntervalStart: s.intervalStart,
		IntervalEnd:   now,
	}
}

func (s *InputService) Status() models.ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return models.ServiceStatus{
		Name:      s.Name(),
		State:     s.state,
		Message:   fmt.Sprintf("Input tracker running: %d keys, %d clicks, %ds idle", s.accumKeypresses, s.accumClicks, s.lastIdleTime),
		LastCheck: time.Now(),
	}
}

func (s *InputService) Health() models.HealthReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	isHealthy := s.state == services.StateRunning || s.state == services.StateStopped
	return models.HealthReport{
		IsHealthy: isHealthy,
		Details: map[string]string{
			"state":         s.state,
			"idle_time_sec": fmt.Sprintf("%d", s.lastIdleTime),
		},
	}
}

func (s *InputService) runLoop(ctx context.Context) {
	sampleTicker := time.NewTicker(100 * time.Millisecond)
	defer sampleTicker.Stop()

	flushIntervalSec := s.cfg.PollIntervalSec
	if flushIntervalSec <= 0 {
		flushIntervalSec = 10
	}
	flushTicker := time.NewTicker(time.Duration(flushIntervalSec) * time.Second)
	defer flushTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sampleTicker.C:
			s.sampleInput()
		case <-flushTicker.C:
			s.mu.Lock()
			s.flushIntervalLocked(ctx)
			s.mu.Unlock()
		}
	}
}

func (s *InputService) sampleInput() {
	if s.tracker == nil {
		return
	}
	snap := s.tracker.Sample()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.accumKeypresses += snap.Keypresses
	s.accumClicks += snap.MouseClicks
	s.accumDistance += snap.MouseMoveDist
	s.lastIdleTime = snap.IdleTimeSec
}

func (s *InputService) flushIntervalLocked(ctx context.Context) {
	now := time.Now()
	if (s.accumKeypresses > 0 || s.accumClicks > 0 || s.accumDistance > 0) && s.db != nil {
		act := &models.InputActivity{
			KeyboardCount: s.accumKeypresses,
			MouseClicks:   s.accumClicks,
			MouseMoveDist: s.accumDistance,
			ScrollCount:   0,
			IdleTimeSec:   s.lastIdleTime,
			IntervalStart: s.intervalStart,
			IntervalEnd:   now,
		}
		if _, err := s.db.InsertInput(ctx, act); err != nil {
			s.log.Error("Failed to record input activity", "error", err)
		} else {
			s.log.Debug("Flushed input activity to database", "keypresses", act.KeyboardCount, "clicks", act.MouseClicks)
			payload, err := json.Marshal(act)
			if err == nil {
				_, _ = s.db.InsertQueue(ctx, &models.SyncQueueItem{
					PayloadType: "input",
					PayloadJSON: string(payload),
					Status:      "pending",
				})
			}
		}

	}

	// Reset accumulators for next interval
	s.intervalStart = now
	s.accumKeypresses = 0
	s.accumClicks = 0
	s.accumDistance = 0
}
