package ui

import (
	"hash/fnv"
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestSpinner(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		coreSpinner(c).Label("Loading")
	}, 200, 100)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	if n := accessNode(t, tt.h.access, platform.RoleProgress, "Loading"); n.Now >= n.Min {
		t.Errorf("the spinner is not of unknown length: %+v", n)
	}
	if !tt.rt.redraw || tt.rt.repaintDue.IsZero() {
		t.Error("the spinner is not painted again for its next spoke")
	}
}

// TestSpinnerColor paints a spinner in the theme's muted color, or in
// its TextColor, as a spinner on a chosen row of a list needs one that
// shows on the accent.
func TestSpinnerColor(t *testing.T) {
	red := RGB(255, 0, 0)
	tt := coreNewTester(func(c *context) {
		coreRow(c).Gap(20).Padding(10).Children(func() {
			coreSpinner(c).Label("Muted")
			coreSpinner(c).TextColor(red).Label("Red")
		})
	}, 200, 100)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	img := tt.Image()
	// How much redder than green the spinner's reddest pixel is.
	redness := func(name string) int {
		n := accessNode(t, tt.h.access, platform.RoleProgress, name)
		most := 0
		for y := int(n.Bounds.Y); y < int(n.Bounds.Y+n.Bounds.H); y++ {
			for x := int(n.Bounds.X); x < int(n.Bounds.X+n.Bounds.W); x++ {
				px := img.RGBAAt(x, y)
				most = max(most, int(px.R)-int(px.G))
			}
		}
		return most
	}
	if r := redness("Muted"); r > 20 {
		t.Errorf("the spinner is not gray: %d redder than green", r)
	}
	if r := redness("Red"); r < 60 {
		t.Errorf("the spinner whose text is red is not: %d redder than green", r)
	}
}

func TestMeter(t *testing.T) {
	levels := &MeterLevels{Warning: 70, Critical: 90}
	battery := &MeterLevels{Warning: 20, Critical: 10}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(10).Padding(10).Children(func() {
			coreMeter(c, 50, 0, 100, levels).Width(100).Label("Fine")
			coreMeter(c, 80, 0, 100, levels).Width(100).Label("Warning")
			coreMeter(c, 95, 0, 100, levels).Width(100).Label("Critical")
			coreMeter(c, 15, 0, 100, battery).Width(100).Label("Battery")
		})
	}, 200, 200)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	img := tt.Image()
	th := LightTheme()
	for _, want := range []struct {
		name  string
		color Color
	}{{"Fine", th.Success}, {"Warning", th.Warning}, {"Critical", th.Danger}, {"Battery", th.Warning}} {
		n := accessNode(t, tt.h.access, platform.RoleMeter, want.name)
		if n.Min != 0 || n.Max != 100 {
			t.Errorf("%s: %+v", want.name, n)
		}
		// Inside the bar's fill, near its start.
		got := img.RGBAAt(int(n.Bounds.X)+5, int(n.Bounds.Y+n.Bounds.H/2))
		if !nearColor(got, want.color) {
			t.Errorf("%s is %v, not %v", want.name, got, want.color)
		}
	}
}

// A Meter's levels fill it in colors of the app's own, those left zero in
// the theme's, and Background colors its track.
func TestMeterColors(t *testing.T) {
	gray, amber, brand, track := RGB(140, 140, 140), RGB(230, 160, 20), RGB(20, 90, 200), RGB(230, 230, 236)
	storage := &MeterLevels{Warning: 80, Critical: 95, NormalColor: gray, WarningColor: amber}
	quota := &MeterLevels{NormalColor: brand}
	values := []float64{50, 85, 97}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(10).Padding(10).Children(func() {
			for i, v := range values {
				coreMeter(c, v, 0, 100, storage).Width(100).Label([]string{"Fine", "Warning", "Critical"}[i])
			}
			coreMeter(c, 30, 0, 100, quota).Width(100).Background(track).Label("Quota")
		})
	}, 200, 200)
	th := tt.rt.c.theme
	for _, want := range []struct {
		color Color
		w     float32
	}{{gray, 50}, {amber, 85}, {th.Danger, 97}, {brand, 30}} {
		if f := fillsOf(tt, want.color); len(f) != 1 || abs(f[0].W-want.w) > 0.01 {
			t.Errorf("%v fills %v, want one %v wide", want.color, f, want.w)
		}
	}
	// The theme's Warning shows for none, nor does its Success.
	if f := fillsOf(tt, th.Warning); len(f) != 0 {
		t.Errorf("the theme's Warning fills %v", f)
	}
	if f := fillsOf(tt, th.Success); len(f) != 0 {
		t.Errorf("the theme's Success fills %v", f)
	}
	if f := fillsOf(tt, track); len(f) != 1 || f[0].W != 100 {
		t.Errorf("the track fills %v", f)
	}
	// A battery's, low values the bad ones, in the app's colors.
	battery := &MeterLevels{Warning: 20, Critical: 10, CriticalColor: amber}
	level := 5.0
	tt = coreNewTester(func(c *context) {
		coreMeter(c, level, 0, 100, battery).Width(100)
	}, 200, 100)
	if f := fillsOf(tt, amber); len(f) != 1 || f[0].W != 5 {
		t.Errorf("an empty battery fills %v", f)
	}
	level = 60
	tt.Frame()
	if f := fillsOf(tt, tt.rt.c.theme.Success); len(f) != 1 || f[0].W != 60 {
		t.Errorf("a charged battery fills %v", f)
	}
}

