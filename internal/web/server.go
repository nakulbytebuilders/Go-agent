package web

import (
	"bufio"
	"context"
	"crypto/sha1"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/monitoring-agent/agent/internal/config"
	"github.com/monitoring-agent/agent/internal/controller"
	"github.com/monitoring-agent/agent/internal/models"
)

//go:embed static/*
var staticFS embed.FS

type WebServer struct {
	ctl       *controller.AgentController
	cfg       config.WebServerConfig
	log       *slog.Logger
	server    *http.Server
	clientsMu sync.Mutex
	clients   map[net.Conn]bool
	startTime time.Time
}

func NewWebServer(ctl *controller.AgentController, cfg config.WebServerConfig, logger *slog.Logger) *WebServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebServer{
		ctl:       ctl,
		cfg:       cfg,
		log:       logger,
		clients:   make(map[net.Conn]bool),
		startTime: time.Now(),
	}
}

func (s *WebServer) Start() error {
	if !s.cfg.Enabled {
		s.log.Info("Web Server is disabled in config.")
		return nil
	}

	mux := http.NewServeMux()

	// WebSocket Live Stream
	mux.HandleFunc("/ws", s.handleWebSocket)

	// WinSentinel API Compatible Endpoints
	mux.HandleFunc("/api/telemetry", s.handleGetTelemetry)
	mux.HandleFunc("/api/activity/summary", s.handleGetActivitySummary)
	mux.HandleFunc("/api/activity/websites", s.handleGetWebsitesSummary)
	mux.HandleFunc("/api/activity/recent", s.handleGetActivityRecent)
	mux.HandleFunc("/api/agent/start", s.handleAgentStart)
	mux.HandleFunc("/api/agent/stop", s.handleAgentStop)
	mux.HandleFunc("/api/export", s.handleExportData)

	// Service Control Endpoints
	mux.HandleFunc("/api/services/status", s.handleServicesStatus)
	mux.HandleFunc("/api/services/start", s.handleServiceStart)
	mux.HandleFunc("/api/services/stop", s.handleServiceStop)
	mux.HandleFunc("/api/services/restart", s.handleServiceRestart)

	// Screenshot Endpoints
	mux.HandleFunc("/api/screenshots", s.handleGetScreenshots)
	mux.HandleFunc("/api/screenshots/take", s.handleTakeScreenshot)
	mux.HandleFunc("/api/screenshots/image", s.handleGetScreenshotImage)
	mux.HandleFunc("/api/screenshots/interval", s.handleScreenshotInterval)

	// Activity Endpoints
	mux.HandleFunc("/api/activities/apps", s.handleGetAppActivities)
	mux.HandleFunc("/api/activities/apps/stats", s.handleGetAppUsageStats)
	mux.HandleFunc("/api/activities/apps/current", s.handleGetCurrentActiveApp)

	mux.HandleFunc("/api/activities/browsers", s.handleGetBrowserActivities)
	mux.HandleFunc("/api/activities/browsers/stats", s.handleGetDomainUsageStats)
	mux.HandleFunc("/api/activities/browsers/current", s.handleGetCurrentActiveBrowser)

	mux.HandleFunc("/api/activities/unified", s.handleGetUnifiedActivities)
	mux.HandleFunc("/api/activities/focus", s.handleGetCurrentActiveFocus)

	// Input & Sync Endpoints
	mux.HandleFunc("/api/input/current", s.handleGetCurrentInputState)
	mux.HandleFunc("/api/input/recent", s.handleGetRecentInputActivities)

	mux.HandleFunc("/api/sync/stats", s.handleGetSyncStats)
	mux.HandleFunc("/api/sync/queue", s.handleGetSyncQueueItems)
	mux.HandleFunc("/api/sync/trigger", s.handleTriggerSyncNow)

	// Static Assets
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("failed to load embedded static sub-FS: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	host := s.cfg.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := s.cfg.Port
	if port <= 0 {
		port = 8080
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	s.log.Info("WinSentinel Agent Web Dashboard active", "url", url)

	if s.cfg.AutoOpen {
		go s.openBrowser(url)
	}

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.log.Error("Web Server HTTP listener failed", "error", err)
		}
	}()

	go s.broadcastLoop()

	return nil
}

