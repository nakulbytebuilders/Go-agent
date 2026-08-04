package controller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/database"
	"github.com/monitoring-agent/agent/internal/logger"
	"github.com/monitoring-agent/agent/internal/manager"
	"github.com/monitoring-agent/agent/internal/models"

	"github.com/monitoring-agent/agent/internal/services/apptracker"
	"github.com/monitoring-agent/agent/internal/services/browsertracker"
	"github.com/monitoring-agent/agent/internal/services/input"
	"github.com/monitoring-agent/agent/internal/services/queue"
	"github.com/monitoring-agent/agent/internal/services/screenshot"
	syncservice "github.com/monitoring-agent/agent/internal/services/sync"
)

type AgentController struct {
	mu             sync.Mutex
	cfg            *config.Config
	db             *database.DatabaseManager
	loggerManager  *logger.LoggerManager
	log            *slog.Logger
	serviceManager *manager.ServiceManager
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewAgentController(configPath string) (*AgentController, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	lm, err := logger.Init(cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	agentLog := lm.AgentLogger
	agentLog.Info("Initializing Agent Controller...")

	db, err := database.NewDatabaseManager(cfg.Database)
	if err != nil {
		agentLog.Error("Failed to initialize database", "error", err)
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	sm := manager.NewServiceManager(agentLog)
	ctx, cancel := context.WithCancel(context.Background())

	ctl := &AgentController{
		cfg:            cfg,
		db:             db,
		loggerManager:  lm,
		log:            agentLog,
		serviceManager: sm,
		ctx:            ctx,
		cancel:         cancel,
	}

	if err := ctl.registerServices(); err != nil {
		return nil, fmt.Errorf("failed to register services: %w", err)
	}

	return ctl, nil
}

func (c *AgentController) registerServices() error {
	inputSvc := input.NewInputService(c.db, c.cfg.Input, c.log)
	appSvc := apptracker.NewAppTrackerService(c.db, c.cfg.AppTracker, c.log)
	browserSvc := browsertracker.NewBrowserTrackerService(c.db, c.cfg.BrowserTracker, c.log)
	ssSvc := screenshot.NewScreenshotService(c.db, c.cfg.Screenshot, c.log, inputSvc)
	queueSvc := queue.NewQueueService(c.db, c.log)
	syncSvc := syncservice.NewSyncService(c.db, c.cfg.Sync, c.cfg.Server, logger.GetSyncLogger())

	if err := c.serviceManager.Register(appSvc); err != nil {
		return err
	}
	if err := c.serviceManager.Register(browserSvc); err != nil {
		return err
	}
	if err := c.serviceManager.Register(ssSvc); err != nil {
		return err
	}
	if err := c.serviceManager.Register(inputSvc); err != nil {
		return err
	}
	if err := c.serviceManager.Register(queueSvc); err != nil {
		return err
	}
	if err := c.serviceManager.Register(syncSvc); err != nil {
		return err
	}

	return nil
}

func (c *AgentController) StartEnabledServices() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.log.Info("Starting enabled monitoring services...")

	if c.cfg.Services.Queue {
		_ = c.serviceManager.Start(c.ctx, "queue")
	}
	if c.cfg.Services.AppTracker {
		_ = c.serviceManager.Start(c.ctx, "appTracker")
	}
	if c.cfg.Services.BrowserTracker {
		_ = c.serviceManager.Start(c.ctx, "browserTracker")
	}
	if c.cfg.Services.Screenshot {
		_ = c.serviceManager.Start(c.ctx, "screenshot")
	}
	if c.cfg.Services.Input {
		_ = c.serviceManager.Start(c.ctx, "input")
	}
	if c.cfg.Services.Sync {
		_ = c.serviceManager.Start(c.ctx, "sync")
	}

	// Start background policy poller to fetch server Configuration settings
	go c.runPolicyPoller(c.ctx)
}

func (c *AgentController) runPolicyPoller(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}

	c.log.Info("Policy poller started — polling server Configuration every 5 seconds")

	c.applyServerPolicy(ctx)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.log.Info("Policy poller stopped")
			return
		case <-ticker.C:
			c.applyServerPolicy(ctx)
		}
	}
}

