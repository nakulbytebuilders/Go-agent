package screenshot

import (
	"context"
	"encoding/json"
	"fmt"

	"image"
	"image/jpeg"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/database"
	"github.com/monitoring-agent/agent/internal/models"
	"github.com/monitoring-agent/agent/internal/services"
)

type ScreenshotService struct {
	mu       sync.RWMutex
	db       *database.DatabaseManager
	cfg      config.ScreenshotConfig
	log      *slog.Logger
	state    string
	cancel   context.CancelFunc
	enabled  bool
	blur     bool
	inputSvc services.Service
}

func NewScreenshotService(db *database.DatabaseManager, cfg config.ScreenshotConfig, log *slog.Logger, inputSvc services.Service) *ScreenshotService {
	return &ScreenshotService{
		db:       db,
		cfg:      cfg,
		log:      log,
		state:    services.StateStopped,
		enabled:  true,
		blur:     false,
		inputSvc: inputSvc,
	}
}

func (s *ScreenshotService) Name() string {
	return "screenshot"
}

func (s *ScreenshotService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state == services.StateRunning {
		s.mu.Unlock()
		return nil
	}
	s.state = services.StateStarting
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.state = services.StateRunning
	s.mu.Unlock()

	s.log.Info("Screenshot service starting", "interval_sec", s.cfg.IntervalSec, "quality", s.cfg.Quality)
	go s.runLoop(runCtx)
	return nil
}

func (s *ScreenshotService) Stop(ctx context.Context) error {
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
	s.state = services.StateStopped
	s.mu.Unlock()

	s.log.Info("Screenshot service stopped")
	return nil
}

func (s *ScreenshotService) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	return s.Start(ctx)
}

func (s *ScreenshotService) Status() models.ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return models.ServiceStatus{
		Name:      s.Name(),
		State:     s.state,
		Message:   fmt.Sprintf("Screenshot capture active: interval %ds", s.cfg.IntervalSec),
		LastCheck: time.Now(),
	}
}

func (s *ScreenshotService) Health() models.HealthReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	isHealthy := s.state == services.StateRunning || s.state == services.StateStopped
	return models.HealthReport{
		IsHealthy: isHealthy,
		Details: map[string]string{
			"state": s.state,
		},
	}
}

func (s *ScreenshotService) TakeScreenshot(ctx context.Context) (*models.ScreenshotRecord, error) {
	img, w, h, err := CaptureScreen()
	if err != nil {
		s.log.Error("Failed to capture screen", "error", err)
		return nil, err
	}

	if s.IsBlurred() {
		img = blurImage(img, 16)
	}

	if err := os.MkdirAll("data/screenshots", 0755); err != nil {
		s.log.Error("Failed to create screenshot dir", "error", err)
		return nil, err
	}

	timestamp := time.Now().Format("20060102_150405")
	filePath := fmt.Sprintf("data/screenshots/screenshot_%s.jpg", timestamp)

	f, err := os.Create(filePath)
	if err != nil {
		s.log.Error("Failed to create screenshot file", "error", err)
		return nil, err
	}
	defer f.Close()

	quality := s.cfg.Quality
	if quality <= 0 {
		quality = 80
	}

	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
		s.log.Error("Failed to encode JPEG screenshot", "error", err)
		return nil, err
	}

	fi, err := f.Stat()
	fileSize := int64(0)
	if err == nil {
		fileSize = fi.Size()
	}

	keyPressCount := int64(0)
	mouseClickCount := int64(0)
	if s.inputSvc != nil {
		if inputTracker, ok := s.inputSvc.(interface{ GetCurrentInputMetrics() models.InputActivity }); ok {
			metrics := inputTracker.GetCurrentInputMetrics()
			keyPressCount = metrics.KeyboardCount
			mouseClickCount = metrics.MouseClicks
		}
	}

	rec := &models.ScreenshotRecord{
		FilePath:        filePath,
		FileSize:        fileSize,
		Width:           w,
		Height:          h,
		KeyPressCount:   keyPressCount,
		MouseClickCount: mouseClickCount,
		CapturedAt:      time.Now(),
		SyncStatus:      "pending",
	}

	if s.db != nil {
		id, err := s.db.InsertScreenshot(ctx, rec)
		if err == nil {
			rec.ID = id
			payload, err := json.Marshal(rec)
			if err == nil {
				_, _ = s.db.InsertQueue(ctx, &models.SyncQueueItem{
					PayloadType: "screenshot",
					PayloadJSON: string(payload),
					Status:      "pending",
				})
			}
		}
	}

	s.log.Info("Screenshot captured successfully", "file", filePath, "width", w, "height", h, "size", fileSize)
	return rec, nil
}

func (s *ScreenshotService) runLoop(ctx context.Context) {
	interval := s.cfg.IntervalSec
	if interval <= 0 {
		interval = 60
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	s.mu.RLock()
	isEnabled := s.enabled
	s.mu.RUnlock()
	if isEnabled {
		_, _ = s.TakeScreenshot(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			isEnabled = s.enabled
			s.mu.RUnlock()
			if isEnabled {
				_, _ = s.TakeScreenshot(ctx)
			} else {
				s.log.Debug("Screenshot capture skipped (disabled by server policy)")
			}
		}
	}
}

func (s *ScreenshotService) SetInterval(intervalSec int) {
	s.mu.Lock()
	if intervalSec <= 0 {
		intervalSec = 60
	}
	s.cfg.IntervalSec = intervalSec
	isRunning := s.state == services.StateRunning
	s.mu.Unlock()

	s.log.Info("Updated screenshot interval", "interval_sec", intervalSec)

	if isRunning {
		ctx := context.Background()
		_ = s.Restart(ctx)
	}
}

func (s *ScreenshotService) GetInterval() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.IntervalSec <= 0 {
		return 60
	}
	return s.cfg.IntervalSec
}

func (s *ScreenshotService) SetEnabled(enabled bool) {
	s.mu.Lock()
	prev := s.enabled
	s.enabled = enabled
	s.mu.Unlock()
	if prev != enabled {
		s.log.Info("Screenshot capture policy updated", "enabled", enabled)
		if !prev && enabled {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, _ = s.TakeScreenshot(ctx)
			}()
		}
	}
}

func (s *ScreenshotService) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *ScreenshotService) SetBlur(blur bool) {
	s.mu.Lock()
	prev := s.blur
	s.blur = blur
	s.mu.Unlock()
	if prev != blur {
		s.log.Info("Screenshot blur policy updated", "blur", blur)
	}
}

func (s *ScreenshotService) IsBlurred() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blur
}

func blurImage(src image.Image, factor int) image.Image {
	if factor <= 1 {
		return src
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(bounds)

	blockSize := factor
	for y := 0; y < h; y += blockSize {
		for x := 0; x < w; x += blockSize {
			c := src.At(x, y)
			for dy := 0; dy < blockSize && y+dy < h; dy++ {
				for dx := 0; dx < blockSize && x+dx < w; dx++ {
					dst.Set(x+dx, y+dy, c)
				}
			}
		}
	}
	return dst
}
