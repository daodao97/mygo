package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestCollapsible(t *testing.T) {
	open, changes := false, 0
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(8).Children(func() {
			if coreCollapsible(c, "Advanced", &open, func() {
				coreText(c, "Inside")
				coreButton(c, "Inner")
			}).Changed() {
				changes++
			}
			coreButton(c, "After")
		})
	}, 400, 400)
	tt.SetPreferences(Preferences{ReduceMotion: true, TextScale: 1})
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	if tt.HasText("Inside") {
		t.Fatal("closed, it shows its content")
	}
	if n := accessNode(t, tt.h.access, platform.RoleDisclosure, "Advanced"); n.States&platform.AccessExpanded != 0 || n.Actions&platform.ActionPress == 0 {
		t.Errorf("closed, the disclosure: %+v", n)
	}
	tt.Click("Advanced")
	if !open || changes != 1 || !tt.HasText("Inside") {
		t.Fatalf("a click: open %v, %d changes, shows %q", open, changes, tt.Texts())
	}
	if n := accessNode(t, tt.h.access, platform.RoleDisclosure, "Advanced"); n.States&platform.AccessExpanded == 0 {
		t.Errorf("open, the disclosure: %+v", n)
	}
	// Tab goes into the content; Space and Enter close and open.
	tt.Key(0, KeyTab)
	if !tt.Focused("Inner") {
		t.Fatal("Tab did not go into the content")
	}
	tt.Key(Shift, KeyTab)
	tt.Key(0, KeySpace)
	if open || tt.HasText("Inside") {
		t.Fatal("Space did not close it")
	}
	tt.Key(0, KeyEnter)
	if !open || changes != 3 {
		t.Fatalf("Enter: open %v, %d changes", open, changes)
	}
}

func TestCollapsibleAnimates(t *testing.T) {
	open := false
	var progress float32
	var clip, inner uint64
	tt := coreNewTester(func(c *context) {
		p := coreCollapsibleBase(c, &open)
		p.Trigger.Children(func() { coreText(c, "More") })
		progress = p.Progress()
		if panel := p.Panel(func() {
			for range 5 {
				coreText(c, "Line")
			}
		}); panel != nil {
			clip, inner = panel.parent.id, panel.id
		}
	}, 400, 400)
	// A while into the animation.
	partway := func() { tt.Advance(60 * time.Millisecond) }
	tt.Click("More")
	partway()
	h := func(id uint64) float32 { return tt.rt.states[id].h }
	if progress <= 0 || progress >= 1 || h(clip) >= h(inner) || h(inner) == 0 {
		t.Fatalf("opening: progress %v, the panel %v high of %v", progress, h(clip), h(inner))
	}
	tt.Advance(250 * time.Millisecond)
	if progress != 1 || h(clip) != h(inner) {
		t.Fatalf("open: progress %v, the panel %v high of %v", progress, h(clip), h(inner))
	}
	// Closing keeps the content until the panel has shrunk.
	tt.Click("More")
	partway()
	if progress <= 0 || progress >= 1 || !tt.HasText("Line") {
		t.Fatalf("closing: progress %v, shows %v", progress, tt.HasText("Line"))
	}
	tt.Advance(250 * time.Millisecond)
	if progress != 0 || tt.HasText("Line") {
		t.Fatalf("closed: progress %v, shows %v", progress, tt.HasText("Line"))
	}
	// At once where the desktop asks for less motion.
	tt.SetPreferences(Preferences{ReduceMotion: true, TextScale: 1})
	tt.Click("More")
	if progress != 1 {
		t.Errorf("with less motion, progress %v", progress)
	}
}

