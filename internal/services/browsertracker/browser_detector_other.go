//go:build !windows

package browsertracker

type ActiveBrowserInfo struct {
	IsBrowser   bool
	BrowserName string
	TabTitle    string
	Domain      string
	URL         string
}

func getActiveBrowserInfo() (ActiveBrowserInfo, error) {
	return ActiveBrowserInfo{
		IsBrowser:   true,
		BrowserName: "Google Chrome",
		TabTitle:    "Google Search - Chrome",
		Domain:      "google.com",
		URL:         "https://google.com",
	}, nil
}
