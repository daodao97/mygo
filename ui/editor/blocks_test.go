package editor

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/ui"
)

// tall is a block as high as what it holds, h.
func tall(line int, above bool, h float32, label string) Block {
	return Block{Line: line, Above: above, Label: label, Content: func(c *ui.Context, e ui.Element, b Block) {
		e.Children(func() { ui.Box(c).FillWidth().Height(h) })
	}}
}

// settle runs frames until the editor asks for none, as blocks measure
// and place themselves.
func (ct *editorTest) settle() {
	ct.t.Helper()
	ct.tt.Frame()
	for n := 0; ct.tt.FramePending(); n++ {
		if n > 10 {
			ct.t.Fatal("the frames don't end")
		}
		ct.tt.Frame()
	}
}

// standing returns an editor of "top", the lines of a table and "end",
// with a block as high as h in place of the table's lines.
func standing(t *testing.T, h float32) (*editorTest, Replacement) {
	t.Helper()
	src := "top\n: a :\n: b :\nend"
	ct := newEditorTest(t, "", src)
	r := hide(4, 16)
	ct.st.SetReplacements([]Replacement{r})
	ct.st.SetBlocks([]Block{tall(1, false, h, "Table")})
	ct.settle()
	return ct, r
}

// A block takes its room between its line and the next, above or below,
// as high as its element lays out, and the lines around it stay text.
func TestBlocksBetweenLines(t *testing.T) {
	src := "one\ntwo\nthree"
	ct := newEditorTest(t, "", src)
	ct.st.SetBlocks([]Block{tall(0, false, 50, "Under"), tall(2, true, 30, "Over")})
	ct.settle()
	ed := &ct.st.ed
	lh := float64(ed.view.lineHeight)
	if got, want := ed.rowTop(1), lh+50; got != want {
		t.Fatalf("the second line is at %v, want %v", got, want)
	}
	if got, want := ed.rowTop(2), 2*lh+50+30; got != want {
		t.Fatalf("the third line is at %v, want %v", got, want)
	}
	if got, want := ed.view.heights.total(), 3*lh+80; got != want {
		t.Fatalf("the text is %v high, want %v", got, want)
	}
	under, ok := ct.tt.Find("Under")
	if !ok {
		t.Fatal("the block isn't built")
	}
	if want := ed.r.Y + float32(lh); under.Y != want || under.H != 50 {
		t.Fatalf("the block is at %v, %v high, want at %v", under.Y, under.H, want)
	}
	if want := ed.r.X + ed.textX; under.X != want {
		t.Fatalf("the block starts at %v, want %v, with the text", under.X, want)
	}
	if want := ed.r.W - ed.textX - ed.trackWidth() - 4; under.W != want {
		t.Fatalf("the block is %v wide, want %v", under.W, want)
	}
	over, _ := ct.tt.Find("Over")
	if want := ed.r.Y + float32(2*lh+50); over.Y != want {
		t.Fatalf("the block above is at %v, want %v", over.Y, want)
	}
	if got := ct.st.Buffer.String(); got != src {
		t.Fatalf("the buffer holds %q", got)
	}
	if _, ok := ct.tt.AccessByRole(ui.RoleImage, "Under"); !ok {
		t.Fatal("assistive technology doesn't see the block's label")
	}
	var n platform.AccessNode
	ed.accessibility(&n)
	if n.Value != src {
		t.Fatalf("assistive technology reads %q", n.Value)
	}
	// A press on it puts the caret in the line next to it.
	ct.tt.ClickAt(under.X+1, under.Y+under.H/2)
	if got := ct.st.Buffer.Pos(ct.st.Cursor()).Line; got != 0 {
		t.Fatalf("a press on the block went to line %d", got)
	}
	// Carets go past blocks as if they were not there.
	ct.caretTo(0)
	ct.run(MoveDown)
	if got := ct.st.Buffer.Pos(ct.st.Cursor()).Line; got != 1 {
		t.Fatalf("the caret went down to line %d", got)
	}
}

// A block's height follows its element, and frames end once it has.
func TestBlocksMeasure(t *testing.T) {
	h := float32(40)
	ct := newEditorTest(t, "", "one\ntwo")
	guess := tall(0, false, 0, "Under")
	guess.Height = 200
	guess.Content = func(c *ui.Context, e ui.Element, b Block) {
		e.Children(func() { ui.Box(c).FillWidth().Height(h) })
	}
	ct.st.SetBlocks([]Block{guess})
	ct.settle()
	ed := &ct.st.ed
	lh := float64(ed.view.lineHeight)
	if got, want := ed.rowTop(1), lh+40; got != want {
		t.Fatalf("the second line is at %v, want %v", got, want)
	}
	h = 90
	ct.settle()
	if got, want := ed.rowTop(1), lh+90; got != want {
		t.Fatalf("grown, the second line is at %v, want %v", got, want)
	}
}

