package sync

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/database"
	"github.com/monitoring-agent/agent/internal/models"
	"github.com/monitoring-agent/agent/internal/services"
)

type SyncService struct {
	mu            sync.RWMutex
	db            *database.DatabaseManager
	cfg           config.SyncConfig
	serverCfg     config.ServerConfig
	log           *slog.Logger
	state         string
	cancel        context.CancelFunc
	httpClient    *http.Client
	lastSyncTime  time.Time
	lastSyncCount int

	// configPath, when set, is the YAML file this service's ServerConfig was
	// loaded from. It lets enrollment be shared across processes that point
	// at the same config file (the boot service and the user-session
	// agent): whichever enrolls first persists agent_id/api_key here, and
	// the other picks them up instead of registering a second device.
	configPath string

	// issueMu/issueLastSent throttle ReportIssue/ReportFailure so a problem
	// that keeps recurring (e.g. the server being unreachable for an hour)
	// produces one dashboard row per issueReportThrottle window instead of
	// one every sync tick. Keyed by issue code; independent of mu, which
	// guards the sync loop's own state and is sometimes already held by the
	// caller when these are invoked (see ReportIssue).
	issueMu       sync.Mutex
	issueLastSent map[string]time.Time
}

// issueReportThrottle is how often the same issue code is allowed to create
// a new row on the dashboard while it keeps happening.
const issueReportThrottle = 10 * time.Minute

// SetConfigPath records the YAML config file this service's credentials
// should be persisted to and re-read from. See configPath.
func (s *SyncService) SetConfigPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configPath = path
}

func NewSyncService(db *database.DatabaseManager, cfg config.SyncConfig, serverCfg config.ServerConfig, log *slog.Logger) *SyncService {
	if log == nil {
		log = slog.Default()
	}
	return &SyncService{
		db:        db,
		cfg:       cfg,
		serverCfg: serverCfg,
		log:       log,
		state:     services.StateStopped,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

func (s *SyncService) Name() string {
	return "sync"
}

func (s *SyncService) Start(ctx context.Context) error {
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

	s.log.Info("Sync service starting for monitor-cloudd cloud backend", "api_url", s.serverCfg.APIURL, "interval_sec", s.cfg.IntervalSec)
	go s.runLoop(runCtx)
	return nil
}

func (s *SyncService) Stop(ctx context.Context) error {
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

	s.log.Info("Sync service stopped")
	return nil
}

func (s *SyncService) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	return s.Start(ctx)
}

func (s *SyncService) SyncNow(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.processSyncBatchLocked(ctx)
}

func (s *SyncService) Status() models.ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msg := fmt.Sprintf("Cloud Sync Engine active (monitor-cloudd API: %s)", s.serverCfg.APIURL)
	if !s.lastSyncTime.IsZero() {
		msg = fmt.Sprintf("Synced %d items to monitor-cloudd at %s", s.lastSyncCount, s.lastSyncTime.Format("15:04:05"))
	}

	return models.ServiceStatus{
		Name:      s.Name(),
		State:     s.state,
		Message:   msg,
		LastCheck: time.Now(),
	}
}

func (s *SyncService) Health() models.HealthReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	isHealthy := s.state == services.StateRunning || s.state == services.StateStopped
	return models.HealthReport{
		IsHealthy: isHealthy,
		Details: map[string]string{
			"state":           s.state,
			"api_url":         s.serverCfg.APIURL,
			"agent_id":        s.serverCfg.AgentID,
			"last_sync_count": fmt.Sprintf("%d", s.lastSyncCount),
		},
	}
}

func (s *SyncService) runLoop(ctx context.Context) {
	intervalSec := s.cfg.IntervalSec
	if intervalSec <= 0 {
		intervalSec = 5
	}
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			_, _ = s.processSyncBatchLocked(ctx)
			s.mu.Unlock()
		case <-heartbeatTicker.C:
			s.mu.Lock()
			_, _ = s.FetchPolicy(ctx)
			s.mu.Unlock()
		}
	}
}

