// Package bootsvc is the part of the agent that must be alive from the
// moment the machine boots, whether or not anyone has logged in yet:
// enrollment and the heartbeat/policy poll that tells the dashboard the
// machine is reachable at all.
//
// It deliberately never touches the local activity database — that stays
// owned by the per-user agent.exe process (apptracker, browsertracker,
// screenshot, input all require a desktop session anyway), so the boot
// service and the user-session agent never contend for the same SQLite
// file. They do share one thing: the YAML config file, so enrollment
// happens exactly once no matter which of the two starts first (see
// config.UpdateServerCredentials / SyncService.SetConfigPath).
package bootsvc

import (
	"context"
	"log/slog"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	syncservice "github.com/monitoring-agent/agent/internal/services/sync"
	"github.com/monitoring-agent/agent/internal/wtssession"
)

// pollInterval is how often the boot service re-enrolls (if needed) and
// polls server policy, which doubles as its heartbeat. It intentionally
// ignores cfg.Server.HeartbeatIntervalSec: that setting is tuned for the
// user-session agent's much chattier sync loop, while this loop exists
// purely to prove the machine itself is reachable.
const pollInterval = 15 * time.Second

// Runner drives the boot-time heartbeat loop. Create one with New and call
// Run; Run blocks until its context is cancelled or Stop is called.
type Runner struct {
	log  *slog.Logger
	sync *syncservice.SyncService

	cancel context.CancelFunc
	done   chan struct{}
}

func New(cfg *config.Config, configPath string, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}

	// db is nil on purpose: FetchPolicy/enrollment are pure HTTP plus the
	// shared config file, and never reach into the database.
	syncSvc := syncservice.NewSyncService(nil, cfg.Sync, cfg.Server, log)
	syncSvc.SetConfigPath(configPath)

	return &Runner{
		log:  log,
		sync: syncSvc,
		done: make(chan struct{}),
	}
}

// Run polls immediately, then on pollInterval, until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	defer close(r.done)

	r.log.Info("Boot service runner starting", "poll_interval_sec", int(pollInterval.Seconds()))

	r.tick(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.Info("Boot service runner stopping")
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	hasSession := wtssession.HasActiveUserSession()

	if _, err := r.sync.FetchPolicy(ctx); err != nil {
		// FetchPolicy already reported this to the dashboard (see
		// SyncService.ReportFailure); this local line just keeps boot.log
		// readable on its own too.
		r.log.Warn("Boot heartbeat failed", "error", err, "has_user_session", hasSession)
		return
	}
	r.log.Info("Boot heartbeat sent", "has_user_session", hasSession)
}

// Stop cancels the running loop and blocks until it has actually exited.
// Safe to call only after Run has been started.
func (r *Runner) Stop() {
	if r.cancel == nil {
		return
	}
	r.cancel()
	<-r.done
}