// A block not built yet is as high as it says, or a row.
func TestBlocksGuess(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("x\n", 2000))
	guess := tall(1500, false, 80, "Far")
	guess.Height = 33
	ct.st.SetBlocks([]Block{guess, tall(1800, false, 80, "Farther")})
	ct.settle()
	ed := &ct.st.ed
	lh := float64(ed.view.lineHeight)
	if got, want := ed.view.heights.total(), 2001*lh+33+lh; got != want {
		t.Fatalf("the text is %v high, want %v", got, want)
	}
	if _, ok := ct.tt.Find("Far"); ok {
		t.Fatal("a block out of view is built")
	}
}

// Only the blocks in view are built, however many there are.
func TestBlocksOnlyInView(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("x\n", 5000))
	built := 0
	bs := make([]Block, 5000)
	for i := range bs {
		bs[i] = Block{Line: i, Height: 10, Content: func(*ui.Context, ui.Element, Block) { built++ }}
	}
	ct.st.SetBlocks(bs)
	ct.settle()
	built = 0
	ct.tt.Frame()
	if built == 0 || built > 100 {
		t.Fatalf("built %d blocks", built)
	}
}

// Blocks go with their lines' text as it is edited, and being set again
// where they are keep their elements and heights.
func TestBlocksMoveWithEdits(t *testing.T) {
	ct := newEditorTest(t, "", "one\ntwo\nthree")
	ct.st.SetBlocks([]Block{tall(1, false, 50, "Under")})
	ct.settle()
	ed := &ct.st.ed
	lineOf := func() int {
		t.Helper()
		bs := ct.st.Blocks()
		if len(bs) != 1 {
			t.Fatalf("%d blocks", len(bs))
		}
		return bs[0].Line
	}
	ct.st.Buffer.Edit(Edit{Start: 0, End: 0, Text: "new\n"})
	ct.settle()
	if got := lineOf(); got != 2 {
		t.Fatalf("after a line above, its line is %d", got)
	}
	// Typed at the start of its line, it stays with the line.
	off := ct.st.Buffer.Offset(Pos{Line: 2})
	ct.st.Buffer.Edit(Edit{Start: off, End: off, Text: "ab"})
	ct.settle()
	if got := lineOf(); got != 2 {
		t.Fatalf("typed in its line, its line is %d", got)
	}
	if off := ct.st.Buffer.Offset(Pos{Line: 2}); ed.view.blocks[0].at != off {
		t.Fatalf("it is at %d, its line at %d", ed.view.blocks[0].at, off)
	}
	id := ed.view.blocks[0].id
	// The app sets them again where the edits moved them.
	ct.st.SetBlocks([]Block{tall(2, false, 50, "Under")})
	if b := &ed.view.blocks[0]; b.id != id || !b.measured || b.height != 50 {
		t.Fatalf("set again, it is %+v", b)
	}
	// Elsewhere, it is another.
	ct.st.SetBlocks([]Block{tall(0, false, 50, "Under")})
	if b := &ed.view.blocks[0]; b.id == id || b.measured {
		t.Fatalf("set elsewhere, it is %+v", b)
	}
	ct.settle()
	// Its line deleted, it stays where the line was.
	ct.st.Buffer.Edit(Edit{Start: 0, End: 4, Text: ""})
	ct.settle()
	if got := lineOf(); got != 0 {
		t.Fatalf("its line deleted, its line is %d", got)
	}
	if ct.st.SetBlocks([]Block{tall(99, false, 1, "No")}); len(ct.st.Blocks()) != 0 {
		t.Fatal("a block of a line that doesn't exist is kept")
	}
}

// Above and below a line stay in that order, as joined lines bring them
// together.
func TestBlocksJoinLines(t *testing.T) {
	ct := newEditorTest(t, "", "one\ntwo")
	ct.st.SetBlocks([]Block{tall(0, false, 10, "Under"), tall(1, true, 20, "Over")})
	ct.settle()
	ct.st.Buffer.Edit(Edit{Start: 3, End: 4, Text: ""})
	ct.settle()
	bs := ct.st.Blocks()
	if len(bs) != 2 || !bs[0].Above || bs[1].Above || bs[0].Line != 0 || bs[1].Line != 0 {
		t.Fatalf("joined: %+v", bs)
	}
}

