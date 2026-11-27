package ui

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// drag presses at from, moves in steps to to, and releases there.
func drag(tt *Tester, fromX, fromY, toX, toY float32) {
	tt.Press(fromX, fromY)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tt.Move(fromX+(toX-fromX)*f, fromY+(toY-fromY)*f)
	}
	tt.Release(toX, toY)
}

func TestDragAndDrop(t *testing.T) {
	var dropped []string
	var over bool
	clicks, numbers := 0, 0
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(20).Padding(20).Children(func() {
			src := coreBox(c).Size(100, 40).Label("Source").Drag("hello")
			if src.Clicked() {
				clicks++
			}
			target := coreBox(c).Size(200, 100).Label("Target")
			if v, ok := coreDrop[string](target); ok {
				dropped = append(dropped, v)
			}
			_, over = coreDragOver[string](target)
			// Inside the target, an element taking numbers, not text.
			inner := coreBox(c).Size(50, 30).Label("Numbers")
			if _, ok := coreDrop[int](inner); ok {
				numbers++
			}
		})
	}, 400, 400)
	src, _ := tt.Find("Source")
	target, _ := tt.Find("Target")
	inner, _ := tt.Find("Numbers")
	// A press that hardly moves is a click.
	drag(tt, src.X+10, src.Y+10, src.X+12, src.Y+11)
	if clicks != 1 || len(dropped) != 0 {
		t.Fatalf("a press moving 2 DIPs: %d clicks, dropped %q", clicks, dropped)
	}
	// Dragged over the target, then dropped on it: no click.
	tt.Press(src.X+10, src.Y+10)
	tt.Move(src.X+30, src.Y+30)
	tt.Move(target.X+20, target.Y+20)
	if !over {
		t.Fatal("the target does not see the value over it")
	}
	tt.Release(target.X+20, target.Y+20)
	if !slices.Equal(dropped, []string{"hello"}) || clicks != 1 || over {
		t.Fatalf("dropped %q, %d clicks, over %v", dropped, clicks, over)
	}
	// The element taking numbers does not take text.
	drag(tt, src.X+10, src.Y+10, inner.X+10, inner.Y+10)
	if numbers != 0 {
		t.Errorf("text dropped on an element taking numbers")
	}
	// Escape gives up the drag.
	tt.Press(src.X+10, src.Y+10)
	tt.Move(target.X+20, target.Y+20)
	tt.Image() // the copy following the pointer paints
	tt.Key(0, KeyEscape)
	tt.Release(target.X+20, target.Y+20)
	if len(dropped) != 1 || clicks != 1 {
		t.Errorf("after Escape: dropped %q, %d clicks", dropped, clicks)
	}
}