type enrollRequest struct {
	EmployeeID  string `json:"employeeId"`
	MachineName string `json:"machineName"`
}

type enrollResponse struct {
	AgentID      string `json:"agentId"`
	APIKey       string `json:"apiKey"`
	EmployeeID   string `json:"employeeId"`
	EmployeeName string `json:"employeeName"`
	ServerURL    string `json:"serverUrl"`
}

func (s *SyncService) enrollAgentIfNeeded(ctx context.Context) error {
	if s.serverCfg.AgentID != "" && s.serverCfg.APIKey != "" {
		return nil
	}

	// A sibling process sharing this config file (the boot service, or an
	// earlier run of this same agent) may have already enrolled. Reuse
	// those credentials instead of registering a second device.
	if s.configPath != "" {
		if agentID, apiKey := config.ReadServerCredentials(s.configPath); agentID != "" && apiKey != "" {
			s.serverCfg.AgentID = agentID
			s.serverCfg.APIKey = apiKey
			s.log.Info("Reusing enrollment persisted by another process", "agent_id", agentID)
			return nil
		}
	}

	apiURL := strings.TrimSuffix(s.serverCfg.APIURL, "/")
	if apiURL == "" {
		apiURL = "http://monitor-cloudd.test/api"
	}

	enrollURL := fmt.Sprintf("%s/agents/enroll", apiURL)
	empID := s.serverCfg.EmployeeID
	if empID == "" {
		empID = "03d06c36-3882-4976-905c-864b2975c065"
	}
	machName := s.serverCfg.MachineName
	if machName == "" {
		machName = "BYTE_BUILDERS"
	}

	reqBody, _ := json.Marshal(enrollRequest{
		EmployeeID:  empID,
		MachineName: machName,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, enrollURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create enroll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("enrollment HTTP request failed to %s: %w", enrollURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("enrollment failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var res enrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("failed to decode enroll response: %w", err)
	}

	s.serverCfg.AgentID = res.AgentID
	s.serverCfg.APIKey = res.APIKey
	s.log.Info("Enrolled successfully with monitor-cloudd backend", "agent_id", res.AgentID, "employee_name", res.EmployeeName)

	if s.configPath != "" {
		if err := config.UpdateServerCredentials(s.configPath, res.AgentID, res.APIKey); err != nil {
			s.log.Warn("Failed to persist enrollment to config file", "error", err)
		}
	}

	return nil
}

func (s *SyncService) processSyncBatchLocked(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	if err := s.enrollAgentIfNeeded(ctx); err != nil {
		s.log.Warn("Agent enrollment check pending", "error", err)
	}

	batchSize := s.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}

	items, err := s.db.FetchPendingQueue(ctx, batchSize)
	if err != nil {
		s.log.Error("Failed to fetch pending queue items for sync", "error", err)
		return 0, err
	}

	if len(items) == 0 {
		return 0, nil
	}

	s.log.Info("Fetched pending queue items for batch sync", "count", len(items))

	syncedCount := 0
	var batchItems []models.SyncQueueItem

	for _, item := range items {
		pType := strings.ToLower(item.PayloadType)
		if pType == "screenshot" || pType == "screenshot_activity" {
			if s.serverCfg.AgentID != "" && s.serverCfg.APIKey != "" {
				err := s.uploadScreenshotToCloud(ctx, item)
				if err != nil {
					s.log.Error("Screenshot upload to monitor-cloudd failed", "error", err)
					s.ReportFailure("warning", "screenshot upload", err)
					if item.ID > 0 && (strings.Contains(err.Error(), "failed to read screenshot file") || strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "cannot find the file")) {
						_ = s.db.UpdateQueueStatus(ctx, item.ID, "failed_missing_file", err.Error())
					}
				} else {
					if item.ID > 0 {
						_ = s.db.UpdateQueueStatus(ctx, item.ID, "completed", "")
					}
					syncedCount++
				}
			}
		} else {
			batchItems = append(batchItems, item)
		}
	}

	if len(batchItems) > 0 {
		if s.serverCfg.AgentID == "" || s.serverCfg.APIKey == "" {
			s.log.Warn("Skipping batch sync: agent not enrolled yet")
			return 0, nil
		}

		err := s.sendBatchToCloud(ctx, batchItems)
		if err != nil {
			s.log.Error("Batch sync to monitor-cloudd failed", "error", err)
			s.ReportFailure("error", "batch sync", err)
		} else {
			for _, item := range batchItems {
				if item.ID > 0 {
					_ = s.db.UpdateQueueStatus(ctx, item.ID, "completed", "")
				}
				syncedCount++
			}
			s.log.Info("Processed sync batch", "synced_count", syncedCount)
		}
	}

	s.lastSyncTime = time.Now()
	s.lastSyncCount = syncedCount

	return syncedCount, nil
}

func (s *SyncService) sendBatchToCloud(ctx context.Context, items []models.SyncQueueItem) error {
	apiURL := strings.TrimSuffix(s.serverCfg.APIURL, "/")
	syncURL := fmt.Sprintf("%s/agents/%s/batch-sync", apiURL, s.serverCfg.AgentID)

	activities := make([]map[string]interface{}, 0)
	metrics := make([]map[string]interface{}, 0)
	idleActivities := make([]map[string]interface{}, 0)

	nowStr := time.Now().Format(time.RFC3339)

	for _, item := range items {
		var payload map[string]interface{}
		_ = json.Unmarshal([]byte(item.PayloadJSON), &payload)
		if payload == nil {
			payload = make(map[string]interface{})
		}

		pType := strings.ToLower(item.PayloadType)

		if pType == "app" || pType == "app_activity" {
			appName := "chrome.exe"
			if val, ok := payload["app_name"].(string); ok && val != "" {
				appName = val
			} else if val, ok := payload["AppName"].(string); ok && val != "" {
				appName = val
			}

			winTitle := "Google Chrome"
			if val, ok := payload["window_title"].(string); ok && val != "" {
				winTitle = val
			} else if val, ok := payload["WindowTitle"].(string); ok && val != "" {
				winTitle = val
			}

			durSec := 5
			if val, ok := payload["duration_sec"].(float64); ok && val > 0 {
				durSec = int(val)
			} else if val, ok := payload["DurationSec"].(float64); ok && val > 0 {
				durSec = int(val)
			} else if val, ok := payload["duration"].(float64); ok && val > 0 {
				durSec = int(val)
			}

			formattedTime := nowStr
			if val, ok := payload["start_time"].(string); ok && val != "" {
				formattedTime = val
			} else if val, ok := payload["StartTime"].(string); ok && val != "" {
				formattedTime = val
			}

			activities = append(activities, map[string]interface{}{
				"applicationName": appName,
				"windowTitle":     winTitle,
				"durationSeconds": durSec,
				"formattedTime":   formattedTime,
			})
		} else if pType == "browser" || pType == "browser_activity" {
			domain := "Google"
			if val, ok := payload["domain"].(string); ok && val != "" {
				domain = val
			} else if val, ok := payload["Domain"].(string); ok && val != "" {
				domain = val
			}

			tabTitle := domain
			if val, ok := payload["tab_title"].(string); ok && val != "" {
				tabTitle = val
			} else if val, ok := payload["TabTitle"].(string); ok && val != "" {
				tabTitle = val
			}

			durSec := 5
			if val, ok := payload["duration_sec"].(float64); ok && val > 0 {
				durSec = int(val)
			} else if val, ok := payload["DurationSec"].(float64); ok && val > 0 {
				durSec = int(val)
			}

			formattedTime := nowStr
			if val, ok := payload["start_time"].(string); ok && val != "" {
				formattedTime = val
			} else if val, ok := payload["StartTime"].(string); ok && val != "" {
				formattedTime = val
			}

			activities = append(activities, map[string]interface{}{
				"applicationName": "chrome.exe",
				"windowTitle":     fmt.Sprintf("%s - %s", tabTitle, domain),
				"durationSeconds": durSec,
				"formattedTime":   formattedTime,
			})
		} else if pType == "idle" {
			name, _ := payload["name"].(string)
			startTime, _ := payload["start_time"].(string)
			endTime, _ := payload["end_time"].(string)
			if name == "" || startTime == "" || endTime == "" {
				continue
			}

			idleActivities = append(idleActivities, map[string]interface{}{
				"name":       name,
				"start_time": startTime,
				"end_time":   endTime,
			})
		} else {
			metrics = append(metrics, map[string]interface{}{
				"timestamp": nowStr,
				"payload":   payload,
			})
		}
	}

	batchPayload := map[string]interface{}{
		"activities":      activities,
		"metrics":         metrics,
		"idle_activities": idleActivities,
	}

	bodyBytes, _ := json.Marshal(batchPayload)
	s.log.Info("Sending batch to cloud", "url", syncURL, "activities_count", len(activities), "metrics_count", len(metrics))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, syncURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.serverCfg.APIKey))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("batch sync returned status %d: %s", resp.StatusCode, string(respBody))
	}

	s.log.Info("Successfully sent batch sync to cloud", "activities_count", len(activities), "metrics_count", len(metrics))

	return nil
}