// A fold hides the blocks of its lines.
func TestBlocksFold(t *testing.T) {
	ct := newEditorTest(t, "go", "func a() {\n\txx()\n}\nz")
	ct.st.SetBlocks([]Block{tall(1, false, 50, "Inner")})
	ct.settle()
	if _, ok := ct.tt.Find("Inner"); !ok {
		t.Fatal("the block isn't built")
	}
	ct.st.Fold(0)
	ct.settle()
	if _, ok := ct.tt.Find("Inner"); ok {
		t.Fatal("a folded line's block is built")
	}
	if got, want := ct.st.ed.view.heights.total(), 3*float64(ct.st.ed.view.lineHeight); got != want {
		t.Fatalf("folded, the text is %v high, want %v", got, want)
	}
}

// A block on lines a replacement hides shows in their place.
func TestBlocksStandIn(t *testing.T) {
	ct, r := standing(t, 60)
	ed := &ct.st.ed
	lh := float64(ed.view.lineHeight)
	if got, want := ed.view.heights.total(), 2*lh+60; got != want {
		t.Fatalf("the text is %v high, want %v", got, want)
	}
	if got, want := ed.rowTop(3), lh+60; got != want {
		t.Fatalf("the line after is at %v, want %v", got, want)
	}
	room, ok := ct.tt.Find("Table")
	if !ok {
		t.Fatal("the block isn't built")
	}
	if want := ed.r.Y + float32(lh); room.Y != want || room.H != 60 {
		t.Fatalf("the block is at %v, %v high, want at %v", room.Y, room.H, want)
	}
	if got := ed.view.heights.lineAt(lh + 30); got != 1 {
		t.Fatalf("the height of the block is line %d's", got)
	}
	var n platform.AccessNode
	ed.accessibility(&n)
	if !strings.Contains(n.Value, ": a :") {
		t.Fatalf("assistive technology reads %q", n.Value)
	}

	// A press on it puts the caret at its start, for the app to show the
	// source there.
	ct.tt.ClickAt(room.X+10, room.Y+room.H/2)
	ct.wantCursor(r.Start)

	// Carets stop on it once, each way, and step over it.
	ct.caretTo(3)
	ct.run(MoveRight)
	ct.wantCursor(r.Start)
	ct.run(MoveRight)
	ct.wantCursor(r.End)
	ct.run(MoveLeft)
	ct.wantCursor(r.Start)
	ct.run(MoveLeft)
	ct.wantCursor(3)
	ct.caretTo(1)
	ct.run(MoveDown)
	ct.wantCursor(r.Start)
	ct.run(MoveDown)
	if got := ct.st.Buffer.Pos(ct.st.Cursor()).Line; got != 3 {
		t.Fatalf("down from the block went to line %d", got)
	}
	ct.run(MoveUp)
	ct.wantCursor(r.Start)
	ct.run(MoveUp)
	if got := ct.st.Buffer.Pos(ct.st.Cursor()).Line; got != 0 {
		t.Fatalf("up from the block went to line %d", got)
	}
	if cr, ok := ed.caretRect(r.Start); !ok || float64(cr.Y) != lh {
		t.Fatalf("the caret on it is at %+v, %v", cr, ok)
	}

	// Deleting next to it deletes all of it, as any replacement.
	ct.caretTo(r.End)
	ct.run(Backspace)
	if got := ct.st.Buffer.String(); got != "top\nend" {
		t.Fatalf("deleted to %q", got)
	}
}

// Only an app block's own interval maps to its hidden line, not the
// sticky diff blocks sharing that line, above or below the text.
func TestBlocksStandInLineAtInterval(t *testing.T) {
	h := heights{lh: 20}
	h.setLines(4)
	h.setHidden([]lineSpan{{1, 2}})
	bs := []block{
		{line: 1, kind: blockDeleted, height: 20, sticky: true},
		{line: 1, kind: blockApp, height: 30, sticky: true},
		{line: 1, kind: blockSpacer, height: 10, sticky: true},
		{line: 1, kind: blockApp, below: true, height: 40, sticky: true},
		{line: 1, kind: blockDeleted, below: true, height: 10, sticky: true},
		// A hidden nonsticky app block takes no room.
		{line: 1, kind: blockApp, below: true, height: 50},
	}
	h.setBlocks(bs)
	top := h.top(1)
	for _, b := range bs {
		if !b.sticky {
			continue
		}
		bottom := top + float64(b.height)
		want := 0
		if b.kind == blockApp {
			want = b.line
		}
		for _, y := range []float64{top, (top + bottom) / 2, bottom - 1} {
			if got := h.lineAt(y); got != want {
				t.Errorf("kind %d at height %v: line %d, want %d", b.kind, y, got, want)
			}
		}
		top = bottom
	}
	if got := h.lineAt(top); got != 3 {
		t.Fatalf("past the blocks: line %d, want 3", got)
	}
}

