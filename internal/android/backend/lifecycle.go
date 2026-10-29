//go:build android && cgo

package backend

// The Activity's lifecycle, the system's memory pressure and its night mode,
// as the Java host reports them (MygoActivity), translated into the calls
// iOS makes at the same moments (internal/ios's goIOSLifecycle). The kinds
// keep iOS's numbers so the two hosts read alike.

import "github.com/egoist/mygo/internal/platform"

// Lifecycle kinds, as Native.onLifecycle sends them.
const (
	lifecyclePause     = 0 // onPause: the Activity loses the foreground
	lifecycleResume    = 1 // onResume: it has the foreground again
	lifecycleLowMemory = 3 // onTrimMemory at a level worth releasing for
	lifecycleStop      = 5 // onStop: it is no longer visible
	lifecycleStart     = 6 // onStart: it is about to be visible
)

// OnLifecycle receives an Activity lifecycle event and queues it for the
// pump, like the other On* methods.
func (b *Backend) OnLifecycle(kind int) {
	b.Post(func() { b.lifecycle(kind) })
}

// lifecycle (pump) tells the app and its window. Events that arrive before
// Init have no one to tell and are dropped: the first onResume comes before
// App.Run, and the window it would focus does not exist yet.
func (b *Backend) lifecycle(kind int) {
	h := b.handler()
	if h == nil {
		return
	}
	w := b.win
	switch kind {
	case lifecyclePause:
		h.DidResignActive()
		if w != nil {
			w.blur()
			b.deliver(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
		}
	case lifecycleResume:
		h.DidBecomeActive()
		if w != nil {
			w.focus()
			b.deliver(platform.SurfaceEvent{Kind: platform.SurfaceFocus})
			b.deliver(platform.SurfaceEvent{Kind: platform.SurfaceShown})
			w.askFrame()
		}
	case lifecycleLowMemory:
		b.deliver(platform.SurfaceEvent{Kind: platform.SurfaceMemoryPressure})
	case lifecycleStop:
		h.DidEnterBackground()
	case lifecycleStart:
		h.WillEnterForeground()
	}
}

// OnTheme records the system's night mode, which Java reports once at start
// and again whenever the configuration's uiMode changes. The Activity
// declares uiMode in configChanges, so a switch to dark does not recreate
// it, and this is the only way the app hears of one.
//
// It stores on the calling thread, as OnSafeArea does, so a frame the change
// asks for reads the new value; only the notification is queued.
func (b *Backend) OnTheme(dark bool) {
	prev := b.dark.Swap(dark)
	if !b.darkKnown.Swap(true) || prev == dark {
		return
	}
	if b.source() == "system" {
		b.Post(b.themeChanged)
	}
}

// themeChanged (pump) tells the app the appearance changed.
func (b *Backend) themeChanged() {
	if h := b.handler(); h != nil {
		h.ThemeChanged()
	}
}

func (b *Backend) source() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.themeSource == "" {
		return "system"
	}
	return b.themeSource
}

// Theme answers IsDark from the night mode Java reports, and SetSource in
// Go: a plain Activity has no per-app night mode to set. The rest is the
// unsupported backend's, as iOS embeds it too.
func (b *Backend) Theme() platform.Theme { return theme{b.Backend.Theme(), b} }

type theme struct {
	platform.Theme
	b *Backend
}

func (t theme) IsDark() bool {
	switch t.b.source() {
	case "dark":
		return true
	case "light":
		return false
	}
	return t.b.dark.Load()
}

// SetSource changes what IsDark answers, and tells the app when that
// changes it. The notification is posted rather than made here, so the
// caller is not re-entered from its own call.
func (t theme) SetSource(source string) {
	before := t.IsDark()
	t.b.mu.Lock()
	t.b.themeSource = source
	t.b.mu.Unlock()
	if t.IsDark() != before {
		t.b.Post(t.b.themeChanged)
	}
}