func (c *AgentController) applyServerPolicy(ctx context.Context) {
	svc, ok := c.serviceManager.GetService("sync")
	if !ok {
		return
	}
	syncSvc, ok := svc.(*syncservice.SyncService)
	if !ok {
		return
	}

	policy, err := syncSvc.FetchPolicy(ctx)
	if err != nil {
		c.log.Warn("Failed to fetch server policy", "error", err)
		return
	}

	if policy.IntervalMs > 0 {
		newIntervalSec := policy.IntervalMs / 1000
		if newIntervalSec < 5 {
			newIntervalSec = 5
		}
		currentInterval := c.GetScreenshotInterval()
		if newIntervalSec != currentInterval {
			c.log.Info("Server policy: updating screenshot interval",
				"old_sec", currentInterval,
				"new_sec", newIntervalSec,
			)
			_ = c.SetScreenshotInterval(newIntervalSec)
		}
	}

	if ssSvc, ok := c.serviceManager.GetService("screenshot"); ok {
		if scSvc, ok := ssSvc.(*screenshot.ScreenshotService); ok {
			scSvc.SetEnabled(policy.CaptureScreenshots && policy.Enabled)
			scSvc.SetBlur(policy.BlurScreenshots)
		}
	}

	c.log.Info("Server policy applied successfully",
		"tracking_enabled", policy.Enabled,
		"capture_screenshots", policy.CaptureScreenshots,
		"blur_screenshots", policy.BlurScreenshots,
		"interval_sec", policy.IntervalMs/1000,
	)
}

func (c *AgentController) GetConfig() *config.Config {
	return c.cfg
}

func (c *AgentController) StartService(name string) error {
	return c.serviceManager.Start(c.ctx, name)
}

func (c *AgentController) StopService(name string) error {
	return c.serviceManager.Stop(c.ctx, name)
}

func (c *AgentController) RestartService(name string) error {
	return c.serviceManager.Restart(c.ctx, name)
}

func (c *AgentController) GetServiceStatuses() map[string]models.ServiceStatus {
	return c.serviceManager.GetStatus()
}

func (c *AgentController) GetRecentScreenshots(limit int) ([]models.ScreenshotRecord, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetRecentScreenshots(c.ctx, limit)
}

func (c *AgentController) TakeScreenshotNow() (*models.ScreenshotRecord, error) {
	svc, ok := c.serviceManager.GetService("screenshot")
	if !ok {
		return nil, fmt.Errorf("screenshot service not found")
	}
	ssSvc, ok := svc.(*screenshot.ScreenshotService)
	if !ok {
		return nil, fmt.Errorf("invalid screenshot service type")
	}
	return ssSvc.TakeScreenshot(c.ctx)
}

func (c *AgentController) GetRecentAppActivities(limit int) ([]models.AppActivity, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetRecentAppActivities(c.ctx, limit)
}

func (c *AgentController) GetAppUsageStats(limit int) ([]models.AppUsageStat, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetAppUsageStats(c.ctx, limit)
}

func (c *AgentController) GetCurrentActiveApp() (*models.AppActivity, error) {
	svc, ok := c.serviceManager.GetService("appTracker")
	if !ok {
		return nil, fmt.Errorf("appTracker service not found")
	}
	appSvc, ok := svc.(*apptracker.AppTrackerService)
	if !ok {
		return nil, fmt.Errorf("invalid appTracker service type")
	}
	act := appSvc.GetCurrentActiveApp()
	return &act, nil
}

func (c *AgentController) GetRecentBrowserActivities(limit int) ([]models.BrowserActivity, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetRecentBrowserActivities(c.ctx, limit)
}

func (c *AgentController) GetDomainUsageStats(limit int) ([]models.BrowserUsageStat, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetDomainUsageStats(c.ctx, limit)
}

func (c *AgentController) GetCurrentActiveBrowser() (*models.BrowserActivity, error) {
	svc, ok := c.serviceManager.GetService("browserTracker")
	if !ok {
		return nil, fmt.Errorf("browserTracker service not found")
	}
	bSvc, ok := svc.(*browsertracker.BrowserTrackerService)
	if !ok {
		return nil, fmt.Errorf("invalid browserTracker service type")
	}
	act := bSvc.GetCurrentActiveBrowser()
	return &act, nil
}

