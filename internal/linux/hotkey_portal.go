//go:build linux && !android && (amd64 || arm64)

package linux

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"unsafe"

	"github.com/egoist/mygo/internal/accelerator"
	"github.com/egoist/mygo/internal/platform"
)

// Wayland lets apps grab no keys: the desktop binds global shortcuts for
// the apps that ask its GlobalShortcuts portal, in a session. A session
// binds its shortcuts once, so a change replaces it with a new session
// binding them all. Desktops remember the keys the user picks for each
// shortcut id, the canonical accelerator, and may ask the user to confirm
// new shortcuts: users may decline them, or pick other keys later, so the
// app hears the keys of each shortcut (HotkeyBound) when the desktop bound
// them and when they change.

const shortcutsPortal = "org.freedesktop.portal.GlobalShortcuts"

// portalShortcuts is the state of the shortcuts bound through the portal.
type portalShortcuts struct {
	available bool
	shortcuts map[int]portalShortcut
	session   string // the session the shortcuts are bound in
	busy      bool   // a session is being created and bound
	stale     bool   // the shortcuts changed while busy
	// changed handles the portal's ShortcutsChanged, from the first
	// shortcut on: apps without global shortcuts link none of it.
	changed func(params ptr)
}

// portalShortcut is a shortcut and what it does, in the app's words.
type portalShortcut struct {
	accelerator.Accelerator
	description string
}

// portalShortcutsOnly makes X11 sessions bind global shortcuts through the
// portal too (TestUsePortalShortcuts).
var portalShortcutsOnly bool

var wayland struct {
	loaded       bool
	displayType  func() uintptr
	setStartupID func(display ptr, id *byte)
}

func (b *Backend) bindShortcut(id int, a accelerator.Accelerator, description string) error {
	p := &b.portal
	if !p.available {
		if portalVersion(shortcutsPortal) == 0 {
			return fmt.Errorf("mygo: global shortcuts need an X11 session or a desktop with the GlobalShortcuts portal: %w", platform.ErrUnsupported)
		}
		p.available = true
		p.changed = b.shortcutsChanged
		conn, _ := bus()
		gDBusConnectionSignalSubscribe(conn, cs(portalName), cs(shortcutsPortal), cs("Activated"), cs(portalPath), nil, 0, cbPortalSignal, 0, 0)
		gDBusConnectionSignalSubscribe(conn, cs(portalName), cs(shortcutsPortal), cs("ShortcutsChanged"), cs(portalPath), nil, 0, cbPortalSignal, 0, 0)
	}
	if p.shortcuts == nil {
		p.shortcuts = map[int]portalShortcut{}
	}
	p.shortcuts[id] = portalShortcut{a, description}
	if err := b.syncShortcuts(); err != nil {
		delete(p.shortcuts, id)
		return err
	}
	return nil
}

func (b *Backend) unbindShortcut(id int) {
	if _, ok := b.portal.shortcuts[id]; ok {
		delete(b.portal.shortcuts, id)
		if err := b.syncShortcuts(); err != nil {
			log.Printf("mygo: global shortcuts: %v", err)
		}
	}
}

