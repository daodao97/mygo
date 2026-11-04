package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestStartupWaitsForNativePresentation(t *testing.T) {
	for _, skip := range []bool{false, true} {
		name := "crossfade"
		if skip {
			name = "direct"
		}
		t.Run(name, func(t *testing.T) { testStartupWaitsForNativePresentation(t, skip) })
	}
}

func testStartupWaitsForNativePresentation(t *testing.T, skip bool) {
	t.Helper()
	h, _, frame := gpuHost(t, func() (gpuRenderer, error) { return &testGPU{}, nil })
	startup := &Startup{Ready: true, SkipTransition: skip}
	var presented func()
	subscribed, canceled, home := 0, 0, 0
	h.conn.OnFirstFrame = func(fn func()) func() {
		subscribed++
		presented = fn
		return func() { canceled++ }
	}
	h.rt.view = func(c *context) {
		startup.coreView(c, func() { coreText(c, "Launch") }, func() { home++ })
	}
	frame()
	frame()
	if home != 0 || subscribed != 1 || !h.shown {
		t.Fatalf("submit bypassed native presentation: home=%d subscriptions=%d submitted=%t", home, subscribed, h.shown)
	}
	presented()
	frame()
	if home == 0 {
		t.Fatal("native presentation did not begin the handoff")
	}
	h.detach()
	if canceled != 1 {
		t.Fatalf("presentation subscription canceled %d times", canceled)
	}
	h.firstFrameShown = false
	presented()
	if h.firstFrameShown {
		t.Fatal("late presentation callback changed a detached window")
	}
}

func TestStartupDirectHandoffWaitsOnlyForReadiness(t *testing.T) {
	startup := &Startup{SkipTransition: true, Duration: time.Hour}
	clicks := 0
	tt, now := clockTester(func(c *context) {
		startup.coreView(c, func() { coreText(c, "Launch") }, func() {
			if coreButton(c, "Home").Clicked() {
				clicks++
			}
		})
	}, 200, 100)
	*now = now.Add(2 * time.Hour)
	tt.Frame()
	if !tt.HasText("Launch") || tt.HasText("Home") {
		t.Fatal("elapsed time bypassed readiness")
	}
	startup.Ready = true
	tt.Frame() // No clock advance: the handoff must be immediate.
	if tt.HasText("Launch") || !tt.HasText("Home") {
		t.Fatal("readiness retained a timed transition")
	}
	if err := tt.Click("Home"); err != nil || clicks != 1 {
		t.Fatalf("content was not immediately interactive: clicks=%d err=%v", clicks, err)
	}
}

func TestStartupCrossfadeAndInput(t *testing.T) {
	startup := &Startup{Duration: 600 * time.Millisecond, Ease: Linear}
	clicks, shortcuts := 0, 0
	tt, now := clockTester(func(c *context) {
		c.Root().Background(RGB(0, 0, 0))
		startup.coreView(c, func() {
			coreBox(c).Fill().Background(RGB(255, 0, 0)).Label("Launch")
		}, func() {
			coreColumn(c).Fill().Background(RGB(0, 0, 255)).Children(func() {
				if coreButton(c, "Go").Clicked() {
					clicks++
				}
				if c.Shortcut(Ctrl, KeyG) {
					shortcuts++
				}
			})
		})
	}, 200, 100)
	if tt.HasText("Go") {
		t.Fatal("content was built before application readiness")
	}
	startup.Ready = true
	tt.Frame()
	*now = now.Add(300 * time.Millisecond)
	tt.Frame()
	p := tt.Image().RGBAAt(190, 90)
	if p.R < 70 || p.R > 190 || p.B < 15 || p.B > 190 {
		t.Fatalf("crossfade did not blend both views at half duration: %v", p)
	}
	// Named clicks reject a covered or inert target. Still send the raw
	// pointer input to verify that the startup layer keeps it from content.
	if err := tt.Click("Go"); err == nil {
		t.Fatal("named click accepted inert content during fade")
	}
	r, ok := tt.Find("Go")
	if !ok {
		t.Fatal("content button was not built during fade")
	}
	tt.ClickAt(center(r))
	tt.Key(Ctrl, KeyG)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	// Inert content may be omitted entirely from the accessibility tree.
	for _, n := range tt.h.access.Nodes {
		if n.Role == platform.RoleButton && n.Label == "Go" {
			tt.send(platform.SurfaceEvent{Kind: platform.AccessAction, ID: n.ID, Action: platform.AccessPress})
		}
	}
	if clicks != 0 || shortcuts != 0 {
		t.Fatalf("input reached content during fade: clicks=%d shortcuts=%d", clicks, shortcuts)
	}
	*now = now.Add(300 * time.Millisecond)
	tt.Frame()
	if tt.HasText("Launch") || !tt.HasText("Go") {
		t.Fatal("crossfade did not finish")
	}
	if err := tt.Click("Go"); err != nil {
		t.Fatal(err)
	}
	tt.Key(Ctrl, KeyG)
	if clicks != 1 || shortcuts != 1 {
		t.Fatalf("completed content remained inert: clicks=%d shortcuts=%d", clicks, shortcuts)
	}
	startup.Ready = false
	tt.Frame()
	if tt.HasText("Launch") {
		t.Fatal("readiness regression replayed startup")
	}
}

func TestStartupReduceMotionAndIndependentWindows(t *testing.T) {
	startup := &Startup{Duration: time.Second}
	view := func(c *context) {
		startup.coreView(c, func() { coreText(c, "Launch") }, func() { coreText(c, "Home") })
	}
	a, now := clockTester(view, 200, 100)
	b, _ := clockTester(view, 200, 100)
	startup.Ready = true
	a.Frame()
	*now = now.Add(500 * time.Millisecond)
	a.Frame()
	a.SetPreferences(Preferences{ReduceMotion: true})
	if a.HasText("Launch") || !a.HasText("Home") {
		t.Fatal("Reduce Motion did not finish an active transition")
	}
	if !b.HasText("Launch") || b.HasText("Home") {
		t.Fatal("another window shared the transition state")
	}
	b.SetPreferences(Preferences{ReduceMotion: true})
	if b.HasText("Launch") || !b.HasText("Home") {
		t.Fatal("Reduce Motion did not switch the second window immediately")
	}
}