// A hidden line's block stays visible exactly when it can stand in. A
// partial-line or visible replacement keeps its text, not a second caret
// target inside its hidden bytes.
func TestBlocksStandInEligibility(t *testing.T) {
	const src = "top\nprefix hidden\ncovered\nend"
	for _, tc := range []struct {
		name   string
		mid    bool
		text   string
		stands bool
	}{
		{name: "whole-line empty", stands: true},
		{name: "mid-line empty", mid: true},
		{name: "whole-line shown text", text: "preview"},
		{name: "mid-line shown text", mid: true, text: "preview"},
	} {
		for _, above := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/below", true: "/above"}[above], func(t *testing.T) {
				ct := newEditorTest(t, "", src)
				buf := ct.st.Buffer
				start := buf.Offset(Pos{Line: 1})
				if tc.mid {
					start = strings.Index(src, "hidden")
				}
				r := Replacement{Start: start, End: buf.Offset(Pos{Line: 3}), Text: tc.text}
				ct.st.SetReplacements([]Replacement{r})
				ct.st.SetBlocks([]Block{tall(2, above, 60, "Covered")})
				ct.settle()
				ed := &ct.st.ed
				if !ed.view.heights.isHidden(2) {
					t.Fatal("the replacement's covered line is not hidden")
				}
				_, stands := ed.standIn(2)
				if stands != tc.stands {
					t.Errorf("stands in: %v, want %v", stands, tc.stands)
				}
				if rows := ed.blockRows(); len(rows) != 1 || rows[0].sticky != stands {
					t.Errorf("block rows %+v disagree with stand-in %v", rows, stands)
				}
				room, built := ct.tt.Find("Covered")
				if built != tc.stands {
					t.Errorf("block built: %v, want %v", built, tc.stands)
				}
				if built {
					ct.tt.ClickAt(room.X+10, room.Y+room.H/2)
					if got := ct.st.Cursor(); got != r.Start {
						t.Errorf("a press on the block went to %d, want replacement start %d", got, r.Start)
					}
				}
				lh := float64(ed.view.lineHeight)
				wantHeight := 2 * lh
				if tc.stands {
					wantHeight += 60
				} else {
					wantHeight += lh // The start line still shows its text.
					wantText := tc.text
					if tc.mid {
						wantText = "prefix " + wantText
					}
					if got := ed.view.line(1).disp; got != wantText {
						t.Errorf("displayed %q, want %q", got, wantText)
					}
				}
				if got := ed.view.heights.total(); got != wantHeight {
					t.Errorf("height %v, want %v", got, wantHeight)
				}
				if got := buf.String(); got != src {
					t.Fatalf("the buffer changed to %q", got)
				}
				ct.caretTo(r.Start)
				ct.run(MoveRight)
				ct.wantCursor(r.End)
				ct.run(MoveLeft)
				ct.wantCursor(r.Start)
			})
		}
	}
}

// Hidden lines next to a block standing in don't take the caret past it.
func TestBlocksStandInBesideHidden(t *testing.T) {
	src := "top\ngone\ntable\nend"
	ct := newEditorTest(t, "", src)
	gone, table := hide(4, 9), hide(9, 15)
	ct.st.SetReplacements([]Replacement{gone, table})
	ct.st.SetBlocks([]Block{tall(2, false, 40, "Table")})
	ct.settle()
	ct.caretTo(3)
	ct.run(MoveRight)
	ct.wantCursor(table.Start)
	ct.run(MoveRight)
	ct.wantCursor(table.End)
	ct.run(MoveLeft)
	ct.wantCursor(table.Start)
	ct.run(MoveLeft)
	ct.wantCursor(3)
}

// A replacement coming or going over a block's line, and a fold over
// lines it hides, say whether the block shows.
func TestBlocksStandInFollows(t *testing.T) {
	ct := newEditorTest(t, "go", "func a() {\n\txx()\n}\nz")
	ed := &ct.st.ed
	ct.st.SetBlocks([]Block{tall(1, false, 50, "Inner")})
	ct.settle()
	lh := float64(ed.view.lineHeight)
	start := ct.st.Buffer.Offset(Pos{Line: 1})
	end := ct.st.Buffer.Offset(Pos{Line: 2})
	ct.st.SetReplacements([]Replacement{hide(start, end)})
	ct.settle()
	if got, want := ed.view.heights.total(), 3*lh+50; got != want {
		t.Fatalf("standing in, the text is %v high, want %v", got, want)
	}
	// The fold hides just the lines the replacement does.
	ct.st.Fold(0)
	ct.settle()
	if got, want := ed.view.heights.total(), 3*lh; got != want {
		t.Fatalf("folded, the text is %v high, want %v", got, want)
	}
	ct.st.Unfold(0)
	ct.settle()
	if got, want := ed.view.heights.total(), 3*lh+50; got != want {
		t.Fatalf("unfolded, the text is %v high, want %v", got, want)
	}
}