// syncShortcuts replaces the session with one binding the shortcuts. A
// session on its way binds them once it is created.
func (b *Backend) syncShortcuts() error {
	p := &b.portal
	if p.busy {
		p.stale = true
		return nil
	}
	if p.session != "" {
		if res, err := portalCall(p.session, "org.freedesktop.portal.Session", "Close", 0, ""); err == nil {
			gVariantUnref(res)
		}
		p.session = ""
	}
	if len(p.shortcuts) == 0 {
		return nil
	}
	options := []ptr{
		vardictEntry("session_handle_token", gVariantNewString(cs(shortcutSessionToken()))),
		// KDE Plasma 5 binds the shortcuts given here, as drafts of the
		// portal had it; portals before 1.17 pass them on.
		vardictEntry("shortcuts", p.list()),
	}
	err := portalRequest(shortcutsPortal, "CreateSession", nil, options, func(code uint32, results ptr) {
		session := vardictString(results, "session_handle")
		if code != 0 || session == "" {
			p.busy = false
			log.Printf("mygo: global shortcuts: the desktop portal did not create a session (response %d)", code)
			b.reportShortcuts(0, false)
			return
		}
		p.stale = false
		if len(p.shortcuts) == 0 {
			p.busy = false
			if res, err := portalCall(session, "org.freedesktop.portal.Session", "Close", 0, ""); err == nil {
				gVariantUnref(res)
			}
			return
		}
		p.session = session
		if err := b.bindShortcuts(); err != nil {
			p.busy = false
			log.Printf("mygo: global shortcuts: %v", err)
			b.reportShortcuts(0, false)
		}
	})
	if err != nil {
		if strings.Contains(err.Error(), "Error.NotAllowed") {
			return fmt.Errorf("mygo: global shortcuts on Wayland need the app installed, with its desktop entry %s: %w", desktopEntryID(), err)
		}
		return fmt.Errorf("mygo: global shortcuts: %w", err)
	}
	p.busy, p.stale = true, false
	return nil
}

// bindShortcuts binds the shortcuts in the new session.
func (b *Backend) bindShortcuts() error {
	p := &b.portal
	args := []ptr{gVariantNewObjectPath(cs(p.session)), p.list(), gVariantNewString(cs(b.portalParent()))}
	return portalRequest(shortcutsPortal, "BindShortcuts", args, nil, func(code uint32, results ptr) {
		p.busy = false
		if code != 0 {
			// KDE answers so when the user declined every shortcut.
			log.Printf("mygo: global shortcuts: the desktop did not bind them (response %d)", code)
			b.reportShortcuts(0, false)
		} else {
			list := gVariantLookupValue(results, cs("shortcuts"), 0)
			b.reportShortcuts(list, false)
			if list != 0 {
				gVariantUnref(list)
			}
		}
		if p.stale {
			if err := b.syncShortcuts(); err != nil {
				log.Printf("mygo: global shortcuts: %v", err)
			}
		}
	})
}

// portalParent identifies to a portal the window its dialog shows over: the
// app's window that has the keyboard, else the first one that shows. GNOME
// opens the dialog of an app that names none behind the windows in front.
// X11 windows only (x11:XID, XWayland's too): a Wayland window would have
// to be exported first (xdg-foreign), which this does not do.
func (b *Backend) portalParent() string {
	if b.x11ID == nil {
		return ""
	}
	var parent *window
	for _, w := range b.windows {
		if w.closed || !gtkWidgetGetVisible(w.win) {
			continue
		}
		if gtkWindowIsActive(w.win) {
			parent = w
			break
		}
		if parent == nil || w.id < parent.id {
			parent = w
		}
	}
	if parent == nil {
		return ""
	}
	gw := gtkWidgetGetWindow(parent.win)
	if gw == 0 {
		return ""
	}
	return fmt.Sprintf("x11:%x", b.x11ID(gw))
}

