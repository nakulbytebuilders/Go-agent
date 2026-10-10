package updater

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CurrentVersion is compiled into agent.exe, so bump it BEFORE running
// build.bat: a binary built first reports the old version whatever the source
// says, and an agent that "updates" to it is offered the same update again.
const CurrentVersion = "1.0.6"

const (
	// maxBinarySize bounds a download so a broken or hostile server cannot
	// fill the disk.
	maxBinarySize = 200 << 20

	checkTimeout        = 30 * time.Second
	downloadTimeout     = 10 * time.Minute
	versionProbeTimeout = 15 * time.Second
)

type UpdateCheckResponse struct {
	UpdateAvailable bool   `json:"update_available"`
	LatestVersion   string `json:"latest_version"`
	DownloadURL     string `json:"download_url"`

	// SHA256 is the hex SHA-256 of the binary at DownloadURL. An update that
	// does not come with one is refused.
	SHA256 string `json:"sha256"`
}

type UpdaterService struct {
	apiURL     string
	configPath string
	log        *slog.Logger
	httpClient *http.Client
	mu         sync.Mutex

	// version is what this process runs and exePath the file an update
	// replaces. They are the compiled-in version and the running executable
	// except in tests.
	version string
	exePath string

	// reportIssue, when set, tells the dashboard about an update that was
	// refused, so a bad release is visible there instead of only in a log.
	reportIssue func(severity, code, message string, extra map[string]interface{})

	// probeVersion asks a downloaded binary which version it is, and
	// relaunch starts the updated agent and ends this process. Both are fields
	// so tests can stand in for them.
	probeVersion func(ctx context.Context, exePath string) (string, error)
	relaunch     func(exePath string) error

	// rejected is "<version>/<sha256>" of the last offer that failed
	// verification, so the same bad binary is not downloaded again every hour.
	// A different offer (a fixed binary, a newer version) clears it.
	rejected string
}

// NewUpdaterService checks apiURL for updates. skipTLSVerify leaves the
// server's HTTPS certificate unchecked and is meant for development servers
// only: the checksum an update is verified against arrives over this same
// connection, so without a verified connection it proves nothing.
func NewUpdaterService(apiURL string, configPath string, skipTLSVerify bool) *UpdaterService {
	exePath, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	u := &UpdaterService{
		apiURL:     strings.TrimSuffix(apiURL, "/"),
		configPath: configPath,
		log:        slog.Default().With("service", "updater"),
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLSVerify},
			},
		},
		version: CurrentVersion,
		exePath: exePath,
	}
	u.probeVersion = probeBinaryVersion
	u.relaunch = u.startAndExit

	return u
}

// SetIssueReporter makes refused updates show up on the dashboard.
func (u *UpdaterService) SetIssueReporter(report func(severity, code, message string, extra map[string]interface{})) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.reportIssue = report
}

func (u *UpdaterService) CheckAndApplyUpdate(ctx context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	offer, err := u.fetchOffer(ctx)
	if err != nil {
		return err
	}

	if !offer.UpdateAvailable || offer.DownloadURL == "" {
		u.log.Info("Agent is up to date", "current_version", u.version)
		return nil
	}

	return u.apply(ctx, offer)
}

func (u *UpdaterService) fetchOffer(ctx context.Context) (*UpdateCheckResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	checkURL := fmt.Sprintf("%s/agents/check-update?version=%s", u.apiURL, url.QueryEscape(u.version))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check server returned status: %d", resp.StatusCode)
	}

	var res UpdateCheckResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to parse update response: %w", err)
	}
	return &res, nil
}

