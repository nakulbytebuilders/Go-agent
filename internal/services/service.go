package services

import (
	"context"

	"github.com/monitoring-agent/agent/internal/models"
)

const (
	StateStopped  = "STOPPED"
	StateStarting = "STARTING"
	StateRunning  = "RUNNING"
	StateStopping = "STOPPING"
	StateError    = "ERROR"
)

type Service interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	Status() models.ServiceStatus
	Health() models.HealthReport
}