func (s *WebServer) Stop(ctx context.Context) error {
	if s.server != nil {
		s.log.Info("Shutting down Web Server...")
		return s.server.Shutdown(ctx)
	}
	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *WebServer) openBrowser(url string) {
	time.Sleep(500 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func (s *WebServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "Not a websocket handshake", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	acceptKey := computeAcceptKey(key)

	res := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n\r\n", acceptKey)

	bufrw.WriteString(res)
	bufrw.Flush()

	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	payload := s.buildTelemetryPayload()
	initialMsg, _ := json.Marshal(map[string]interface{}{
		"type":         "initial_state",
		"agentStatus":  s.getAgentStatusMap(),
		"data":         payload,
		"topProcesses": map[string]interface{}{"processes": GetTopProcesses()},
	})
	_ = sendWSFrame(conn, initialMsg)

	go func() {
		defer func() {
			conn.Close()
			s.clientsMu.Lock()
			delete(s.clients, conn)
			s.clientsMu.Unlock()
		}()
		r := bufio.NewReader(conn)
		for {
			_, err := r.ReadByte()
			if err != nil {
				break
			}
		}
	}()
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func sendWSFrame(conn net.Conn, payload []byte) error {
	length := len(payload)
	var header []byte
	header = append(header, 0x81)

	if length <= 125 {
		header = append(header, byte(length))
	} else if length <= 65535 {
		header = append(header, 126, byte(length>>8), byte(length&0xFF))
	} else {
		header = append(header, 127)
		for i := 7; i >= 0; i-- {
			header = append(header, byte((length>>(i*8))&0xFF))
		}
	}

	frame := append(header, payload...)
	_, err := conn.Write(frame)
	return err
}

func (s *WebServer) broadcastLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.clientsMu.Lock()
		clientCount := len(s.clients)
		s.clientsMu.Unlock()

		if clientCount == 0 {
			continue
		}

		payload := s.buildTelemetryPayload()
		frameData, err := json.Marshal(map[string]interface{}{
			"type":        "telemetry",
			"agentStatus": s.getAgentStatusMap(),
			"data":        payload,
		})
		if err != nil {
			continue
		}

		s.clientsMu.Lock()
		for conn := range s.clients {
			if err := sendWSFrame(conn, frameData); err != nil {
				conn.Close()
				delete(s.clients, conn)
			}
		}
		s.clientsMu.Unlock()
	}
}

func (s *WebServer) buildTelemetryPayload() SystemTelemetry {
	tele := GetSystemMetrics()

	focus, err := s.ctl.GetCurrentActiveFocus()
	if err == nil && focus != nil {
		procName := focus.Name
		if procName == "" {
			procName = "explorer.exe"
		}
		pidVal := uint32(0)
		if focus.PID > 0 {
			pidVal = uint32(focus.PID)
		}
		tele.ActiveWindow = ActiveWinInfo{
			Title:       focus.Title,
			ProcessName: procName,
			PID:         pidVal,
		}
	} else {
		tele.ActiveWindow = ActiveWinInfo{
			Title:       "Desktop / System",
			ProcessName: "explorer.exe",
			PID:         0,
		}
	}

	inp, err := s.ctl.GetCurrentInputState()
	if err == nil && inp != nil {
		tele.UserIdleMs = inp.IdleTimeSec * 1000
		tele.IsUserIdle = inp.IdleTimeSec >= 60
	}

	return tele
}

func (s *WebServer) getAgentStatusMap() map[string]interface{} {
	uptime := int64(time.Since(s.startTime).Seconds())
	return map[string]interface{}{
		"isRunning":             true,
		"uptimeSeconds":         uptime,
		"collectedSamplesCount": uptime,
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

func (s *WebServer) handleGetTelemetry(w http.ResponseWriter, r *http.Request) {
	tele := s.buildTelemetryPayload()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"agentStatus": s.getAgentStatusMap(),
		"data":        tele,
		"processes":   GetTopProcesses(),
	})
}

func (s *WebServer) handleGetActivitySummary(w http.ResponseWriter, r *http.Request) {
	stats, err := s.ctl.GetAppUsageStats(10)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "summary": []interface{}{}})
		return
	}
	type summaryItem struct {
		ProcessName  string `json:"processName"`
		TotalSeconds int64  `json:"totalSeconds"`
	}
	var res []summaryItem
	for _, item := range stats {
		res = append(res, summaryItem{
			ProcessName:  item.AppName,
			TotalSeconds: item.TotalDurationSec,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"summary": res,
	})
}

