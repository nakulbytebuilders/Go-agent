//go:build windows

package browsertracker

import "testing"

func TestParseBrowserTitleDoesNotInventDomains(t *testing.T) {
	cases := []struct {
		title      string
		wantTab    string
		wantDomain string
	}{
		{"New chat - ChatGPT - Google Chrome", "New chat - ChatGPT", "chatgpt.com"},
		{"repo - GitHub - Google Chrome", "repo - GitHub", "github.com"},
		{"Docs - docs.example.org - Google Chrome", "Docs - docs.example.org", "docs.example.org"},
		// No site we can recognise: no domain, rather than "log.com" / "admin.com".
		{"Log in - Nova User Monitor - Google Chrome", "Log in - Nova User Monitor", ""},
		{"Admin Overview - Nova User Monitor - Google Chrome", "Admin Overview - Nova User Monitor", ""},
		{"New Tab - Google Chrome", "New Tab", ""},
	}

	for _, c := range cases {
		tab, domain, rawURL := parseBrowserTitle(c.title, "Google Chrome")
		if tab != c.wantTab || domain != c.wantDomain {
			t.Errorf("parseBrowserTitle(%q) = (%q, %q), want (%q, %q)", c.title, tab, domain, c.wantTab, c.wantDomain)
		}
		if rawURL != "" {
			t.Errorf("parseBrowserTitle(%q) invented a URL %q; the title never carries one", c.title, rawURL)
		}
	}
}
