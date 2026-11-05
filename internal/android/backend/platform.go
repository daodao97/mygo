// Platform features of the Android backend: the clipboard, opening URLs,
// notifications and the accessibility tree.
//
// # Why this file has no build constraint
//
// Every other file of this package is android-only (app.go, egl.go,
// surface.go), and this one is deliberately not: it holds the parts that are
// platform *logic* rather than platform *calls* — mapping transfer formats to
// Android's MIME types, checking a URL before it becomes an Intent, turning an
// AccessTree into the JSON the Java side reads, mapping a failed notification
// to platform.ErrNotificationsDenied — and those can be tested on the desktop
// CI with CGO_ENABLED=0, which is the only place a test of this package can
// run at all. The Android-only halves (the //export functions and the JNI
// glue) live in internal/android (package main) because //export belongs to
// the library's main package.
//
// What is *not* here, on purpose: the methods that make the package's Backend
// use these implementations (Backend.Clipboard, Backend.Shell,
// Backend.ShowNotification, ...). They can only be declared where the Backend
// type exists — i.e. under GOOS=android — so they live in platform_android.go,
// which this file's sibling file would be if the android backend could keep
// the wiring next to the logic. Until that file exists the embedded
// internal/unsupported backend answers those calls and Android gets its stubs
// (see the "Structurally absent on Android" note at the bottom).
//
// # Who calls Java, and on which thread
//
// Android's clipboard, notification and accessibility APIs are Java objects in
// the Activity's process, so the Go side reaches them through the
// PlatformBridge that internal/android installs at startup (setPlatformBridge).
// The JNI glue behind it attaches the calling Go thread to the JavaVM on first
// use, so a bridge call may come from any goroutine; the Java side is what
// hops to the Android UI thread where an API needs it (announcements and
// accessibility events do). The platform contract already says the caller is
// the main thread, and on Android "main thread" is the backend's pump.
package backend

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

// ErrPlatformBridge is what the features here report when the Java side never
// attached: the library is running (this is a Go call), but nothing can answer
// it — the Java host did not call MygoPlatform.install before using them. It
// is a distinct error rather than platform.ErrUnsupported because the platform
// does support the feature; the app just did not wire it up.
var ErrPlatformBridge = errors.New("mygo: the Android platform bridge is not attached")

// ErrScanUnavailable is the Java side saying it cannot open the picker at all:
// no activity to start it, or the system refused. It is not the person
// cancelling, which is what ScanCode's callback reports for that.
var ErrScanUnavailable = errors.New("mygo: Android could not open the image picker")

// PlatformBridge is the Java half of the platform features: ClipboardManager,
// the ACTION_VIEW intent, NotificationManager and the accessibility provider,
// as internal/android's JNI implementation of it exposes them.
//
// Every method reports failure rather than doing nothing, so that the Go side
// can map it: a clipboard the app may not read (see the Android 10 note on
// androidClipboard.ReadText), a notification the user disallowed, a URL no
// activity takes. The implementations must not panic: a panic raised inside a
// call that started on a Java thread would unwind through JNI.
type PlatformBridge interface {
	// ClipboardRead returns what the clipboard holds and the types the clip
	// advertises. An empty payload with no types is an empty clipboard — or
	// the read the platform refused; Android gives no way to tell the two
	// apart (see androidClipboard.ReadText).
	ClipboardRead() (p ClipboardPayload, mimes []string, err error)

	// ClipboardWrite replaces the clipboard with one of what payload holds.
	// Both representations belong to one clip, the way ClipData carries
	// text/plain and text/html of the same item.
	ClipboardWrite(p ClipboardPayload) error

	// ClipboardClear empties the clipboard.
	ClipboardClear() error

	// OpenExternalURL starts the activity that handles url. The Java side
	// validates the scheme again before it becomes an Intent: the Go side
	// checks it first (validateExternalURL), but the two checks exist for
	// different reasons — the Go one is the contract, the Java one is what
	// the process that builds the Intent must never skip.
	OpenExternalURL(url string) error

	// NotificationsAllowed reports whether the user currently allows the app
	// to post notifications. On Android 13 (API 33) and later this is the
	// POST_NOTIFICATIONS runtime permission and is false until the user
	// grants it; earlier it is the app's notification settings.
	NotificationsAllowed() bool

	// PostNotification posts n and reports why it could not. It returns
	// platform.ErrNotificationsDenied when the permission is what is
	// missing, so the caller can tell "the user said no" from "this failed".
	PostNotification(n *platform.Notification) error

	// CancelNotification removes the notification shown with n.ID, and
	// CancelAllNotifications the ones this app posted.
	CancelNotification(id string) error
	CancelAllNotifications() error

	// AnnounceAs puts text through the platform's accessibility
	// announcement: TalkBack reads it after what it is already reading.
	AnnounceAs(text string) error

	// SetAccessibilityTreeJSON gives the Java side the serialized
	// AccessTree (encodeAccessTree) its AccessibilityNodeProvider serves.
	SetAccessibilityTreeJSON(json string) error

	// SecretSet, SecretGet and SecretDelete are the secure store, backed by
	// the Android Keystore on the Java side. accessibility is the value's
	// lifetime in the terms platform.Secrets takes it in; the Java side
	// maps it to what the Keystore offers, which is less than iOS's Keychain.
	//
	// Get reports a missing secret as platform.ErrSecretNotFound and any
	// other failure as-is: a Keystore that cannot unwrap (the key was
	// invalidated by a lock-screen change, say) is not "no value".
	SecretSet(namespace, key string, value []byte, accessibility string) error
	SecretGet(namespace, key string) ([]byte, error)
	SecretDelete(namespace, key string) error

	// ScanCode opens the system's image picker for a picture of a QR code and
	// reports what it holds, later, through ScanResult with the same token.
	// The prompt is the picker's title where the platform takes one.
	//
	// It returns as soon as the picker is up: the answer arrives as a JNI
	// entry point, on the pump, like every other platform callback.
	ScanCode(token uint64, prompt string) error
	// ScanCancel dismisses the picker opened for token, if it is still up.
	ScanCancel(token uint64) error
}