func TestListReorder(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f"}
	var moves [][]int
	sel := -1
	var chosen Selection[string]
	s := ListState{
		Selected:  &sel,
		Selection: &chosen,
		Key:       func(i int) any { return items[i] },
		Reorder: func(rows []int, to int) {
			moves = append(moves, append(slices.Clone(rows), to))
			moved := make([]string, 0, len(rows))
			for _, r := range rows {
				moved = append(moved, items[r])
			}
			var rest []string
			at := 0
			for i, it := range items {
				if i == to {
					at = len(rest)
				}
				if !slices.Contains(rows, i) {
					rest = append(rest, it)
				}
			}
			if to == len(items) {
				at = len(rest)
			}
			items = slices.Insert(rest, at, moved...)
		},
	}
	tt := coreNewTester(func(c *context) {
		coreList(c, &s, len(items), func(i int) {
			coreText(c, items[i]).Padding(8)
		}).Grow(1)
	}, 300, 400)
	b, _ := tt.Find("b")
	d, _ := tt.Find("d")
	// b below d's middle: before e.
	drag(tt, b.X, b.Y+b.H/2, d.X, d.Y+d.H)
	if len(moves) != 1 || !slices.Equal(moves[0], []int{1, 4}) || !slices.Equal(items, []string{"a", "c", "d", "b", "e", "f"}) {
		t.Fatalf("moves %v, items %q", moves, items)
	}
	// The rows chosen move together, the badge counting them.
	tt.Click("c")
	tt.ClickWith(Cmd, "e")
	e, _ := tt.Find("e")
	a, _ := tt.Find("a")
	tt.Press(e.X, e.Y+e.H/2)
	tt.Move(e.X, e.Y)
	tt.Move(a.X, a.Y+2)
	if d := tt.rt.drag; d == nil || d.count != 2 {
		t.Errorf("the drag of two rows is %+v", d)
	}
	tt.Release(a.X, a.Y+2)
	if len(moves) != 2 || !slices.Equal(moves[1], []int{1, 4, 0}) || !slices.Equal(items, []string{"c", "e", "a", "d", "b", "f"}) {
		t.Errorf("moves %v, items %q", moves, items)
	}
	// Dropped where they are, nothing moves.
	f, _ := tt.Find("f")
	drag(tt, f.X, f.Y+f.H/2, f.X, f.Y+f.H/2+6)
	if len(moves) != 2 {
		t.Errorf("a row dropped on itself moved: %v", moves)
	}
	// They go where they dropped, though the pointer moved on before the
	// next frame.
	f, _ = tt.Find("f")
	c, _ := tt.Find("c")
	tt.Press(f.X, f.Y+f.H/2)
	tt.Move(f.X, f.Y)
	tt.Move(c.X, c.Y+2)
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerUp, X: float64(c.X), Y: float64(c.Y + 2)})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: float64(f.X), Y: float64(f.Y + f.H)})
	tt.settle()
	if len(moves) != 3 || !slices.Equal(moves[2], []int{5, 0}) {
		t.Errorf("moves %v, items %q", moves, items)
	}
}

// A list's rows dragged out with DragRows drag the value it makes of them,
// for targets inside the list or around it to take: the list takes none
// of it, so it neither reorders nor shows a place between its rows.
func TestListDragRows(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f"}
	sel := -1
	var chosen Selection[string]
	var asked [][]int
	s := ListState{
		Selected:  &sel,
		Selection: &chosen,
		Key:       func(i int) any { return items[i] },
		DragRows: func(rows []int) any {
			asked = append(asked, slices.Clone(rows))
			var names []string
			for _, r := range rows {
				if items[r] == "f" {
					return nil // f stays
				}
				names = append(names, items[r])
			}
			return names
		},
	}
	var intoD, around, trash [][]string
	overAround := false
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			wrap := coreColumn(c).Height(300).Children(func() {
				coreList(c, &s, len(items), func(i int) {
					row := coreRow(c).Padding(8).Children(func() { coreText(c, items[i]) })
					if items[i] == "d" {
						if v, ok := coreDrop[[]string](row); ok {
							intoD = append(intoD, v)
						}
					}
				}).Grow(1)
			})
			if v, ok := coreDrop[[]string](wrap); ok {
				around = append(around, v)
			}
			_, overAround = coreDragOver[[]string](wrap)
			bin := coreBox(c).Size(200, 60).Label("Trash")
			if v, ok := coreDrop[[]string](bin); ok {
				trash = append(trash, v)
			}
		})
	}, 300, 400)
	find := func(label string) Rect {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("no %q", label)
		}
		return Rect{float32(r.X), float32(r.Y), float32(r.W), float32(r.H)}
	}
	b, d, bin := find("b"), find("d"), find("Trash")
	drag(tt, b.X, b.Y+b.H/2, bin.X+20, bin.Y+20)
	if len(trash) != 1 || !slices.Equal(trash[0], []string{"b"}) {
		t.Fatalf("dropped on the trash: %q", trash)
	}
	// On a row taking them, inside the list.
	drag(tt, b.X, b.Y+b.H/2, d.X, d.Y+d.H/2)
	if len(intoD) != 1 || !slices.Equal(intoD[0], []string{"b"}) {
		t.Fatalf("dropped on d: %q", intoD)
	}
	// Past the rows, the list takes nothing: the element around it does,
	// and the rows stay as they are.
	f := find("f")
	tt.Press(b.X, b.Y+b.H/2)
	tt.Move(b.X, b.Y+b.H/2+10)
	tt.Move(f.X, f.Y+f.H+40)
	if !overAround {
		t.Error("the list took rows dragged past its own")
	}
	tt.Release(f.X, f.Y+f.H+40)
	if len(around) != 1 || !slices.Equal(items, []string{"a", "b", "c", "d", "e", "f"}) {
		t.Fatalf("around %q, items %q", around, items)
	}
	// The rows chosen drag together, the badge counting them.
	tt.Click("c")
	tt.ClickWith(Cmd, "e")
	e := find("e")
	tt.Press(e.X, e.Y+e.H/2)
	tt.Move(e.X, e.Y+e.H/2+10)
	tt.Move(bin.X+20, bin.Y+20)
	if d := tt.rt.drag; d == nil || d.count != 2 {
		t.Errorf("the drag of two rows is %+v", d)
	}
	tt.Release(bin.X+20, bin.Y+20)
	if len(trash) != 2 || !slices.Equal(trash[1], []string{"c", "e"}) {
		t.Fatalf("dropped on the trash: %q, rows asked for %v", trash, asked)
	}
	// Rows DragRows makes nothing of do not drag.
	drag(tt, f.X, f.Y+f.H/2, bin.X+20, bin.Y+20)
	if len(trash) != 2 {
		t.Errorf("f dragged: %q", trash)
	}
}

