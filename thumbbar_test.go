//go:build !js

package mygo

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestThumbBar(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	icon, other, third := []byte("png"), []byte("png2"), []byte("png3")
	clicks := make(chan int, 8)
	button := func() ThumbButton {
		return ThumbButton{Icon: icon, Click: func(b *ThumbButton, win *Window) {
			if win != w {
				t.Error("Click ran with another window")
			}
			for i := range w.thumbBar {
				if b == &w.thumbBar[i] {
					clicks <- i
					return
				}
			}
			t.Error("Click ran with a button outside the toolbar")
		}}
	}

	// A first call missing an icon fails and leaves the toolbar undefined.
	if err := w.SetThumbBar([]ThumbButton{{ToolTip: "Missing"}}); err == nil {
		t.Fatal("SetThumbBar accepted a button without an icon")
	}

	if err := w.SetThumbBar([]ThumbButton{button(), {Icon: other, ToolTip: "Stop", Disabled: true, Click: button().Click}}); err != nil {
		t.Fatalf("SetThumbBar: %v", err)
	}
	if got := onMainValue(func() []platform.ThumbButton { return fw.ThumbBar }); len(got) != 2 ||
		!bytes.Equal(got[0].Icon, icon) || !bytes.Equal(got[1].Icon, other) || got[1].ToolTip != "Stop" || !got[1].Disabled {
		t.Errorf("fake toolbar = %+v", got)
	}

	// A click runs the Click of the button at the position the shell
	// reports.
	onMain(func() { fw.H.(platform.ThumbBarHandler).ThumbButtonClicked(1) })
	if i := <-clicks; i != 1 {
		t.Errorf("Click reported button %d", i)
	}
	onMain(func() { fw.H.(platform.ThumbBarHandler).ThumbButtonClicked(0) })
	if i := <-clicks; i != 0 {
		t.Errorf("Click reported button %d", i)
	}

	// Later calls fill a nil icon with the saved PNG; a new icon is cloned.
	changed := append([]byte(nil), third...)
	if err := w.SetThumbBar([]ThumbButton{
		{ToolTip: "KeepNil"},
		{Icon: changed, ToolTip: "Go", Click: button().Click},
	}); err != nil {
		t.Fatalf("SetThumbBar again: %v", err)
	}
	changed[0] = 'X'
	if got := onMainValue(func() []platform.ThumbButton { return fw.ThumbBar }); !bytes.Equal(got[0].Icon, icon) ||
		!bytes.Equal(got[1].Icon, third) || got[1].ToolTip != "Go" || got[1].Disabled {
		t.Errorf("updated toolbar = %+v", got)
	}
	if saved := onMainValue(func() []ThumbButton { return w.thumbBar }); !bytes.Equal(saved[0].Icon, icon) || !bytes.Equal(saved[1].Icon, third) {
		t.Errorf("core saved icons = %q, %q", saved[0].Icon, saved[1].Icon)
	}

	// With no Click set or an out-of-range id, no click should run.
	onMain(func() { fw.H.(platform.ThumbBarHandler).ThumbButtonClicked(0) })
	select {
	case i := <-clicks:
		t.Errorf("Click ran for button %d", i)
	default:
	}
	onMain(func() { fw.H.(platform.ThumbBarHandler).ThumbButtonClicked(1) })
	if i := <-clicks; i != 1 {
		t.Errorf("Click reported button %d", i)
	}
	for _, bad := range []int{-1, 2} {
		onMain(func() { fw.H.(platform.ThumbBarHandler).ThumbButtonClicked(bad) })
		select {
		case i := <-clicks:
			t.Errorf("Click ran for out-of-range id %d -> %d", bad, i)
		default:
		}
	}
}

// The button count is checked by the core, so these errors do not need a
// real backend.
func TestThumbBarCount(t *testing.T) {
	w, _ := testWindow(t, WindowOptions{})
	if err := w.SetThumbBar(nil); err == nil {
		t.Error("SetThumbBar(nil) succeeded")
	}
	many := make([]ThumbButton, 8)
	for i := range many {
		many[i].Icon = []byte("png")
	}
	if err := w.SetThumbBar(many); err == nil {
		t.Error("SetThumbBar accepted 8 buttons")
	}
	buttons := []ThumbButton{{Icon: []byte("png")}, {Icon: []byte("png2")}}
	if err := w.SetThumbBar(buttons); err != nil {
		t.Fatalf("SetThumbBar: %v", err)
	}
	if err := w.SetThumbBar(buttons[:1]); err == nil {
		t.Error("SetThumbBar changed the number of buttons")
	}
}
