package updater

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const CurrentVersion = "1.0.1"

type UpdateCheckResponse struct {
	UpdateAvailable bool   `json:"update_available"`
	LatestVersion   string `json:"latest_version"`
	DownloadURL     string `json:"download_url"`
}

type UpdaterService struct {
	apiURL     string
	configPath string
	log        *slog.Logger
	httpClient *http.Client
	mu         sync.Mutex
}

func NewUpdaterService(apiURL string, configPath string) *UpdaterService {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &UpdaterService{
		apiURL:     strings.TrimSuffix(apiURL, "/"),
		configPath: configPath,
		log:        slog.Default().With("service", "updater"),
		httpClient: &http.Client{Transport: tr, Timeout: 60 * time.Second},
	}
}

func (u *UpdaterService) CheckAndApplyUpdate(ctx context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	checkURL := fmt.Sprintf("%s/agents/check-update?version=%s", u.apiURL, CurrentVersion)
	req, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
	if err != nil {
		return err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("update check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update check server returned status: %d", resp.StatusCode)
	}

	var res UpdateCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("failed to parse update response: %w", err)
	}

	if !res.UpdateAvailable || res.DownloadURL == "" {
		u.log.Info("Agent is up to date", "current_version", CurrentVersion)
		return nil
	}

	u.log.Info("New agent version available! Preparing auto-update...", "current", CurrentVersion, "latest", res.LatestVersion, "url", res.DownloadURL)

	return u.performSelfUpdate(ctx, res.DownloadURL, res.LatestVersion)
}

func (u *UpdaterService) performSelfUpdate(ctx context.Context, downloadURL string, newVersion string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot get current executable path: %w", err)
	}

	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink for executable: %w", err)
	}

	installDir := filepath.Dir(exePath)
	tempNewExe := filepath.Join(installDir, "agent_new.exe")
	oldExe := filepath.Join(installDir, "agent_old.exe")

	// 1. Download new agent.exe
	u.log.Info("Downloading update binary...", "url", downloadURL)
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download new agent binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download update, status: %d", resp.StatusCode)
	}

	out, err := os.OpenFile(tempNewExe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = io.Copy(out, resp.Body)
	_ = out.Close()
	if err != nil {
		_ = os.Remove(tempNewExe)
		return fmt.Errorf("failed to save downloaded binary: %w", err)
	}

	u.log.Info("Download complete. Applying binary swap...", "new_version", newVersion)

	// 2. Remove previous agent_old.exe if exists
	_ = os.Remove(oldExe)

	// 3. Rename current running agent.exe to agent_old.exe (Windows allows renaming running EXEs)
	if err := os.Rename(exePath, oldExe); err != nil {
		_ = os.Remove(tempNewExe)
		return fmt.Errorf("failed to rename current executable to old: %w", err)
	}

	// 4. Rename agent_new.exe to agent.exe
	if err := os.Rename(tempNewExe, exePath); err != nil {
		// Rollback rename if swap failed
		_ = os.Rename(oldExe, exePath)
		return fmt.Errorf("failed to rename new executable: %w", err)
	}

	u.log.Info("Binary swap successful! Launching updated agent process...", "exe", exePath)

	// 5. Launch the updated agent process
	cmd := exec.Command(exePath, "-config", u.configPath)
	cmd.Dir = installDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000008, // DETACHED_PROCESS | CREATE_NO_WINDOW
	}

	if err := cmd.Start(); err != nil {
		u.log.Error("Failed to start updated agent process", "error", err)
		return err
	}

	u.log.Info("Updated agent launched successfully. Exiting current process for self-restart.")

	// 6. Exit current process cleanly
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()

	return nil
}

func (u *UpdaterService) StartUpdateLoop(ctx context.Context, checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// Initial check on startup
	go func() {
		time.Sleep(10 * time.Second)
		if err := u.CheckAndApplyUpdate(ctx); err != nil {
			u.log.Warn("Auto-update check failed", "error", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := u.CheckAndApplyUpdate(ctx); err != nil {
				u.log.Warn("Auto-update check failed", "error", err)
			}
		}
	}
}