// Rows dragged out with a DragLabel show it beside the pointer in place
// of a copy of the row, which would cover the row they are dragged onto;
// near the window's edge, on the other side of the pointer, not over it.
// Rows it names "" keep the copy, and rows that do not drag, or a list
// that reorders, never ask for a label.
func TestListDragLabel(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	red := RGB(220, 30, 30)
	var asked []string
	s := ListState{
		DragRows: func(rows []int) any {
			if items[rows[0]] == "e" {
				return nil
			}
			return items[rows[0]]
		},
		DragLabel: func(rows []int) string {
			asked = append(asked, items[rows[0]])
			if items[rows[0]] == "c" {
				return ""
			}
			return "row " + items[rows[0]]
		},
	}
	var surface Color
	tt := coreNewTester(func(c *context) {
		surface = c.theme.Surface
		coreList(c, &s, len(items), func(i int) {
			row := coreRow(c).Height(40).Children(func() { coreText(c, items[i]) })
			if items[i] == "d" {
				row.Background(red)
				coreDrop[string](row)
			} else {
				row.Background(RGB(30, 30, 220))
			}
		}).Height(300)
	}, 300, 400)
	rowY := func(label string) float32 {
		r, _ := tt.Find(label)
		return r.Y + r.H/2
	}
	isRed := func(x, y float32) bool {
		c := tt.Image().RGBAAt(int(x), int(y))
		return c.R > 200 && c.G < 60 && c.B < 60
	}
	isSurface := func(x, y float32) bool {
		c, want := tt.Image().RGBAAt(int(x), int(y)), surface
		near := func(a, b uint8) bool { return a+6 >= b && b+6 >= a }
		return near(c.R, want.R) && near(c.G, want.G) && near(c.B, want.B)
	}
	// dragOnto presses row from at x, moves onto d's row there, and
	// returns where the pointer is, still pressed.
	dragOnto := func(from string, x float32) (float32, float32) {
		y0, y := rowY(from), rowY("d")
		tt.Press(x, y0)
		for k := 1; k <= 4; k++ {
			tt.Move(x, y0+(y-y0)*float32(k)/4)
		}
		return x, y
	}

	x, y := dragOnto("b", 150)
	if dr := tt.rt.drag; dr == nil || dr.label != "row b" {
		t.Fatalf("the drag is %+v", dr)
	}
	if !isRed(x-40, y) {
		t.Error("the row under the pointer is covered")
	}
	if !isSurface(x+20, y+16) {
		t.Errorf("no label below and to the right of the pointer: %v", tt.Image().RGBAAt(int(x)+20, int(y)+16))
	}
	tt.Release(x, y)

	// Near the right edge, it goes to the left of the pointer.
	x, y = dragOnto("b", 290)
	if !isRed(x-4, y+16) || !isSurface(x-20, y+16) {
		t.Errorf("at the edge, the label is not left of the pointer: %v under it, %v left of it",
			tt.Image().RGBAAt(int(x)-4, int(y)+16), tt.Image().RGBAAt(int(x)-20, int(y)+16))
	}
	tt.Release(x, y)

	// Named "", the row keeps its copy, which covers the row it is over.
	x, y = dragOnto("c", 150)
	if dr := tt.rt.drag; dr == nil || dr.label != "" {
		t.Fatalf("the drag of c is %+v", dr)
	}
	if isRed(x-40, y) {
		t.Error("no copy of c under the pointer")
	}
	tt.Release(x, y)

	// A row that does not drag is never named.
	asked = asked[:0]
	x, y = dragOnto("e", 150)
	tt.Release(x, y)
	if len(asked) != 0 {
		t.Errorf("DragLabel asked for rows that do not drag: %q", asked)
	}

	// A list that reorders never asks for a label.
	asked = asked[:0]
	s.DragRows, s.Reorder = nil, func([]int, int) {}
	x, y = dragOnto("b", 150)
	if dr := tt.rt.drag; dr == nil || dr.label != "" {
		t.Errorf("a reorder drag has a label: %+v", dr)
	}
	tt.Release(x, y)
	if len(asked) != 0 {
		t.Errorf("DragLabel asked while reordering: %q", asked)
	}
}