// A pointer resting on a block rests on no text.
func TestBlocksNoHover(t *testing.T) {
	ct := newEditorTest(t, "", "one\ntwo")
	ct.st.SetBlocks([]Block{tall(1, true, 60, "Over")})
	ct.settle()
	r, _ := ct.tt.Find("Over")
	ct.st.OnHover = func(HoverRequest) {}
	ct.settle()
	ct.tt.Move(r.X+2, r.Y+r.H/2)
	if h := &ct.st.ed.hover; h.pending {
		t.Fatalf("a hover is waiting at %d", h.restOff)
	}
}

// The source comes back with the replacement gone, the block with it.
func TestBlocksStandInReveal(t *testing.T) {
	ct, _ := standing(t, 60)
	ed := &ct.st.ed
	ct.st.SetReplacements(nil)
	ct.st.SetBlocks(nil)
	ct.settle()
	if got, want := ed.view.heights.total(), 4*float64(ed.view.lineHeight); got != want {
		t.Fatalf("the text is %v high, want %v", got, want)
	}
	if _, ok := ct.tt.Find("Table"); ok {
		t.Fatal("the block is still built")
	}
}

// A fold over a block standing in hides it, and carets don't stop there.
func TestBlocksStandInFolded(t *testing.T) {
	ct := newEditorTest(t, "go", "func a() {\n\txx()\n\tyy()\n}\nz")
	start := ct.st.Buffer.Offset(Pos{Line: 1})
	end := ct.st.Buffer.Offset(Pos{Line: 2})
	ct.st.SetReplacements([]Replacement{hide(start, end)})
	ct.st.SetBlocks([]Block{tall(1, false, 50, "Inner")})
	ct.settle()
	if _, ok := ct.tt.Find("Inner"); !ok {
		t.Fatal("the block isn't built")
	}
	ct.st.Fold(0)
	ct.settle()
	if _, ok := ct.tt.Find("Inner"); ok {
		t.Fatal("a folded block is built")
	}
	if _, ok := ct.st.ed.standIn(1); ok {
		t.Fatal("a folded block stands in")
	}
	if rows := ct.st.ed.blockRows(); len(rows) != 1 || rows[0].sticky {
		t.Fatalf("a folded block has sticky rows: %+v", rows)
	}
	ct.st.Unfold(0)
	ct.settle()
	if _, ok := ct.tt.Find("Inner"); !ok {
		t.Fatal("the unfolded stand-in block is not built")
	}
	if _, ok := ct.st.ed.standIn(1); !ok {
		t.Fatal("the unfolded block does not stand in")
	}
}

// What shows of the selections on a block standing in: behind it when
// selected, and the caret before it.
func TestBlocksStandInPaint(t *testing.T) {
	theme := &Theme{Background: ui.RGB(255, 255, 255), Selection: ui.RGB(0, 0, 255), InactiveSelection: ui.RGB(0, 0, 255), Cursor: ui.RGB(255, 0, 0)}
	ct, r := standing(t, 60)
	ct.st.Theme, ct.st.NoActiveLine = theme, true
	ct.settle()
	at := func(x, y float32) (uint8, uint8, uint8) {
		img := ct.tt.Image()
		s := float32(img.Bounds().Dx()) / 600
		c := img.RGBAAt(int(x*s), int(y*s))
		return c.R, c.G, c.B
	}
	room, _ := ct.tt.Find("Table")
	if r, g, b := at(room.X+10, room.Y+30); r != 255 || g != 255 || b != 255 {
		t.Fatalf("something drew in the block's room: %d %d %d", r, g, b)
	}
	ct.st.Select(0, len(ct.st.Buffer.String()))
	ct.settle()
	if r, g, b := at(room.X+10, room.Y+30); b < 200 || r > 60 || g > 60 {
		t.Fatalf("the selection doesn't cover it: %d %d %d", r, g, b)
	}
	ct.caretTo(r.Start)
	ct.tt.Frame()
	if r, g, b := at(room.X-2, room.Y+30); r < 200 || g > 60 || b > 60 {
		t.Fatalf("the caret doesn't show before it: %d %d %d", r, g, b)
	}
}