func (s *SyncService) uploadScreenshotToCloud(ctx context.Context, item models.SyncQueueItem) error {
	var payload map[string]interface{}
	_ = json.Unmarshal([]byte(item.PayloadJSON), &payload)
	if payload == nil {
		return fmt.Errorf("empty screenshot payload")
	}

	filePath := ""
	if val, ok := payload["file_path"].(string); ok && val != "" {
		filePath = val
	} else if val, ok := payload["FilePath"].(string); ok && val != "" {
		filePath = val
	}

	if filePath == "" {
		return fmt.Errorf("missing screenshot file_path")
	}

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		execPath, _ := os.Executable()
		altPath := filepath.Join(filepath.Dir(execPath), filePath)
		fileData, err = os.ReadFile(altPath)
		if err != nil {
			return fmt.Errorf("failed to read screenshot file %s: %w", filePath, err)
		}
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("screenshot", filepath.Base(filePath))
	if err != nil {
		return err
	}
	_, _ = part.Write(fileData)

	capturedAt := time.Now().Format(time.RFC3339)
	if val, ok := payload["captured_at"].(string); ok && val != "" {
		capturedAt = val
	} else if val, ok := payload["CapturedAt"].(string); ok && val != "" {
		capturedAt = val
	}

	activeApp := "Desktop"
	if val, ok := payload["active_app"].(string); ok && val != "" {
		activeApp = val
	} else if val, ok := payload["ActiveApp"].(string); ok && val != "" {
		activeApp = val
	}

	windowTitle := "Desktop Capture"
	if val, ok := payload["window_title"].(string); ok && val != "" {
		windowTitle = val
	} else if val, ok := payload["WindowTitle"].(string); ok && val != "" {
		windowTitle = val
	}

	keyPressCount := getInt64FromMap(payload, "key_press_count", "KeyPressCount", "key_presses", "keyPressCount", "keystrokes")
	mouseClickCount := getInt64FromMap(payload, "mouse_click_count", "MouseClickCount", "mouse_clicks", "mouseClickCount")

	metadataJSON, _ := json.Marshal(map[string]interface{}{
		"timestamp":       capturedAt,
		"activeApp":       activeApp,
		"windowTitle":     windowTitle,
		"keyPressCount":   keyPressCount,
		"mouseClickCount": mouseClickCount,
	})
	_ = writer.WriteField("metadata", string(metadataJSON))

	_ = writer.Close()

	apiURL := strings.TrimSuffix(s.serverCfg.APIURL, "/")
	uploadURL := fmt.Sprintf("%s/agents/%s/uploads", apiURL, s.serverCfg.AgentID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.serverCfg.APIKey))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.log.Error("uploadScreenshotToCloud HTTP request failed", "error", err, "url", uploadURL)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("screenshot upload returned status %d: %s", resp.StatusCode, string(respBody))
		s.log.Error("uploadScreenshotToCloud server error", "error", err, "status", resp.StatusCode)
		return err
	}

	s.log.Info("Uploaded screenshot to monitor-cloudd cloud", "file", filePath, "agent_id", s.serverCfg.AgentID)
	return nil
}

