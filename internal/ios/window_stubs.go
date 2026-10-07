//go:build ios && cgo

package ios

import "github.com/egoist/mygo/internal/platform"

// Desktop window controls are inert on the single UIKit content window.
func (w *window) WebViewHandle() uintptr                     { return 0 }
func (w *window) SetBounds(r platform.Rect)                  {}
func (w *window) SetContentBounds(r platform.Rect)           {}
func (w *window) SetMinimumSize(s platform.Size)             {}
func (w *window) SetMaximumSize(s platform.Size)             {}
func (w *window) SetResizable(v bool)                        {}
func (w *window) IsResizable() bool                          { return false }
func (w *window) SetMovable(v bool)                          {}
func (w *window) IsMovable() bool                            { return false }
func (w *window) SetMinimizable(v bool)                      {}
func (w *window) IsMinimizable() bool                        { return false }
func (w *window) SetMaximizable(v bool)                      {}
func (w *window) IsMaximizable() bool                        { return false }
func (w *window) SetClosable(v bool)                         {}
func (w *window) IsClosable() bool                           { return false }
func (w *window) SetAlwaysOnTop(v bool)                      {}
func (w *window) IsAlwaysOnTop() bool                        { return false }
func (w *window) Minimize()                                  {}
func (w *window) IsMinimized() bool                          { return false }
func (w *window) Maximize()                                  {}
func (w *window) Unmaximize()                                {}
func (w *window) IsMaximized() bool                          { return false }
func (w *window) Restore()                                   {}
func (w *window) SetFullScreen(v bool)                       {}
func (w *window) Center()                                    {}
func (w *window) SetOpacity(v float64)                       {}
func (w *window) Opacity() float64                           { return 1 }
func (w *window) SetHasShadow(v bool)                        {}
func (w *window) HasShadow() bool                            { return false }
func (w *window) SetIgnoreMouseEvents(v bool)                {}
func (w *window) SetContentProtection(v bool)                {}
func (w *window) SetVibrancy(material string)                {}
func (w *window) SetProgressBar(state string, value float64) {}
func (w *window) FlashFrame(flash bool)                      {}
func (w *window) SetSkipTaskbar(v bool)                      {}
func (w *window) SetVisibleOnAllWorkspaces(v bool)           {}
func (w *window) SetIcon(png []byte) error                   { return platform.ErrUnsupported }
func (w *window) SetMenu(m *platform.Menu)                   {}
func (w *window) SetAutoHideMenu(v bool)                     {}
func (w *window) TitleBar() platform.TitleBar                { return platform.TitleBar{} }
func (w *window) StartDrag()                                 {}
func (w *window) TitleBarDoubleClicked()                     {}
func (w *window) LoadURL(url string)                         {}
func (w *window) LoadHTML(html, baseURL string)              {}
func (w *window) LoadFile(path, readAccessDir string)        {}
func (w *window) Reload(ignoreCache bool)                    {}
func (w *window) StopLoading()                               {}
func (w *window) GoBack()                                    {}
func (w *window) GoForward()                                 {}
func (w *window) CanGoBack() bool                            { return false }
func (w *window) CanGoForward() bool                         { return false }
func (w *window) URL() string                                { return "" }
func (w *window) IsLoading() bool                            { return false }
func (w *window) Eval(js string)                             {}
func (w *window) CallAsyncFunction(body string, cb func(result string, err error)) {
	cb("", platform.ErrUnsupported)
}
func (w *window) SetZoom(factor float64)                     {}
func (w *window) Zoom() float64                              { return 1 }
func (w *window) SetUserAgent(ua string)                     {}
func (w *window) UserAgent() string                          { return "" }
func (w *window) OpenDevTools()                              {}
func (w *window) CloseDevTools()                             {}
func (w *window) IsDevToolsOpened() bool                     { return false }
func (w *window) CapturePage(cb func(png []byte, err error)) { cb(nil, platform.ErrUnsupported) }
func (w *window) PrintToPDF(opts platform.PDFOptions, cb func(pdf []byte, err error)) {
	cb(nil, platform.ErrUnsupported)
}
func (w *window) DroppedFiles() []string { return nil }
func (w *window) Print()                 {}