// An interactive block takes its presses, the wheel still scrolling.
func TestBlocksInteractive(t *testing.T) {
	ct := newEditorTest(t, "", "one\n"+strings.Repeat("more\n", 200))
	clicks := 0
	ct.st.SetBlocks([]Block{{Line: 0, Interactive: true, Content: func(c *ui.Context, e ui.Element, b Block) {
		e.Children(func() {
			btn := ui.ButtonBase(c).Size(80, 24).KeepFocus().Label("Open")
			if btn.Clicked() {
				clicks++
			}
		})
	}}})
	ct.settle()
	if err := ct.tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	ct.settle()
	if clicks != 1 {
		t.Fatalf("clicked %d times", clicks)
	}
	if !ct.st.Focused() {
		t.Fatal("the click took the keyboard from the editor")
	}
	r, ok := ct.tt.Find("Open")
	if !ok {
		t.Fatal("the button isn't built")
	}
	ct.tt.Wheel(r.X+r.W/2, r.Y+r.H/2, 0, 60)
	ct.settle()
	if ct.st.ScrollY() == 0 {
		t.Fatal("the wheel over the block didn't scroll")
	}
}

// Blocks follow the scroll, and a block measured above the view leaves
// what shows where it is.
func TestBlocksScroll(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("x\n", 400))
	ct.st.SetBlocks([]Block{tall(5, false, 100, "Under")})
	ct.settle()
	ed := &ct.st.ed
	room, _ := ct.tt.Find("Under")
	ct.tt.Wheel(room.X+1, room.Y+1, 0, 40)
	ct.settle()
	moved, ok := ct.tt.Find("Under")
	if !ok {
		t.Fatal("the block went")
	}
	if got, want := moved.Y, room.Y-float32(ed.scrollY); got != want {
		t.Fatalf("the block is at %v, want %v", got, want)
	}
	if ed.elementsMoved() {
		t.Fatal("placed blocks count as moved")
	}
	ed.scrollY += 10
	if !ed.elementsMoved() {
		t.Fatal("scrolling doesn't count as moving them")
	}
	ed.scrollY -= 10

	// Keep the block partly visible so a real build measures its growth
	// above a text line that was already on screen, for either placement.
	for _, above := range []bool{false, true} {
		t.Run(map[bool]string{false: "below", true: "above"}[above], func(t *testing.T) {
			ct := newEditorTest(t, "", strings.Repeat("x\n", 400))
			ed := &ct.st.ed
			height := float32(80)
			builds := 0
			line := 2
			if above {
				line++
			}
			ct.st.SetBlocks([]Block{{Line: line, Above: above, Label: "High", Content: func(c *ui.Context, e ui.Element, b Block) {
				builds++
				e.Children(func() { ui.Box(c).FillWidth().Height(height) })
			}}})
			ct.st.LineBackgrounds = []LineBackground{{First: 3, Last: 3, Color: ui.RGB(0, 255, 0)}}
			ct.settle()
			ct.st.SetScrollY(ed.rowTop(3) - float64(ed.view.lineHeight)/2)
			ct.settle()
			room, ok := ct.tt.Find("High")
			// Find reports the clipped visible box. Only the bottom of this
			// already measured block is on screen, at the viewport's top.
			if !ok || len(ed.built) != 1 || room.Y != ed.r.Y || room.H <= 0 || room.H >= height {
				t.Fatalf("the block is not built and partly visible above the viewport: %+v (%v)", room, ok)
			}
			// Read rendered pixels, not the scroll anchor's own identity equation.
			screenY := func() int {
				img := ct.tt.Image()
				x := img.Bounds().Dx() / 2
				for y := 0; y < img.Bounds().Dy(); y++ {
					c := img.RGBAAt(x, y)
					if c.R == 0 && c.G == 255 && c.B == 0 {
						return y
					}
				}
				t.Fatal("the marked text line is not visible")
				return -1
			}
			beforeY, beforeScroll := screenY(), ed.scrollY
			beforeHeight, beforeBuilds := ed.view.blocks[0].height, builds
			if !ed.view.blocks[0].measured || beforeHeight != height {
				t.Fatalf("the block's initial height was not measured: %+v", ed.view.blocks[0])
			}
			height += 60
			ct.settle()
			b := &ed.view.blocks[0]
			if builds <= beforeBuilds || !b.measured || b.height != height || b.height == beforeHeight {
				t.Fatalf("the block's height increase was not built and measured: %+v, builds %d to %d", b, beforeBuilds, builds)
			}
			if got, want := ed.scrollY, beforeScroll+float64(height-beforeHeight); got != want {
				t.Errorf("the measured growth scrolled to %v, want %v", got, want)
			}
			if afterY := screenY(); afterY != beforeY {
				t.Errorf("the visible text line moved from screen y %d to %d", beforeY, afterY)
			}
		})
	}
}

