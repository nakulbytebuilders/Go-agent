//go:build darwin

package browsertracker

import (
	"net/url"
	"os/exec"
	"strings"
)

type ActiveBrowserInfo struct {
	IsBrowser   bool
	BrowserName string
	TabTitle    string
	Domain      string
	URL         string
}

// knownBrowsers maps the macOS frontmost-process name (as reported by
// System Events) to a display name and whether it exposes a Chromium-style
// AppleScript dictionary (URL/title of the active tab can be queried
// directly, no title-string parsing needed).
var knownBrowsers = map[string]struct {
	displayName string
	chromiumAS  bool
}{
	"Google Chrome":  {"Google Chrome", true},
	"Safari":         {"Safari", false},
	"Firefox":        {"Mozilla Firefox", false},
	"Microsoft Edge": {"Microsoft Edge", true},
	"Brave Browser":  {"Brave Browser", true},
	"Opera":          {"Opera", true},
	"Vivaldi":        {"Vivaldi", true},
}

const frontAppScript = `
tell application "System Events"
	return name of first application process whose frontmost is true
end tell
`

func getActiveBrowserInfo() (ActiveBrowserInfo, error) {
	out, err := exec.Command("osascript", "-e", frontAppScript).Output()
	if err != nil {
		return ActiveBrowserInfo{}, nil
	}
	appName := strings.TrimSpace(string(out))

	meta, isBrowser := knownBrowsers[appName]
	if !isBrowser {
		return ActiveBrowserInfo{IsBrowser: false}, nil
	}

	if meta.chromiumAS {
		if title, url, ok := queryChromiumTab(appName); ok {
			return buildBrowserInfoFromURL(meta.displayName, title, url), nil
		}
	} else if appName == "Safari" {
		if title, url, ok := querySafariTab(); ok {
			return buildBrowserInfoFromURL(meta.displayName, title, url), nil
		}
	}

	// Fallback: parse the window title (covers Firefox, and any Chromium
	// browser where AppleScript access hasn't been granted yet).
	winTitle := ""
	if out, err := exec.Command("osascript", "-e", windowTitleScript(appName)).Output(); err == nil {
		winTitle = strings.TrimSpace(string(out))
	}
	tabTitle, domain, rawURL := parseBrowserTitle(winTitle, meta.displayName)
	return ActiveBrowserInfo{
		IsBrowser:   true,
		BrowserName: meta.displayName,
		TabTitle:    tabTitle,
		Domain:      domain,
		URL:         rawURL,
	}, nil
}

func windowTitleScript(processName string) string {
	return `tell application "System Events" to tell process "` + processName + `" to return name of front window`
}

func queryChromiumTab(appName string) (title, rawURL string, ok bool) {
	script := `tell application "` + appName + `" to return (title of active tab of front window) & "|||" & (URL of active tab of front window)`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|||", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func querySafariTab() (title, rawURL string, ok bool) {
	script := `tell application "Safari" to return (name of front document) & "|||" & (URL of front document)`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|||", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func buildBrowserInfoFromURL(browserName, title, rawURL string) ActiveBrowserInfo {
	domain := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Hostname() != "" {
		domain = u.Hostname()
	}
	if title == "" {
		title = "New Tab"
	}
	return ActiveBrowserInfo{
		IsBrowser:   true,
		BrowserName: browserName,
		TabTitle:    title,
		Domain:      domain,
		URL:         rawURL,
	}
}

func parseBrowserTitle(title string, browserName string) (tabTitle string, domain string, rawURL string) {
	if title == "" {
		return "New Tab", "New Tab", ""
	}

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
