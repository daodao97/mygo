package ui

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

// TestKeyAfterStatePanicsInTester checks that calling Element.Key after
// reading or binding the element's state or ID panics in Tester with a
// clear message directing callers to Context.Key.
func TestKeyAfterStatePanicsInTester(t *testing.T) {
	cases := []struct {
		name string
		fn   func(c *Context)
	}{
		{
			"Local",
			func(c *Context) {
				b := Box(c).Size(80, 32)
				_ = Local(b, "n", func() int { return 1 })
				b.Key("k")
			},
		},
		{
			"Clicked",
			func(c *Context) {
				b := ButtonBase(c).Size(80, 32)
				_ = b.Clicked()
				b.Key("k")
			},
		},
		{
			"Hovered",
			func(c *Context) {
				b := Row(c)
				_ = b.Hovered()
				b.Key("k")
			},
		},
		{
			"Focused",
			func(c *Context) {
				b := ButtonBase(c).Size(80, 32)
				_ = b.Focused()
				b.Key("k")
			},
		},
		{
			"OnUnmount",
			func(c *Context) {
				Box(c).OnUnmount(func() {}).Key("k")
			},
		},
		{
			"FocusBind",
			func(c *Context) {
				want := 1
				Box(c).FocusBind(&want, 1).Key("k")
			},
		},
		{
			"Bounds",
			func(c *Context) {
				b := Box(c).Size(40, 40)
				_ = b.Bounds()
				b.Key("k")
			},
		},
		{
			"AnimateTo",
			func(c *Context) {
				b := Box(c)
				_ = b.AnimateTo("x", 10, Animation{Duration: 100 * time.Millisecond})
				b.Key("k")
			},
		},
		{
			"Draggable",
			func(c *Context) {
				var off Point
				Box(c).Size(40, 40).Draggable(&off, DraggableOptions{}).Key("k")
			},
		},
		{
			"ID",
			func(c *Context) {
				b := Box(c)
				_ = b.ID()
				b.Key("k")
			},
		},
		{
			"ContextMenu",
			func(c *Context) {
				Box(c).Size(40, 40).ContextMenu(func(m *Menu) { m.Item("Run") }).Key("k")
			},
		},
		{
			"PopoverMenu",
			func(c *Context) {
				Box(c).Size(40, 40).PopoverMenu(func(m *Menu) { m.Item("Run") }).Key("k")
			},
		},
		{
			"Menu",
			func(c *Context) {
				Button(c, "More").Menu(func(m *Menu) { m.Item("Run") }).Key("k")
			},
		},
		{
			"PopoverMenuButton",
			func(c *Context) {
				Button(c, "More").PopoverMenuButton(func(m *Menu) { m.Item("Run") }).Key("k")
			},
		},
	}
	const want = "after the element's state was read"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), want) {
					t.Errorf("recovered %v, want substring %q", r, want)
				}
			}()
			NewTester(tc.fn, 200, 100)
		})
	}
}

// TestLateKeyLogsOnceOutsideTester checks that outside Tester (strict == false),
// calling Key after reading state does not panic, logs and records one
// warning per keyed ID, and still lets the app keep running.
func TestLateKeyLogsOnceOutsideTester(t *testing.T) {
	var buf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prevOut)
	// Restore stderr if another test expects it.
	if prevOut == nil {
		defer log.SetOutput(os.Stderr)
	}

	h := &headless{w: 200, h: 100, scale: 1}
	rt := newRuntime(func(c *context) {
		b := coreBox(c).Size(80, 32)
		_ = coreLocal(b, "n", func() int { return 1 })
		b.Key("late")
	}, h)
	defer rt.close()

	rt.runFrame()
	rt.runFrame()

	if len(rt.warnings) != 1 {
		t.Fatalf("warnings = %q, want 1 deduplicated warning", rt.warnings)
	}
	if !strings.Contains(rt.warnings[0], "after the element's state was read") {
		t.Errorf("warning = %q, want late Key message", rt.warnings[0])
	}
	if got := strings.Count(buf.String(), "after the element's state was read"); got != 1 {
		t.Errorf("log count = %d, want 1 (log: %q)", got, buf.String())
	}
	var seenID uint64
	for id := range rt.lateKeys {
		seenID = id
	}
	if allocs := testing.AllocsPerRun(100, func() {
		rt.lateKey(seenID, "late")
	}); allocs != 0 {
		t.Errorf("deduplicated lateKey allocs = %v, want 0", allocs)
	}
}