func (s *WebServer) handleGetWebsitesSummary(w http.ResponseWriter, r *http.Request) {
	stats, err := s.ctl.GetDomainUsageStats(10)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "summary": []interface{}{}})
		return
	}
	type summaryItem struct {
		Domain       string `json:"domain"`
		TotalSeconds int64  `json:"totalSeconds"`
	}
	var res []summaryItem
	for _, item := range stats {
		res = append(res, summaryItem{
			Domain:       item.Domain,
			TotalSeconds: item.TotalDurationSec,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"summary": res,
	})
}

func (s *WebServer) handleGetActivityRecent(w http.ResponseWriter, r *http.Request) {
	recs, err := s.ctl.GetUnifiedActivities(50)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "activity": []interface{}{}})
		return
	}
	type activityItem struct {
		Timestamp       int64  `json:"timestamp"`
		ProcessName     string `json:"processName"`
		Title           string `json:"title"`
		PID             uint32 `json:"pid"`
		DurationSeconds int64  `json:"durationSeconds"`
	}
	var res []activityItem
	for _, rec := range recs {
		pidVal := uint32(0)
		if rec.PID > 0 {
			pidVal = uint32(rec.PID)
		}
		res = append(res, activityItem{
			Timestamp:       rec.StartTime.UnixMilli(),
			ProcessName:     rec.Name,
			Title:           rec.Title,
			PID:             pidVal,
			DurationSeconds: rec.DurationSec,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"activity": res,
	})
}

func (s *WebServer) handleAgentStart(w http.ResponseWriter, r *http.Request) {
	s.ctl.StartEnabledServices()
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Agent services started"})
}

func (s *WebServer) handleAgentStop(w http.ResponseWriter, r *http.Request) {
	statuses := s.ctl.GetServiceStatuses()
	for name := range statuses {
		_ = s.ctl.StopService(name)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Agent services stopped"})
}

func (s *WebServer) handleExportData(w http.ResponseWriter, r *http.Request) {
	recs, _ := s.ctl.GetUnifiedActivities(500)
	w.Header().Set("Content-Disposition", "attachment; filename=telemetry_export.json")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recs)
}

func (s *WebServer) handleServicesStatus(w http.ResponseWriter, r *http.Request) {
	statuses := s.ctl.GetServiceStatuses()
	writeJSON(w, http.StatusOK, statuses)
}

type serviceActionReq struct {
	Name string `json:"name"`
}

func (s *WebServer) handleServiceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req serviceActionReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Service name required")
		return
	}
	err := s.ctl.StartService(req.Name)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Service started successfully"})
}

func (s *WebServer) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req serviceActionReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Service name required")
		return
	}
	err := s.ctl.StopService(req.Name)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Service stopped successfully"})
}

func (s *WebServer) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req serviceActionReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Service name required")
		return
	}
	err := s.ctl.RestartService(req.Name)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Service restarted successfully"})
}

func (s *WebServer) handleGetScreenshots(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 30
	}
	records, err := s.ctl.GetRecentScreenshots(limit)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "screenshots": []interface{}{}})
		return
	}

	type ssItem struct {
		URLPath     string `json:"urlPath"`
		FilePath    string `json:"filePath"`
		Timestamp   int64  `json:"timestamp"`
		ActiveApp   string `json:"activeApp"`
		WindowTitle string `json:"windowTitle"`
	}

	var screenshots []ssItem
	for _, rec := range records {
		screenshots = append(screenshots, ssItem{
			URLPath:     fmt.Sprintf("/api/screenshots/image?path=%s", base64.StdEncoding.EncodeToString([]byte(rec.FilePath))),
			FilePath:    rec.FilePath,
			Timestamp:   rec.CapturedAt.UnixMilli(),
			ActiveApp:   "Screen Activity",
			WindowTitle: filepath.Base(rec.FilePath),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"screenshots": screenshots,
	})
}

