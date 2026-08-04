package models

import (
	"time"
)

type AppActivity struct {
	ID          int64     `json:"id" db:"id"`
	AppName     string    `json:"app_name" db:"app_name"`
	WindowTitle string    `json:"window_title" db:"window_title"`
	PID         int32     `json:"pid" db:"pid"`
	DurationSec int64     `json:"duration_sec" db:"duration_sec"`
	StartTime   time.Time `json:"start_time" db:"start_time"`
	EndTime     time.Time `json:"end_time" db:"end_time"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type AppUsageStat struct {
	AppName          string    `json:"app_name" db:"app_name"`
	TotalDurationSec int64     `json:"total_duration_sec" db:"total_duration_sec"`
	LaunchCount      int64     `json:"launch_count" db:"launch_count"`
	LastActive       time.Time `json:"last_active" db:"last_active"`
}


type BrowserActivity struct {
	ID          int64     `json:"id" db:"id"`
	BrowserName string    `json:"browser_name" db:"browser_name"`
	TabTitle    string    `json:"tab_title" db:"tab_title"`
	Domain      string    `json:"domain" db:"domain"`
	URL         string    `json:"url" db:"url"`
	DurationSec int64     `json:"duration_sec" db:"duration_sec"`
	StartTime   time.Time `json:"start_time" db:"start_time"`
	EndTime     time.Time `json:"end_time" db:"end_time"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type BrowserUsageStat struct {
	Domain           string    `json:"domain" db:"domain"`
	TotalDurationSec int64     `json:"total_duration_sec" db:"total_duration_sec"`
	VisitCount       int64     `json:"visit_count" db:"visit_count"`
	LastVisited      time.Time `json:"last_visited" db:"last_visited"`
}

type UnifiedActivity struct {
	ID          int64     `json:"id" db:"id"`
	Category    string    `json:"category" db:"category"` // "APP" or "WEB"
	Name        string    `json:"name" db:"name"`         // App Name or Browser Name
	Title       string    `json:"title" db:"title"`       // Window Title or Tab Title
	Domain      string    `json:"domain" db:"domain"`     // Web domain (if WEB)
	PID         int32     `json:"pid" db:"pid"`
	DurationSec int64     `json:"duration_sec" db:"duration_sec"`
	StartTime   time.Time `json:"start_time" db:"start_time"`
	EndTime     time.Time `json:"end_time" db:"end_time"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type ActivityLog struct {
	ID          int64     `json:"id" db:"id"`
	Category    string    `json:"category" db:"category"` // "APP" or "WEB"
	AppName     string    `json:"app_name" db:"app_name"`
	WindowTitle string    `json:"window_title" db:"window_title"`
	Domain      string    `json:"domain" db:"domain"`
	URL         string    `json:"url" db:"url"`
	PID         int32     `json:"pid" db:"pid"`
	DurationSec int64     `json:"duration_sec" db:"duration_sec"`
	StartTime   time.Time `json:"start_time" db:"start_time"`
	EndTime     time.Time `json:"end_time" db:"end_time"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}




type ScreenshotRecord struct {
	ID              int64     `json:"id" db:"id"`
	FilePath        string    `json:"file_path" db:"file_path"`
	FileSize        int64     `json:"file_size" db:"file_size"`
	Width           int       `json:"width" db:"width"`
	Height          int       `json:"height" db:"height"`
	KeyPressCount   int64     `json:"key_press_count" db:"key_press_count"`
	MouseClickCount int64     `json:"mouse_click_count" db:"mouse_click_count"`
	CapturedAt      time.Time `json:"captured_at" db:"captured_at"`
	SyncStatus      string    `json:"sync_status" db:"sync_status"` // pending, uploaded, failed
}

type InputActivity struct {
	ID            int64     `json:"id" db:"id"`
	KeyboardCount int64     `json:"keyboard_count" db:"keyboard_count"`
	MouseClicks   int64     `json:"mouse_clicks" db:"mouse_clicks"`
	MouseMoveDist float64   `json:"mouse_move_dist" db:"mouse_move_dist"`
	ScrollCount   int64     `json:"scroll_count" db:"scroll_count"`
	IdleTimeSec   int64     `json:"idle_time_sec" db:"idle_time_sec"`
	IntervalStart time.Time `json:"interval_start" db:"interval_start"`
	IntervalEnd   time.Time `json:"interval_end" db:"interval_end"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type SyncQueueItem struct {
	ID          int64     `json:"id" db:"id"`
	PayloadType string    `json:"payload_type" db:"payload_type"` // app, browser, screenshot, input
	PayloadJSON string    `json:"payload_json" db:"payload_json"`
	RetryCount  int       `json:"retry_count" db:"retry_count"`
	Status      string    `json:"status" db:"status"` // pending, processing, completed, failed
	LastError   string    `json:"last_error" db:"last_error"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type ServiceStatus struct {
	Name      string    `json:"name"`
	State     string    `json:"state"` // RUNNING, STOPPED, STARTING, STOPPING, ERROR
	Message   string    `json:"message"`
	LastCheck time.Time `json:"last_check"`
}

type HealthReport struct {
	IsHealthy bool              `json:"is_healthy"`
	Details   map[string]string `json:"details"`
}