type PolicyResponse struct {
	PolicyID           string `json:"policyId"`
	PolicyName         string `json:"policyName"`
	IntervalMs         int    `json:"intervalMs"`
	Format             string `json:"format"`
	JpegQuality        int    `json:"jpegQuality"`
	CaptureTarget      string `json:"captureTarget"`
	Enabled            bool   `json:"enabled"`
	CaptureScreenshots bool   `json:"captureScreenshots"`
	BlurScreenshots    bool   `json:"blurScreenshots"`
	MaxKeystrokes      *int   `json:"maxKeystrokes"`
	MaxMouseClicks     *int   `json:"maxMouseClicks"`
	WorkHoursOnly      bool   `json:"workHoursOnly"`
	WorkHoursStart     string `json:"workHoursStart"`
	WorkHoursEnd       string `json:"workHoursEnd"`
	Timezone           string `json:"timezone"`
}

// issueThrottled reports whether code was already reported within the last
// issueReportThrottle window, and if not, marks it as reported now.
func (s *SyncService) issueThrottled(code string) bool {
	s.issueMu.Lock()
	defer s.issueMu.Unlock()
	if s.issueLastSent == nil {
		s.issueLastSent = make(map[string]time.Time)
	}
	if last, ok := s.issueLastSent[code]; ok && time.Since(last) < issueReportThrottle {
		return true
	}
	s.issueLastSent[code] = time.Now()
	return false
}

