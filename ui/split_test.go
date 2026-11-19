package ui

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func rgbaOf(c Color) color.RGBA {
	return color.RGBA{c.R, c.G, c.B, c.A}
}

// SplitGap spaces the panes by gap DIPs from the first frame, omits the
// resting divider line, keeps the accent highlight on hover and drag, and
// resizes across the gap by pointer drag, keyboard arrows and SplitReset.
func TestSplitGap(t *testing.T) {
	bg := RGB(250, 248, 242)
	accent := LightTheme().Accent
	left := float32(100)
	var a, b Rect
	tt := NewTester(func(c *Context) {
		t := LightTheme()
		t.Background = bg
		c.SetTheme(t)
		c.Root().Background(bg)
		Split(c, &left, func() {
			a = Box(c).Fill().Background(RGB(220, 230, 245)).Bounds()
		}, func() {
			b = Box(c).Fill().Background(RGB(230, 245, 225)).Bounds()
		}).SplitGap(8).SplitReset(120).Fill()
	}, 300, 100)
	tt.Frame()
	if a != (Rect{0, 0, 100, 100}) || b != (Rect{108, 0, 192, 100}) {
		t.Fatalf("panes at %v and %v, want {0 0 100 100} and {108 0 192 100}", a, b)
	}
	// At rest there is no divider line in the gap.
	if got := tt.Image().RGBAAt(104, 50); got != rgbaOf(bg) {
		t.Fatalf("resting gap pixel = %v, want background %v", got, bg)
	}
	// Hovering the gap shows the resize cursor and the accent highlight.
	tt.Move(104, 50)
	if tt.Cursor() != CursorResizeEW {
		t.Fatalf("cursor over gap = %v, want CursorResizeEW", tt.Cursor())
	}
	if got := tt.Image().RGBAAt(104, 50); got != rgbaOf(accent) {
		t.Fatalf("hovered gap pixel = %v, want accent %v", got, accent)
	}
	// Dragging anywhere across the 8 DIP gap moves the split and keeps the accent highlight.
	tt.Press(107, 50)
	tt.Move(137, 50)
	if left != 130 {
		t.Fatalf("during drag: left=%v, want 130", left)
	}
	if got := tt.Image().RGBAAt(134, 50); got != rgbaOf(accent) {
		t.Fatalf("dragged gap pixel = %v, want accent %v", got, accent)
	}
	tt.Release(137, 50)
	tt.Move(10, 50)
	tt.Frame()
	if b.X != 138 || b.W != 162 {
		t.Fatalf("after drag: left=%v, second=%v", left, b)
	}
	if got := tt.Image().RGBAAt(134, 50); got != rgbaOf(bg) {
		t.Fatalf("after drag release, gap pixel = %v, want background %v", got, bg)
	}
	// Keyboard arrows and double-click SplitReset respect the gap.
	tt.Key(0, KeyRight)
	tt.Frame()
	if left != 140 || b.X != 148 {
		t.Fatalf("after KeyRight: left=%v, second=%v", left, b)
	}
	tt.ClickAt(144, 50)
	tt.ClickAt(144, 50)
	tt.Frame()
	if left != 120 || b.X != 128 {
		t.Fatalf("after SplitReset double-click: left=%v, second=%v", left, b)
	}
}

