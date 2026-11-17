//go:build linux && !android && (amd64 || arm64)

package linux

import "github.com/ebitengine/purego"

// Tray icons reach the panel through the StatusNotifierWatcher, which the
// shell owns on KDE, quickshell and others. While it is away, as when the
// shell restarts under the app, libayatana-appindicator falls back to a
// GtkStatusIcon, and a tray call then could kill the app inside the
// library (#1784). A tray holds its calls from when a watcher it saw
// goes away until one returns, then gives the indicator what the app set
// in the meantime. Where no watcher was ever seen, as on GNOME without the
// AppIndicator extension or a plain X11 window manager, the fallback icon
// is the only one, and the calls go through as before.

const statusNotifierWatcher = "org.kde.StatusNotifierWatcher"

// trayWatcher is what the backend knows of the watcher: whether it
// listens to its owner, whether one owned it since, and whether that
// owner is gone with none in its place.
var trayWatcher struct{ subscribed, seen, away bool }

// watchTrayWatcher follows the watcher's owner, from the first tray on: a
// subscription to NameOwnerChanged, then one NameHasOwner for the owner
// now. Without a bus the trays call the indicator as before. Its callback
// is made here, once per process, so that apps that never reach NewTray
// link none of this.
func watchTrayWatcher() {
	if trayWatcher.subscribed {
		return
	}
	trayWatcher.subscribed = true
	conn, err := bus()
	if err != nil {
		return
	}
	gDBusConnectionSignalSubscribe(conn, cs("org.freedesktop.DBus"), cs("org.freedesktop.DBus"), cs("NameOwnerChanged"),
		cs("/org/freedesktop/DBus"), cs(statusNotifierWatcher), 0, purego.NewCallback(watcherOwnerChanged), 0, 0)
	res, err := dbusCall("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "NameHasOwner",
		tuple(gVariantNewString(cs(statusNotifierWatcher))), "(b)")
	if err != nil {
		return
	}
	v := gVariantGetChildValue(res, 0)
	if gVariantGetBoolean(v) {
		trayWatcher.seen = true
	}
	gVariantUnref(v)
	gVariantUnref(res)
}

// watcherOwnerChanged handles NameOwnerChanged (name, old owner, new
// owner) for the watcher, on the GLib main loop.
func watcherOwnerChanged(conn, sender, path, iface, signal, params, data ptr) {
	v := gVariantGetChildValue(params, 2)
	owned := goStr(gVariantGetString(v, nil)) != ""
	gVariantUnref(v)
	theBackend.watcherChanged(owned)
}

// watcherChanged notes that the watcher has an owner or lost it. When one
// appears, the trays drop libayatana's fallback tooltips and those that
// held calls give the indicator their state again.
func (b *Backend) watcherChanged(owned bool) {
	if !owned {
		trayWatcher.away = trayWatcher.seen
		return
	}
	trayWatcher.seen, trayWatcher.away = true, false
	for _, t := range b.trays {
		if t.ind != 0 {
			dropFallbackTooltips(t.ind)
		}
		t.resume()
	}
}

// held reports whether the tray's calls wait for the watcher, and marks
// the tray to give the indicator its state when the watcher returns.
func (t *tray) held() bool {
	if trayWatcher.away {
		t.pending = true
	}
	return trayWatcher.away
}

// resume gives the indicator what the app set while the watcher was away,
// once: the image under a new file name, which makes the host load it
// again, the title and tooltip, even empty, and the menu.
func (t *tray) resume() {
	if !t.pending || t.ind == 0 {
		return
	}
	t.pending = false
	if t.image != nil {
		_ = t.SetImage(t.image, t.template)
	}
	t.SetTitle(t.title)
	t.SetToolTip(t.tip)
	t.SetMenu(t.spec)
}

// dropFallbackTooltips disconnects the handlers of an indicator's
// new-tooltip signal. libayatana connects one to its fallback
// GtkStatusIcon and leaves it connected when it frees that icon for a
// watcher that appeared, so the next tooltip would go to freed memory. A
// watcher appearing is when it is about to free it; MyGo connects no
// handler there. Releases without the signal need nothing.
var dropFallbackTooltips = func(ind ptr) {
	// Older GLib, 2.56 among them, has g_signal_handlers_disconnect_matched
	// ignore a match by ID alone; g_signal_handler_find takes it.
	var find func(instance ptr, mask int32, signal, detail uint32, closure, fn, data ptr) uint64
	signal := indicatorSignal("new-tooltip")
	if signal == 0 || !bind(libGObject, &find, "g_signal_handler_find") {
		return
	}
	for id := find(ind, 1, signal, 0, 0, 0, 0); id != 0; id = find(ind, 1, signal, 0, 0, 0, 0) { // G_SIGNAL_MATCH_ID
		gSignalHandlerDisconnect(ind, id)
	}
}

// indicatorSignal returns the ID of the AppIndicator's signal, 0 where
// the library lacks it. It is asked once an indicator exists, which
// registers the type.
func indicatorSignal(name string) uint32 {
	var lookup func(name *byte, typ uintptr) uint32
	var typeFromName func(name *byte) uintptr
	if !bind(libGObject, &lookup, "g_signal_lookup") || !bind(libGObject, &typeFromName, "g_type_from_name") {
		return 0
	}
	typ := typeFromName(cs("AppIndicator"))
	if typ == 0 {
		return 0
	}
	return lookup(cs(name), typ)
}