// classifyError turns a raw Go error from an HTTP call this service made
// into a short, stable "kind" tag plus a plain-language hint a non-technical
// person can act on. It only ever inspects the error's text, so it works
// for both errors this file wraps with status codes and plain network
// errors from the HTTP client.
func classifyError(err error) (kind, hint string) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "status 401") || strings.Contains(msg, "status 403") || strings.Contains(msg, "unauthorized"):
		return "AUTH_FAILED", "The server rejected this device's saved credentials (invalid or revoked API key). Reinstall the agent on this machine to re-enroll it."
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dial tcp") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timeout") || strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "tls"):
		return "UNREACHABLE", "This machine could not reach the monitor-cloudd server. Check its internet connection and that the server URL in agent.yaml is correct."
	case strings.Contains(msg, "status 5"):
		return "SERVER_ERROR", "The monitor-cloudd server itself returned an error. This is usually temporary; if it keeps happening, check the server."
	case strings.Contains(msg, "no such file") || strings.Contains(msg, "cannot find the file") || strings.Contains(msg, "failed to read screenshot file"):
		return "FILE_MISSING", "A local file this agent needed (e.g. a screenshot) was missing or deleted before it could be uploaded."
	default:
		return "UNKNOWN", "See the error text above for details."
	}
}

// ReportFailure classifies a raw error from a sync/enrollment/policy
// operation into a plain-language issue and reports it (see ReportIssue).
// op names the operation that failed (e.g. "fetch policy", "batch sync")
// and is folded into both the message and the throttling code, so e.g. a
// network outage during policy fetch and one during batch sync are reported
// (and throttled) separately.
func (s *SyncService) ReportFailure(severity, op string, err error) {
	if err == nil {
		return
	}
	kind, hint := classifyError(err)
	code := kind + "_" + strings.ToUpper(strings.ReplaceAll(op, " ", "_"))
	message := fmt.Sprintf("%s failed: %s. %s", op, err.Error(), hint)
	s.ReportIssue(severity, code, message, map[string]interface{}{"operation": op, "error": err.Error()})
}