// SplitGap(0) puts the panes flush with no resting line while keeping the
// hover/drag accent highlight above both panes and a grip-sized accessible box
// on both horizontal and vertical splits; SplitPercent and SplitVertical share
// the room left after the gap.
func TestSplitGapZeroAndPercent(t *testing.T) {
	bg := RGB(255, 255, 255)
	paneA := RGB(220, 230, 245)
	paneB := RGB(230, 245, 225)
	accent := LightTheme().Accent
	grip := LightTheme().Space(1.5)
	pct := float32(50)
	top := float32(40)
	var a, b, va, vb Rect
	tt := NewTester(func(c *Context) {
		t := LightTheme()
		t.Background = bg
		c.SetTheme(t)
		Column(c).Fill().Children(func() {
			SplitPercent(c, &pct, func() {
				a = Box(c).Fill().Background(paneA).Bounds()
			}, func() {
				b = Box(c).Fill().Background(paneB).Bounds()
			}).SplitGap(0).Height(40).FillWidth()
			SplitVertical(c, &top, func() {
				va = Box(c).Fill().Bounds()
			}, func() {
				vb = Box(c).Fill().Bounds()
			}).SplitGap(10).Grow(1).FillWidth()
		})
	}, 200, 160)
	tt.Frame()
	if a != (Rect{0, 0, 100, 40}) || b != (Rect{100, 0, 100, 40}) {
		t.Fatalf("SplitGap(0) percent panes: %v and %v", a, b)
	}
	if got := tt.Image().RGBAAt(100, 20); got != rgbaOf(paneB) {
		t.Fatalf("SplitGap(0) resting seam = %v, want paneB %v", got, paneB)
	}
	tt.Move(100, 20)
	if got := tt.Image().RGBAAt(100, 20); got != rgbaOf(accent) {
		t.Fatalf("SplitGap(0) hovered seam = %v, want accent %v", got, accent)
	}
	tt.Press(100, 20)
	tt.Move(120, 20)
	if got := tt.Image().RGBAAt(120, 20); got != rgbaOf(accent) {
		t.Fatalf("SplitGap(0) dragged seam = %v, want accent %v", got, accent)
	}
	tt.Release(120, 20)
	tt.Move(10, 20)
	if got := tt.Image().RGBAAt(120, 20); got != rgbaOf(paneB) {
		t.Fatalf("SplitGap(0) after release = %v, want paneB %v", got, paneB)
	}
	if va != (Rect{0, 40, 200, 40}) || vb != (Rect{0, 90, 200, 70}) {
		t.Fatalf("SplitVertical SplitGap(10) panes: %v and %v", va, vb)
	}

	// Vertical SplitGap(0) and accessible bounds for both horizontal and vertical splits.
	top = 60
	ttV := NewTester(func(c *Context) {
		t := LightTheme()
		t.Background = bg
		c.SetTheme(t)
		SplitVertical(c, &top, func() {
			Box(c).Fill().Background(paneA)
		}, func() {
			Box(c).Fill().Background(paneB)
		}).SplitGap(0).Fill()
	}, 200, 160)
	ttV.Frame()
	if got := ttV.Image().RGBAAt(100, 60); got != rgbaOf(paneB) {
		t.Fatalf("vertical SplitGap(0) resting seam = %v, want paneB %v", got, paneB)
	}
	ttV.Move(100, 60)
	if ttV.Cursor() != CursorResizeNS {
		t.Fatalf("vertical SplitGap(0) cursor = %v, want CursorResizeNS", ttV.Cursor())
	}
	if got := ttV.Image().RGBAAt(100, 60); got != rgbaOf(accent) {
		t.Fatalf("vertical SplitGap(0) hovered seam = %v, want accent %v", got, accent)
	}

	hSplit, ok := tt.AccessByRole(RoleSplitter, "")
	if !ok {
		t.Fatalf("horizontal RoleSplitter not found")
	}
	if hSplit.Bounds.W != grip || hSplit.Bounds.X < 120-grip/2-0.01 || hSplit.Bounds.X > 120-grip/2+0.01 {
		t.Fatalf("horizontal SplitGap(0) splitter bounds = %v, want X=%v W=%v", hSplit.Bounds, 120-grip/2, grip)
	}
	vSplit, ok := ttV.AccessByRole(RoleSplitter, "")
	if !ok {
		t.Fatalf("vertical RoleSplitter not found")
	}
	if vSplit.Bounds.H != grip || vSplit.Bounds.Y != 60-grip/2 {
		t.Fatalf("vertical SplitGap(0) splitter bounds = %v, want Y=%v H=%v", vSplit.Bounds, 60-grip/2, grip)
	}

	// Platform-side accessBounds (before Tester overwrites Bounds from state)
	// also reports the grip-sized handle box on the RoleSplitter node.
	tt.rt.access = true
	tt.Frame()
	foundPlatformH := false
	for _, n := range tt.rt.accessTree().Nodes {
		if n.Role == platform.RoleSplitter {
			foundPlatformH = true
			if n.Bounds.W != float64(grip) || n.Bounds.H != 40 {
				t.Fatalf("platform horizontal SplitGap(0) splitter Bounds = %+v, want W=%v H=40", n.Bounds, grip)
			}
			break
		}
	}
	if !foundPlatformH {
		t.Fatal("no platform horizontal RoleSplitter node")
	}
	ttV.rt.access = true
	ttV.Frame()
	foundPlatformV := false
	for _, n := range ttV.rt.accessTree().Nodes {
		if n.Role == platform.RoleSplitter {
			foundPlatformV = true
			if n.Bounds.H != float64(grip) {
				t.Fatalf("platform vertical SplitGap(0) splitter Bounds.H = %v, want %v", n.Bounds.H, grip)
			}
		}
	}
	if !foundPlatformV {
		t.Fatal("no platform vertical RoleSplitter node")
	}

	clipPct := float32(50)
	ttClip := NewTester(func(c *Context) {
		Box(c).Size(300, 80).Clip().Children(func() {
			SplitPercent(c, &clipPct, func() {
				Box(c).Fill()
			}, func() {
				Box(c).Fill()
			}).SplitGap(0).Size(300, 200)
		})
	}, 300, 120)
	clipSplit, ok := ttClip.AccessByRole(RoleSplitter, "")
	if !ok {
		t.Fatalf("clipped RoleSplitter not found")
	}
	if clipSplit.Offscreen || clipSplit.Bounds.W != grip || clipSplit.Bounds.H != 80 {
		t.Fatalf("clipped SplitGap(0) splitter = %+v, want Offscreen=false W=%v H=80", clipSplit, grip)
	}

	offPct := float32(50)
	ttOff := NewTester(func(c *Context) {
		Scroll(c).Size(300, 80).Children(func() {
			Box(c).Size(300, 120).Shrink(0)
			SplitPercent(c, &offPct, func() {
				Box(c).Fill()
			}, func() {
				Box(c).Fill()
			}).SplitGap(0).Size(300, 100).Shrink(0)
		})
	}, 300, 120)
	offSplit, ok := ttOff.AccessByRole(RoleSplitter, "")
	if !ok {
		t.Fatalf("offscreen RoleSplitter not found")
	}
	if !offSplit.Offscreen || offSplit.Bounds.H != 0 {
		t.Fatalf("offscreen SplitGap(0) splitter = %+v, want Offscreen=true H=0", offSplit)
	}
}

