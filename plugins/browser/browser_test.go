package browser

import "testing"

func TestCheck(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://example.com/a": true, "HTTP://127.0.0.1:8080/": true,
		"ftp://example.com": false, "https://user:pw@example.com": false, "https://": false, "/relative": false,
	} {
		if got := check(raw) == nil; got != ok {
			t.Errorf("%q: taken %v, want %v", raw, got, ok)
		}
	}
}
