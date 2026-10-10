package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

const (
	oldBinary = "old agent 1.0.4"
	newBinary = "new agent 1.0.5"
)

func sum(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// fixture is an install directory with an agent.exe in it, a fake update
// server, and an updater pointed at both. It counts downloads and relaunches.
type fixture struct {
	t         *testing.T
	dir       string
	exe       string
	updater   *UpdaterService
	offer     UpdateCheckResponse
	served    string // what /download returns
	downloads atomic.Int32
	relaunch  atomic.Int32
	issues    []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := &fixture{t: t, dir: t.TempDir(), served: newBinary}
	f.exe = filepath.Join(f.dir, "agent.exe")
	if err := os.WriteFile(f.exe, []byte(oldBinary), 0o755); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents/check-update", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(f.offer)
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		f.downloads.Add(1)
		_, _ = w.Write([]byte(f.served))
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) {
		f.downloads.Add(1)
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	f.offer = UpdateCheckResponse{
		UpdateAvailable: true,
		LatestVersion:   "1.0.5",
		DownloadURL:     server.URL + "/download",
		SHA256:          sum(newBinary),
	}

	f.updater = NewUpdaterService(server.URL+"/api", "agent.yaml", false)
	f.updater.version = "1.0.4"
	f.updater.exePath = f.exe
	f.updater.probeVersion = func(context.Context, string) (string, error) { return "1.0.5", nil }
	f.updater.relaunch = func(string) error { f.relaunch.Add(1); return nil }
	f.updater.SetIssueReporter(func(severity, code, message string, extra map[string]interface{}) {
		f.issues = append(f.issues, code)
	})

	return f
}

func (f *fixture) read(path string) string {
	f.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func (f *fixture) exists(name string) bool {
	_, err := os.Stat(filepath.Join(f.dir, name))
	return err == nil
}

func (f *fixture) assertNotUpdated() {
	f.t.Helper()
	if got := f.read(f.exe); got != oldBinary {
		f.t.Fatalf("agent.exe must be untouched, got %q", got)
	}
	if f.exists("agent_new.exe") {
		f.t.Fatal("the rejected download must not be left behind")
	}
	if f.relaunch.Load() != 0 {
		f.t.Fatal("the agent must not be relaunched")
	}
}

func TestVerifiedUpdateReplacesTheAgentAndRelaunchesIt(t *testing.T) {
	f := newFixture(t)

	if err := f.updater.CheckAndApplyUpdate(context.Background()); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if got := f.read(f.exe); got != newBinary {
		t.Fatalf("agent.exe should be the new binary, got %q", got)
	}
	if got := f.read(filepath.Join(f.dir, "agent_old.exe")); got != oldBinary {
		t.Fatalf("the previous binary must be kept as agent_old.exe for rollback, got %q", got)
	}
	if f.exists("agent_new.exe") {
		t.Fatal("agent_new.exe must be gone after the swap")
	}
	if f.relaunch.Load() != 1 {
		t.Fatalf("expected one relaunch, got %d", f.relaunch.Load())
	}
}

func TestNothingHappensWhenNoUpdateIsAvailable(t *testing.T) {
	f := newFixture(t)
	f.offer.UpdateAvailable = false

	if err := f.updater.CheckAndApplyUpdate(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.downloads.Load() != 0 {
		t.Fatal("nothing should be downloaded")
	}
	f.assertNotUpdated()
}

func TestDownloadThatDoesNotMatchItsChecksumIsRefused(t *testing.T) {
	f := newFixture(t)
	f.served = "tampered binary"

	if err := f.updater.CheckAndApplyUpdate(context.Background()); err == nil {
		t.Fatal("a checksum mismatch must be an error")
	}
	f.assertNotUpdated()

	if len(f.issues) != 1 || f.issues[0] != "UPDATE_CHECKSUM_MISMATCH" {
		t.Fatalf("the dashboard must be told, got %v", f.issues)
	}

	// The same bad offer is not downloaded again every hour.
	_ = f.updater.CheckAndApplyUpdate(context.Background())
	if f.downloads.Load() != 1 {
		t.Fatalf("a refused offer must not be downloaded again, downloads=%d", f.downloads.Load())
	}

	// A fixed binary (different checksum) is tried.
	f.served = newBinary
	f.offer.SHA256 = sum(newBinary)
	f.offer.LatestVersion = "1.0.6"
	f.updater.probeVersion = func(context.Context, string) (string, error) { return "1.0.6", nil }
	if err := f.updater.CheckAndApplyUpdate(context.Background()); err != nil {
		t.Fatalf("a corrected offer should install: %v", err)
	}
	if f.read(f.exe) != newBinary {
		t.Fatal("the corrected update should have been installed")
	}
}

func TestUpdateWithoutAChecksumIsRefusedBeforeDownloading(t *testing.T) {
	f := newFixture(t)
	f.offer.SHA256 = ""

	if err := f.updater.CheckAndApplyUpdate(context.Background()); err == nil {
		t.Fatal("an offer with no checksum must be refused")
	}
	if f.downloads.Load() != 0 {
		t.Fatal("nothing should be downloaded without a checksum to verify it against")
	}
	f.assertNotUpdated()
	if len(f.issues) != 1 || f.issues[0] != "UPDATE_NO_CHECKSUM" {
		t.Fatalf("the dashboard must be told, got %v", f.issues)
	}
}

func TestBinaryThatIsNotTheOfferedVersionIsRefused(t *testing.T) {
	f := newFixture(t)
	// The server says 1.0.5 and the checksum matches, but the binary was never
	// rebuilt: it still reports the old version.
	f.updater.probeVersion = func(context.Context, string) (string, error) { return "1.0.4", nil }

	if err := f.updater.CheckAndApplyUpdate(context.Background()); err == nil {
		t.Fatal("a version mismatch must be an error")
	}
	f.assertNotUpdated()
	if len(f.issues) != 1 || f.issues[0] != "UPDATE_VERSION_MISMATCH" {
		t.Fatalf("the dashboard must be told, got %v", f.issues)
	}

	_ = f.updater.CheckAndApplyUpdate(context.Background())
	if f.downloads.Load() != 1 {
		t.Fatalf("this is what stops an update loop: the same offer must not be downloaded again, downloads=%d", f.downloads.Load())
	}
}

func TestBinaryThatCannotBeRunIsRefused(t *testing.T) {
	f := newFixture(t)
	f.updater.probeVersion = func(context.Context, string) (string, error) { return "", errors.New("not a valid Win32 application") }

	if err := f.updater.CheckAndApplyUpdate(context.Background()); err == nil {
		t.Fatal("a binary that cannot run must be refused")
	}
	f.assertNotUpdated()
}

func TestOfferThatIsNotNewerIsIgnored(t *testing.T) {
	for _, offered := range []string{"1.0.4", "1.0.3", ""} {
		f := newFixture(t)
		f.offer.LatestVersion = offered

		if err := f.updater.CheckAndApplyUpdate(context.Background()); err != nil {
			t.Fatalf("offered %q: unexpected error %v", offered, err)
		}
		if f.downloads.Load() != 0 {
			t.Fatalf("offered %q: nothing should be downloaded", offered)
		}
		f.assertNotUpdated()
	}
}

func TestFailedDownloadIsRetriedNextTime(t *testing.T) {
	f := newFixture(t)
	f.offer.DownloadURL = f.offer.DownloadURL[:len(f.offer.DownloadURL)-len("/download")] + "/missing"

	if err := f.updater.CheckAndApplyUpdate(context.Background()); err == nil {
		t.Fatal("a 404 must be an error")
	}
	f.assertNotUpdated()
	if len(f.issues) != 0 {
		t.Fatalf("a download failure is transient and not a bad release, got issues %v", f.issues)
	}

	_ = f.updater.CheckAndApplyUpdate(context.Background())
	if f.downloads.Load() != 2 {
		t.Fatalf("a transient failure must be retried, downloads=%d", f.downloads.Load())
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.5", "1.0.4", 1},
		{"1.0.4", "1.0.5", -1},
		{"1.0.5", "1.0.5", 0},
		{"1.0.10", "1.0.9", 1},
		{"1.1.0", "1.0.99", 1},
		{"2.0", "1.9.9", 1},
		{"1.0", "1.0.0", 0},
		{"v1.0.5", "1.0.5", 0},
		{"1.0.5+dirty", "1.0.5", 0},
		{"1.0.5-rc1", "1.0.4", 1},
		{"", "1.0.0", -1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
