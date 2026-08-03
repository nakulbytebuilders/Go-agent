//go:build windows

package browsertracker

import (
	"net/url"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW             = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

type ActiveBrowserInfo struct {
	IsBrowser   bool
	BrowserName string
	TabTitle    string
	Domain      string
	URL         string
}

var knownBrowsers = map[string]string{
	"chrome.exe":    "Google Chrome",
	"msedge.exe":    "Microsoft Edge",
	"firefox.exe":   "Mozilla Firefox",
	"brave.exe":     "Brave Browser",
	"opera.exe":     "Opera",
	"vivaldi.exe":   "Vivaldi",
	"arc.exe":       "Arc",
	"iexplore.exe":  "Internet Explorer",
}

func getActiveBrowserInfo() (ActiveBrowserInfo, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ActiveBrowserInfo{}, nil
	}

	// 1. Get PID
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ActiveBrowserInfo{}, nil
	}

	// 2. Get Exe Name
	exeName := ""
	hProcess, _, _ := procOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
	if hProcess != 0 {
		defer procCloseHandle.Call(hProcess)

		pathBuf := make([]uint16, 1024)
		size := uint32(len(pathBuf))
		r, _, _ := procQueryFullProcessImageNameW.Call(hProcess, 0, uintptr(unsafe.Pointer(&pathBuf[0])), uintptr(unsafe.Pointer(&size)))
		if r != 0 {
			fullPath := syscall.UTF16ToString(pathBuf[:size])
			exeName = strings.ToLower(filepath.Base(fullPath))
		}
	}

	browserDisplayName, isBrowser := knownBrowsers[exeName]
	if !isBrowser {
		return ActiveBrowserInfo{IsBrowser: false}, nil
	}

	// 3. Get Window Title
	buf := make([]uint16, 512)
	ret, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	fullTitle := ""
	if ret > 0 {
		fullTitle = syscall.UTF16ToString(buf[:ret])
	}

	tabTitle, domain, rawURL := parseBrowserTitle(fullTitle, browserDisplayName)

	return ActiveBrowserInfo{
		IsBrowser:   true,
		BrowserName: browserDisplayName,
		TabTitle:    tabTitle,
		Domain:      domain,
		URL:         rawURL,
	}, nil
}

func parseBrowserTitle(title string, browserName string) (tabTitle string, domain string, rawURL string) {
	if title == "" {
		return "New Tab", "New Tab", ""
	}

	// Strip trailing browser suffix e.g. " - Google Chrome", " — Mozilla Firefox"
	suffixes := []string{
		" - " + browserName,
		" — " + browserName,
		" - Google Chrome",
		" - Microsoft Edge",
		" — Mozilla Firefox",
		" - Brave",
		" - Opera",
	}

	cleanedTitle := title
	for _, suf := range suffixes {
		if strings.HasSuffix(cleanedTitle, suf) {
			cleanedTitle = strings.TrimSuffix(cleanedTitle, suf)
			break
		}
	}
	cleanedTitle = strings.TrimSpace(cleanedTitle)

	// Check if title is a direct URL or contains http:// / https://
	if strings.HasPrefix(cleanedTitle, "http://") || strings.HasPrefix(cleanedTitle, "https://") {
		if u, err := url.Parse(cleanedTitle); err == nil {
			return cleanedTitle, u.Hostname(), cleanedTitle
		}
	}

	// Extract domain from title or known website patterns
	domain = extractDomainFromTitle(cleanedTitle)

	return cleanedTitle, domain, "https://" + domain
}

func extractDomainFromTitle(title string) string {
	lower := strings.ToLower(title)

	// Check if title contains an explicit domain like github.com, google.com, etc.
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return r == ' ' || r == '|' || r == '-' || r == ':' || r == '—' || r == '/' || r == '(' || r == ')'
	})

	for _, w := range words {
		w = strings.Trim(w, " .,;")
		if strings.Contains(w, ".") && (strings.HasSuffix(w, ".com") || strings.HasSuffix(w, ".org") ||
			strings.HasSuffix(w, ".net") || strings.HasSuffix(w, ".io") || strings.HasSuffix(w, ".dev") ||
			strings.HasSuffix(w, ".in") || strings.HasSuffix(w, ".ai") || strings.HasSuffix(w, ".gov") ||
			strings.HasSuffix(w, ".edu") || strings.HasSuffix(w, ".tv") || strings.HasSuffix(w, ".co")) {
			return w
		}
	}

	// Common web app keyword mappings
	switch {
	case strings.Contains(lower, "github"):
		return "github.com"
	case strings.Contains(lower, "google"):
		return "google.com"
	case strings.Contains(lower, "youtube"):
		return "youtube.com"
	case strings.Contains(lower, "gmail"):
		return "mail.google.com"
	case strings.Contains(lower, "stack overflow") || strings.Contains(lower, "stackoverflow"):
		return "stackoverflow.com"
	case strings.Contains(lower, "chatgpt") || strings.Contains(lower, "openai"):
		return "chatgpt.com"
	case strings.Contains(lower, "claude"):
		return "claude.ai"
	case strings.Contains(lower, "reddit"):
		return "reddit.com"
	case strings.Contains(lower, "linkedin"):
		return "linkedin.com"
	case strings.Contains(lower, "twitter") || strings.Contains(lower, "x.com"):
		return "x.com"
	case strings.Contains(lower, "facebook"):
		return "facebook.com"
	case strings.Contains(lower, "amazon"):
		return "amazon.com"
	case strings.Contains(lower, "wikipedia"):
		return "wikipedia.org"
	case strings.Contains(lower, "new tab"):
		return "New Tab"
	default:
		if len(words) > 0 {
			firstWord := strings.Trim(words[0], " .,;")
			if len(firstWord) > 2 {
				return firstWord + ".com"
			}
		}
		return "web-browsing"
	}
}