// A label in a right to left script draws within its box, which is as
// wide as the text, not at the right of the width it wraps at.
func TestListDragLabelRTL(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	s := ListState{
		DragRows:  func(rows []int) any { return items[rows[0]] },
		DragLabel: func([]int) string { return "قاعدة البيانات" },
	}
	var surface Color
	tt := coreNewTester(func(c *context) {
		surface = c.theme.Surface
		coreList(c, &s, len(items), func(i int) {
			coreRow(c).Height(40).Children(func() { coreText(c, items[i]) })
		}).Height(300)
	}, 500, 400)
	from, _ := tt.Find("a")
	to, _ := tt.Find("d")
	x, y0, y := float32(40), from.Y+from.H/2, to.Y+to.H/2
	tt.Press(x, y0)
	for k := 1; k <= 4; k++ {
		tt.Move(x, y0+(y-y0)*float32(k)/4)
	}
	defer tt.Release(x, y)
	img := tt.Image()
	isSurface := func(px int) bool {
		c := img.RGBAAt(px, int(y)+16)
		near := func(a, b uint8) bool { return a+6 >= b && b+6 >= a }
		return near(c.R, surface.R) && near(c.G, surface.G) && near(c.B, surface.B)
	}
	right := int(x) + 20
	for right < 500 && isSurface(right+1) {
		right++
	}
	inked := func(from, to int) bool {
		for px := from; px < to; px++ {
			for py := int(y) + 10; py < int(y)+40; py++ {
				if c := img.RGBAAt(px, py); c.R < 100 && c.G < 100 && c.B < 100 {
					return true
				}
			}
		}
		return false
	}
	if !inked(int(x)+14, right) {
		t.Error("no text in the label")
	}
	if inked(right+4, 500) {
		t.Errorf("text right of the label, which ends at %d", right)
	}
}

func TestListDragRowsAndReorder(t *testing.T) {
	s := ListState{Reorder: func([]int, int) {}, DragRows: func([]int) any { return nil }}
	defer func() {
		if recover() == nil {
			t.Error("a list both reordered and dragged its rows out")
		}
	}()
	tt := coreNewTester(func(c *context) {
		coreList(c, &s, 1, func(int) { coreText(c, "a") })
	}, 100, 100)
	tt.Frame()
}

