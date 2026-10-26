//go:build !js

package mygo

import (
	"errors"
	"image/color"
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestPage(t *testing.T) {
	w := NewWindow(WindowOptions{Hidden: true, URL: "https://example.com/a", Page: PageOptions{
		PreloadScript: "window.preloaded = true", UserAgent: "agent", UserAgentSuffix: " MyApp/1.2 ", ZoomFactor: 1.5, DevTools: DevToolsEnabled,
	}})
	t.Cleanup(w.Destroy)
	fw := fb.Windows()[len(fb.Windows())-1]
	p := w.Page()
	if p == nil || p.Window() != w {
		t.Fatalf("Page %v", p)
	}
	o := fw.Opts
	preloaded := slices.ContainsFunc(o.UserScripts, func(s platform.UserScript) bool { return s.Source == "window.preloaded = true" })
	if o.UserAgent != "agent" || o.UserAgentSuffix != "MyApp/1.2" || o.Zoom != 1.5 || !o.DevTools || !preloaded {
		t.Errorf("page options: user agent %q, suffix %q, zoom %v, devtools %v, preloaded %v", o.UserAgent, o.UserAgentSuffix, o.Zoom, o.DevTools, preloaded)
	}
	if got := p.URL(); got != "https://example.com/a" {
		t.Errorf("URL %q", got)
	}
	if err := p.LoadURL("https://example.com/b"); err != nil || p.URL() != "https://example.com/b" {
		t.Errorf("LoadURL: %v, URL %q", err, p.URL())
	}
	if got := p.ProcessID(); got != 0 {
		t.Errorf("ProcessID before the backend knows one: %d", got)
	}
	fw.SetProcessID(4242)
	if got := p.ProcessID(); got != 4242 {
		t.Errorf("ProcessID %d, want 4242", got)
	}
	if _, err := EvalAs[int]((*Page)(nil), "1"); !errors.Is(err, errNoPage) {
		t.Errorf("EvalAs of no page: %v", err)
	}
}

func TestPageProcessIDAfterClose(t *testing.T) {
	w := NewWindow(WindowOptions{Hidden: true, URL: "https://example.com/"})
	fb.Windows()[len(fb.Windows())-1].SetProcessID(7)
	p := w.Page()
	if got := p.ProcessID(); got != 7 {
		t.Fatalf("ProcessID %d, want 7", got)
	}
	w.Destroy()
	if got := p.ProcessID(); got != 0 {
		t.Errorf("ProcessID of a closed window's page: %d", got)
	}
}

func TestPageThemeColor(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{Hidden: true})
	p := w.Page()
	type change struct {
		c  color.NRGBA
		ok bool
	}
	var changes []change // listeners run on the main thread, as page does
	off := p.OnThemeColorChanged(func(c color.NRGBA, ok bool) { changes = append(changes, change{c, ok}) })
	defer off()
	expect := func(what string, want ...change) {
		t.Helper()
		if !slices.Equal(changes, want) {
			t.Errorf("%s: changes %v, want %v", what, changes, want)
		}
		changes = nil
	}
	if c, ok := p.ThemeColor(); ok {
		t.Errorf("ThemeColor of a new page: %v", c)
	}

	blue := color.NRGBA{R: 0x1e, G: 0x90, B: 0xff, A: 0xff}
	page(fw, `{"t":"theme-color","v":"#1e90ffff"}`)
	if c, ok := p.ThemeColor(); !ok || c != blue {
		t.Errorf("ThemeColor = %v, %v; want %v", c, ok, blue)
	}
	expect("a meta", change{blue, true})
	page(fw, `{"t":"theme-color","v":"#1e90ffff"}`)
	expect("the same color")
	half := color.NRGBA{R: 255, A: 0x80}
	page(fw, `{"t":"theme-color","v":"#ff000080"}`)
	expect("a translucent color", change{half, true})
	page(fw, `{"t":"theme-color","v":""}`)
	if c, ok := p.ThemeColor(); ok {
		t.Errorf("ThemeColor after the meta went: %v", c)
	}
	expect("the meta removed", change{})
	page(fw, `{"t":"theme-color","v":""}`)
	page(fw, `{"t":"theme-color","v":"not a color"}`)
	expect("no color still")

	// Another page keeps the color until it tells its own, and changes
	// nothing when that is the same.
	page(fw, `{"t":"theme-color","v":"#1e90ffff"}`)
	expect("a meta again", change{blue, true})
	onMain(func() { fw.H.NavigationCommitted("https://next.example/") })
	if c, ok := p.ThemeColor(); !ok || c != blue {
		t.Errorf("ThemeColor as the next page loads = %v, %v", c, ok)
	}
	page(fw, `{"t":"theme-color","v":"#1e90ffff"}`)
	page(fw, `{"t":"dom-ready"}`)
	onMain(fw.H.LoadFinished)
	expect("a page of the same color")

	// A page without the bridge, an image, has none once it loaded.
	onMain(func() { fw.H.NavigationCommitted("https://next.example/a.png") })
	expect("an image loading")
	onMain(fw.H.LoadFinished)
	expect("an image loaded", change{})
	onMain(func() { fw.H.LoadFailed("https://next.example/b", 1, "nope") })
	expect("a failed load")

	off()
	page(fw, `{"t":"theme-color","v":"#1e90ffff"}`)
	expect("after off")
	w.Destroy()
	if c, ok := p.ThemeColor(); ok {
		t.Errorf("ThemeColor of a closed window's page: %v", c)
	}
}