// ClipboardPayload is what Android's clipboard holds of what this backend can
// decode: the plain text of one item and its HTML markup, each with a flag
// saying whether it is there, because an empty string is a value a user can
// copy and "there is no text" is not the same thing.
type ClipboardPayload struct {
	Plain, HTML       string
	HasPlain, HasHTML bool
}

// empty reports whether the payload holds nothing at all.
func (p ClipboardPayload) empty() bool { return !p.HasPlain && !p.HasHTML }

// bridgeMu guards the installed bridge. The bridge is installed once, at
// startup, before the app can call anything (internal/android's init), and read
// from the pump; the mutex rather than a plain variable is what makes a call
// that arrives between those two points well defined instead of a data race.
var (
	bridgeMu sync.RWMutex
	bridge   PlatformBridge
)

// SetPlatformBridge installs the Java half. internal/android calls it once, in
// init, so that the platform features answer even before Java attaches: the
// bridge then reports ErrPlatformBridge, which says what is wrong, instead of
// the package falling back to the unsupported backend's silence.
func SetPlatformBridge(b PlatformBridge) {
	bridgeMu.Lock()
	bridge = b
	bridgeMu.Unlock()
}

// platformBridge returns the installed bridge, or nil when none is.
func platformBridge() PlatformBridge {
	bridgeMu.RLock()
	defer bridgeMu.RUnlock()
	return bridge
}

// ---------------------------------------------------------------------------
// Clipboard
// ---------------------------------------------------------------------------

// androidClipboard is platform.Clipboard over Android's ClipboardManager.
//
// # What Android carries, and what it does not
//
// A clip holds one item with, at most, a plain-text representation, an HTML
// representation, a URI and an Intent; the representations of a clip that came
// from an image are a content:// URI, which is not the image's bytes. So this
// clipboard reads and writes text and HTML and reports everything else as
// unavailable (transfer.ErrFormat for reads, platform.ErrUnsupported for
// writes), rather than pretending a URI is a PNG.
//
// # The Android 10 background-read restriction is a platform rule, not a bug
//
// From API 29 the system only lets the app that currently has the focus read
// the clipboard; an app in the background gets a null ClipData, with this in
// logcat:
//
//	W/ClipboardService: Denying clipboard access to <pkg>, application is not
//	in focus nor is it a system service for user 0
//
// The backend cannot lift that: reading is answered from Java's
// ClipboardManager, and the platform refuses it there. What makes it worse is
// that the refusal is *indistinguishable from an empty clipboard*: there is no
// API that says "the user's clipboard is empty" as opposed to "you may not
// look", and hasPrimaryClip() (API 28) answers false for both. So ReadText and
// ReadHTML return "" in both cases, and a caller that needs the clipboard of
// another app reads it while its own window has the focus — which is the only
// answer the platform offers. This is a constraint of the platform, not
// something this backend failed to implement.
type androidClipboard struct{}

// Formats lists the clipboard's representations as transfer formats: what the
// clip advertises, kept to the ones this backend can decode.
//
// A clip that carries only, say, an image advertises image/* and is reported as
// holding nothing this backend can read — the list is empty rather than
// wrong — which is also what a background app sees when the platform refuses
// the read (see the type's note).
func (androidClipboard) Formats() []transfer.Format {
	_, mimes, err := readClipboard()
	if err != nil {
		return nil
	}
	return readableFormats(mimes)
}

// AvailableFormats lists the clipboard's representations as the platform names
// them: the clip's MIME types, exactly as ClipDescription reports them.
func (androidClipboard) AvailableFormats() []string {
	_, mimes, err := readClipboard()
	if err != nil {
		return nil
	}
	return mimes
}