// A press on a control taking clicks alone, inside an element that drags
// a value, drags that element once it moves, and is no click; a press that
// does not move clicks the control. Controls taking the pointer's moves,
// as a text input, keep them.
func TestDragAroundControls(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	var moves [][]int
	clicked := ""
	s := ListState{Reorder: func(rows []int, to int) {
		moves = append(moves, append(slices.Clone(rows), to))
	}}
	var dropped []string
	text := ""
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			coreList(c, &s, len(items), func(i int) {
				b := coreButtonBase(c).Padding(8).Children(func() { coreText(c, items[i]) })
				if b.Clicked() {
					clicked += items[i]
				}
			}).Height(200)
			coreRow(c).Gap(8).Padding(8).Drag("card").Children(func() {
				coreButton(c, "Open")
				coreTextInput(c, &text).Label("Note").Width(120)
			})
			target := coreBox(c).Size(200, 60).Label("Target")
			if v, ok := coreDrop[string](target); ok {
				dropped = append(dropped, v)
			}
		})
	}, 300, 400)
	b, _ := tt.Find("b")
	d, _ := tt.Find("d")
	// b's button, below d's middle: before the end.
	drag(tt, b.X, b.Y+b.H/2, d.X, d.Y+d.H)
	if len(moves) != 1 || !slices.Equal(moves[0], []int{1, 4}) || clicked != "" {
		t.Fatalf("moves %v, clicked %q", moves, clicked)
	}
	// A press that does not move clicks the button.
	if err := tt.Click("a"); err != nil {
		t.Fatal(err)
	}
	if clicked != "a" || len(moves) != 1 {
		t.Errorf("clicked %q, moves %v", clicked, moves)
	}
	// A button in an element with Drag drags it.
	open, _ := tt.Find("Open")
	target, _ := tt.Find("Target")
	drag(tt, open.X+4, open.Y+4, target.X+20, target.Y+20)
	if !slices.Equal(dropped, []string{"card"}) {
		t.Errorf("dropped %q", dropped)
	}
	// A text input keeps the pointer's moves, to choose text.
	note, _ := tt.Find("Note")
	drag(tt, note.X+4, note.Y+4, target.X+20, target.Y+20)
	if len(dropped) != 1 {
		t.Errorf("a text input dragged its row: %q", dropped)
	}
}

func TestGridReorder(t *testing.T) {
	var moves [][]int
	sel := -1
	s := GridState{Selected: &sel, Reorder: func(items []int, to int) {
		moves = append(moves, append(slices.Clone(items), to))
	}}
	tt := coreNewTester(func(c *context) {
		coreGridView(c, &s, 12, 100, 60, func(i int) {
			coreTextf(c, "Item %d", i)
		}).Grow(1)
	}, 448, 400)
	from, _ := tt.Find("Item 0")
	to, _ := tt.Find("Item 2")
	// Item 0 on the right half of item 2: before item 3.
	drag(tt, from.X, from.Y, to.X+to.W+20, to.Y)
	if len(moves) != 1 || !slices.Equal(moves[0], []int{0, 3}) {
		t.Fatalf("moves %v", moves)
	}
	// On the left half of an item: before it.
	five, _ := tt.Find("Item 5")
	drag(tt, to.X, to.Y, five.X-20, five.Y)
	if len(moves) != 2 || !slices.Equal(moves[1], []int{2, 5}) {
		t.Errorf("moves %v", fmt.Sprint(moves))
	}
	// Where they dropped, though the pointer moved on before the next
	// frame.
	tt.Press(from.X, from.Y)
	tt.Move(from.X+20, from.Y)
	tt.Move(to.X+to.W+20, to.Y)
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerUp, X: float64(to.X + to.W + 20), Y: float64(to.Y)})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: float64(five.X), Y: float64(five.Y + 80)})
	tt.settle()
	if len(moves) != 3 || !slices.Equal(moves[2], []int{0, 3}) {
		t.Errorf("moves %v", fmt.Sprint(moves))
	}
}

// A value let go over no element taking it, as past the window's edge,
// tells its source where, once; one given up with Escape does not.
func TestDragMissed(t *testing.T) {
	var missed []Point
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(20).Padding(20).Children(func() {
			src := coreBox(c).Size(100, 40).Label("Source").Drag("hello")
			if at, ok := src.DragMissed(); ok {
				missed = append(missed, at)
			}
			if _, ok := coreDrop[string](coreBox(c).Size(200, 100).Label("Target")); ok {
				t.Error("the target took a value let go elsewhere")
			}
		})
	}, 400, 400)
	src, _ := tt.Find("Source")
	drag(tt, src.X+10, src.Y+10, 450, -30)
	tt.Frame()
	if !slices.Equal(missed, []Point{{450, -30}}) {
		t.Fatalf("let go past the edge: missed %v", missed)
	}
	missed = nil
	tt.Press(src.X+10, src.Y+10)
	tt.Move(src.X+40, src.Y+40)
	tt.Move(450, 200)
	tt.Key(0, KeyEscape)
	tt.Release(450, 200)
	tt.Frame()
	if len(missed) != 0 {
		t.Fatalf("given up with Escape: missed %v", missed)
	}
}

