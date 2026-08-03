package manager_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/monitoring-agent/agent/internal/manager"
	"github.com/monitoring-agent/agent/internal/models"
	"github.com/monitoring-agent/agent/internal/services"
)

type DummyService struct {
	name  string
	state string
}

func (d *DummyService) Name() string { return d.name }
func (d *DummyService) Start(ctx context.Context) error {
	d.state = services.StateRunning
	return nil
}
func (d *DummyService) Stop(ctx context.Context) error {
	d.state = services.StateStopped
	return nil
}
func (d *DummyService) Restart(ctx context.Context) error {
	return d.Start(ctx)
}
func (d *DummyService) Status() models.ServiceStatus {
	return models.ServiceStatus{
		Name:      d.name,
		State:     d.state,
		LastCheck: time.Now(),
	}
}
func (d *DummyService) Health() models.HealthReport {
	return models.HealthReport{IsHealthy: true}
}

func TestServiceManagerLifecycle(t *testing.T) {
	discardLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	sm := manager.NewServiceManager(discardLogger)

	svc1 := &DummyService{name: "svc1", state: services.StateStopped}
	svc2 := &DummyService{name: "svc2", state: services.StateStopped}

	if err := sm.Register(svc1); err != nil {
		t.Fatalf("Failed to register svc1: %v", err)
	}
	if err := sm.Register(svc2); err != nil {
		t.Fatalf("Failed to register svc2: %v", err)
	}

	// Duplicate registration error check
	if err := sm.Register(svc1); err == nil {
		t.Errorf("Expected error registering duplicate service")
	}

	ctx := context.Background()
	results := sm.StartAll(ctx)
	for name, err := range results {
		if err != nil {
			t.Errorf("Service %s failed to start: %v", name, err)
		}
	}

	statusMap := sm.GetStatus()
	if statusMap["svc1"].State != services.StateRunning || statusMap["svc2"].State != services.StateRunning {
		t.Errorf("Expected services to be RUNNING, got %v", statusMap)
	}

	sm.StopAll(ctx)
	statusMapAfter := sm.GetStatus()
	if statusMapAfter["svc1"].State != services.StateStopped {
		t.Errorf("Expected svc1 to be STOPPED, got %s", statusMapAfter["svc1"].State)
	}
}