func (s *WebServer) handleTakeScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	rec, err := s.ctl.TakeScreenshotNow()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	msg := fmt.Sprintf("Screenshot captured: %s", rec.FilePath)
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": msg})
}

func (s *WebServer) handleGetScreenshotImage(w http.ResponseWriter, r *http.Request) {
	filePathParam := r.URL.Query().Get("path")
	if filePathParam == "" {
		http.Error(w, "Missing path parameter", http.StatusBadRequest)
		return
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(filePathParam)
	filePath := filePathParam
	if err == nil && len(decodedBytes) > 0 {
		filePath = string(decodedBytes)
	}

	cleanPath := filepath.Clean(filePath)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *WebServer) handleScreenshotInterval(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			IntervalSec int `json:"interval_sec"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.IntervalSec <= 0 {
			req.IntervalSec = 60
		}
		err := s.ctl.SetScreenshotInterval(req.IntervalSec)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
		msg := fmt.Sprintf("Screenshot interval successfully set to %d seconds", req.IntervalSec)
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": msg, "interval_sec": req.IntervalSec})
		return
	}

	interval := s.ctl.GetScreenshotInterval()
	writeJSON(w, http.StatusOK, map[string]interface{}{"interval_sec": interval})
}

func (s *WebServer) handleGetAppActivities(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	recs, err := s.ctl.GetRecentAppActivities(limit)
	if err != nil {
		recs = []models.AppActivity{}
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *WebServer) handleGetAppUsageStats(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 10
	}
	stats, err := s.ctl.GetAppUsageStats(limit)
	if err != nil {
		stats = []models.AppUsageStat{}
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *WebServer) handleGetCurrentActiveApp(w http.ResponseWriter, r *http.Request) {
	act, err := s.ctl.GetCurrentActiveApp()
	if err != nil || act == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, act)
}

func (s *WebServer) handleGetBrowserActivities(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	recs, err := s.ctl.GetRecentBrowserActivities(limit)
	if err != nil {
		recs = []models.BrowserActivity{}
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *WebServer) handleGetDomainUsageStats(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 10
	}
	stats, err := s.ctl.GetDomainUsageStats(limit)
	if err != nil {
		stats = []models.BrowserUsageStat{}
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *WebServer) handleGetCurrentActiveBrowser(w http.ResponseWriter, r *http.Request) {
	act, err := s.ctl.GetCurrentActiveBrowser()
	if err != nil || act == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, act)
}

func (s *WebServer) handleGetUnifiedActivities(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	recs, err := s.ctl.GetUnifiedActivities(limit)
	if err != nil {
		recs = []models.UnifiedActivity{}
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *WebServer) handleGetCurrentActiveFocus(w http.ResponseWriter, r *http.Request) {
	focus, err := s.ctl.GetCurrentActiveFocus()
	if err != nil || focus == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, focus)
}

func (s *WebServer) handleGetCurrentInputState(w http.ResponseWriter, r *http.Request) {
	inp, err := s.ctl.GetCurrentInputState()
	if err != nil || inp == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, inp)
}

func (s *WebServer) handleGetRecentInputActivities(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	recs, err := s.ctl.GetRecentInputActivities(limit)
	if err != nil {
		recs = []models.InputActivity{}
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *WebServer) handleGetSyncStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.ctl.GetSyncStats()
	if err != nil {
		stats = map[string]int64{}
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *WebServer) handleGetSyncQueueItems(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	items, err := s.ctl.GetSyncQueueItems(limit)
	if err != nil {
		items = []models.SyncQueueItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *WebServer) handleTriggerSyncNow(w http.ResponseWriter, r *http.Request) {
	count, err := s.ctl.TriggerSyncNow()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"synced_count": 0, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"synced_count": count})
}
