//go:build windows && (amd64 || arm64)

package windows

import "testing"

// TestServedAtBase checks which base URLs LoadHTML serves its document at:
// not about: URLs, which WebView2 never requests, so that their pages show
// the document as those without a base URL do (#1029).
func TestServedAtBase(t *testing.T) {
	for base, want := range map[string]bool{
		"":                                false,
		"about:blank":                     false,
		"about:blank#top":                 false,
		"About:Blank":                     false,
		"about:srcdoc":                    false,
		"app://localhost/":                true,
		"https://example.com/about:blank": true,
		"file:///C:/app/index.html":       true,
	} {
		if got := servedAtBase(base); got != want {
			t.Errorf("servedAtBase(%q) = %v, want %v", base, got, want)
		}
	}
}
