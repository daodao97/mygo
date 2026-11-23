package ui

import "testing"

// A view sees the modifier keys held on their own, draws again as they change, and lets them
// go with the window's focus.
func TestModifiersHeld(t *testing.T) {
	var seen []Modifiers
	tt := coreNewTester(func(c *context) {
		seen = append(seen, c.Modifiers())
		if c.Modifiers()&Cmd != 0 {
			coreText(c, "Shortcuts")
		}
	}, 200, 100)
	if tt.HasText("Shortcuts") {
		t.Fatal("shortcuts with nothing held")
	}
	tt.HoldModifiers(Cmd)
	if !tt.HasText("Shortcuts") || seen[len(seen)-1] != Cmd {
		t.Fatalf("holding Cmd drew %v", seen)
	}
	// A key pressed with them says what is held too.
	tt.Key(Cmd|Shift, KeyA)
	if seen[len(seen)-1] != Cmd|Shift {
		t.Errorf("after Cmd+Shift+A: %v", seen[len(seen)-1])
	}
	tt.HoldModifiers(0)
	if tt.HasText("Shortcuts") {
		t.Error("letting go kept the shortcuts")
	}
	tt.HoldModifiers(Cmd)
	tt.SetFocused(false)
	if tt.HasText("Shortcuts") {
		t.Error("the window losing the keyboard kept Cmd held")
	}
}

func TestHoldModifiersAcrossPointerEvents(t *testing.T) {
	var events []InputEvent
	var ctxMods Modifiers
	tt := coreNewTester(func(c *context) {
		ctxMods = c.Modifiers()
		coreBox(c).Size(200, 100).HandleInput(func(ev InputEvent) bool {
			events = append(events, ev)
			return ev.Kind == InputPointerDown
		})
	}, 200, 100)

	expect := func(name string, wantKind InputKind, wantMods Modifiers, fn func()) {
		t.Helper()
		events = nil
		fn()
		if len(events) == 0 {
			t.Fatalf("%s: no events delivered", name)
		}
		for _, ev := range events {
			if ev.Mods != wantMods {
				t.Errorf("%s (%v): Mods = %v, want %v", name, ev.Kind, ev.Mods, wantMods)
			}
		}
		if events[len(events)-1].Kind != wantKind {
			t.Errorf("%s: last kind = %v, want %v", name, events[len(events)-1].Kind, wantKind)
		}
		if ctxMods != wantMods {
			t.Errorf("%s: Context.Modifiers() = %v, want %v", name, ctxMods, wantMods)
		}
	}

	tt.HoldModifiers(Alt)
	expect("Move", InputPointerMove, Alt, func() { tt.Move(20, 20) })
	expect("Press", InputPointerDown, Alt, func() { tt.Press(20, 20) })
	expect("Move while pressed", InputPointerMove, Alt, func() { tt.Move(30, 20) })
	expect("Release", InputPointerUp, Alt, func() { tt.Release(30, 20) })
	expect("ClickAtWith", InputPointerUp, Alt|Shift, func() { tt.ClickAtWith(Shift, 40, 20) })
	expect("Scroll", InputScroll, Alt, func() { tt.Scroll(40, 20, 0, -10) })
	expect("Pinch", InputZoom, Alt, func() { tt.Pinch(40, 20, 1.2) })

	tt.HoldModifiers(0)
	expect("Move after release", InputPointerMove, 0, func() { tt.Move(50, 20) })
	expect("Press after release", InputPointerDown, 0, func() { tt.Press(50, 20) })
	expect("Release after release", InputPointerUp, 0, func() { tt.Release(50, 20) })

	tt.HoldModifiers(Alt)
	tt.SetFocused(false)
	expect("Move after blur", InputPointerMove, 0, func() { tt.Move(60, 20) })
}

func TestHoldModifiersAcrossDragAndDrop(t *testing.T) {
	var overMods, dropMods []Modifiers
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(20).Padding(20).Children(func() {
			coreBox(c).Size(100, 40).Label("Source").Drag("item")
			target := coreBox(c).Size(200, 100).Label("Target")
			if _, ok := coreDragOver[string](target); ok {
				overMods = append(overMods, c.Modifiers())
			}
			if _, ok := coreDrop[string](target); ok {
				dropMods = append(dropMods, c.Modifiers())
			}
		})
	}, 400, 400)

	src, _ := tt.Find("Source")
	target, _ := tt.Find("Target")

	tt.Press(src.X+10, src.Y+10)
	tt.Move(src.X+30, src.Y+30)
	tt.HoldModifiers(Alt)
	tt.Move(target.X+20, target.Y+20)
	if len(overMods) == 0 || overMods[len(overMods)-1] != Alt {
		t.Fatalf("DragOver saw modifiers %v, want Alt", overMods)
	}
	tt.Release(target.X+20, target.Y+20)
	if len(dropMods) != 1 || dropMods[0] != Alt {
		t.Fatalf("Drop with Alt held saw modifiers %v, want [Alt]", dropMods)
	}

	tt.HoldModifiers(0)
	tt.Press(src.X+10, src.Y+10)
	tt.Move(src.X+30, src.Y+30)
	tt.Move(target.X+20, target.Y+20)
	tt.Release(target.X+20, target.Y+20)
	if len(dropMods) != 2 || dropMods[1] != 0 {
		t.Fatalf("Drop after HoldModifiers(0) saw modifiers %v, want [Alt 0]", dropMods)
	}
}
