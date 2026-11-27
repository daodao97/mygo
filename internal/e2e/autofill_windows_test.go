//go:build windows

package e2e

import (
	"testing"

	"github.com/egoist/mygo"
	win "github.com/egoist/mygo/internal/windows"
)

// WebView2 saves what is typed into a page's fields and suggests it later.
// The setting lives on the profile, so each session is turned off on its
// own. WKWebView and WebKitGTK keep no form history.
func TestGeneralAutofillOff(t *testing.T) {
	for _, tc := range []struct {
		name string
		page mygo.PageOptions
	}{
		{"the default session", mygo.PageOptions{}},
		{"another session", mygo.PageOptions{Session: mygo.NewSession("autofill-e2e")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWindow(t, mygo.WindowOptions{Title: "Autofill", Width: 300, Height: 200, Page: tc.page})
			w.Page().LoadHTML("<p>autofill</p>", "")
			waitFor(t, w, "document.readyState == 'complete'")
			var on, ok bool
			mygo.RunOnMain(func() { on, ok = win.TestGeneralAutofill(w.NativeHandle()) })
			if !ok {
				t.Skip("the WebView2 Runtime has no ICoreWebView2Settings4")
			}
			if on {
				t.Error("general autofill is on")
			}
		})
	}
}