// A source the frame builds no more, as one in a view that went away
// while it was dragged, goes on: its copy follows the pointer, and it
// drops where it is let go of.
func TestDragSourceGone(t *testing.T) {
	var dropped []string
	shown := true
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(20).Padding(20).Children(func() {
			if shown {
				coreBox(c).Size(100, 40).Label("Source").Background(RGB(255, 0, 0)).Drag("hello")
			} else {
				coreBox(c).Size(100, 40).Label("Other")
			}
			target := coreBox(c).Size(200, 100).Label("Target")
			if v, ok := coreDrop[string](target); ok {
				dropped = append(dropped, v)
			}
		})
	}, 400, 400)
	src, _ := tt.Find("Source")
	target, _ := tt.Find("Target")
	tt.Press(src.X+10, src.Y+10)
	tt.Move(src.X+30, src.Y+30)
	shown = false
	tt.Frame()
	if _, ok := tt.Find("Source"); ok {
		t.Fatal("the source is still built")
	}
	tt.Move(target.X+20, target.Y+20)
	if c := tt.Image().RGBAAt(int(target.X+40), int(target.Y+20)); c.R < 150 || c.G > 100 {
		t.Errorf("no copy of the source under the pointer: %v", c)
	}
	tt.Release(target.X+20, target.Y+20)
	if !slices.Equal(dropped, []string{"hello"}) {
		t.Fatalf("dropped %q", dropped)
	}
	// It is forgotten once let go of.
	tt.Frame()
	if c := tt.Image().RGBAAt(int(target.X+40), int(target.Y+20)); c.R > 150 && c.G < 100 {
		t.Errorf("the copy stays after the drop: %v", c)
	}
}

// An element taking the value built under a pointer that stands still, as
// in a view that just showed, is the one the value is over, and takes it.
func TestDragTargetBuiltUnderPointer(t *testing.T) {
	var dropped []string
	var over, shown bool
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(20).Padding(20).Children(func() {
			coreBox(c).Size(100, 40).Label("Source").Drag("hello")
			if shown {
				target := coreBox(c).Size(200, 100).Label("Target")
				if v, ok := coreDrop[string](target); ok {
					dropped = append(dropped, v)
				}
				_, over = coreDragOver[string](target)
			}
		})
	}, 400, 400)
	src, _ := tt.Find("Source")
	x, y := src.X+20, src.Y+src.H+60
	tt.Press(src.X+10, src.Y+10)
	tt.Move(src.X+30, src.Y+30)
	tt.Move(x, y)
	shown = true
	tt.Frame()
	tt.Frame()
	if !over {
		t.Fatal("the target built under the pointer does not see the value")
	}
	tt.Release(x, y)
	if !slices.Equal(dropped, []string{"hello"}) {
		t.Fatalf("dropped %q", dropped)
	}
}

// A button in a row DragRows leaves out, returning nil, keeps its click
// when the press moves a little, as far as would start a drag: the row
// has nothing to drag. A row that drags still takes the press.
func TestDragRowsNilKeepsButtonClick(t *testing.T) {
	items := []string{"header", "file"}
	clicked := 0
	var dropped []any
	s := ListState{DragRows: func(rows []int) any {
		if rows[0] == 0 {
			return nil
		}
		return "file"
	}}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Fill().Children(func() {
			coreList(c, &s, len(items), func(i int) {
				coreRow(c).Padding(8).Gap(8).Children(func() {
					coreText(c, items[i])
					if coreButton(c, "Add "+items[i]).Clicked() && i == 0 {
						clicked++
					}
				})
			}).Height(200)
			target := coreBox(c).Size(200, 60).Label("Target")
			if v, ok := coreDrop[string](target); ok {
				dropped = append(dropped, v)
			}
		})
	}, 300, 400)
	add, _ := tt.Find("Add header")
	x, y := add.X+add.W/2, add.Y+add.H/2
	tt.Press(x, y)
	tt.Move(x+3, y+3)
	tt.Move(x+6, y+5)
	tt.Release(x+6, y+5)
	if clicked != 1 {
		t.Fatalf("a press moving 8 DIPs on a row that does not drag: %d clicks", clicked)
	}
	file, _ := tt.Find("Add file")
	target, _ := tt.Find("Target")
	drag(tt, file.X+4, file.Y+4, target.X+20, target.Y+20)
	if len(dropped) != 1 || dropped[0] != "file" {
		t.Errorf("the row that drags dropped %v", dropped)
	}
}

