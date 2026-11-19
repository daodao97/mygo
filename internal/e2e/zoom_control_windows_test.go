//go:build windows

package e2e

import (
	"math"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
	win "github.com/egoist/mygo/internal/windows"
	"github.com/egoist/mygo/ui"
)

// TestZoomControl checks that Ctrl+wheel zooms a page only when
// PageOptions.ZoomControl asks, as WebView2's settings read back, in the
// pages of windows and of web views in native UI.
func TestZoomControl(t *testing.T) {
	zoom := mygo.PageOptions{ZoomControl: true}
	plain := newWindow(t, mygo.WindowOptions{Title: "No zoom control", Width: 300, Height: 200})
	on := newWindow(t, mygo.WindowOptions{Title: "Zoom control", Width: 300, Height: 200, Page: zoom})

	var views atomic.Pointer[[2]*mygo.WebView]
	var frames atomic.Int32
	host := newWindow(t, mygo.WindowOptions{Title: "Web views' zoom control", Width: 400, Height: 300, Content: ui.View(func(c *ui.Context) {
		frames.Add(1)
		ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
			if v := views.Load(); v != nil {
				ui.WebView(c, v[0]).Grow(1)
				ui.WebView(c, v[1]).Grow(1)
			}
		})
	})})
	eventually(t, "a frame", func() bool { return frames.Load() > 0 })
	plainView, err := host.NewWebView(mygo.WebViewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	onView, err := host.NewWebView(mygo.WebViewOptions{Page: zoom})
	if err != nil {
		t.Fatal(err)
	}
	host.Update(func() { views.Store(&[2]*mygo.WebView{plainView, onView}) })

	// The pages load, so that the web views are made: WebView2 makes its
	// own after the window.
	for _, p := range []*mygo.Page{plain.Page(), on.Page(), plainView.Page(), onView.Page()} {
		p.LoadHTML(gesturesPage, "")
		waitForPage(t, p, "window.ready")
	}

	for _, tc := range []struct {
		name    string
		w       *mygo.Window
		webView int
		want    bool
	}{
		{"a window's page by default", plain, -1, false},
		{"a window's page with ZoomControl", on, -1, true},
		{"a web view by default", host, 0, false},
		{"a web view with ZoomControl", host, 1, true},
	} {
		var got, ok bool
		mygo.RunOnMain(func() { got, ok = win.TestZoomControl(tc.w.NativeHandle(), tc.webView) })
		if !ok {
			t.Fatalf("%s has no web view", tc.name)
		}
		if got != tc.want {
			t.Errorf("%s: Ctrl+wheel zooms %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestUserZoomReported zooms a page as the user's Ctrl+wheel does, through
// WebView2's controller behind SetZoomFactor's back, and runs what its
// ZoomFactorChanged runs, which WebView2 raises for the user's zoom only:
// Page.ZoomFactor follows, and SetZoomFactor still sets the zoom after it.
func TestUserZoomReported(t *testing.T) {
	w := newWindow(t, mygo.WindowOptions{Title: "User zoom", Width: 300, Height: 200, Page: mygo.PageOptions{ZoomControl: true}})
	w.Page().LoadHTML(gesturesPage, "")
	waitFor(t, w, "window.ready")

	var (
		got float64
		hr  uintptr
		ok  bool
	)
	mygo.RunOnMain(func() { got, hr, ok = win.TestUserZoom(w.NativeHandle(), 1.5) })
	if !ok {
		t.Fatal("the page has no web view")
	}
	if int32(uint32(hr)) < 0 || math.Abs(got-1.5) > 1e-6 {
		t.Fatalf("the controller's zoom put to 1.5 reads %v (HRESULT %#x)", got, hr)
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	if !wait.Poll(func() bool { return near(w.Page().ZoomFactor(), 1.5) }) {
		t.Fatalf("ZoomFactor stays %v after the user's zoom to 1.5 (the controller's %v)", w.Page().ZoomFactor(), got)
	}

	w.Page().SetZoomFactor(1.25)
	if z := w.Page().ZoomFactor(); !near(z, 1.25) {
		t.Errorf("ZoomFactor after SetZoomFactor(1.25) = %v", z)
	}
	// A script's round trip, after which WebView2 has told of the zoom.
	waitFor(t, w, "window.ready")
	if z := w.Page().ZoomFactor(); !near(z, 1.25) {
		t.Errorf("ZoomFactor once WebView2 told of SetZoomFactor(1.25) = %v", z)
	}
}