// A Progress fills in the color TextColor gives it, determinate or not,
// over the track Background gives it, from the right right to left.
func TestProgressColor(t *testing.T) {
	brand, track := RGB(20, 90, 200), RGB(230, 230, 236)
	value := 0.25
	tt := coreNewTester(func(c *context) {
		coreProgress(c, value).Width(200).TextColor(brand).Background(track).Label("Storage")
	}, 200, 100)
	if f := fillsOf(tt, brand); len(f) != 1 || f[0].X != 0 || f[0].W != 50 {
		t.Errorf("a quarter fills %v in its color", f)
	}
	if f := fillsOf(tt, track); len(f) != 1 || f[0].W != 200 {
		t.Errorf("the track fills %v", f)
	}
	if f := fillsOf(tt, tt.rt.c.theme.Accent); len(f) != 0 {
		t.Errorf("the accent fills %v", f)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	if n := accessNode(t, tt.h.access, platform.RoleProgress, "Storage"); n.Now != 0.25 {
		t.Errorf("it reads %+v", n)
	}
	// Its place follows the clock, 1.4 s from one end to the other: half
	// of the way there, wherever the clock started.
	tt.Advance(time.Duration(1400+700-tt.Now().UnixMilli()%1400) * time.Millisecond)
	value = -1
	tt.Frame()
	if f := fillsOf(tt, brand); len(f) != 1 || f[0].X != 70 || f[0].W != 60 {
		t.Errorf("of unknown length, it slides %v in its color", f)
	}
	value = 0.25
	tt.SetDirection(RightToLeft)
	if f := fillsOf(tt, brand); len(f) != 1 || f[0].X != 150 || f[0].W != 50 {
		t.Errorf("right to left, a quarter fills %v", f)
	}
}

// nearColor reports whether a pixel is about a color.
func nearColor(got color.RGBA, want Color) bool {
	d := func(a, b uint8) bool { return int(a)-int(b) < 8 && int(b)-int(a) < 8 }
	return d(got.R, want.R) && d(got.G, want.G) && d(got.B, want.B)
}

func TestRating(t *testing.T) {
	stars, changes := 0, 0
	tt := coreNewTester(func(c *context) {
		if coreRating(c, &stars, 5).Label("Rating").Changed() {
			changes++
		}
	}, 300, 100)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	r := accessNode(t, tt.h.access, platform.RoleSlider, "Rating").Bounds
	// Five stars of 16.25 DIPs, two DIPs apart, at the start of the row.
	star := float32(18.25)
	tt.ClickAt(float32(r.X)+star*2.5, float32(r.Y+r.H/2))
	if stars != 3 || changes != 1 {
		t.Fatalf("a click on the third star: %d (%d changes)", stars, changes)
	}
	// A click on the star set clears it.
	tt.ClickAt(float32(r.X)+star*2.5, float32(r.Y+r.H/2))
	if stars != 0 {
		t.Fatalf("a second click: %d", stars)
	}
	tt.Key(0, KeyRight)
	tt.Key(0, KeyRight)
	if stars != 2 {
		t.Fatalf("Right twice: %d", stars)
	}
	tt.Key(0, KeyEnd)
	if n := accessNode(t, tt.h.access, platform.RoleSlider, "Rating"); stars != 5 || n.Now != 5 || n.Max != 5 || n.Actions&platform.ActionIncrement == 0 {
		t.Errorf("End: %d, %+v", stars, n)
	}
}

func TestStepper(t *testing.T) {
	v := 5.0
	tt := coreNewTester(func(c *context) {
		coreStepper(c, &v, 0, 10, 0.5).Label("Copies")
	}, 200, 200)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	n := accessNode(t, tt.h.access, platform.RoleStepper, "Copies")
	b := n.Bounds
	up := func() (float32, float32) { return float32(b.X + b.W/2), float32(b.Y + b.H/4) }
	tt.ClickAt(up())
	if v != 5.5 {
		t.Fatalf("a click on the up arrow: %v", v)
	}
	tt.ClickAt(float32(b.X+b.W/2), float32(b.Y+b.H*3/4))
	if v != 5 {
		t.Fatalf("a click on the down arrow: %v", v)
	}
	tt.Key(0, KeyUp)
	tt.Key(0, KeyEnd)
	if v != 10 {
		t.Fatalf("Up and End: %v", v)
	}
	tt.ClickAt(up())
	if v != 10 {
		t.Errorf("past the top: %v", v)
	}
	// Assistive technology steps it.
	n = accessNode(t, tt.h.access, platform.RoleStepper, "Copies")
	if n.Max != 10 || n.Actions&platform.ActionDecrement == 0 {
		t.Fatalf("the stepper: %+v", n)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.AccessAction, ID: n.ID, Action: platform.AccessDecrement})
	if v != 9.5 {
		t.Errorf("decremented to %v", v)
	}
	// Held, it keeps stepping.
	tt.Press(float32(b.X+b.W/2), float32(b.Y+b.H*3/4))
	tt.Advance(700 * time.Millisecond)
	tt.Release(float32(b.X+b.W/2), float32(b.Y+b.H*3/4))
	if v > 8 {
		t.Errorf("held 0.7 s, it went down to %v", v)
	}
}

