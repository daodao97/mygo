//go:build windows && (amd64 || arm64)

package windows

import "unsafe"

// A window's taskbar button: progress, hiding it, flashing it, its icon.
// Explorer creates the button when the window is first shown (and again
// when Explorer restarts) and then sends TaskbarButtonCreated, so the state
// is kept on the window and applied then.

var (
	clsidTaskbarList = guid("56fdf344-fd6d-11d0-958a-006097c9a090")
	iidITaskbarList3 = guid("ea1afb91-9e28-4b86-90e9-9e9f8a5eefaf")
)

// ITaskbarList3 vtable indices and progress flags (ShObjIdl_core.h).
const (
	tbHrInit           = 3
	tbAddTab           = 4
	tbDeleteTab        = 5
	tbSetProgressValue = 9
	tbSetProgressState = 10

	// smCxIcon and smCxSmIcon name the system's big and small icon sizes.
	smCxIcon, smCxSmIcon = 11, 49

	tbpfNoProgress    = 0
	tbpfIndeterminate = 1
	tbpfNormal        = 2
	tbpfError         = 4
	tbpfPaused        = 8
)

// iconSize is a system icon size metric at a DPI, scaled from its default
// at 96 DPI when the system does not say.
func iconSize(metric uintptr, def, dpi int) int {
	if has(procGetSystemMetricsForDpi) {
		if n, _, _ := procGetSystemMetricsForDpi.Call(metric, uintptr(dpi)); n != 0 {
			return int(n)
		}
	}
	return def * dpi / 96
}

// taskbar returns the shell's ITaskbarList3, created on first use, or 0.
func (b *Backend) taskbar() uintptr {
	if b.taskbarList == 0 && !b.taskbarFailed {
		var tl uintptr
		hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, clsctxInprocServer,
			uintptr(unsafe.Pointer(&iidITaskbarList3)), uintptr(unsafe.Pointer(&tl)))
		if failed(hr) || tl == 0 || failed(comCall(tl, tbHrInit)) {
			b.taskbarFailed = true
			return 0
		}
		b.taskbarList = tl
	}
	return b.taskbarList
}

// applyTaskbar puts the window's state on its taskbar button.
func (w *window) applyTaskbar() {
	tl := w.b.taskbar()
	if tl == 0 {
		return
	}
	if w.skipTaskbar {
		comCall(tl, tbDeleteTab, w.hwnd)
		return
	}
	w.applyProgress(tl)
	if thumbApply != nil {
		thumbApply(w)
	}
}

// applyProgress updates only the taskbar's progress, so frequent
// SetProgressBar calls do not rebuild the toolbar's icons or call
// ThumbBarAddButtons again.
func (w *window) applyProgress(tl uintptr) {
	flags := map[string]uintptr{"normal": tbpfNormal, "indeterminate": tbpfIndeterminate, "paused": tbpfPaused, "error": tbpfError}[w.progress.state]
	comCall(tl, tbSetProgressState, w.hwnd, flags)
	if flags != tbpfNoProgress && flags != tbpfIndeterminate {
		const total = 10000
		comCall(tl, tbSetProgressValue, w.hwnd, uintptr(w.progress.value*total), total)
	}
}

func (w *window) SetProgressBar(state string, value float64) {
	w.progress.state, w.progress.value = state, value
	if !w.skipTaskbar {
		if tl := w.b.taskbar(); tl != 0 {
			w.applyProgress(tl)
		}
	}
}

func (w *window) SetSkipTaskbar(v bool) {
	if v == w.skipTaskbar {
		return
	}
	w.skipTaskbar = v
	tl := w.b.taskbar()
	if tl == 0 {
		return
	}
	if v {
		comCall(tl, tbDeleteTab, w.hwnd)
		// The button is gone, and with it the toolbar: a new one gets
		// both again on TaskbarButtonCreated.
		w.taskbarButton, w.thumb.live = false, false
		return
	}
	comCall(tl, tbAddTab, w.hwnd)
	w.applyProgress(tl)
}

func (w *window) FlashFrame(flash bool) {
	fi := flashWInfo{HWnd: w.hwnd, Flags: flashwStop}
	if flash {
		fi.Flags = flashwAll | flashwTimerNoFG // until the window comes to the foreground
	}
	fi.Size = uint32(unsafe.Sizeof(fi))
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
}

// SetVisibleOnAllWorkspaces does nothing: Windows offers apps no way to
// pin a window to every virtual desktop.
func (w *window) SetVisibleOnAllWorkspaces(bool) {}

// SetIcon sets the window's own icons, in the sizes of the title bar and
// the taskbar at its DPI.
func (w *window) SetIcon(png []byte) error {
	var small, big uintptr
	if png != nil {
		dpi := dpiOf(w.hwnd)
		var err error
		if small, err = iconFromPNG(png, iconSize(smCxSmIcon, 16, dpi)); err != nil {
			return err
		}
		if big, err = iconFromPNG(png, iconSize(smCxIcon, 32, dpi)); err != nil {
			procDestroyIcon.Call(small)
			return err
		}
	}
	w.freeIcons()
	w.icons = [2]uintptr{small, big}
	if png == nil {
		small, big = w.b.icon, w.b.icon // the application's
	}
	const iconSmall, iconBig = 0, 1
	procSendMessageW.Call(w.hwnd, wmSetIcon, iconSmall, small)
	procSendMessageW.Call(w.hwnd, wmSetIcon, iconBig, big)
	return nil
}

func (w *window) freeIcons() {
	for _, icon := range w.icons {
		if icon != 0 {
			procDestroyIcon.Call(icon)
		}
	}
	w.icons = [2]uintptr{}
}
