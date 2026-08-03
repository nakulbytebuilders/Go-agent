package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/models"
	_ "modernc.org/sqlite"
)

type DatabaseManager struct {
	mu  sync.RWMutex
	db  *sql.DB
	cfg config.DatabaseConfig
}

func NewDatabaseManager(cfg config.DatabaseConfig) (*DatabaseManager, error) {
	dbPath := cfg.Path
	if dbPath == "" {
		dbPath = "data/agent.db"
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(1)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(1)
	}

	mgr := &DatabaseManager{
		db:  db,
		cfg: cfg,
	}

	if err := mgr.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return mgr, nil
}

func (m *DatabaseManager) initSchema() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	schema := `
	CREATE TABLE IF NOT EXISTS activities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		category TEXT NOT NULL,
		app_name TEXT,
		window_title TEXT,
		domain TEXT,
		url TEXT,
		pid INTEGER DEFAULT 0,
		duration_sec INTEGER DEFAULT 0,
		start_time DATETIME NOT NULL,
		end_time DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS screenshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		width INTEGER DEFAULT 0,
		height INTEGER DEFAULT 0,
		captured_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		sync_status TEXT DEFAULT 'pending'
	);

	CREATE TABLE IF NOT EXISTS input_activities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		keyboard_count INTEGER DEFAULT 0,
		mouse_clicks INTEGER DEFAULT 0,
		mouse_move_dist REAL DEFAULT 0.0,
		scroll_count INTEGER DEFAULT 0,
		idle_time_sec INTEGER DEFAULT 0,
		interval_start DATETIME NOT NULL,
		interval_end DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS sync_queue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		payload_type TEXT NOT NULL,
		payload_json TEXT NOT NULL,
		retry_count INTEGER DEFAULT 0,
		status TEXT DEFAULT 'pending',
		last_error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_activities_category ON activities(category);
	CREATE INDEX IF NOT EXISTS idx_activities_start ON activities(start_time);
	CREATE INDEX IF NOT EXISTS idx_screenshots_captured ON screenshots(captured_at);
	CREATE INDEX IF NOT EXISTS idx_sync_queue_status ON sync_queue(status);
	`

	if _, err := m.db.Exec(schema); err != nil {
		return err
	}
	return nil
}