// When the viewport is entirely within a block, keep its internal scroll
// position rather than anchoring text that is not visible yet.
func TestBlocksScrollWithinTallBlock(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("x\n", 400))
	height := float32(800)
	ct.st.SetBlocks([]Block{{Line: 0, Content: func(c *ui.Context, e ui.Element, b Block) {
		e.Children(func() { ui.Box(c).FillWidth().Height(height) })
	}}})
	ct.settle()
	ed := &ct.st.ed
	ct.st.SetScrollY(float64(ed.view.lineHeight) + 20)
	ct.settle()
	before := ed.scrollY
	if ed.rowTop(1) <= before+float64(ed.viewH) {
		t.Fatal("text after the block is unexpectedly visible")
	}
	height += 60
	ct.settle()
	if b := &ed.view.blocks[0]; !b.measured || b.height != height {
		t.Fatalf("the taller block was not measured: %+v", b)
	}
	if ed.scrollY != before {
		t.Fatalf("scrolling within a tall block moved from %v to %v", before, ed.scrollY)
	}
}

// An editor as high as its content counts its blocks.
func TestBlocksFitContent(t *testing.T) {
	st := &State{Buffer: NewBuffer("one\ntwo"), FitContent: true}
	st.SetBlocks([]Block{tall(0, false, 70, "Under")})
	var height float32
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() { height = View(c, st).FillWidth().Bounds().H })
	}, 600, 400)
	tt.Frame()
	for n := 0; tt.FramePending() && n < 10; n++ {
		tt.Frame()
	}
	tt.Frame()
	want := 2*st.ed.view.lineHeight + 70
	if height != want {
		t.Fatalf("the editor is %v high, want %v", height, want)
	}
	if r, ok := tt.Find("Under"); !ok || r.H != 70 {
		t.Fatalf("the block is %+v, %v", r, ok)
	}
}

func TestBlocksNewBuffer(t *testing.T) {
	ct := newEditorTest(t, "", "one\ntwo")
	ct.st.SetBlocks([]Block{tall(0, false, 50, "Under")})
	ct.settle()
	ct.st.Buffer = NewBuffer("other")
	ct.settle()
	if len(ct.st.Blocks()) != 0 {
		t.Fatal("the blocks outlived their buffer")
	}
	if _, ok := ct.tt.Find("Under"); ok {
		t.Fatal("the block is still built")
	}
}

// A page moves by blocks standing in as by rows, not stopping at each.
func TestBlocksStandInPage(t *testing.T) {
	var b strings.Builder
	var rs []Replacement
	var bs []Block
	for i := range 200 {
		start := b.Len()
		b.WriteString(": a :\n")
		if i%4 == 0 {
			rs = append(rs, hide(start, b.Len()))
			bs = append(bs, Block{Line: i, Height: 20, Content: func(*ui.Context, ui.Element, Block) {}})
		}
	}
	ct := newEditorTest(t, "", b.String())
	ct.st.SetReplacements(rs)
	ct.st.SetBlocks(bs)
	ct.settle()
	ct.caretTo(ct.st.Buffer.Offset(Pos{Line: 1}))
	ct.run(MovePageDown)
	if got := ct.st.Buffer.Pos(ct.st.Cursor()).Line; got < 10 {
		t.Fatalf("a page down went to line %d", got)
	}
}

// Backward movement stops at the first shown text or stand-in, never
// an invisible zero. Forward movement must still cross the replacements.
func TestBlocksStandInWalkHiddenPrefix(t *testing.T) {
	for _, prefix := range []struct{ name, text string }{
		{"none", ""}, {"empty-line", "\n"}, {"one", "gone\n"}, {"multiple", "gone\nalso gone\n"},
	} {
		for _, withBlock := range []bool{false, true} {
			for _, tail := range []string{"", "end"} {
				for _, step := range []struct {
					name string
					cmd  Command
					fwd  bool
				}{{"left", MoveLeft, false}, {"up", MoveUp, false}, {"page-up", MovePageUp, false}, {"right", MoveRight, true}, {"down", MoveDown, true}} {
					name := prefix.name + "/" + map[bool]string{false: "text", true: "stand-in"}[withBlock] + "/" + tail + "/" + step.name
					t.Run(name, func(t *testing.T) {
						src := prefix.text
						var rs []Replacement
						if src != "" {
							rs = append(rs, hide(0, len(src)))
						}
						if withBlock {
							src += "table\n"
							rs = append(rs, hide(len(prefix.text), len(src)))
						}
						src += tail
						ct := newEditorTest(t, "", src)
						ct.st.SetReplacements(rs)
						if withBlock {
							line := ct.st.Buffer.Pos(len(prefix.text)).Line
							ct.st.SetBlocks([]Block{tall(line, false, 40, "Block")})
						}
						ct.settle()
						ed := &ct.st.ed
						start, want := len(src), len(prefix.text)
						if step.fwd {
							start, want = 0, len(src)
						}
						ct.st.Select(start, start)
						visited := start == len(prefix.text)
						for n := 0; ; n++ {
							at := ct.st.Cursor()
							ed.run(step.cmd)
							to := ct.st.Cursor()
							visited = visited || to == len(prefix.text)
							if _, ok := ed.caretRect(to); !ok {
								t.Fatalf("%v moved from %d to an invisible stop %d, want endpoint %d", step.cmd, at, to, want)
							}
							if to == at {
								if to != want {
									t.Fatalf("%v stopped at %d, want endpoint %d", step.cmd, to, want)
								}
								break
							}
							if (to > at) != step.fwd || n > len(src)+1 {
								t.Fatalf("%v moved from %d to %d, want endpoint %d", step.cmd, at, to, want)
							}
						}
						if withBlock && !visited {
							t.Fatal("movement skipped the valid stand-in stop")
						}
					})
				}
			}
		}
	}
}