// SplitGap updates immediately when changed or removed on later frames.
func TestSplitGapDynamic(t *testing.T) {
	bg := RGB(255, 255, 255)
	paneB := RGB(230, 245, 225)
	left := float32(100)
	useGap := true
	gap := float32(8)
	tt := NewTester(func(c *Context) {
		t := LightTheme()
		t.Background = bg
		c.SetTheme(t)
		s := Split(c, &left, func() {
			Box(c).Fill()
		}, func() {
			Box(c).Fill().Background(paneB)
		})
		if useGap {
			s.SplitGap(gap)
		}
		s.Fill()
	}, 300, 100)
	tt.Frame()
	if got := tt.Image().RGBAAt(108, 50); got != rgbaOf(paneB) {
		t.Fatalf("frame 1 at x=108: %v, want paneB %v", got, paneB)
	}
	gap = 16
	tt.Frame()
	if got := tt.Image().RGBAAt(115, 50); got != rgbaOf(bg) {
		t.Fatalf("after changing SplitGap to 16, pixel x=115 = %v, want bg %v", got, bg)
	}
	if got := tt.Image().RGBAAt(116, 50); got != rgbaOf(paneB) {
		t.Fatalf("after changing SplitGap to 16, pixel x=116 = %v, want paneB %v", got, paneB)
	}
	useGap = false
	tt.Frame()
	if got := tt.Image().RGBAAt(100, 50); got != rgbaOf(LightTheme().Border) {
		t.Fatalf("after omitting SplitGap, divider pixel = %v, want Border %v", got, LightTheme().Border)
	}
	if got := tt.Image().RGBAAt(101, 50); got != rgbaOf(paneB) {
		t.Fatalf("after omitting SplitGap, second pane x=101 = %v, want paneB %v", got, paneB)
	}
}

// SplitGap applies in the same build pass without extra rebuilds, allocates no
// extra per-frame closures compared to Split, works with Context.Key, and
// rejects a trailing Element.Key as Split does.
func TestSplitGapSamePassAndKey(t *testing.T) {
	left := float32(100)
	gap := float32(8)
	builds := 0
	var secondX float32
	tt := NewTester(func(c *Context) {
		builds++
		Split(c.Key("sp"), &left, func() {
			Box(c).Fill()
		}, func() {
			secondX = Box(c).Fill().Bounds().X
		}).SplitGap(gap).Fill()
	}, 300, 100)
	if builds != 1 {
		t.Fatalf("initial mount ran %d build passes, want 1", builds)
	}
	tt.Frame()
	if secondX != 108 {
		t.Fatalf("second pane X = %v, want 108", secondX)
	}
	buildsBefore := builds
	gap = 14
	tt.Frame()
	if builds-buildsBefore != 1 {
		t.Fatalf("changing SplitGap ran %d passes, want 1", builds-buildsBefore)
	}
	tt.Frame()
	if secondX != 114 {
		t.Fatalf("after gap=14, second pane X = %v, want 114", secondX)
	}

	plainLeft := float32(100)
	ttPlain := NewTester(func(c *Context) {
		Split(c, &plainLeft, func() { Box(c).Fill() }, func() { Box(c).Fill() }).Fill()
	}, 300, 100)
	plainAllocs := testing.AllocsPerRun(20, func() { ttPlain.Frame() })
	gapAllocs := testing.AllocsPerRun(20, func() { tt.Frame() })
	if gapAllocs > plainAllocs {
		t.Fatalf("SplitGap frame allocs = %v, want <= plain Split (%v)", gapAllocs, plainAllocs)
	}

	defer func() {
		want := "ui: Key on a Split, whose state was initialized: use Context.Key before construction"
		if r := recover(); r != want {
			t.Fatalf("Split(...).SplitGap(8).Key recovered %v, want %q", r, want)
		}
	}()
	NewTester(func(c *Context) {
		Split(c, &left, func() {}, func() {}).SplitGap(8).Key("late")
	}, 300, 100)
}