func (c *AgentController) GetRecentInputActivities(limit int) ([]models.InputActivity, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetRecentInputActivities(c.ctx, limit)
}

func (c *AgentController) GetCurrentInputState() (*models.InputActivity, error) {
	svc, ok := c.serviceManager.GetService("input")
	if !ok {
		return nil, fmt.Errorf("input service not found")
	}
	inputSvc, ok := svc.(*input.InputService)
	if !ok {
		return nil, fmt.Errorf("invalid input service type")
	}
	act := inputSvc.GetCurrentInputMetrics()
	return &act, nil
}

func (c *AgentController) GetSyncQueueItems(limit int) ([]models.SyncQueueItem, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetSyncQueueItems(c.ctx, limit)
}

func (c *AgentController) GetSyncStats() (map[string]int64, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetSyncStats(c.ctx)
}

func (c *AgentController) TriggerSyncNow() (int, error) {
	svc, ok := c.serviceManager.GetService("sync")
	if !ok {
		return 0, fmt.Errorf("sync service not found")
	}
	syncSvc, ok := svc.(*syncservice.SyncService)
	if !ok {
		return 0, fmt.Errorf("invalid sync service type")
	}
	return syncSvc.SyncNow(c.ctx)
}

func (c *AgentController) GetUnifiedActivities(limit int) ([]models.UnifiedActivity, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.db.GetUnifiedActivities(c.ctx, limit)
}

func (c *AgentController) GetCurrentActiveFocus() (*models.UnifiedActivity, error) {
	if bSvc, ok := c.serviceManager.GetService("browserTracker"); ok {
		if tracker, ok := bSvc.(*browsertracker.BrowserTrackerService); ok {
			b := tracker.GetCurrentActiveBrowser()
			if b.BrowserName != "None Active" && b.BrowserName != "" {
				return &models.UnifiedActivity{
					Category:    "WEB",
					Name:        b.BrowserName,
					Title:       b.TabTitle,
					Domain:      b.Domain,
					DurationSec: b.DurationSec,
					StartTime:   b.StartTime,
					EndTime:     b.EndTime,
				}, nil
			}
		}
	}

	if appSvc, ok := c.serviceManager.GetService("appTracker"); ok {
		if tracker, ok := appSvc.(*apptracker.AppTrackerService); ok {
			app := tracker.GetCurrentActiveApp()
			return &models.UnifiedActivity{
				Category:    "APP",
				Name:        app.AppName,
				Title:       app.WindowTitle,
				PID:         app.PID,
				DurationSec: app.DurationSec,
				StartTime:   app.StartTime,
				EndTime:     app.EndTime,
			}, nil
		}
	}

	return &models.UnifiedActivity{
		Category: "APP",
		Name:     "System",
		Title:    "Idle Desktop",
	}, nil
}

func (c *AgentController) SetScreenshotInterval(intervalSec int) error {
	svc, ok := c.serviceManager.GetService("screenshot")
	if !ok {
		return fmt.Errorf("screenshot service not found")
	}
	scSvc, ok := svc.(*screenshot.ScreenshotService)
	if !ok {
		return fmt.Errorf("invalid screenshot service type")
	}
	scSvc.SetInterval(intervalSec)
	return nil
}

func (c *AgentController) GetScreenshotInterval() int {
	svc, ok := c.serviceManager.GetService("screenshot")
	if !ok {
		return 60
	}
	scSvc, ok := svc.(*screenshot.ScreenshotService)
	if !ok {
		return 60
	}
	return scSvc.GetInterval()
}

func (c *AgentController) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.log.Info("Initiating AgentController graceful shutdown...")

	if c.cancel != nil {
		c.cancel()
	}

	shutdownCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	_ = c.serviceManager.StopAll(shutdownCtx)

	if c.db != nil {
		_ = c.db.Close()
	}

	c.log.Info("AgentController shutdown complete.")
}
