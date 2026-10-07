package mygo

// OnFirstFrame observes the first native UI frame committed for display.
// On iOS it waits for Metal drawable presentation or the software layer's
// transaction completion. It is separate from app readiness and startup
// overlay dismissal. A late subscriber is called asynchronously once.
func (w *Window) OnFirstFrame(fn func()) (off func()) {
	onMain(func() {
		if w.native == nil {
			off = func() {}
			return
		}
		if w.firstFrame {
			live := true
			off = func() { onMain(func() { live = false }) }
			postMain(func() {
				if live && w.native != nil {
					fn()
				}
			})
		} else {
			off = w.onFirstFrame.add(fn, true)
		}
	})
	if off == nil {
		off = func() {}
	}
	return
}

func (w *Window) presented() {
	if w.native == nil || w.firstFrame {
		return
	}
	w.firstFrame = true
	fire(&w.onFirstFrame)
}
