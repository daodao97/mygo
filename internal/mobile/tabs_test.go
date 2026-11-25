package mobile

import (
	"testing"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
)

// quietWindow is a window handler that takes focus and blur silently, as a
// tab's window does while the bar reorders what is in front.
type quietWindow struct{ platform.WindowHandler }

func (quietWindow) Focused() {}
func (quietWindow) Blurred() {}

// SetTitle sends SetTabTitle for a tab in range, nothing for one out of
// range, as the public Tabs bounds the index.
func TestSetTabTitle(t *testing.T) {
	b, h := startBackend(t)
	h.answer = func(op uint64, r *wire.Reader) []byte {
		if op == wire.CallCreateWindow {
			var w wire.Writer
			w.String("")
			w.Int(0)
			return w.Buf
		}
		return nil
	}
	var tb *tabBar
	onMain(b, func() {
		for i := 0; i < 2; i++ {
			if _, err := b.newWindow(&platform.WindowOptions{Title: "t"}, quietWindow{}); err != nil {
				t.Fatal(err)
			}
		}
		w1, w2 := b.windows[1], b.windows[2]
		bar, err := b.NewTabs([]platform.Tab{
			{Title: "one", Window: w1},
			{Title: "two", Window: w2},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		tb = bar.(*tabBar)
	})
	h.onUI(func() {})
	h.mu.Lock()
	h.calls = nil
	h.msgs = nil
	h.mu.Unlock()

	onMain(b, func() { tb.SetTitle(1, "two again") })
	h.onUI(func() {}) // drain the call the transport queued
	var got struct {
		id, index int
		title     string
	}
	var found bool
	h.mu.Lock()
	for i, c := range h.calls {
		if c == wire.CallSetTabTitle {
			r := wire.Reader{Buf: h.msgs[i]}
			r.Uvarint()
			got.id, got.index, got.title = r.Int(), r.Int(), r.String()
			found = true
		}
	}
	h.mu.Unlock()
	if !found {
		t.Fatal("no SetTabTitle call")
	}
	if got.id != tb.id || got.index != 1 || got.title != "two again" {
		t.Errorf("SetTabTitle = id %d index %d title %q, want %d/1/\"two again\"", got.id, got.index, got.title, tb.id)
	}

	// Out-of-range indexes send nothing.
	h.mu.Lock()
	h.calls = nil
	h.msgs = nil
	h.mu.Unlock()
	onMain(b, func() {
		tb.SetTitle(-1, "x")
		tb.SetTitle(2, "x")
	})
	h.onUI(func() {})
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.calls {
		if c == wire.CallSetTabTitle {
			t.Error("out-of-range SetTitle reached the host")
		}
	}
}