// Navigation merges replacement spans without changing cached folds.
// The overlapping folds leave spare capacity in the merged fold slice.
func TestBlocksStandInNavigationKeepsFoldCache(t *testing.T) {
	ct := newEditorTest(t, "", "head\nfolded\nhidden\nend")
	ed := &ct.st.ed
	r := hide(ct.st.Buffer.Offset(Pos{Line: 2}), ct.st.Buffer.Offset(Pos{Line: 3}))
	ct.st.SetReplacements([]Replacement{r})
	ct.st.SetBlocks([]Block{tall(2, false, 40, "Block")})
	ed.folds = []fold{{0, 1}, {0, 1}}
	ct.settle()
	ct.caretTo(ed.view.text.LineEnd(0))
	ct.run(MoveRight)
	ct.wantCursor(r.Start)
	spans := ed.hiddenSpans()
	if len(spans) != 1 || spans[0] != (lineSpan{1, 1}) {
		t.Fatalf("navigation changed the cached folds: %+v", spans)
	}
	if _, ok := ed.standIn(2); !ok {
		t.Fatal("navigation folded the stand-in away")
	}
}

// Walking ends only at the last text offset or the first shown text or
// stand-in, not at an arbitrary no-progress step or an invisible zero.
func TestBlocksStandInWalkEnds(t *testing.T) {
	rng := rand.New(rand.NewPCG(1651, 2))
	for range 150 {
		var b strings.Builder
		for range 1 + rng.IntN(12) {
			b.WriteString([]string{"", "a", ": b :", "語語"}[rng.IntN(4)])
			b.WriteByte('\n')
		}
		src := b.String()
		ct := newEditorTest(t, "", src)
		buf := ct.st.Buffer
		lines := buf.LineCount()
		var rs []Replacement
		var bs []Block
		for range rng.IntN(5) {
			first := rng.IntN(lines)
			last := min(lines-1, first+1+rng.IntN(3))
			rs = append(rs, hide(buf.Offset(Pos{Line: first}), buf.Offset(Pos{Line: last})))
			if rng.IntN(4) > 0 {
				bs = append(bs, Block{Line: first, Height: 15, Content: func(*ui.Context, ui.Element, Block) {}})
			}
		}
		ct.st.SetReplacements(rs)
		ct.st.SetBlocks(bs)
		ct.st.SoftWrap = rng.IntN(2) == 0
		ct.tt.Frame()
		ed := &ct.st.ed
		for _, step := range []struct {
			cmd Command
			fwd bool
		}{{MoveRight, true}, {MoveLeft, false}, {MoveDown, true}, {MoveUp, false}} {
			want := firstShownCaret(t, ed)
			if step.fwd {
				ct.st.Select(0, 0)
				want = len(src)
			} else {
				ct.st.Select(len(src), len(src))
			}
			for n := 0; ; n++ {
				at := ct.st.Cursor()
				ed.run(step.cmd)
				to := ct.st.Cursor()
				if _, ok := ed.caretRect(to); !ok {
					t.Fatalf("%q with %+v and %+v: %v from %d left an invisible caret at %d", src, rs, bs, step.cmd, at, to)
				}
				if to == at {
					if to != want {
						t.Fatalf("%q with %+v and %+v: %v stopped at %d, want endpoint %d, step %d", src, rs, bs, step.cmd, to, want, n)
					}
					break
				}
				if (to > at) != step.fwd || n > len(src)+1 {
					t.Fatalf("%q with %+v and %+v: %v from %d went to %d, step %d", src, rs, bs, step.cmd, at, to, n)
				}
			}
		}
	}
}