// ReadData reads what the clipboard holds of formats. Nil reads all, and a
// format the clipboard cannot carry is transfer.ErrFormat, so a caller that
// asked for an image learns it is unavailable instead of getting an empty
// image.
func (androidClipboard) ReadData(formats []transfer.Format) (transfer.Data, error) {
	p, mimes, err := readClipboard()
	if err != nil {
		return transfer.Data{}, err
	}
	offered := readableFormats(mimes)
	if len(formats) == 0 {
		formats = offered
	}
	var reps []transfer.Representation
	for _, f := range formats {
		if !contains(offered, f) {
			continue
		}
		switch f {
		case transfer.Text:
			reps = append(reps, transfer.Bytes(transfer.Text, []byte(p.Plain)))
		case transfer.HTML:
			reps = append(reps, transfer.Bytes(transfer.HTML, []byte(p.HTML)))
		}
	}
	if len(reps) == 0 {
		if len(mimes) == 0 {
			// Nothing is on the clipboard: empty data, not a missing
			// representation, which is what the desktop backends report
			// for a clipboard no one owns.
			return transfer.Data{}, nil
		}
		return transfer.Data{}, transfer.ErrFormat
	}
	return transfer.New(transfer.NewItem(reps...)), nil
}

// WriteData puts data on the clipboard.
//
// The clipboard holds one item, so this writes the first item that offers text
// or HTML and drops the rest — including a second item's representations, or
// an image's bytes, which this backend cannot put on the clipboard at all (it
// reports ErrUnsupported instead of writing a clip the reader cannot decode).
//
// The representations are copied now, on this call, so the providers are
// released immediately: Android's clipboard keeps its own copy in the system
// process, and nothing here reads data later the way a lazy desktop provider
// is read when a paste asks for it.
func (androidClipboard) WriteData(data transfer.Data, released func()) error {
	p, ok := payloadFromData(data)
	if !ok && len(data.Formats()) > 0 {
		return platform.ErrUnsupported
	}
	if err := writeClipboard(p); err != nil {
		// A failed write must not release the providers (Clipboard's
		// contract): the caller keeps data usable and can try again.
		return err
	}
	if released != nil {
		released()
	}
	return nil
}

// Flush does nothing and reports success: Android's clipboard is owned by the
// system process for as long as the app is the owner, so there is nothing to
// materialize or to ask the platform to persist — the persistence Linux needs a
// clipboard manager for is not a thing here.
func (androidClipboard) Flush() error { return nil }

// Close does nothing: this backend keeps no providers of its own to drop
// (WriteData copies what it is given and releases the caller's providers at
// once).
func (androidClipboard) Close() {}

// ReadText returns the clipboard's plain text, "" when it holds none — or when
// the platform refused the read because the app is in the background (see the
// type's note): the interface has no way to say which.
func (androidClipboard) ReadText() string {
	p, _, err := readClipboard()
	if err != nil {
		return ""
	}
	return p.Plain
}

// WriteText puts text on the clipboard, without an HTML representation.
func (c androidClipboard) WriteText(text string) {
	_ = c.WriteData(transfer.TextData(text), nil)
}

// ReadHTML returns the clipboard's HTML markup, "" when it holds none.
func (androidClipboard) ReadHTML() string {
	p, _, err := readClipboard()
	if err != nil {
		return ""
	}
	return p.HTML
}

// WriteHTML puts markup on the clipboard, without plain text: Android keeps
// the two representations of one clip together and this one has no plain text
// to give, so a paste into a field that takes only text gets nothing — which is
// what the platform does with an HTML-only clip anyway.
func (androidClipboard) WriteHTML(markup string) {
	_ = androidClipboard{}.WriteData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(markup)))), nil)
}

// ReadImage returns nil: what an image on the clipboard is here is a
// content:// URI, and the backend does not resolve it. Reading it would need
// the Activity's ContentResolver, a decode, and a size limit, which is the
// same work as reading any other URI this app does not own.
func (androidClipboard) ReadImage() []byte { return nil }

// WriteImage reports that the clipboard cannot take an image: putting one there
// needs a FileProvider content:// URI the app can serve, which is the app's
// to arrange, not the backend's.
func (androidClipboard) WriteImage([]byte) error { return platform.ErrUnsupported }

// Clear empties the clipboard. The clipboard is the system's, so this clears
// it for everyone, not just for this app.
func (androidClipboard) Clear() { _ = clearClipboard() }

// readClipboard reads the clipboard through the bridge.
func readClipboard() (ClipboardPayload, []string, error) {
	b := platformBridge()
	if b == nil {
		return ClipboardPayload{}, nil, ErrPlatformBridge
	}
	return b.ClipboardRead()
}

// writeClipboard writes through the bridge.
func writeClipboard(p ClipboardPayload) error {
	b := platformBridge()
	if b == nil {
		return ErrPlatformBridge
	}
	return b.ClipboardWrite(p)
}

// clearClipboard clears through the bridge.
func clearClipboard() error {
	b := platformBridge()
	if b == nil {
		return ErrPlatformBridge
	}
	return b.ClipboardClear()
}

