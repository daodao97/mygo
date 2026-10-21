//go:build android && cgo

package backend

// This file stubs the Window and Page methods Android has no window manager for: a phone
// has one surface, the Activity's, and no way to move, resize, stack or full-screen a
// window of the app's own. The shape is internal/browser/window_gen.go's: the methods with
// meaning here are in window.go, and the rest answer what a window that cannot do them
// should.
//
// Generated from internal/platform/platform.go's Window and Page interfaces. A method added
// to either interface becomes a compile error here rather than a silent hole.

import "github.com/egoist/mygo/internal/platform"

func (w *window) SetMinimumSize(s platform.Size) {}

func (w *window) SetMaximumSize(s platform.Size) {}

func (w *window) SetResizable(v bool) {}

func (w *window) IsResizable() bool { return false }

func (w *window) SetMovable(v bool) {}

func (w *window) IsMovable() bool { return false }

func (w *window) SetMinimizable(v bool) {}

func (w *window) IsMinimizable() bool { return false }

func (w *window) SetMaximizable(v bool) {}

func (w *window) IsMaximizable() bool { return false }

func (w *window) SetClosable(v bool) {}

func (w *window) IsClosable() bool { return false }

func (w *window) SetAlwaysOnTop(v bool) {}

func (w *window) IsAlwaysOnTop() bool { return false }

func (w *window) Minimize() {}

func (w *window) IsMinimized() bool { return false }

func (w *window) Maximize() {}

func (w *window) Unmaximize() {}

func (w *window) IsMaximized() bool { return false }

func (w *window) Restore() {}

func (w *window) SetFullScreen(v bool) {}

func (w *window) IsFullScreen() bool { return false }

func (w *window) Center() {}

func (w *window) SetBackgroundColor(c platform.Color) {}

func (w *window) SetOpacity(v float64) {}

func (w *window) Opacity() float64 { return 0 }

func (w *window) SetHasShadow(v bool) {}

func (w *window) HasShadow() bool { return false }

func (w *window) SetIgnoreMouseEvents(v bool) {}

func (w *window) SetContentProtection(v bool) {}

func (w *window) SetVibrancy(material string) {}

func (w *window) SetProgressBar(state string, value float64) {}

func (w *window) FlashFrame(flash bool) {}

func (w *window) SetSkipTaskbar(v bool) {}

func (w *window) SetVisibleOnAllWorkspaces(v bool) {}

func (w *window) SetIcon(png []byte) error { return nil }

func (w *window) SetMenu(m *platform.Menu) {}

func (w *window) SetAutoHideMenu(v bool) {}

func (w *window) TitleBar() platform.TitleBar { return platform.TitleBar{} }

// SafeArea is the room the device's chrome takes, which the Java host
// reports through Native.onSafeArea (see jni.go). It is the phone's
// counterpart to a desktop's TitleBar, and it is read from the backend
// rather than computed here because only Android knows: a status bar's
// height, a notch's shape, a gesture bar and a camera cutout are all the
// system's, and they differ per device.
func (w *window) SafeArea() platform.SafeArea {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.b.safeArea
}

func (w *window) StartDrag() {}

func (w *window) TitleBarDoubleClicked() {}

func (w *window) PlaceFlyout(f platform.Flyout, size platform.Size) {}

func (w *window) WebViewHandle() uintptr { return 0 }

func (w *window) LoadURL(url string) {}

func (w *window) LoadHTML(html, baseURL string) {}

func (w *window) LoadFile(path, readAccessDir string) {}

func (w *window) Reload(ignoreCache bool) {}

func (w *window) StopLoading() {}

func (w *window) GoBack() {}

func (w *window) GoForward() {}

func (w *window) CanGoBack() bool { return false }

func (w *window) CanGoForward() bool { return false }

func (w *window) URL() string { return "" }

func (w *window) IsLoading() bool { return false }

func (w *window) Eval(js string) {}

func (w *window) SetUserScripts(scripts []platform.UserScript) {}

func (w *window) SetZoom(factor float64) {}

func (w *window) Zoom() float64 { return 0 }

func (w *window) SetUserAgent(ua string) {}

func (w *window) UserAgent() string { return "" }

func (w *window) OpenDevTools() {}

func (w *window) CloseDevTools() {}

func (w *window) IsDevToolsOpened() bool { return false }

func (w *window) DroppedFiles() []string { return nil }

func (w *window) Print() {}

// The callbacks answer at once, on the calling thread: there is no page here to run the
// body in and none to print, and answering late would only make the caller wait for
// something that is never coming.

func (w *window) CallAsyncFunction(body string, cb func(result string, err error)) { cb("", nil) }

func (w *window) CapturePage(cb func(png []byte, err error)) { cb(nil, nil) }

func (w *window) PrintToPDF(opts platform.PDFOptions, cb func(pdf []byte, err error)) { cb(nil, nil) }