// TestKeyAfterChildrenAndModifiersWorks checks that Children, pure modifiers,
// Button(c, label).Key, and Tooltip(...).Key neither panic nor warn, and that
// buttons, tooltips, and menus chained after Key work.
func TestKeyAfterChildrenAndModifiersWorks(t *testing.T) {
	clicks := 0
	tipClicks := 0
	menuPicked := 0
	popPicked := 0
	btnMenuPicked := 0

	tt := NewTester(func(c *Context) {
		Column(c).Padding(80, 40).Gap(16).Key("col").Children(func() {
			Row(c).Gap(8).Children(func() {
				Text(c, "inside")
			}).Key("row")

			if Button(c, "Save").Key("save").Clicked() {
				clicks++
			}

			Box(c).Label("box-tip").Size(80, 32).Tooltip("Box tip").Key("box")

			if Button(c, "SaveTip").Tooltip("Save tip").Key("save-tip").Clicked() {
				tipClicks++
			}

			Box(c).Label("ctx-keyed").Size(80, 32).Key("ctx-keyed").ContextMenu(func(m *Menu) {
				if m.Item("Run").Chosen() {
					menuPicked++
				}
			})

			Box(c).Label("pop-keyed").Size(80, 32).Key("pop-keyed").PopoverMenu(func(m *Menu) {
				if m.Item("PopRun").Chosen() {
					popPicked++
				}
			})

			Button(c, "MenuBtn").Key("menu-keyed").Menu(func(m *Menu) {
				if m.Item("BtnRun").Chosen() {
					btnMenuPicked++
				}
			})
		})
	}, 800, 600)

	if len(tt.rt.warnings) != 0 {
		t.Fatalf("unexpected warnings: %q", tt.rt.warnings)
	}

	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatalf("clicks = %d, want 1", clicks)
	}

	if err := tt.Click("SaveTip"); err != nil {
		t.Fatal(err)
	}
	if tipClicks != 1 {
		t.Fatalf("tipClicks = %d, want 1", tipClicks)
	}

	// Hover the keyed box whose Tooltip was chained before Key.
	boxRect, ok := tt.Find("box-tip")
	if !ok {
		t.Fatal("box-tip not found")
	}
	tt.Move(boxRect.X+boxRect.W/2, boxRect.Y+boxRect.H/2)
	tt.Advance(time.Second)
	if !tt.HasText("Box tip") {
		t.Fatalf("Box(c).Tooltip(...).Key(...) did not show tooltip; texts %q", tt.Texts())
	}

	// Hover the keyed button whose Tooltip was chained before Key.
	btnRect, ok := tt.Find("SaveTip")
	if !ok {
		t.Fatal("SaveTip not found")
	}
	tt.Move(btnRect.X+btnRect.W/2, btnRect.Y+btnRect.H/2)
	tt.Advance(time.Second)
	if !tt.HasText("Save tip") {
		t.Fatalf("Button(c, ...).Tooltip(...).Key(...) did not show tooltip; texts %q", tt.Texts())
	}

	// ContextMenu, PopoverMenu, and Menu chained after Key open and deliver choices.
	ctxRect, ok := tt.Find("ctx-keyed")
	if !ok {
		t.Fatal("ctx-keyed not found")
	}
	tt.RightClickAt(ctxRect.X+ctxRect.W/2, ctxRect.Y+ctxRect.H/2)
	if got := tt.Menu(); len(got) != 1 || got[0] != "Run" {
		t.Fatalf("menu = %q, want [Run]", got)
	}
	if err := tt.ChooseMenuItem("Run"); err != nil {
		t.Fatal(err)
	}
	if menuPicked != 1 {
		t.Fatalf("menuPicked = %d, want 1", menuPicked)
	}

	popRect, ok := tt.Find("pop-keyed")
	if !ok {
		t.Fatal("pop-keyed not found")
	}
	tt.RightClickAt(popRect.X+popRect.W/2, popRect.Y+popRect.H/2)
	if err := tt.Click("PopRun"); err != nil {
		t.Fatal(err)
	}
	if popPicked != 1 {
		t.Fatalf("popPicked = %d, want 1", popPicked)
	}

	if err := tt.Click("MenuBtn"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("BtnRun"); err != nil {
		t.Fatal(err)
	}
	if btnMenuPicked != 1 {
		t.Fatalf("btnMenuPicked = %d, want 1", btnMenuPicked)
	}

	if len(tt.rt.warnings) != 0 {
		t.Fatalf("unexpected warnings after interactions: %q", tt.rt.warnings)
	}
}
