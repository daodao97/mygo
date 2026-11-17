// Package unsupported is the backend for platforms without a native
// implementation. Init fails, everything else is inert.
package unsupported

import (
	"errors"
	"runtime"
	"time"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

// Backend reports that the platform is not supported.
type Backend struct{}

// New returns the unsupported backend.
func New() *Backend { return &Backend{} }

var errUnsupported = errors.New("mygo: " + runtime.GOOS + "/" + runtime.GOARCH + " is not supported yet")

func (*Backend) Name() string                                        { return "unsupported" }
func (*Backend) Init(platform.AppHandler, platform.AppOptions) error { return errUnsupported }
func (*Backend) Run() error                                          { return errUnsupported }
func (*Backend) Quit()                                               {}
func (*Backend) IsMainThread() bool                                  { return true }
func (*Backend) Signal()                                             {}
func (*Backend) Step()                                               {}
func (*Backend) Wake()                                               {}
func (*Backend) NewWindow(*platform.WindowOptions, platform.WindowHandler) (platform.Window, error) {
	return nil, errUnsupported
}
func (*Backend) SetApplicationMenu(*platform.Menu)                          {}
func (*Backend) PopupMenu(*platform.Menu, platform.Window, *platform.Point) {}
func (*Backend) UpdateMenuItem(*platform.MenuItem)                          {}
func (*Backend) App() platform.AppController                                { return App{} }
func (*Backend) Dialogs() platform.Dialogs                                  { return Dialogs{} }
func (*Backend) Clipboard() platform.Clipboard                              { return Clipboard{} }
func (*Backend) Shell() platform.Shell                                      { return Shell{} }
func (*Backend) Screen() platform.Screen                                    { return Screen{} }
func (*Backend) Theme() platform.Theme                                      { return Theme{} }
func (*Backend) Power() platform.Power                                      { return Power{} }
func (*Backend) Network() platform.Network                                  { return Network{} }
func (*Backend) NowPlaying() platform.NowPlaying                            { return NowPlaying{} }
func (*Backend) Secrets() platform.Secrets                                  { return Secrets{} }
func (*Backend) NewTray(string, platform.TrayHandler) (platform.Tray, error) {
	return nil, errUnsupported
}
func (*Backend) RegisterHotkey(int, string, string) error { return errUnsupported }
func (*Backend) UnregisterHotkey(int)                     {}
func (*Backend) NotificationsSupported() bool             { return false }
func (*Backend) WebViewAvailable() bool                   { return false }
func (*Backend) RemoveNotification(string)                {}

func (*Backend) ShowNotification(_ *platform.Notification, done func(error)) {
	done(errUnsupported)
}

func (*Backend) RemoveAllNotifications() {}

// App does nothing, or reports that it cannot.
type App struct{}

func (App) SetActivationPolicy(string)                        {}
func (App) Activate()                                         {}
func (App) AllowForeground()                                  {}
func (App) WithActivationToken(_ string, fn func())           { fn() }
func (App) Hide()                                             {}
func (App) Unhide()                                           {}
func (App) IsHidden() bool                                    { return false }
func (App) IsActive() bool                                    { return false }
func (App) SetBadge(string)                                   {}
func (App) Badge() string                                     { return "" }
func (App) Bounce(bool) int                                   { return 0 }
func (App) CancelBounce(int)                                  {}
func (App) SetDockIcon([]byte) error                          { return errUnsupported }
func (App) ShowAboutPanel(platform.AboutPanelOptions)         {}
func (App) PreferredLanguages() []string                      { return []string{"en-US"} }
func (App) Region() platform.Region                           { return platform.Region{FirstWeekday: -1} }
func (App) SavedLocale() (string, bool)                       { return "", false }
func (App) SaveLocale(string) bool                            { return false }
func (App) SetLanguages([]string, bool)                       {}
func (App) Package() (platform.PackageInfo, bool)             { return platform.PackageInfo{}, false }
func (App) RegisterURLScheme(string, string, string) error    { return errUnsupported }
func (App) UnregisterURLScheme(string, string, string) error  { return errUnsupported }
func (App) IsURLSchemeRegistered(string, string, string) bool { return false }
func (App) SetOpenAtLogin(bool, string, string, string) error { return errUnsupported }
func (App) OpenAtLogin(string, string, string) bool           { return false }
func (App) OpenedAtLogin() bool                               { return false }
func (App) SetDockMenu(*platform.Menu)                        {}
func (App) ClearBrowsingData(_ platform.DataStore, _ platform.SiteData, done func(error)) {
	done(errUnsupported)
}
func (App) NewDataStore(platform.DataStoreOptions) (platform.DataStore, error) {
	return nil, errUnsupported
}
func (App) AddRecentDocument(string) error      { return errUnsupported }
func (App) ClearRecentDocuments() error         { return errUnsupported }
func (App) SetJumpList(platform.JumpList) error { return errUnsupported }

func (App) ListCookies(_ platform.DataStore, _ string, done func([]platform.Cookie, error)) {
	done(nil, platform.ErrUnsupported)
}
func (App) SetCookie(_ platform.DataStore, _ platform.Cookie, done func(error)) {
	done(platform.ErrUnsupported)
}
func (App) DeleteCookie(_ platform.DataStore, _ platform.CookieKey, done func(error)) {
	done(platform.ErrUnsupported)
}
func (App) ClearCookies(_ platform.DataStore, done func(error)) { done(platform.ErrUnsupported) }
func (App) CompileContentRules(_, _, _ string, done func(any, error)) {
	done(nil, platform.ErrUnsupported)
}

// Dialogs does nothing, or reports that it cannot.
type Dialogs struct{}

func (Dialogs) ShowOpenDialog(_ platform.Window, _ *platform.OpenDialogOptions, cb func([]string, error)) {
	cb(nil, errUnsupported)
}
func (Dialogs) ShowSaveDialog(_ platform.Window, _ *platform.SaveDialogOptions, cb func(string, error)) {
	cb("", errUnsupported)
}
func (Dialogs) ShowMessageBox(_ platform.Window, _ *platform.MessageBoxOptions, cb func(platform.MessageBoxResult, error)) {
	cb(platform.MessageBoxResult{}, errUnsupported)
}

// Clipboard does nothing, or reports that it cannot.
type Clipboard struct{}

func (Clipboard) WatchChanges(func()) (func(), error) {
	return nil, platform.ErrUnsupported
}

func (Clipboard) ReadData([]transfer.Format) (transfer.Data, error) {
	return transfer.Data{}, errUnsupported
}
func (Clipboard) WriteData(transfer.Data, bool, func()) error { return errUnsupported }
func (Clipboard) Formats() []transfer.Format                  { return nil }
func (Clipboard) Flush() error                                { return errUnsupported }
func (Clipboard) Close()                                      {}

func (Clipboard) ReadText() string           { return "" }
func (Clipboard) WriteText(string)           {}
func (Clipboard) ReadHTML() string           { return "" }
func (Clipboard) WriteHTML(string)           {}
func (Clipboard) ReadImage() []byte          { return nil }
func (Clipboard) WriteImage([]byte) error    { return errUnsupported }
func (Clipboard) Clear()                     {}
func (Clipboard) AvailableFormats() []string { return nil }

// Shell does nothing, or reports that it cannot.
type Shell struct{}

func (Shell) OpenExternal(string) error              { return errUnsupported }
func (Shell) OpenPath(string) error                  { return errUnsupported }
func (Shell) ShowItemInFolder(string)                {}
func (Shell) TrashItem(string) error                 { return errUnsupported }
func (Shell) TrashItemResult(string) (string, error) { return "", platform.ErrUnsupported }
func (Shell) PreviewItem(string) error               { return errUnsupported }
func (Shell) Beep()                                  {}
func (Shell) FileIcon(string, int) ([]byte, error)   { return nil, errUnsupported }
func (Shell) TypeIcon(string, int) ([]byte, error)   { return nil, errUnsupported }
func (Shell) OpenApplication(string) error           { return errUnsupported }
func (Shell) DefaultApp(string) (platform.Application, error) {
	return platform.Application{}, errUnsupported
}

func (*Backend) Applications() ([]platform.Application, error) { return nil, errUnsupported }

// Screen does nothing, or reports that it cannot.
type Screen struct{}

func (Screen) Displays() []platform.Display { return nil }
func (Screen) CursorPoint() platform.Point  { return platform.Point{} }

// Theme does nothing, or reports that it cannot.
type Theme struct{}

func (Theme) IsDark() bool     { return false }
func (Theme) SetSource(string) {}
func (Theme) UIFont() string   { return "" }

func (Theme) FontRendering() platform.FontRendering { return platform.FontRendering{} }

func (Theme) Preferences() platform.Preferences { return platform.Preferences{} }

// Power does nothing, or reports that it cannot.
type Power struct{}

func (Power) Watch()                        {}
func (Power) KeepAwake(bool, string) func() { return func() {} }
func (Power) KeepRunning(string) func()     { return func() {} }
func (Power) OnBattery() bool               { return false }
func (Power) IdleTime() time.Duration       { return 0 }

// NowPlaying does nothing, or reports that it cannot.
type NowPlaying struct{}

func (NowPlaying) Set(*platform.NowPlayingInfo) {}

// Network does nothing, or reports that it cannot.
type Network struct{}

func (Network) Watch()                         {}
func (Network) Status() platform.NetworkStatus { return platform.NetworkStatus{} }

// Secrets does nothing, or reports that it cannot.
type Secrets struct{}

func (Secrets) SetSecret(string, string, []byte) error { return platform.ErrUnsupported }
func (Secrets) Secret(string, string) ([]byte, error)  { return nil, platform.ErrUnsupported }
func (Secrets) DeleteSecret(string, string) error      { return platform.ErrUnsupported }