func TestStepSlider(t *testing.T) {
	v := 20.0
	tt := coreNewTester(func(c *context) {
		coreStepSlider(c, &v, 0, 100, 25).Width(220).Label("Quality")
	}, 300, 100)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	r := accessNode(t, tt.h.access, platform.RoleSlider, "Quality").Bounds
	// 10 DIPs of padding on each side, 200 of track: 60% is 60, 50 nearest.
	tt.ClickAt(float32(r.X)+10+120, float32(r.Y+10))
	if v != 50 {
		t.Fatalf("a click at 60%%: %v", v)
	}
	tt.Key(0, KeyRight)
	if v != 75 {
		t.Errorf("Right: %v", v)
	}
	if tickCount(0, 100, 25) != 5 || tickCount(0, 1, 0.01) != 0 || snap(0.30000000000000004, 0, 1, 0.1) != 0.3 {
		t.Error("ticks or snapping")
	}
}

// A slider reaches a hi its steps add up to, though not in floating point:
// 3 steps of 0.1 are 0.30000000000000004.
func TestStepSliderReachesEnd(t *testing.T) {
	for _, tc := range [][3]float64{{0, 0.3, 0.1}, {0, 0.7, 0.1}, {0, 0.6, 0.2}, {0, 1.2, 0.4}, {-0.3, 0.3, 0.1}} {
		lo, hi, step := tc[0], tc[1], tc[2]
		v := lo
		tt := coreNewTester(func(c *context) { coreStepSlider(c, &v, lo, hi, step).Width(220) }, 300, 100)
		tt.Key(0, KeyTab)
		tt.Key(0, KeyEnd)
		if v != hi {
			t.Errorf("End of %v: %v", tc, v)
		}
		v = lo
		for range 10 {
			tt.Key(0, KeyRight)
		}
		if v != hi {
			t.Errorf("Right in %v: %v", tc, v)
		}
	}
	// A hi between steps stays out of reach, however close to the next.
	for _, tc := range [][4]float64{{0, 0.25, 0.1, 0.2}, {0, 0.2999999, 0.1, 0.2}, {0, 10, 4, 8}} {
		if got := snap(tc[1], tc[0], tc[1], tc[2]); got != tc[3] {
			t.Errorf("snap to the end of %v: %v", tc, got)
		}
	}
	// Nothing goes past hi.
	if got := snap(0.29999999999999, 0, 0.29999999999999, 0.1); got > 0.29999999999999 {
		t.Errorf("snap past hi: %v", got)
	}
}

