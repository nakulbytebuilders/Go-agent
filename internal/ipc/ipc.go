package ipc

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/monitoring-agent/agent/internal/controller"
	"github.com/monitoring-agent/agent/internal/models"
)

type AgentIPCBridge struct {
	controller *controller.AgentController
}

func NewAgentIPCBridge(ctl *controller.AgentController) *AgentIPCBridge {
	return &AgentIPCBridge{
		controller: ctl,
	}
}

func (b *AgentIPCBridge) GetServicesStatus() map[string]models.ServiceStatus {
	return b.controller.GetServiceStatuses()
}

func (b *AgentIPCBridge) StartService(name string) (bool, string) {
	err := b.controller.StartService(name)
	if err != nil {
		return false, err.Error()
	}
	return true, "Service started successfully"
}

func (b *AgentIPCBridge) StopService(name string) (bool, string) {
	err := b.controller.StopService(name)
	if err != nil {
		return false, err.Error()
	}
	return true, "Service stopped successfully"
}

func (b *AgentIPCBridge) RestartService(name string) (bool, string) {
	err := b.controller.RestartService(name)
	if err != nil {
		return false, err.Error()
	}
	return true, "Service restarted successfully"
}

func (b *AgentIPCBridge) GetRecentScreenshots(limit int) []models.ScreenshotRecord {
	if limit <= 0 {
		limit = 10
	}
	records, err := b.controller.GetRecentScreenshots(limit)
	if err != nil {
		return []models.ScreenshotRecord{}
	}
	return records
}

func (b *AgentIPCBridge) TakeScreenshotNow() (bool, string) {
	rec, err := b.controller.TakeScreenshotNow()
	if err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("Screenshot captured: %s", rec.FilePath)
}

func (b *AgentIPCBridge) GetScreenshotBase64(filePath string) string {
	if filePath == "" {
		return ""
	}
	cleanPath := filepath.Clean(filePath)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
}

func (b *AgentIPCBridge) GetRecentAppActivities(limit int) []models.AppActivity {
	if limit <= 0 {
		limit = 50
	}
	records, err := b.controller.GetRecentAppActivities(limit)
	if err != nil {
		return []models.AppActivity{}
	}
	return records
}

func (b *AgentIPCBridge) GetAppUsageStats(limit int) []models.AppUsageStat {
	if limit <= 0 {
		limit = 10
	}
	stats, err := b.controller.GetAppUsageStats(limit)
	if err != nil {
		return []models.AppUsageStat{}
	}
	return stats
}

func (b *AgentIPCBridge) GetCurrentActiveApp() models.AppActivity {
	act, err := b.controller.GetCurrentActiveApp()
	if err != nil || act == nil {
		return models.AppActivity{}
	}
	return *act
}

func (b *AgentIPCBridge) GetRecentBrowserActivities(limit int) []models.BrowserActivity {
	if limit <= 0 {
		limit = 50
	}
	records, err := b.controller.GetRecentBrowserActivities(limit)
	if err != nil {
		return []models.BrowserActivity{}
	}
	return records
}

func (b *AgentIPCBridge) GetDomainUsageStats(limit int) []models.BrowserUsageStat {
	if limit <= 0 {
		limit = 10
	}
	stats, err := b.controller.GetDomainUsageStats(limit)
	if err != nil {
		return []models.BrowserUsageStat{}
	}
	return stats
}

func (b *AgentIPCBridge) GetCurrentActiveBrowser() models.BrowserActivity {
	act, err := b.controller.GetCurrentActiveBrowser()
	if err != nil || act == nil {
		return models.BrowserActivity{}
	}
	return *act
}

func (b *AgentIPCBridge) GetUnifiedActivities(limit int) []models.UnifiedActivity {
	if limit <= 0 {
		limit = 50
	}
	records, err := b.controller.GetUnifiedActivities(limit)
	if err != nil {
		return []models.UnifiedActivity{}
	}
	return records
}

func (b *AgentIPCBridge) GetCurrentActiveFocus() models.UnifiedActivity {
	act, err := b.controller.GetCurrentActiveFocus()
	if err != nil || act == nil {
		return models.UnifiedActivity{}
	}
	return *act
}

func (b *AgentIPCBridge) SetScreenshotInterval(intervalSec int) string {
	err := b.controller.SetScreenshotInterval(intervalSec)
	if err != nil {
		return fmt.Sprintf("Error setting interval: %v", err)
	}
	return fmt.Sprintf("Screenshot interval successfully set to %d seconds", intervalSec)
}

func (b *AgentIPCBridge) GetScreenshotInterval() int {
	return b.controller.GetScreenshotInterval()
}