func (m *DatabaseManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *DatabaseManager) InsertUnifiedActivity(ctx context.Context, act *models.UnifiedActivity) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `INSERT INTO activities (category, app_name, window_title, domain, url, pid, duration_sec, start_time, end_time, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := m.db.ExecContext(ctx, query, act.Category, act.Name, act.Title, act.Domain, "", act.PID, act.DurationSec, act.StartTime, act.EndTime, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *DatabaseManager) InsertActivity(ctx context.Context, act *models.AppActivity) (int64, error) {
	unified := &models.UnifiedActivity{
		Category:    "APP",
		Name:        act.AppName,
		Title:       act.WindowTitle,
		PID:         act.PID,
		DurationSec: act.DurationSec,
		StartTime:   act.StartTime,
		EndTime:     act.EndTime,
	}
	return m.InsertUnifiedActivity(ctx, unified)
}

func (m *DatabaseManager) InsertBrowser(ctx context.Context, act *models.BrowserActivity) (int64, error) {
	unified := &models.UnifiedActivity{
		Category:    "WEB",
		Name:        act.BrowserName,
		Title:       act.TabTitle,
		Domain:      act.Domain,
		PID:         0,
		DurationSec: act.DurationSec,
		StartTime:   act.StartTime,
		EndTime:     act.EndTime,
	}
	return m.InsertUnifiedActivity(ctx, unified)
}

func (m *DatabaseManager) InsertScreenshot(ctx context.Context, rec *models.ScreenshotRecord) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `INSERT INTO screenshots (file_path, file_size, width, height, captured_at, sync_status)
			  VALUES (?, ?, ?, ?, ?, ?)`
	res, err := m.db.ExecContext(ctx, query, rec.FilePath, rec.FileSize, rec.Width, rec.Height, rec.CapturedAt, rec.SyncStatus)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *DatabaseManager) InsertInputActivity(ctx context.Context, inp *models.InputActivity) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `INSERT INTO input_activities (keyboard_count, mouse_clicks, mouse_move_dist, scroll_count, idle_time_sec, interval_start, interval_end, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := m.db.ExecContext(ctx, query, inp.KeyboardCount, inp.MouseClicks, inp.MouseMoveDist, inp.ScrollCount, inp.IdleTimeSec, inp.IntervalStart, inp.IntervalEnd, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *DatabaseManager) InsertInput(ctx context.Context, inp *models.InputActivity) (int64, error) {
	return m.InsertInputActivity(ctx, inp)
}

func (m *DatabaseManager) InsertQueue(ctx context.Context, item *models.SyncQueueItem) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `INSERT INTO sync_queue (payload_type, payload_json, retry_count, status, last_error, created_at, updated_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?)`
	res, err := m.db.ExecContext(ctx, query, item.PayloadType, item.PayloadJSON, item.RetryCount, item.Status, item.LastError, time.Now(), time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *DatabaseManager) UpdateQueueStatus(ctx context.Context, id int64, status string, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `UPDATE sync_queue SET status = ?, last_error = ?, retry_count = retry_count + 1, updated_at = ? WHERE id = ?`
	_, err := m.db.ExecContext(ctx, query, status, lastError, time.Now(), id)
	return err
}

func (m *DatabaseManager) DeleteQueueItem(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `DELETE FROM sync_queue WHERE id = ?`
	_, err := m.db.ExecContext(ctx, query, id)
	return err
}

func (m *DatabaseManager) FetchPendingQueue(ctx context.Context, limit int) ([]models.SyncQueueItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, payload_type, payload_json, retry_count, status, last_error, created_at, updated_at
			  FROM sync_queue WHERE status = 'pending'
			  ORDER BY CASE
			      WHEN payload_type IN ('app', 'app_activity', 'browser', 'browser_activity') THEN 0
			      WHEN payload_type = 'screenshot' THEN 1
			      ELSE 2
			  END, id ASC LIMIT ?`
	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.SyncQueueItem
	for rows.Next() {
		var item models.SyncQueueItem
		if err := rows.Scan(&item.ID, &item.PayloadType, &item.PayloadJSON, &item.RetryCount, &item.Status, &item.LastError, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (m *DatabaseManager) GetRecentScreenshots(ctx context.Context, limit int) ([]models.ScreenshotRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, file_path, file_size, width, height, captured_at, sync_status
			  FROM screenshots ORDER BY id DESC LIMIT ?`
	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.ScreenshotRecord
	for rows.Next() {
		var rec models.ScreenshotRecord
		if err := rows.Scan(&rec.ID, &rec.FilePath, &rec.FileSize, &rec.Width, &rec.Height, &rec.CapturedAt, &rec.SyncStatus); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, nil
}

func (m *DatabaseManager) GetRecentAppActivities(ctx context.Context, limit int) ([]models.AppActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, app_name, window_title, pid, duration_sec, start_time, end_time, created_at
			  FROM activities WHERE category = 'APP' ORDER BY id DESC LIMIT ?`
	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.AppActivity
	for rows.Next() {
		var act models.AppActivity
		if err := rows.Scan(&act.ID, &act.AppName, &act.WindowTitle, &act.PID, &act.DurationSec, &act.StartTime, &act.EndTime, &act.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, act)
	}
	return records, nil
}

func (m *DatabaseManager) GetAppUsageStats(ctx context.Context, limit int) ([]models.AppUsageStat, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	query := `SELECT app_name, SUM(duration_sec) as total_duration, COUNT(*) as usage_count
			  FROM activities WHERE category = 'APP' GROUP BY app_name ORDER BY total_duration DESC LIMIT ?`
	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.AppUsageStat
	for rows.Next() {
		var s models.AppUsageStat
		if err := rows.Scan(&s.AppName, &s.TotalDurationSec, &s.LaunchCount); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

func (m *DatabaseManager) GetRecentBrowserActivities(ctx context.Context, limit int) ([]models.BrowserActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, app_name as browser_name, window_title as tab_title, domain, url, duration_sec, start_time, end_time, created_at
			  FROM activities WHERE category = 'WEB' ORDER BY id DESC LIMIT ?`
	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.BrowserActivity
	for rows.Next() {
		var b models.BrowserActivity
		if err := rows.Scan(&b.ID, &b.BrowserName, &b.TabTitle, &b.Domain, &b.URL, &b.DurationSec, &b.StartTime, &b.EndTime, &b.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, b)
	}
	return records, nil
}

func (m *DatabaseManager) GetDomainUsageStats(ctx context.Context, limit int) ([]models.BrowserUsageStat, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	query := `SELECT domain, SUM(duration_sec) as total_duration, COUNT(*) as visit_count
			  FROM activities WHERE category = 'WEB' AND domain != '' GROUP BY domain ORDER BY total_duration DESC LIMIT ?`
	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.BrowserUsageStat
	for rows.Next() {
		var s models.BrowserUsageStat
		if err := rows.Scan(&s.Domain, &s.TotalDurationSec, &s.VisitCount); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

func (m *DatabaseManager) GetRecentInputActivities(ctx context.Context, limit int) ([]models.InputActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, keyboard_count, mouse_clicks, mouse_move_dist, scroll_count, idle_time_sec, interval_start, interval_end, created_at
			  FROM input_activities ORDER BY id DESC LIMIT ?`
	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.InputActivity
	for rows.Next() {
		var inp models.InputActivity
		if err := rows.Scan(&inp.ID, &inp.KeyboardCount, &inp.MouseClicks, &inp.MouseMoveDist, &inp.ScrollCount, &inp.IdleTimeSec, &inp.IntervalStart, &inp.IntervalEnd, &inp.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, inp)
	}
	return records, nil
}

func (m *DatabaseManager) GetSyncQueueItems(ctx context.Context, limit int) ([]models.SyncQueueItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, payload_type, payload_json, retry_count, status, last_error, created_at, updated_at
			  FROM sync_queue ORDER BY id DESC LIMIT ?`
	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.SyncQueueItem
	for rows.Next() {
		var item models.SyncQueueItem
		if err := rows.Scan(&item.ID, &item.PayloadType, &item.PayloadJSON, &item.RetryCount, &item.Status, &item.LastError, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (m *DatabaseManager) GetSyncStats(ctx context.Context) (map[string]int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]int64{
		"pending":   0,
		"completed": 0,
		"failed":    0,
		"total":     0,
	}

	query := `SELECT status, COUNT(*) FROM sync_queue GROUP BY status`
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err == nil {
			stats[status] = count
			stats["total"] += count
		}
	}
	return stats, nil
}

func (m *DatabaseManager) GetUnifiedActivities(ctx context.Context, limit int) ([]models.UnifiedActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, category, app_name as name, window_title as title, domain, pid, duration_sec, start_time, end_time, created_at
			  FROM activities ORDER BY id DESC LIMIT ?`

	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.UnifiedActivity
	for rows.Next() {
		var u models.UnifiedActivity
		if err := rows.Scan(&u.ID, &u.Category, &u.Name, &u.Title, &u.Domain, &u.PID, &u.DurationSec, &u.StartTime, &u.EndTime, &u.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, u)
	}
	return records, nil
}