// apply downloads the offered binary, proves it is the one the server meant
// and that it really is the version on offer, and only then swaps it in.
func (u *UpdaterService) apply(ctx context.Context, offer *UpdateCheckResponse) error {
	// The server's "update available" is not taken on its word: an offer that
	// is not newer than what already runs would otherwise be installed over and
	// over.
	if compareVersions(offer.LatestVersion, u.version) <= 0 {
		u.log.Warn("Server offered an update that is not newer than this agent; ignoring it",
			"current_version", u.version, "offered_version", offer.LatestVersion)
		return nil
	}

	wantSum := strings.ToLower(strings.TrimSpace(offer.SHA256))
	offerKey := offer.LatestVersion + "/" + wantSum
	if offerKey == u.rejected {
		u.log.Debug("Skipping an update that already failed verification", "version", offer.LatestVersion)
		return nil
	}

	if !isSHA256Hex(wantSum) {
		return u.reject(offerKey, offer, "UPDATE_NO_CHECKSUM",
			"The server offered an agent update without a valid SHA-256 checksum, so it was not installed.")
	}

	u.log.Info("New agent version available! Preparing auto-update...", "current", u.version, "latest", offer.LatestVersion, "url", offer.DownloadURL)

	installDir := filepath.Dir(u.exePath)
	tempNewExe := filepath.Join(installDir, "agent_new.exe")
	oldExe := filepath.Join(installDir, "agent_old.exe")

	// 1. Download the new agent, hashing it on the way in.
	gotSum, err := u.download(ctx, offer.DownloadURL, tempNewExe)
	if err != nil {
		_ = os.Remove(tempNewExe)
		return err
	}

	// 2. It must be exactly the file the server vouched for.
	if gotSum != wantSum {
		_ = os.Remove(tempNewExe)
		return u.reject(offerKey, offer, "UPDATE_CHECKSUM_MISMATCH",
			fmt.Sprintf("The downloaded agent update does not match its SHA-256 checksum (expected %s, got %s), so it was not installed.", wantSum, gotSum))
	}

	// 3. And it must say it is the version being offered. This catches a
	// release where the binary on the server was not rebuilt with the new
	// version, which would otherwise be "updated" to again every hour.
	gotVersion, err := u.probeVersion(ctx, tempNewExe)
	if err != nil || gotVersion != offer.LatestVersion {
		_ = os.Remove(tempNewExe)
		reason := fmt.Sprintf("it reports version %q", gotVersion)
		if err != nil {
			reason = "it could not be run: " + err.Error()
		}
		return u.reject(offerKey, offer, "UPDATE_VERSION_MISMATCH",
			fmt.Sprintf("The server offered agent version %s but the downloaded binary is not that version (%s), so it was not installed.", offer.LatestVersion, reason))
	}

	u.log.Info("Download verified. Applying binary swap...", "new_version", offer.LatestVersion, "sha256", gotSum)

	// 4. Remove previous agent_old.exe if exists.
	_ = os.Remove(oldExe)

	// 5. Rename the current running agent.exe to agent_old.exe (Windows allows renaming running EXEs).
	if err := os.Rename(u.exePath, oldExe); err != nil {
		_ = os.Remove(tempNewExe)
		return fmt.Errorf("failed to rename current executable to old: %w", err)
	}

	// 6. Rename agent_new.exe to agent.exe.
	if err := os.Rename(tempNewExe, u.exePath); err != nil {
		// Rollback rename if swap failed
		_ = os.Rename(oldExe, u.exePath)
		return fmt.Errorf("failed to rename new executable: %w", err)
	}

	u.log.Info("Binary swap successful! Launching updated agent process...", "exe", u.exePath)

	// 7. Launch the updated agent and let this process end.
	if err := u.relaunch(u.exePath); err != nil {
		u.log.Error("Failed to start updated agent process", "error", err)
		return err
	}

	u.log.Info("Updated agent launched successfully. Exiting current process for self-restart.")
	return nil
}

// reject remembers a bad offer, tells the dashboard, and returns the error
// the update loop logs.
func (u *UpdaterService) reject(offerKey string, offer *UpdateCheckResponse, code, message string) error {
	u.rejected = offerKey
	u.log.Error("Refusing agent update", "code", code, "offered_version", offer.LatestVersion, "reason", message)

	if u.reportIssue != nil {
		u.reportIssue("error", code, message, map[string]interface{}{
			"current_version": u.version,
			"offered_version": offer.LatestVersion,
			"download_url":    offer.DownloadURL,
		})
	}
	return fmt.Errorf("%s: %s", code, message)
}

// download saves url to dest and returns the hex SHA-256 of what was saved.
func (u *UpdaterService) download(ctx context.Context, downloadURL, dest string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	u.log.Info("Downloading update binary...", "url", downloadURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download new agent binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download update, status: %d", resp.StatusCode)
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(resp.Body, maxBinarySize+1))
	closeErr := out.Close()
	switch {
	case copyErr != nil:
		return "", fmt.Errorf("failed to save downloaded binary: %w", copyErr)
	case closeErr != nil:
		return "", fmt.Errorf("failed to save downloaded binary: %w", closeErr)
	case written > maxBinarySize:
		return "", fmt.Errorf("downloaded update is larger than the %d byte limit", maxBinarySize)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// probeBinaryVersion runs exePath with -version and returns what it prints.
func probeBinaryVersion(ctx context.Context, exePath string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exePath, "-version")
	cmd.Dir = filepath.Dir(exePath)
	cmd.SysProcAttr = detachedSysProcAttr()

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// startAndExit launches the agent at exePath and ends this process shortly
// after, so the new one takes over.
func (u *UpdaterService) startAndExit(exePath string) error {
	cmd := exec.Command(exePath, "-config", u.configPath)
	cmd.Dir = filepath.Dir(exePath)
	cmd.SysProcAttr = detachedSysProcAttr()

	if err := cmd.Start(); err != nil {
		return err
	}

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

// compareVersions compares dotted numeric versions: -1 if a is older than b,
// 0 if they are the same, 1 if a is newer. A leading "v" and anything after a
// "-" or "+" is ignored, and a missing part counts as 0.
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}

	var parts []int
	for _, p := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(p)
		parts = append(parts, n)
	}
	return parts
}

func isSHA256Hex(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