func TestAccordion(t *testing.T) {
	general, privacy, advanced := true, false, false
	tt := coreNewTester(func(c *context) {
		coreAccordion(c, func() {
			coreAccordionItem(c, "General", &general, func() { coreButton(c, "Save") })
			coreAccordionItem(c, "Privacy", &privacy, func() { coreText(c, "Tracking") })
			coreAccordionItem(c, "Advanced", &advanced, func() { coreText(c, "Logs") })
		})
	}, 400, 400)
	tt.SetPreferences(Preferences{ReduceMotion: true, TextScale: 1})
	if !tt.HasText("Save") || tt.HasText("Tracking") {
		t.Fatalf("shows %q", tt.Texts())
	}
	tt.Click("General")
	if general || tt.HasText("Save") || !tt.Focused("General") {
		t.Fatalf("a click: open %v, focused %v", general, tt.Focused("General"))
	}
	// The arrows, Home and End move between the headers.
	for _, step := range []struct {
		key  Key
		want string
	}{{KeyDown, "Privacy"}, {KeyDown, "Advanced"}, {KeyDown, "Advanced"}, {KeyHome, "General"}, {KeyUp, "General"}, {KeyEnd, "Advanced"}, {KeyUp, "Privacy"}} {
		tt.Key(0, step.key)
		if !tt.Focused(step.want) {
			t.Fatalf("%v did not move to %s", step.key, step.want)
		}
	}
	tt.Key(0, KeyEnter)
	if !privacy || !tt.HasText("Tracking") {
		t.Fatal("Enter did not open Privacy")
	}
	// Tab goes through the content of the sections open.
	tt.Key(Shift, KeyTab)
	tt.Key(0, KeySpace)
	if !general || !tt.HasText("Save") {
		t.Fatal("Space did not open General")
	}
	tt.Key(0, KeyTab)
	if !tt.Focused("Save") {
		t.Fatal("Tab did not go into General's content")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	for _, s := range []struct {
		name string
		open bool
	}{{"General", true}, {"Privacy", true}, {"Advanced", false}} {
		if n := accessNode(t, tt.h.access, platform.RoleDisclosure, s.name); (n.States&platform.AccessExpanded != 0) != s.open {
			t.Errorf("%s: %+v", s.name, n)
		}
	}
}

// The arrows, Home and End pass over the headers that are disabled, and
// scroll to the one that gets the focus.
func TestAccordionDisabledHeaders(t *testing.T) {
	var open [5]bool
	var off [5]bool
	names := []string{"A", "B", "C", "D", "E"}
	tt := coreNewTester(func(c *context) {
		acc := coreAccordionBase(c)
		acc.Root.Children(func() {
			for i, name := range names {
				item := acc.Item(&open[i])
				item.Trigger.Height(30).Disabled(off[i]).Children(func() { coreText(c, name) })
			}
		})
	}, 200, 200)
	off = [5]bool{false, true, false, true, true}
	tt.Frame()
	tt.Click("A")
	for _, step := range []struct {
		key  Key
		want string
	}{{KeyDown, "C"}, {KeyDown, "C"}, {KeyUp, "A"}, {KeyUp, "A"}, {KeyEnd, "C"}, {KeyHome, "A"}} {
		tt.Key(0, step.key)
		if !tt.Focused(step.want) {
			t.Fatalf("%v did not move to %s", step.key, step.want)
		}
	}

	// An item disables its header too.
	shut := false
	tt = coreNewTester(func(c *context) {
		coreAccordion(c, func() {
			coreAccordionItem(c, "One", &shut, func() {})
			coreAccordionItem(c, "Two", &shut, func() {}).Disabled(true)
			coreAccordionItem(c, "Three", &shut, func() {})
		})
	}, 200, 200)
	tt.Click("One")
	tt.Key(0, KeyDown)
	if !tt.Focused("Three") {
		t.Fatal("Down did not pass over the disabled item")
	}
}

func TestAccordionKeysScroll(t *testing.T) {
	var open [20]bool
	var sc *node
	tt := coreNewTester(func(c *context) {
		sc = coreScroll(c).Height(100).Width(200)
		sc.Children(func() {
			coreAccordion(c, func() {
				for i := range open {
					coreAccordionItem(c, "Section "+string(rune('A'+i)), &open[i], func() {})
				}
			})
		})
	}, 200, 200)
	tt.Click("Section A")
	tt.Key(0, KeyEnd)
	if !tt.Focused("Section T") || sc.st.scrollY == 0 {
		t.Errorf("End: focused %v, scrolled to %v", tt.Focused("Section T"), sc.st.scrollY)
	}
	tt.Key(0, KeyHome)
	if !tt.Focused("Section A") || sc.st.scrollY > 1 { // but for the border
		t.Errorf("Home: focused %v, scrolled to %v", tt.Focused("Section A"), sc.st.scrollY)
	}
}