// An element takes values of several types, one Drop for each.
func TestDropSeveralTypes(t *testing.T) {
	var words []string
	var numbers []int
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Gap(20).Padding(20).Children(func() {
			coreBox(c).Size(100, 40).Label("Word").Drag("hello")
			coreBox(c).Size(100, 40).Label("Number").Drag(7)
			target := coreBox(c).Size(200, 100).Label("Target")
			if v, ok := coreDrop[string](target); ok {
				words = append(words, v)
			}
			if v, ok := coreDrop[int](target); ok {
				numbers = append(numbers, v)
			}
		})
	}, 400, 400)
	word, _ := tt.Find("Word")
	number, _ := tt.Find("Number")
	target, _ := tt.Find("Target")
	drag(tt, word.X+10, word.Y+10, target.X+20, target.Y+20)
	drag(tt, number.X+10, number.Y+10, target.X+20, target.Y+20)
	if !slices.Equal(words, []string{"hello"}) || !slices.Equal(numbers, []int{7}) {
		t.Errorf("dropped words %q, numbers %v", words, numbers)
	}
}

// A list that reorders its rows takes other values dropped on it too.
func TestListReorderTakesOtherDrops(t *testing.T) {
	items := []string{"a", "b", "c"}
	var moves [][]int
	var words []string
	s := ListState{
		Key:     func(i int) any { return items[i] },
		Reorder: func(rows []int, to int) { moves = append(moves, append(slices.Clone(rows), to)) },
	}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Children(func() {
			coreBox(c).Size(100, 40).Label("Word").Drag("hello")
			list := coreList(c, &s, len(items), func(i int) {
				coreText(c, items[i]).Padding(8)
			}).Height(200)
			if v, ok := coreDrop[string](list); ok {
				words = append(words, v)
			}
		})
	}, 300, 400)
	word, _ := tt.Find("Word")
	b, _ := tt.Find("b")
	drag(tt, word.X+10, word.Y+10, b.X+5, b.Y+5)
	if !slices.Equal(words, []string{"hello"}) {
		t.Errorf("the list took words %q", words)
	}
	a, _ := tt.Find("a")
	c, _ := tt.Find("c")
	drag(tt, a.X, a.Y+a.H/2, c.X, c.Y+c.H)
	if len(moves) != 1 || !slices.Equal(moves[0], []int{0, 3}) {
		t.Errorf("moves %v", moves)
	}
}

// A grid that reorders its items takes other values dropped on it too,
// though it reorders as it builds, before the app's Drop.
func TestGridReorderTakesOtherDrops(t *testing.T) {
	var moves [][]int
	var words []string
	s := GridState{Reorder: func(items []int, to int) {
		moves = append(moves, append(slices.Clone(items), to))
	}}
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Children(func() {
			coreBox(c).Size(100, 40).Label("Word").Drag("hello")
			grid := coreGridView(c, &s, 12, 100, 60, func(i int) {
				coreTextf(c, "Item %d", i)
			}).Grow(1)
			if v, ok := coreDrop[string](grid); ok {
				words = append(words, v)
			}
		})
	}, 448, 400)
	word, _ := tt.Find("Word")
	one, _ := tt.Find("Item 1")
	drag(tt, word.X+10, word.Y+10, one.X+5, one.Y+5)
	if !slices.Equal(words, []string{"hello"}) {
		t.Errorf("the grid took words %q", words)
	}
	from, _ := tt.Find("Item 0")
	to, _ := tt.Find("Item 2")
	drag(tt, from.X, from.Y, to.X+to.W+20, to.Y)
	if len(moves) != 1 || !slices.Equal(moves[0], []int{0, 3}) {
		t.Errorf("moves %v", moves)
	}
}