// payloadFromData picks what Android's clipboard can hold out of data: the
// first item that offers text or HTML, with its sibling representation when it
// has one. ok is false when no item offers either, which the caller reports as
// unsupported (unless data is empty, which means "clear it").
func payloadFromData(data transfer.Data) (p ClipboardPayload, ok bool) {
	for _, item := range data.Items() {
		formats := item.Formats()
		hasText, hasHTML := contains(formats, transfer.Text), contains(formats, transfer.HTML)
		if !hasText && !hasHTML {
			continue
		}
		if hasText {
			if b, err := item.Read(transfer.Text); err == nil {
				p.Plain, p.HasPlain = string(b), true
			}
		}
		if hasHTML {
			if b, err := item.Read(transfer.HTML); err == nil {
				p.HTML, p.HasHTML = string(b), true
			}
		}
		return p, !p.empty()
	}
	return ClipboardPayload{}, false
}

// readableFormats maps the MIME types a clip advertises to the transfer formats
// this backend can decode, dropping what it cannot — see androidMIME for the
// two it can.
func readableFormats(mimes []string) []transfer.Format {
	var out []transfer.Format
	for _, mime := range mimes {
		f, ok := formatForMIME(mime)
		if !ok || contains(out, f) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// androidMIME maps a transfer format to the MIME type Android's clipboard names
// it by. The two representations a clip can hold that this backend decodes are
// text/plain and text/html; everything else is not available (see
// androidClipboard).
func androidMIME(f transfer.Format) (string, bool) {
	switch f {
	case transfer.Text:
		return "text/plain", true
	case transfer.HTML:
		return "text/html", true
	}
	return "", false
}

// formatForMIME is androidMIME the other way, for the types a clip advertises.
// The comparison is case-insensitive and ignores parameters ("text/plain;
// charset=utf-8" is text/plain): a clip is free to advertise either, and a
// caller that asked for text should not be told there is none because the
// other app wrote its MIME type in capitals.
func formatForMIME(mime string) (transfer.Format, bool) {
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "text/plain":
		return transfer.Text, true
	case "text/html":
		return transfer.HTML, true
	}
	return "", false
}

// contains reports whether formats holds f.
func contains(formats []transfer.Format, f transfer.Format) bool {
	for _, have := range formats {
		if have == f {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Shell
// ---------------------------------------------------------------------------

// androidShell is platform.Shell over Android's intents.
//
// Only one of the five methods has anything to translate: Android has no
// desktop shell to open a path with, no file manager to reveal one in, no trash
// and no system beep for an app to ask for, so those report unsupported (or,
// where the contract has no error to report, do nothing). The desktop-specific
// mistake would be to route OpenPath through a file:// ACTION_VIEW intent,
// which FileUriExposedException forbids from API 24 on for exactly the reason
// the check exists: one app's filesystem is not another app's.
type androidShell struct{}

// externalSchemes are the only URL schemes OpenExternal hands to the system.
//
// This is an allow list, and that is the point: an ACTION_VIEW intent built
// from a caller's string is an IPC message to another app, and the schemes that
// are dangerous here are the ones that do not name a web page but a component:
// file:// (the receiving app's private files), content:// (its providers),
// intent:// and android-app:// (explicit dispatch to a component the URL names),
// javascript:/data:/blob: (which a WebView will execute). Dropping them is what
// makes an untrusted string safe to pass on; see validateExternalURL.
var externalSchemes = []string{"http", "https", "mailto", "tel"}

// OpenExternal hands url to the app that handles it, an http/https URL to a
// browser, mailto to a mail client, tel to the dialer.
//
// # The security boundary
//
// The argument is untrusted (it can come from a page), so it never reaches an
// Intent as it is given:
//
//   - the scheme is checked against externalSchemes (validateExternalURL), so
//     an explicit-component or file-access scheme is refused here, before
//     anything leaves the process;
//   - the Java side checks the same allow list again before it builds the
//     intent (MygoPlatform.openExternalUrl), because a check that only exists
//     on the Go side is one refactor away from being skipped;
//   - the intent is left implicit: no component and no setPackage, so the
//     system picks the handler and the URL never chooses a recipient; a
//     setPackage("com.android.chrome") sprinkled in "to be safe" would turn a
//     URL into a way to launch a specific app, and is not done here;
//   - nothing is added to it (no EXTRA_*, no clip data), so there is no second
//     string for the receiving app to read that was not checked;
//   - a value with a control character or a whitespace-only scheme is refused
//     rather than trimmed, because trimming is how " java\nscript:" becomes
//     something else downstream.
func (androidShell) OpenExternal(url string) error {
	valid, err := validateExternalURL(url)
	if err != nil {
		return err
	}
	b := platformBridge()
	if b == nil {
		return ErrPlatformBridge
	}
	return b.OpenExternalURL(valid)
}

// OpenPath reports that Android has no shell: what a path means to this app is
// not what it means to another one, and the platform's way to hand a file to
// another app is a FileProvider URI built by the app that owns the file.
func (androidShell) OpenPath(string) error { return platform.ErrUnsupported }

// ShowItemInFolder does nothing: Android has no file manager with a "reveal in
// folder" request, and the contract gives it no error to report.
func (androidShell) ShowItemInFolder(string) {}

// TrashItem reports unsupported: Android's trash is a user-visible feature of
// the file manager, not an API an app can move a file into. Deleting content
// the app itself owns belongs to that app (MediaStore, Storage Access
// Framework), not to a shell call.
func (androidShell) TrashItem(string) error { return platform.ErrUnsupported }

// Beep does nothing: the platform has no "the application beeps" call — sound
// belongs to a notification or to the app's own audio, which is a decision the
// backend should not make for it.
func (androidShell) Beep() {}

// FileIcon reports unsupported: the icon a file manager draws is a desktop
// notion (Finder/Explorer ask the OS for a registered icon), while on Android
// an app that wants a file's icon decodes the bitmap it already has or loads
// one from its own resources. There is no path-keyed icon lookup to forward.
func (androidShell) FileIcon(string, int) ([]byte, error) { return nil, platform.ErrUnsupported }

// OpenApplication reports unsupported: "the application at a path" is a desktop
// notion too (a .app bundle or an .exe). Android starts apps by package name
// through an explicit intent, which is a different call with a different
// argument, so there is nothing to translate path into.
func (androidShell) OpenApplication(string) error { return platform.ErrUnsupported }

// validateExternalURL checks url against externalSchemes and returns it
// unchanged.
//
// The URL is returned as it was given, not re-encoded: the string that was
// checked is the string that becomes the intent's data, so what the Java side
// sees cannot differ from what was validated here. Re-serializing through
// url.URL is how "%2F" turns into "/" and a validated path turns into a
// different one.
func validateExternalURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("mygo: empty URL")
	}
	if strings.ContainsAny(raw, "\x00\r\n\t") {
		return "", errors.New("mygo: URL contains a control character")
	}
	if strings.TrimSpace(raw) != raw {
		return "", errors.New("mygo: URL has leading or trailing whitespace")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("mygo: invalid URL: " + err.Error())
	}
	scheme := strings.ToLower(u.Scheme)
	if !containsString(externalSchemes, scheme) {
		return "", errors.New("mygo: refusing to open a " + describeScheme(scheme) + " URL")
	}
	switch scheme {
	case "http", "https":
		// "https:" or "https:///" is not a web address: without a host the
		// receiving app has nothing to load, and a URL that a browser would
		// read as relative is what a redirect to "http:foo" exploits.
		if u.Host == "" {
			return "", errors.New("mygo: " + scheme + " URL has no host")
		}
	case "mailto", "tel":
		// These have no authority: "mailto:" with nothing after it is not a
		// recipient. (u.Opaque is what url.Parse leaves for them; a
		// "mailto://" form parses with a Host instead.)
		if u.Opaque == "" && u.Host == "" {
			return "", errors.New("mygo: " + scheme + " URL has no address")
		}
	}
	return raw, nil
}

// describeScheme names an empty scheme for the error it appears in.
func describeScheme(scheme string) string {
	if scheme == "" {
		return "scheme-less"
	}
	return scheme
}

// containsString reports whether list holds s.
func containsString(list []string, s string) bool {
	for _, have := range list {
		if have == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

// androidNotificationsSupported reports whether this process can post a
// notification at all: the platform can, so this is true whenever the Java side
// attached. A false NotificationsAllowed is a different answer — the platform
// could, the user does not allow it — and stays visible through
// ShowNotification, which reports platform.ErrNotificationsDenied rather than
// failing silently (see androidShowNotification).
func androidNotificationsSupported() bool { return platformBridge() != nil }

// androidShowNotification posts n and reports the outcome to done.
//
// done is called exactly once, with nil when the system has the notification
// and with an error when it does not:
//
//   - nothing attached (ErrPlatformBridge),
//   - the user does not allow notifications (platform.ErrNotificationsDenied),
//     checked before the post so that the answer is the user's, not a
//     NotificationManager that quietly accepts and drops;
//   - the post failed for another reason (the Java side's message).
//
// This is the branch the platform makes easy to get wrong: from Android 13 a
// post without POST_NOTIFICATIONS is *accepted and dropped*, so an app that
// only checks the return of notify() reports success and shows nothing.
func androidShowNotification(n *platform.Notification, done func(error)) {
	b := platformBridge()
	if b == nil {
		done(ErrPlatformBridge)
		return
	}
	if !b.NotificationsAllowed() {
		done(platform.ErrNotificationsDenied)
		return
	}
	done(b.PostNotification(n))
}

// androidRemoveNotification removes the notification shown with id.
func androidRemoveNotification(id string) {
	if b := platformBridge(); b != nil {
		_ = b.CancelNotification(id)
	}
}

// androidRemoveAllNotifications removes the app's notifications. Android scopes
// notifications to the package, so this is the app's own and no earlier run's:
// the notifications of a previous process are the same app's and go too.
func androidRemoveAllNotifications() {
	if b := platformBridge(); b != nil {
		_ = b.CancelAllNotifications()
	}
}

// notificationClickSink receives the identifier of a notification the user
// tapped, to deliver it as AppHandler.NotificationClicked.
//
// It is installed by the Android wiring rather than here for the same reason
// the Clipboard method is: reaching the handler needs the Backend, which only
// exists under GOOS=android. Until it is installed, the tap is reported as
// undelivered (DispatchNotificationClicked returns false) instead of being
// dropped in silence.
var (
	clickMu   sync.Mutex
	clickSink func(id string)
)

// SetNotificationClickSink installs the delivery of notification taps.
func SetNotificationClickSink(fn func(id string)) {
	clickMu.Lock()
	clickSink = fn
	clickMu.Unlock()
}

// DispatchNotificationClicked hands the identifier of a tapped notification to
// the app, as AppHandler.NotificationClicked does on the desktop backends. It
// reports whether something took it; internal/android's JNI entry logs the ones
// that were not, which is what makes a missing wiring visible in logcat instead
// of looking like a notification that never arrived.
func DispatchNotificationClicked(id string) bool {
	clickMu.Lock()
	fn := clickSink
	clickMu.Unlock()
	if fn == nil {
		return false
	}
	fn(id)
	return true
}

// ---------------------------------------------------------------------------
// Accessibility
// ---------------------------------------------------------------------------

// updateAccessibilityTree serializes tree and hands it to the Java side, whose
// AccessibilityNodeProvider serves it to TalkBack.
func updateAccessibilityTree(tree *platform.AccessTree) error {
	b := platformBridge()
	if b == nil {
		return ErrPlatformBridge
	}
	encoded, err := encodeAccessTree(tree)
	if err != nil {
		return err
	}
	return b.SetAccessibilityTreeJSON(encoded)
}

// announceAs puts text through the accessibility announcement.
func announceAs(text string) error {
	b := platformBridge()
	if b == nil {
		return ErrPlatformBridge
	}
	if text == "" {
		return nil
	}
	return b.AnnounceAs(text)
}

// AccessActionSink receives the actions assistive technology performs on the
// elements of the tree: pressing a button, moving to a node, setting a value.
// It is the Android counterpart of the SurfaceEvent{Kind: AccessAction} the
// desktop backends deliver to the content.
type AccessActionSink interface {
	AccessAction(id uint64, action platform.AccessActionKind)
}

var (
	accessMu   sync.Mutex
	accessSink AccessActionSink
)

// SetAccessActionSink installs the receiver of accessibility actions.
//
// Nothing installs one yet: the consumer is the content of a surface, which on
// Android is not wired to a WindowHandler, so the tree reaches TalkBack but an
// action on a node stops here. DispatchAccessAction says so by returning false,
// which the JNI entry logs — the honest state of this feature, rather than a
// node that TalkBack can focus and press with no effect.
func SetAccessActionSink(s AccessActionSink) {
	accessMu.Lock()
	accessSink = s
	accessMu.Unlock()
}

// DispatchAccessAction hands an action on an element to the installed sink and
// reports whether one took it.
func DispatchAccessAction(id uint64, action platform.AccessActionKind) bool {
	accessMu.Lock()
	s := accessSink
	accessMu.Unlock()
	if s == nil {
		return false
	}
	s.AccessAction(id, action)
	return true
}

// accessTreeJSON is the wire form of platform.AccessTree: what the Java side
// parses into AccessibilityNodeInfo objects.
//
// It is JSON rather than a run of JNI calls because the tree arrives whole, as
// one snapshot: a call per node per property would be hundreds of crossings of
// the JNI boundary for a screen of content, and the tree is small (the visible
// elements) by construction. The shape is deliberately flat — nodes carry the
// index of their parent, not their children — because that is how AccessTree
// describes itself (AccessNode.Parent), and a flat array is both smaller and
// impossible to make inconsistent.
//
// The numbers are always written, even when they do not apply to the node's
// role (a button's min/max are 0): a field that disappears is a field the
// reader must remember to default, and one that is always there is one the
// reader cannot forget.
type accessTreeJSON struct {
	Focus         uint64           `json:"focus"`
	Nodes         []accessNodeJSON `json:"nodes"`
	Announcements []string         `json:"announcements,omitempty"`
}

// accessNodeJSON is one element of accessTreeJSON.
type accessNodeJSON struct {
	ID          uint64     `json:"id"`
	Parent      int        `json:"parent"`
	Role        string     `json:"role"`
	Label       string     `json:"label,omitempty"`
	Value       string     `json:"value,omitempty"`
	Description string     `json:"description,omitempty"`
	Placeholder string     `json:"placeholder,omitempty"`
	Bounds      [4]float64 `json:"bounds"`
	States      []string   `json:"states,omitempty"`
	Actions     []string   `json:"actions,omitempty"`
	Min         float64    `json:"min"`
	Max         float64    `json:"max"`
	Now         float64    `json:"now"`
	Step        float64    `json:"step"`
	Level       int        `json:"level"`
	PosInSet    int        `json:"posInSet"`
	SetSize     int        `json:"setSize"`
	SelStart    int        `json:"selStart"`
	SelEnd      int        `json:"selEnd"`
}

// encodeAccessTree turns tree into the JSON the Java side reads. A nil tree is
// an error rather than an empty one: "no content" is an empty tree, and asking
// for a serialization of nothing is a bug in the caller.
func encodeAccessTree(tree *platform.AccessTree) (string, error) {
	if tree == nil {
		return "", errors.New("mygo: nil accessibility tree")
	}
	out := accessTreeJSON{Focus: tree.Focus, Announcements: tree.Announcements}
	out.Nodes = make([]accessNodeJSON, 0, len(tree.Nodes))
	for _, n := range tree.Nodes {
		out.Nodes = append(out.Nodes, accessNodeJSON{
			ID:          n.ID,
			Parent:      n.Parent,
			Role:        accessRoleName(n.Role),
			Label:       n.Label,
			Value:       n.Value,
			Description: n.Description,
			Placeholder: n.Placeholder,
			Bounds:      [4]float64{n.Bounds.X, n.Bounds.Y, n.Bounds.W, n.Bounds.H},
			States:      accessStateNames(n.States),
			Actions:     accessActionNames(n.Actions),
			Min:         n.Min,
			Max:         n.Max,
			Now:         n.Now,
			Step:        n.Step,
			Level:       n.Level,
			PosInSet:    n.PosInSet,
			SetSize:     n.SetSize,
			SelStart:    n.SelStart,
			SelEnd:      n.SelEnd,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// accessRoles names every AccessRole. The names are the Java side's vocabulary
// too (MygoPlatform maps them to android.widget class names), so both sides
// read the same list; the table is exhaustive on purpose, and a test checks it:
// a role that maps to "" would reach TalkBack as a nameless view.
var accessRoles = []struct {
	role platform.AccessRole
	name string
}{
	{platform.RoleGroup, "group"},
	{platform.RoleText, "text"},
	{platform.RoleButton, "button"},
	{platform.RoleLink, "link"},
	{platform.RoleCheckBox, "checkbox"},
	{platform.RoleRadio, "radio"},
	{platform.RoleSwitch, "switch"},
	{platform.RoleSlider, "slider"},
	{platform.RoleProgress, "progress"},
	{platform.RoleTextField, "textfield"},
	{platform.RoleImage, "image"},
	{platform.RoleList, "list"},
	{platform.RoleScroll, "scroll"},
	{platform.RoleDialog, "dialog"},
	{platform.RolePopup, "popup"},
	{platform.RoleTooltip, "tooltip"},
	{platform.RolePopUpButton, "popupbutton"},
	{platform.RoleTabList, "tablist"},
	{platform.RoleTab, "tab"},
	{platform.RoleSplitter, "splitter"},
	{platform.RoleStatus, "status"},
	{platform.RoleTable, "table"},
	{platform.RoleRow, "row"},
	{platform.RoleCell, "cell"},
	{platform.RoleColumnHeader, "columnheader"},
	{platform.RoleTree, "tree"},
	{platform.RoleTreeItem, "treeitem"},
	{platform.RoleListItem, "listitem"},
	{platform.RoleMenuButton, "menubutton"},
	{platform.RoleToolbar, "toolbar"},
	{platform.RoleRadioGroup, "radiogroup"},
	{platform.RoleToggleButton, "togglebutton"},
	{platform.RoleComboBox, "combobox"},
	{platform.RoleDisclosure, "disclosure"},
	{platform.RoleMeter, "meter"},
	{platform.RoleStepper, "stepper"},
	{platform.RoleColorWell, "colorwell"},
	{platform.RoleAlertDialog, "alertdialog"},
	{platform.RoleMenu, "menu"},
	{platform.RoleMenuBar, "menubar"},
	{platform.RoleMenuItem, "menuitem"},
	{platform.RoleMenuItemCheckBox, "menuitemcheckbox"},
	{platform.RoleMenuItemRadio, "menuitemradio"},
	{platform.RoleHeading, "heading"},
}

// accessRoleName names role, or "" when it is not one this build knows (a
// newer wire format, say) — the Java side reads "" as a plain view rather than
// inventing a role for it.
func accessRoleName(role platform.AccessRole) string {
	for _, r := range accessRoles {
		if r.role == role {
			return r.name
		}
	}
	return ""
}

// accessStates names the bits of states, in a fixed order.
var accessStates = []struct {
	state platform.AccessStates
	name  string
}{
	{platform.AccessFocusable, "focusable"},
	{platform.AccessDisabled, "disabled"},
	{platform.AccessChecked, "checked"},
	{platform.AccessMixed, "mixed"},
	{platform.AccessSelected, "selected"},
	{platform.AccessExpanded, "expanded"},
	{platform.AccessMultiline, "multiline"},
	{platform.AccessPassword, "password"},
	{platform.AccessReadOnly, "readonly"},
	{platform.AccessSelectable, "selectable"},
	{platform.AccessOffscreen, "offscreen"},
	{platform.AccessMultiselectable, "multiselectable"},
	{platform.AccessSegment, "segment"},
	{platform.AccessSearch, "search"},
	{platform.AccessInvalid, "invalid"},
	{platform.AccessSortAscending, "sortascending"},
	{platform.AccessSortDescending, "sortdescending"},
	{platform.AccessExpandable, "expandable"},
	{platform.AccessVertical, "vertical"},
}

// accessStateNames lists the names of the set bits of states, in table order so
// that the same tree always serializes to the same bytes.
func accessStateNames(states platform.AccessStates) []string {
	var out []string
	for _, s := range accessStates {
		if states&s.state != 0 {
			out = append(out, s.name)
		}
	}
	return out
}

// accessActions names the bits of actions.
var accessActions = []struct {
	action platform.AccessActions
	name   string
}{
	{platform.ActionPress, "press"},
	{platform.ActionFocus, "focus"},
	{platform.ActionIncrement, "increment"},
	{platform.ActionDecrement, "decrement"},
	{platform.ActionSetValue, "setvalue"},
	{platform.ActionScrollIntoView, "scrollintoview"},
	{platform.ActionExpand, "expand"},
}

// accessActionNames lists the names of the set bits of actions.
func accessActionNames(actions platform.AccessActions) []string {
	var out []string
	for _, a := range accessActions {
		if actions&a.action != 0 {
			out = append(out, a.name)
		}
	}
	return out
}

// AccessActionKind names the kinds of an accessibility action, in a fixed
// order: the Java side sends the name (accessibilityAction in MygoPlatform),
// which is what accessActionKind reads back.
//
// A name rather than the number because the two sides are compiled apart: an
// int is a value both sides must keep in step silently, while a name that does
// not parse is an error the Go side can report.
var accessActionKinds = []struct {
	action platform.AccessActionKind
	name   string
}{
	{platform.AccessPress, "press"},
	{platform.AccessFocus, "focus"},
	{platform.AccessIncrement, "increment"},
	{platform.AccessDecrement, "decrement"},
	{platform.AccessSetValue, "setvalue"},
	{platform.AccessScrollIntoView, "scrollintoview"},
	{platform.AccessExpand, "expand"},
	{platform.AccessCollapse, "collapse"},
}

// accessActionKind parses the name of an accessibility action.
func accessActionKind(name string) (platform.AccessActionKind, bool) {
	for _, a := range accessActionKinds {
		if a.name == name {
			return a.action, true
		}
	}
	return 0, false
}

// ParseAccessActionKind parses the wire name of an accessibility action ("press",
// "focus", "setvalue"), which is the form the Java side sends through the JNI
// entry: internal/android is a different package, so the name-to-kind table
// crosses to it through this function rather than by exporting the table.
func ParseAccessActionKind(name string) (platform.AccessActionKind, bool) {
	return accessActionKind(name)
}

// ---------------------------------------------------------------------------
// Structurally absent on Android
// ---------------------------------------------------------------------------
//
// The Android backend embeds internal/unsupported's Backend, so the features a
// desktop has and Android has no object for are answered by its single-line
// stubs rather than by anything here. Each one is a no-op or an
// ErrUnsupported, and each line below is the one that answers today:
//
//	tray        unsupported.go:43  NewTray      → errUnsupported
//	hotkeys     unsupported.go:44  RegisterHotkey → errUnsupported, :45 UnregisterHotkey → no-op
//	notification support
//	            unsupported.go:46  NotificationsSupported → false
//	            unsupported.go:49  ShowNotification → done(errUnsupported)
//	            unsupported.go:47  RemoveNotification → no-op
//	            unsupported.go:53  RemoveAllNotifications → no-op
//	menus       unsupported.go:33-35 SetApplicationMenu / PopupMenu / UpdateMenuItem → no-op
//	dialogs     unsupported.go:81-89 show nothing and call back errUnsupported
//	badge       unsupported.go:62  SetBadge → no-op, :63 Badge → ""
//	Dock        unsupported.go:66  SetDockIcon → errUnsupported, :76 SetDockMenu → no-op,
//	                              :64 Bounce → 0, :65 CancelBounce → no-op
//	activation  unsupported.go:57  SetActivationPolicy → no-op (Android has no such policy)
//	monitors    unsupported.go:120 Displays → nil, :121 CursorPoint → zero
//	vibrancy    unsupported.go:115 SetVibrancy is a Window method, and NewWindow
//	                              (:30) reports errUnsupported: there are no mygo windows
//	                              on Android to set a material on
//	power       unsupported.go:135-138 Watch → no-op, KeepAwake → a no-op release,
//	                              OnBattery → false, IdleTime → 0
//
// Two of those are what this file exists to change once the wiring in
// platform_android.go is applied: NotificationsSupported and ShowNotification.
// The rest stay stubbed, and that is the right answer for them: an Android app
// has no tray to put an icon in, no global hotkey to register (its shortcuts
// live in its own window), no Dock to badge and no multi-monitor geometry
// (Display.getX/Y are 0 outside freeform windows, and the Activity gets one
// surface anyway).
