package browsertracker

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

type BrowserTrackerService struct {
	mu             sync.RWMutex
	db             *database.DatabaseManager
	cfg            config.BrowserTrackerConfig
	log            *slog.Logger
	state          string
	cancel         context.CancelFunc

	// Active browser session state
	currentBrowser ActiveBrowserInfo
	sessionStart   time.Time
	lastPoll       time.Time
}

// maxPollGap is the longest silence between two polls that still counts as
// normal. A longer one means the machine slept or the process froze, and that
// time must not be added to whichever page was active before it.
func maxPollGap(pollIntervalSec int) time.Duration {
	gap := 30 * time.Second
	if scaled := time.Duration(pollIntervalSec) * 5 * time.Second; scaled > gap {
		gap = scaled
	}
	return gap
}

func NewBrowserTrackerService(db *database.DatabaseManager, cfg config.BrowserTrackerConfig, log *slog.Logger) *BrowserTrackerService {
	return &BrowserTrackerService{
		db:    db,
		cfg:   cfg,
		log:   log,
		state: services.StateStopped,
	}
}

func (s *BrowserTrackerService) Name() string {
	return "browserTracker"
}

func (s *BrowserTrackerService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state == services.StateRunning {
		s.mu.Unlock()
		return nil
	}
	s.state = services.StateStarting
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.state = services.StateRunning

	// Initialize active session info
	info, _ := getActiveBrowserInfo()
	s.currentBrowser = info
	// Round(0) drops the monotonic reading so that time spent asleep is visible.
	s.sessionStart = time.Now().Round(0)
	s.lastPoll = s.sessionStart

	s.mu.Unlock()

	s.log.Info("BrowserTracker service starting", "poll_interval_sec", s.cfg.PollIntervalSec)
	go s.runLoop(runCtx)
	return nil
}

func (s *BrowserTrackerService) Stop(ctx context.Context) error {
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

	// Flush active browser session to DB before stopping
	if s.currentBrowser.IsBrowser && s.currentBrowser.BrowserName != "" && !s.sessionStart.IsZero() {
		s.flushCurrentActivityLocked(ctx)
	}

	s.state = services.StateStopped
	s.mu.Unlock()

	s.log.Info("BrowserTracker service stopped")
	return nil
}

func (s *BrowserTrackerService) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	return s.Start(ctx)
}

func (s *BrowserTrackerService) GetCurrentActiveBrowser() models.BrowserActivity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.currentBrowser.IsBrowser {
		return models.BrowserActivity{
			BrowserName: "None Active",
			TabTitle:    "No active browser in foreground",
			Domain:      "-",
		}
	}

	now := time.Now()
	duration := int64(now.Sub(s.sessionStart).Seconds())
	if duration < 0 {
		duration = 0
	}

	return models.BrowserActivity{
		BrowserName: s.currentBrowser.BrowserName,
		TabTitle:    s.currentBrowser.TabTitle,
		Domain:      s.currentBrowser.Domain,
		URL:         s.currentBrowser.URL,
		DurationSec: duration,
		StartTime:   s.sessionStart,
		EndTime:     now,
	}
}

func (s *BrowserTrackerService) Status() models.ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msg := "Browser tracker running: waiting for browser..."
	if s.currentBrowser.IsBrowser {
		msg = fmt.Sprintf("Browser tracker running: '%s' (%s)", s.currentBrowser.BrowserName, s.currentBrowser.Domain)
	}

	return models.ServiceStatus{
		Name:      s.Name(),
		State:     s.state,
		Message:   msg,
		LastCheck: time.Now(),
	}
}

func (s *BrowserTrackerService) Health() models.HealthReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	isHealthy := s.state == services.StateRunning || s.state == services.StateStopped
	return models.HealthReport{
		IsHealthy: isHealthy,
		Details: map[string]string{
			"state":          s.state,
			"active_browser": s.currentBrowser.BrowserName,
			"active_domain":  s.currentBrowser.Domain,
		},
	}
}

func (s *BrowserTrackerService) runLoop(ctx context.Context) {
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
			s.pollActiveBrowser(ctx)
		}
	}
}

func (s *BrowserTrackerService) pollActiveBrowser(ctx context.Context) {
	info, err := getActiveBrowserInfo()
	if err != nil {
		s.log.Debug("Failed to get active browser info", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Round(0)

	// After a sleep or freeze, close the old segment where the last poll saw it
	// instead of at "now", and start over from now.
	if !s.lastPoll.IsZero() && now.Sub(s.lastPoll) > maxPollGap(s.cfg.PollIntervalSec) {
		if s.currentBrowser.IsBrowser {
			s.flushUntilLocked(ctx, s.lastPoll)
		}
		s.currentBrowser = info
		s.sessionStart = now
		s.lastPoll = now
		return
	}
	s.lastPoll = now

	// Check if active browser, tab title, domain changed, OR if 10 seconds elapsed in current session
	if info.IsBrowser != s.currentBrowser.IsBrowser ||
		info.BrowserName != s.currentBrowser.BrowserName ||
		info.TabTitle != s.currentBrowser.TabTitle ||
		info.Domain != s.currentBrowser.Domain ||
		now.Sub(s.sessionStart) >= 10*time.Second {

		// Flush previous browser activity session if it was a browser
		if s.currentBrowser.IsBrowser {
			s.flushCurrentActivityLocked(ctx)
		}

		// Update to new browser session segment
		s.currentBrowser = info
		s.sessionStart = now

		if info.IsBrowser {
			s.log.Debug("Active browser tab session segment recorded", "browser", info.BrowserName, "domain", info.Domain, "title", info.TabTitle)
		}
	}
}

func (s *BrowserTrackerService) flushCurrentActivityLocked(ctx context.Context) {
	s.flushUntilLocked(ctx, time.Now().Round(0))
}

// flushUntilLocked records the current segment as ending at end.
func (s *BrowserTrackerService) flushUntilLocked(ctx context.Context, end time.Time) {
	now := end
	durationSec := int64(now.Sub(s.sessionStart).Seconds())

	if durationSec > 0 && s.currentBrowser.IsBrowser && s.db != nil {
		b := &models.BrowserActivity{
			BrowserName: s.currentBrowser.BrowserName,
			TabTitle:    s.currentBrowser.TabTitle,
			Domain:      s.currentBrowser.Domain,
			URL:         s.currentBrowser.URL,
			DurationSec: durationSec,
			StartTime:   s.sessionStart,
			EndTime:     now,
		}
		if id, err := s.db.InsertBrowser(ctx, b); err != nil {
			s.log.Error("Failed to record browser activity", "error", err)
		} else {
			b.ID = id
			s.log.Debug("Recorded browser activity", "domain", b.Domain, "duration_sec", durationSec)
			payload, err := json.Marshal(b)
			if err == nil {
				_, _ = s.db.InsertQueue(ctx, &models.SyncQueueItem{
					PayloadType: "browser",
					PayloadJSON: string(payload),
					Status:      "pending",
				})
			}
		}

	}
}