// Only the page's bridge tells the theme color, not another frame, which
// lacks its secret.
func TestPageThemeColorNeedsSecret(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{Hidden: true})
	onMain(func() { fw.H.Message(`{"t":"theme-color","v":"#ff0000ff"}`) })
	if c, ok := w.Page().ThemeColor(); ok {
		t.Errorf("ThemeColor from a message without the secret: %v", c)
	}
}

func TestPageAudio(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{Hidden: true})
	p := w.Page()
	var changes []bool // listeners run on the main thread, as SetAudible does
	off := p.OnAudibleChanged(func(audible bool) { changes = append(changes, audible) })
	expect := func(what string, audible bool, want ...bool) {
		t.Helper()
		if got := p.IsAudible(); got != audible {
			t.Errorf("%s: IsAudible %v, want %v", what, got, audible)
		}
		if !slices.Equal(changes, want) {
			t.Errorf("%s: changes %v, want %v", what, changes, want)
		}
		changes = nil
	}
	expect("a new page", false)
	onMain(func() { fw.SetAudible(true) })
	expect("playing", true, true)
	onMain(func() { fw.SetAudible(true) })
	expect("playing again", true) // told only when it changes
	onMain(func() { fw.SetAudible(false) })
	expect("stopped", false, false)
	off()
	onMain(func() { fw.SetAudible(true) })
	expect("after off", true)

	if p.IsAudioMuted() {
		t.Error("a new page is muted")
	}
	p.SetAudioMuted(true)
	if !p.IsAudioMuted() || !fw.IsAudioMuted() {
		t.Errorf("SetAudioMuted(true): IsAudioMuted %v, backend's %v", p.IsAudioMuted(), fw.IsAudioMuted())
	}
	p.SetAudioMuted(false)
	if p.IsAudioMuted() || fw.IsAudioMuted() {
		t.Errorf("SetAudioMuted(false): IsAudioMuted %v, backend's %v", p.IsAudioMuted(), fw.IsAudioMuted())
	}

	p.SetAudioMuted(true)
	w.Destroy()
	if p.IsAudible() || p.IsAudioMuted() {
		t.Errorf("a closed window's page: IsAudible %v, IsAudioMuted %v", p.IsAudible(), p.IsAudioMuted())
	}
	p.SetAudioMuted(false) // does nothing
}