func TestRangeSlider(t *testing.T) {
	low, high := 20.0, 80.0
	tt := coreNewTester(func(c *context) {
		coreRangeSlider(c, &low, &high, 0, 100, 10).Width(220).Label("Price")
	}, 300, 100)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	minimum := accessNode(t, tt.h.access, platform.RoleSlider, "Price minimum")
	maximum := accessNode(t, tt.h.access, platform.RoleSlider, "Price maximum")
	if minimum.Now != 20 || maximum.Now != 80 {
		t.Fatalf("the knobs: %+v, %+v", minimum, maximum)
	}
	// A press on the track moves the nearest knob.
	r := accessNode(t, tt.h.access, platform.RoleGroup, "Price").Bounds
	tt.ClickAt(float32(r.X)+10+200*0.68, float32(r.Y+10))
	if high != 70 || low != 20 {
		t.Fatalf("a click at 68%%: %v to %v", low, high)
	}
	// The keys move the knob with the focus, which Tab moves between.
	tt.Key(Shift, KeyTab)
	tt.Key(0, KeyRight)
	if low != 30 {
		t.Fatalf("Right on the low knob: %v", low)
	}
	tt.Key(0, KeyEnd)
	if low != 70 {
		t.Errorf("End stops the low knob at the high one: %v", low)
	}
	// The high knob, dragged and let go before a frame, stays where the
	// pointer let go, though it moved on.
	tt.Press(float32(r.X)+10+200*float32(high/100), float32(r.Y+10))
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerUp, X: r.X + 10 + 200*0.9, Y: r.Y + 10})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: r.X + 10 + 200*0.3, Y: r.Y + 10})
	tt.settle()
	if high != 90 {
		t.Errorf("let go at 90%% before a frame: %v", high)
	}
}

// A slider let go before a frame takes the value where the pointer let
// go, not where it went after.
func TestSliderReleaseBeforeFrame(t *testing.T) {
	v := 0.0
	tt := coreNewTester(func(c *context) {
		coreSlider(c, &v, 0, 100).Width(220).Label("Volume")
	}, 300, 100)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	r := accessNode(t, tt.h.access, platform.RoleSlider, "Volume").Bounds
	// 10 DIPs of padding on each side, 200 of track.
	tt.Press(float32(r.X)+20, float32(r.Y+10))
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerUp, X: r.X + 110, Y: r.Y + 10})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: r.X + 180, Y: r.Y + 10})
	tt.settle()
	if v != 50 {
		t.Errorf("let go at 50%% before a frame, then moved on: %v", v)
	}
}

func TestAvatar(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		coreRow(c).Gap(8).Children(func() {
			coreAvatar(c, "Ada Lovelace", nil)
			coreAvatar(c, "grace", nil)
		})
	}, 200, 100)
	if !tt.HasText("AL") || !tt.HasText("G") {
		t.Errorf("initials: %q", tt.Texts())
	}
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	accessNode(t, tt.h.access, platform.RoleImage, "Ada Lovelace")
}

func TestInitials(t *testing.T) {
	for name, want := range map[string]string{
		"Ada Lovelace":          "AL",
		"grace":                 "G",
		"  jean-luc  picard ":   "JP",
		"Grace Brewster Hopper": "GH",
		"李 小龙":                  "李小",
		"--":                    "",
		"":                      "",
		"élodie 2nd":            "É2",
		"o'brien":               "OB",
		"Ada\tLovelace\nByron ": "AB",
	} {
		if got := initials(name); got != want {
			t.Errorf("initials(%q) = %q, want %q", name, got, want)
		}
	}
	// The color is the hue of FNV-1a, as hash/fnv computes it.
	h := fnv.New32a()
	h.Write([]byte("Ada Lovelace"))
	want := hslColor(float64(h.Sum32()%360), 0.45, 0.55)
	var got Color
	coreNewTester(func(c *context) { got = coreAvatar(c, "Ada Lovelace", nil).bg }, 100, 100)
	if got != want {
		t.Errorf("Avatar's color is %v, want %v", got, want)
	}
}