// ids returns the shortcuts' ids in the order they were registered.
func (p *portalShortcuts) ids() []int {
	ids := make([]int, 0, len(p.shortcuts))
	for id := range p.shortcuts {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// list returns the a(sa{sv}) that describes the shortcuts to the portal,
// in the order they were registered: the desktop shows a shortcut's
// description where it asks the user to confirm it and in its settings.
func (p *portalShortcuts) list() ptr {
	ids := p.ids()
	items := make([]ptr, len(ids))
	for i, id := range ids {
		s := p.shortcuts[id]
		description := s.description
		if description == "" {
			description = s.String()
		}
		items[i] = tuple(gVariantNewString(cs(s.String())), vardict(
			vardictEntry("description", gVariantNewString(cs(description))),
			vardictEntry("preferred_trigger", gVariantNewString(cs(s.XDGTrigger()))),
		))
	}
	return gVariantNewArray(0, unsafe.Pointer(&items[0]), uintptr(len(items)))
}

// reportShortcuts tells the app the keys the desktop has for its
// shortcuts, from the a(sa{sv}) the portal lists them in (0: it bound
// none), and logs those it has no keys for: users may decline them, and
// KDE Plasma 5 binds none with portals from 1.17 on. A shortcut the list
// leaves out has no keys, unless the list is of those that changed.
func (b *Backend) reportShortcuts(list ptr, changed bool) {
	keys := map[string]string{}
	if list != 0 && goStr(gVariantGetTypeString(list)) == "a(sa{sv})" {
		for i := uintptr(0); i < gVariantNChildren(list); i++ {
			item := gVariantGetChildValue(list, i)
			id, props := gVariantGetChildValue(item, 0), gVariantGetChildValue(item, 1)
			keys[goStr(gVariantGetString(id, nil))] = vardictString(props, "trigger_description")
			gVariantUnref(id)
			gVariantUnref(props)
			gVariantUnref(item)
		}
	}
	for _, id := range b.portal.ids() {
		s, ok := b.portal.shortcuts[id]
		if !ok {
			continue // unregistered by the app as it heard of another's keys
		}
		k, listed := keys[s.String()]
		if changed && !listed {
			continue
		}
		if k == "" {
			log.Printf("mygo: global shortcuts: the desktop bound no keys to %s", s.Accelerator)
		}
		b.h.HotkeyBound(id, k)
	}
}

// shortcutsChanged handles the portal's ShortcutsChanged signal, (session
// handle, shortcuts), sent when the user gave the shortcuts other keys.
func (b *Backend) shortcutsChanged(params ptr) {
	session := gVariantGetChildValue(params, 0)
	ours := goStr(gVariantGetString(session, nil)) == b.portal.session
	gVariantUnref(session)
	if !ours {
		return
	}
	list := gVariantGetChildValue(params, 1)
	b.reportShortcuts(list, true)
	gVariantUnref(list)
}

// shortcutActivated handles the portal's Activated signal: (session
// handle, shortcut id, timestamp, options).
func (b *Backend) shortcutActivated(params ptr) {
	str := func(i uintptr) string {
		v := gVariantGetChildValue(params, i)
		defer gVariantUnref(v)
		return goStr(gVariantGetString(v, nil))
	}
	if str(0) != b.portal.session {
		return
	}
	name := str(1)
	options := gVariantGetChildValue(params, 3)
	token := vardictString(options, "activation_token")
	gVariantUnref(options)
	for id, s := range b.portal.shortcuts {
		if s.String() == name {
			withActivationToken(token, func() { b.h.HotkeyPressed(id) })
			return
		}
	}
}

// shortcutSessionToken names the session after the app: KDE names the
// shortcuts of apps without an ID after it, and keeps the keys the user
// picked for them.
func shortcutSessionToken() string {
	return "mygo_" + strings.Map(func(r rune) rune {
		if r < 0x80 && (r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return r
		}
		return '_'
	}, strings.TrimSuffix(desktopEntryID(), ".desktop"))
}

// withActivationToken runs fn with the XDG activation token of the key
// press that caused it: the window fn shows or focuses first uses it, which
// lets it take the focus on Wayland.
func withActivationToken(token string, fn func()) {
	if !wayland.loaded {
		wayland.loaded = true
		if !bind(libGDK, &wayland.displayType, "gdk_wayland_display_get_type") ||
			!bind(libGDK, &wayland.setStartupID, "gdk_wayland_display_set_startup_notification_id") {
			wayland.setStartupID = nil
		}
	}
	display := gdkDisplayGetDefault()
	if token == "" || wayland.setStartupID == nil || !gTypeCheckInstanceIsA(display, wayland.displayType()) {
		fn()
		return
	}
	wayland.setStartupID(display, cs(token))
	defer wayland.setStartupID(display, nil)
	fn()
}
