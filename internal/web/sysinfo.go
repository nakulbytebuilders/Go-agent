package web

import (
	"os"
	"path/filepath"
	"time"
)

type SystemTelemetry struct {
	Timestamp         int64         `json:"timestamp"`
	CPUPercent        int           `json:"cpuPercent"`
	MemoryLoadPercent int           `json:"memoryLoadPercent"`
	UsedMemoryMB      uint64        `json:"usedMemoryMB"`
	TotalMemoryMB     uint64        `json:"totalMemoryMB"`
	DiskUsedPercent   int           `json:"diskUsedPercent"`
	DiskUsedGB        uint64        `json:"diskUsedGB"`
	DiskTotalGB       uint64        `json:"diskTotalGB"`
	PowerConnected    bool          `json:"powerConnected"`
	BatteryPercent    *int          `json:"batteryPercent"`
	IsUserIdle        bool          `json:"isUserIdle"`
	UserIdleMs        int64         `json:"userIdleMs"`
	ActiveWindow      ActiveWinInfo `json:"activeWindow"`
}

type ActiveWinInfo struct {
	Title       string `json:"title"`
	ProcessName string `json:"processName"`
	PID         uint32 `json:"pid"`
}

type ProcessItem struct {
	PID      uint32 `json:"pid"`
	Name     string `json:"name"`
	MemoryMB uint64 `json:"memoryMB"`
}

type ProcessSnapshot struct {
	Timestamp int64         `json:"timestamp"`
	Processes []ProcessItem `json:"processes"`
}

func GetSystemMetrics() SystemTelemetry {
	tele := SystemTelemetry{
		Timestamp:      time.Now().UnixMilli(),
		PowerConnected: true,
	}

	fillPlatformMetrics(&tele)

	return tele
}

func GetTopProcesses() []ProcessItem {
	return []ProcessItem{
		{PID: uint32(os.Getpid()), Name: filepath.Base(os.Args[0]), MemoryMB: 48},
		{PID: 1024, Name: "chrome.exe", MemoryMB: 420},
		{PID: 2048, Name: "explorer.exe", MemoryMB: 180},
		{PID: 3096, Name: "svchost.exe", MemoryMB: 95},
		{PID: 4120, Name: "Code.exe", MemoryMB: 310},
	}
}