// ReportIssue tells the monitor-cloudd dashboard about a problem this agent
// hit, in plain language, so the owner can see *why* a machine looks wrong
// without remoting into it. It always logs locally first — so the local log
// still has it even if the HTTP call fails or the device isn't enrolled yet
// — then fires the report in its own goroutine with a short timeout, so it
// never blocks the caller (which may be holding mu).
//
// code is a short, stable machine-readable tag (e.g. "SYNC_AUTH_FAILED")
// used both to throttle repeats and, on the dashboard, to look up a
// suggested fix. message is the human-readable sentence shown as-is.
func (s *SyncService) ReportIssue(severity, code, message string, extra map[string]interface{}) {
	if s.issueThrottled(code) {
		return
	}
	s.log.Error("Issue detected", "severity", severity, "code", code, "message", message, "context", extra)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := s.sendIssueReport(ctx, severity, code, message, extra); err != nil {
			s.log.Warn("Failed to deliver issue report to dashboard (it is still in the local log)", "error", err, "code", code)
		}
	}()
}

// ReportIssueSync is ReportIssue's blocking twin: it delivers the report
// before returning instead of in a background goroutine. Use it only when
// the caller is about to exit (e.g. a recovered panic) and a fire-and-forget
// goroutine might never get to run before the process dies.
func (s *SyncService) ReportIssueSync(ctx context.Context, severity, code, message string, extra map[string]interface{}) error {
	if s.issueThrottled(code) {
		return nil
	}
	s.log.Error("Issue detected", "severity", severity, "code", code, "message", message, "context", extra)
	return s.sendIssueReport(ctx, severity, code, message, extra)
}

func (s *SyncService) sendIssueReport(ctx context.Context, severity, code, message string, extra map[string]interface{}) error {
	agentID := s.serverCfg.AgentID
	apiKey := s.serverCfg.APIKey
	if agentID == "" || apiKey == "" {
		// Not enrolled yet — there is no device identity to attach this
		// report to on the server, so the local log line above is all we
		// can do for now.
		return nil
	}

	apiURL := strings.TrimSuffix(s.serverCfg.APIURL, "/")
	issueURL := fmt.Sprintf("%s/agents/%s/issues", apiURL, agentID)

	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"severity": severity,
		"code":     code,
		"message":  message,
		"context":  extra,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, issueURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dashboard rejected issue report with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (s *SyncService) FetchPolicy(ctx context.Context) (*PolicyResponse, error) {
	if s.serverCfg.AgentID == "" || s.serverCfg.APIKey == "" {
		if err := s.enrollAgentIfNeeded(ctx); err != nil {
			return nil, fmt.Errorf("cannot fetch policy without enrollment: %w", err)
		}
	}

	if s.serverCfg.AgentID == "" {
		return nil, fmt.Errorf("agent not enrolled yet")
	}

	apiURL := strings.TrimSuffix(s.serverCfg.APIURL, "/")
	policyURL := fmt.Sprintf("%s/agents/%s/policy?running=true", apiURL, s.serverCfg.AgentID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, policyURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.serverCfg.APIKey))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		wrapped := fmt.Errorf("policy fetch failed: %w", err)
		s.ReportFailure("error", "fetch policy", wrapped)
		return nil, wrapped
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		wrapped := fmt.Errorf("policy fetch returned status %d: %s", resp.StatusCode, string(bodyBytes))
		s.ReportFailure("error", "fetch policy", wrapped)
		return nil, wrapped
	}

	var policy PolicyResponse
	if err := json.NewDecoder(resp.Body).Decode(&policy); err != nil {
		wrapped := fmt.Errorf("failed to decode policy response: %w", err)
		s.ReportFailure("error", "fetch policy", wrapped)
		return nil, wrapped
	}

	s.log.Info("Fetched server policy",
		"enabled", policy.Enabled,
		"captureScreenshots", policy.CaptureScreenshots,
		"intervalMs", policy.IntervalMs,
		"blurScreenshots", policy.BlurScreenshots,
	)

	return &policy, nil
}

func getInt64FromMap(m map[string]interface{}, keys ...string) int64 {
	for _, key := range keys {
		if val, ok := m[key]; ok && val != nil {
			switch v := val.(type) {
			case float64:
				return int64(v)
			case int64:
				return v
			case int:
				return int64(v)
			case string:
				if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

