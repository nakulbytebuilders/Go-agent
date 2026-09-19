//go:build linux

package browsertracker

import (
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type ActiveBrowserInfo struct {
	IsBrowser   bool
	BrowserName string
	TabTitle    string
	Domain      string
	URL         string
}

// knownBrowsers maps the Linux process "comm" name (from /proc/<pid>/comm,
// truncated to 15 chars by the kernel) to a display name.
var knownBrowsers = map[string]string{
	"chrome":          "Google Chrome",
	"google-chrome":   "Google Chrome",
	"msedge":          "Microsoft Edge",
	"firefox":         "Mozilla Firefox",
	"firefox-esr":     "Mozilla Firefox",
	"brave":           "Brave Browser",
	"brave-browser":   "Brave Browser",
	"opera":           "Opera",
	"vivaldi-bin":     "Vivaldi",
	"chromium":        "Chromium",
	"chromium-browse": "Chromium",
}

func getActiveBrowserInfo() (ActiveBrowserInfo, error) {
	winIDOut, err := exec.Command("xdotool", "getactivewindow").Output()
	if err != nil {
		return ActiveBrowserInfo{}, nil
	}
	winID := strings.TrimSpace(string(winIDOut))
	if winID == "" {
		return ActiveBrowserInfo{}, nil
	}

	pid := 0
	if out, err := exec.Command("xdotool", "getwindowpid", winID).Output(); err == nil {
		pid, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	}
	if pid == 0 {
		return ActiveBrowserInfo{}, nil
	}

	comm := ""
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm"); err == nil {
		comm = strings.ToLower(strings.TrimSpace(string(data)))
	}

	browserDisplayName, isBrowser := knownBrowsers[comm]
	if !isBrowser {
		return ActiveBrowserInfo{IsBrowser: false}, nil
	}

	fullTitle := ""
	if out, err := exec.Command("xdotool", "getwindowname", winID).Output(); err == nil {
		fullTitle = strings.TrimSpace(string(out))
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

	suffixes := []string{
		" - " + browserName,
		" — " + browserName,
		" - Google Chrome",
		" - Chromium",
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

	if strings.HasPrefix(cleanedTitle, "http://") || strings.HasPrefix(cleanedTitle, "https://") {
		if u, err := url.Parse(cleanedTitle); err == nil {
			return cleanedTitle, u.Hostname(), cleanedTitle
		}
	}

	domain = extractDomainFromTitle(cleanedTitle)

	return cleanedTitle, domain, "https://" + domain
}

func extractDomainFromTitle(title string) string {
	lower := strings.ToLower(title)

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
