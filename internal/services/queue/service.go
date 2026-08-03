package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/monitoring-agent/agent/internal/database"
	"github.com/monitoring-agent/agent/internal/models"
	"github.com/monitoring-agent/agent/internal/services"
)

type QueueService struct {
	mu     sync.RWMutex
	db     *database.DatabaseManager
	log    *slog.Logger
	state  string
	cancel context.CancelFunc
}

func NewQueueService(db *database.DatabaseManager, log *slog.Logger) *QueueService {
	return &QueueService{
		db:    db,
		log:   log,
		state: services.StateStopped,
	}
}

func (s *QueueService) Name() string {
	return "queue"
}

func (s *QueueService) Start(ctx context.Context) error {
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

	s.log.Info("Queue service starting")
	go s.runLoop(runCtx)
	return nil
}

func (s *QueueService) Stop(ctx context.Context) error {
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

	s.log.Info("Queue service stopped")
	return nil
}

func (s *QueueService) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	return s.Start(ctx)
}

func (s *QueueService) Enqueue(ctx context.Context, payloadType string, payloadJSON string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	item := &models.SyncQueueItem{
		PayloadType: payloadType,
		PayloadJSON: payloadJSON,
		Status:      "pending",
		RetryCount:  0,
	}

	id, err := s.db.InsertQueue(ctx, item)
	if err != nil {
		s.log.Error("Failed to enqueue payload", "type", payloadType, "error", err)
		return 0, err
	}
	s.log.Debug("Enqueued payload to sync_queue", "id", id, "type", payloadType)
	return id, nil
}

func (s *QueueService) GetPendingQueue(ctx context.Context, limit int) ([]models.SyncQueueItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return s.db.FetchPendingQueue(ctx, limit)
}

func (s *QueueService) Status() models.ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msg := "Offline queue service active"
	if s.db != nil {
		stats, err := s.db.GetSyncStats(context.Background())
		if err == nil {
			msg = fmt.Sprintf("Queue active: %d pending, %d completed", stats["pending"], stats["completed"])
		}
	}

	return models.ServiceStatus{
		Name:      s.Name(),
		State:     s.state,
		Message:   msg,
		LastCheck: time.Now(),
	}
}

func (s *QueueService) Health() models.HealthReport {
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

func (s *QueueService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkPendingItems(ctx)
		}
	}
}

func (s *QueueService) checkPendingItems(ctx context.Context) {
	if s.db == nil {
		return
	}
	items, err := s.db.FetchPendingQueue(ctx, 10)
	if err != nil {
		s.log.Debug("Error checking pending queue items", "error", err)
		return
	}
	if len(items) > 0 {
		s.log.Debug("Pending queue items ready for sync", "count", len(items))
	}
}
