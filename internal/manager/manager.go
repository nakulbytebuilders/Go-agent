package manager

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/monitoring-agent/agent/internal/models"
	"github.com/monitoring-agent/agent/internal/services"
)

type ServiceManager struct {
	mu       sync.RWMutex
	services map[string]services.Service
	log      *slog.Logger
}

func NewServiceManager(log *slog.Logger) *ServiceManager {
	return &ServiceManager{
		services: make(map[string]services.Service),
		log:      log,
	}
}

func (sm *ServiceManager) Register(svc services.Service) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	name := svc.Name()
	if _, exists := sm.services[name]; exists {
		return fmt.Errorf("service '%s' is already registered", name)
	}

	sm.services[name] = svc
	sm.log.Info("Service registered dynamically", "service", name)
	return nil
}

func (sm *ServiceManager) Start(ctx context.Context, serviceName string) error {
	sm.mu.RLock()
	svc, exists := sm.services[serviceName]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service '%s' not found", serviceName)
	}

	sm.log.Info("Starting service", "service", serviceName)
	return svc.Start(ctx)
}

func (sm *ServiceManager) Stop(ctx context.Context, serviceName string) error {
	sm.mu.RLock()
	svc, exists := sm.services[serviceName]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service '%s' not found", serviceName)
	}

	sm.log.Info("Stopping service", "service", serviceName)
	return svc.Stop(ctx)
}

func (sm *ServiceManager) Restart(ctx context.Context, serviceName string) error {
	sm.mu.RLock()
	svc, exists := sm.services[serviceName]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service '%s' not found", serviceName)
	}

	sm.log.Info("Restarting service", "service", serviceName)
	return svc.Restart(ctx)
}

func (sm *ServiceManager) StartAll(ctx context.Context) map[string]error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	results := make(map[string]error)
	for name, svc := range sm.services {
		if err := svc.Start(ctx); err != nil {
			sm.log.Error("Failed to start service", "service", name, "error", err)
			results[name] = err
		} else {
			results[name] = nil
		}
	}
	return results
}

func (sm *ServiceManager) StopAll(ctx context.Context) map[string]error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	results := make(map[string]error)
	for name, svc := range sm.services {
		if err := svc.Stop(ctx); err != nil {
			sm.log.Error("Failed to stop service", "service", name, "error", err)
			results[name] = err
		} else {
			results[name] = nil
		}
	}
	return results
}

func (sm *ServiceManager) GetStatus() map[string]models.ServiceStatus {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	statuses := make(map[string]models.ServiceStatus)
	for name, svc := range sm.services {
		statuses[name] = svc.Status()
	}
	return statuses
}

func (sm *ServiceManager) GetService(serviceName string) (services.Service, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	svc, ok := sm.services[serviceName]
	return svc, ok
}
