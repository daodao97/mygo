package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Deleting a block's line attaches it to the surviving line at that
// position, without changing whether it goes above or below that line.
func TestBlocksDeletedLinePlacement(t *testing.T) {
	for _, tc := range []struct {
		name, source, survivor string
		line, want             int
	}{
		{"first", "delete\nsurvivor\ntail", "survivor", 0, 0},
		{"middle", "head\ndelete\nsurvivor\ntail", "survivor", 1, 1},
		{"last", "survivor\ndelete", "survivor", 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ct := newEditorTest(t, "", tc.source)
			ct.st.SetBlocks([]Block{tall(tc.line, true, 20, "Above"), tall(tc.line, false, 30, "Below")})
			ct.settle()
			ed := &ct.st.ed
			ids := [2]int{ed.view.blocks[0].id, ed.view.blocks[1].id}
			start := ct.st.Buffer.Offset(Pos{Line: tc.line})
			end := len(tc.source)
			if tc.line+1 < ct.st.Buffer.LineCount() {
				end = ct.st.Buffer.Offset(Pos{Line: tc.line + 1})
			} else {
				start-- // Delete the preceding newline with the final line.
			}
			ct.st.Buffer.Edit(Edit{Start: start, End: end})
			ct.settle()
			if got := ct.st.Buffer.Line(tc.want); got != tc.survivor {
				t.Fatalf("surviving line is %q, want %q", got, tc.survivor)
			}
			for i, b := range ct.st.Blocks() {
				if b.Line != tc.want || b.Above != (i == 0) || ed.view.blocks[i].id != ids[i] {
					t.Fatalf("block %d changed placement or identity: %+v", i, b)
				}
			}
			for _, label := range []string{"Above", "Below"} {
				room, ok := ct.tt.Find(label)
				want := float32(ed.rowTop(tc.want)) + ed.view.lineHeight
				if label == "Above" {
					want = float32(ed.rowTop(tc.want)) - 20
				}
				if !ok || room.Y != ed.r.Y+want {
					t.Fatalf("%s is at %+v (%v), want y %v", label, room, ok, ed.r.Y+want)
				}
			}
		})
	}
}

// The block follows the actual wrap constraint, even with wrapping off.
// Known ASCII column breaks independently check that constraint, rather
// than restating the textWidth formula shared by blocks and text.
func TestBlocksTextWidth(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("x", 256))
	// Above the first line, the block stays visible even at the minimum
	// width, where the text wraps through more than a viewport of rows.
	ct.st.SetBlocks([]Block{tall(0, true, 40, "Width")})
	measure := func(column, width int) (float32, int) {
		t.Helper()
		ct.st.WrapColumn = column
		ct.tt.SetSize(width, 400)
		ct.st.SoftWrap = false
		ct.settle()
		plain, ok := ct.tt.Find("Width")
		if !ok || len(ct.st.ed.view.line(0).rows) != 1 {
			t.Fatalf("column %d, width %d: missing block or wrapped text with SoftWrap off", column, width)
		}
		ct.st.SoftWrap = true
		ct.settle()
		room, ok := ct.tt.Find("Width")
		v := &ct.st.ed.view
		if !ok || room.W != plain.W || room.W != v.wrapWidth {
			t.Fatalf("column %d, width %d: block width %v (found %v), unwrapped %v, text constraint %v", column, width, room.W, ok, plain.W, v.wrapWidth)
		}
		l := v.line(0)
		if len(l.rows) < 2 {
			t.Fatal("the long text did not wrap")
		}
		return room.W, l.rows[0].to - l.rows[0].from
	}
	wide, wideChars := measure(0, 600)
	narrow, narrowChars := measure(0, 180)
	wider, widerChars := measure(0, 900)
	if !(narrow < wide && wide < wider && narrowChars < wideChars && wideChars < widerChars) {
		t.Fatalf("resize did not change block width and actual row capacity together: widths %v/%v/%v, characters %d/%d/%d", narrow, wide, wider, narrowChars, wideChars, widerChars)
	}
	checkColumn := func(column int) (float32, int) {
		t.Helper()
		width, chars := measure(column, 600)
		// Flooring DIP widths can cost one ASCII cell, but never add one.
		if chars < column-1 || chars > column {
			t.Fatalf("WrapColumn %d put %d characters in the first text row", column, chars)
		}
		return width, chars
	}
	col20, chars20 := checkColumn(20)
	if width, chars := measure(20, 900); width != col20 || chars != chars20 {
		t.Fatal("a wider viewport escaped the 20-column constraint")
	}
	if width, chars := measure(20, 180); width >= col20 || chars >= chars20 {
		t.Fatal("a narrow viewport did not constrain the block and text together")
	}
	col40, _ := checkColumn(40)
	if col40 <= col20 {
		t.Fatal("a larger column did not widen the block")
	}
	minimum, minChars := measure(1, 600)
	col8, chars8 := checkColumn(8)
	if minimum != col8 || minChars != chars8 {
		t.Fatal("the minimum width is not the observed 8-column layout")
	}
	if width, chars := measure(0, 600); width != wide || chars != wideChars {
		t.Fatal("removing WrapColumn reused a stale block width or text layout")
	}
}

// Replacement byte offsets and block indices name different things, even
// when elements of both kinds share the same overlay.
func TestPlacedElementIdentities(t *testing.T) {
	ct := newEditorTest(t, "", "head\nobject\ntail")
	r := Replacement{Start: ct.st.Buffer.Offset(Pos{Line: 1}), End: ct.st.Buffer.Offset(Pos{Line: 2}), Content: func(c *ui.Context, e ui.Element, r Replacement) {
		e.Children(func() { ui.Box(c).Fill() })
	}}
	ct.st.SetReplacements([]Replacement{r})
	ct.st.SetBlocks([]Block{tall(0, false, 20, "First"), tall(2, true, 30, "Second")})
	ct.settle()
	ps := ct.st.ed.placed
	if len(ps) != 3 {
		t.Fatalf("placed %d elements, want 1 replacement and 2 blocks", len(ps))
	}
	if ps[0].block || ps[0].start != r.Start || ps[0].blockIndex != 0 {
		t.Fatalf("the replacement lost its byte offset: %+v", ps[0])
	}
	for i, p := range ps[1:] {
		if !p.block || p.blockIndex != i || p.start != 0 {
			t.Fatalf("block %d overloads a text offset: %+v", i, p)
		}
	}
}

// IME anchors and painted stand-in carets share one full-height cell.
// Blocks stay fixed horizontally while the source scrolls sideways.
func TestBlocksStandInCaretGeometryAndShape(t *testing.T) {
	for _, shape := range []CursorShape{CursorBar, CursorBlock, CursorUnderline} {
		for _, focused := range []bool{true, false} {
			ct := newEditorTest(t, "", strings.Repeat("x", 200)+"\ntable\nsource\nend")
			ct.st.Theme, ct.st.NoActiveLine, ct.st.CursorShape = redTheme, true, shape
			r := hide(ct.st.Buffer.Offset(Pos{Line: 1}), ct.st.Buffer.Offset(Pos{Line: 3}))
			ct.st.SetReplacements([]Replacement{r})
			ct.st.SetBlocks([]Block{tall(1, false, 30, "First"), tall(2, true, 50, "Second")})
			ct.st.Select(r.Start, r.Start)
			ct.settle()
			ed := &ct.st.ed
			ed.scrollX = 37
			ed.reveal = revealFit
			ct.tt.SetFocused(focused)
			ct.settle()
			if ed.scrollX != 37 {
				t.Fatalf("shape %v, focused %v: revealing a fixed block changed horizontal scroll to %v", shape, focused, ed.scrollX)
			}
			want := ui.Rect{X: ed.scrollX - 3, Y: ed.view.lineHeight, W: 2, H: 80}
			if shape != CursorBar {
				want.W = min(ed.view.charWidth, ed.textX-ed.gutterW-1)
				want.X = ed.scrollX - 1 - want.W
			}
			cr, ok := ed.caretRect(r.Start)
			if !ok || cr != want {
				t.Fatalf("shape %v, focused %v: caret %+v (%v), want %+v", shape, focused, cr, ok, want)
			}
			offset := ed.view.text.UTF16(r.Start)
			bounds, _, ok := ed.BoundsForRange(ui.TextInputRange{Start: offset, End: offset})
			if !ok || bounds != (ui.Rect{X: ed.textX - 1 - want.W, Y: cr.Y - float32(ed.scrollY), W: 1, H: cr.H}) {
				t.Fatalf("IME bounds %+v (%v) disagree with the stand-in cell %+v", bounds, ok, cr)
			}
			x, y := ed.r.X+ed.textX-1-want.W, ed.r.Y+cr.Y-float32(ed.scrollY)
			red := func(px, py float32) bool {
				r, g, _ := ct.pixel(px, py)
				return r > 200 && g < 230
			}
			middle := red(x+want.W/2, y+40)
			top, bottom := red(x+want.W/2, y+0.5), red(x+want.W/2, y+79.5)
			switch {
			case !focused && shape != CursorBlock:
				if middle || top || bottom {
					t.Fatalf("an unfocused %v caret is visible", shape)
				}
			case shape == CursorBar:
				if !middle || !top || !bottom || red(x+4, y+40) {
					t.Fatal("the bar does not cover the full stand-in at its insertion point")
				}
			case shape == CursorBlock && focused:
				r, g, b := ct.pixel(x+want.W/2, y+40)
				if !middle || !top || !bottom || r < 250 || g < 60 || g != b {
					t.Fatalf("the focused block is not full-height translucent red: %d,%d,%d", r, g, b)
				}
			case shape == CursorBlock:
				if middle || !top || !bottom {
					t.Fatal("the unfocused block is not outlined over the full stand-in")
				}
			case shape == CursorUnderline:
				if middle || top || !bottom {
					t.Fatal("the underline is not at the bottom of the whole stand-in")
				}
			}
			ct.tt.Advance(blinkInterval)
			ct.tt.Frame()
			if focused {
				if red(x+want.W/2, y+40) || red(x+want.W/2, y+79.5) {
					t.Fatalf("the %v caret did not blink off", shape)
				}
			} else if shape == CursorBlock && !red(x+want.W/2, y+79.5) {
				t.Fatal("the unfocused block outline blinked off")
			}
		}
	}
}

// Opaque content must not hide any part of a stand-in's caret cell.
// Check the pixels independently of caretRect, including the gutter clip.
func TestBlocksStandInOpaqueCaret(t *testing.T) {
	for _, numbers := range []LineNumbers{LineNumbersOn, LineNumbersOff} {
		for _, tc := range []struct {
			name    string
			shape   CursorShape
			focused bool
		}{{"bar", CursorBar, true}, {"block", CursorBlock, true}, {"outline", CursorBlock, false}, {"underline", CursorUnderline, true}} {
			t.Run(tc.name, func(t *testing.T) {
				ct, rep := standing(t, 80)
				ct.st.Theme, ct.st.NoActiveLine = redTheme, true
				ct.st.Font, ct.st.LineNumbers, ct.st.CursorShape = ui.Font{Size: 24}, numbers, tc.shape
				ct.st.SetBlocks([]Block{{Line: 1, Label: "Opaque", Content: func(c *ui.Context, e ui.Element, b Block) {
					e.Children(func() { ui.Box(c).FillWidth().Height(80).Background(ui.RGB(0, 0, 0)) })
				}}})
				ct.st.Select(rep.Start, rep.Start)
				ct.tt.SetFocused(tc.focused)
				ct.settle()
				ed := &ct.st.ed
				room, ok := ct.tt.Find("Opaque")
				if !ok || room.X != ed.r.X+ed.textX || room.H != 80 {
					t.Fatalf("opaque content is not at the text origin: %+v (%v)", room, ok)
				}
				if r, g, b := ct.pixel(room.X+1, room.Y+40); r != 0 || g != 0 || b != 0 {
					t.Fatalf("the content is not opaque black: %d,%d,%d", r, g, b)
				}
				width := float32(2)
				if tc.shape != CursorBar {
					width = min(ed.view.charWidth, ed.textX-ed.gutterW-1)
				}
				left, right := room.X-1-width, room.X-1
				red := func(x, y float32) bool {
					r, g, _ := ct.pixel(x, y)
					return r > 200 && g < 230
				}
				for x := left + 0.5; x < right; x++ {
					if !red(x, room.Y+79.5) || tc.shape != CursorUnderline && !red(x, room.Y+0.5) {
						t.Fatalf("numbers %v: the full %v cell is not visible before opaque content at x %v", numbers, tc.shape, x)
					}
					if tc.focused && tc.shape != CursorUnderline && !red(x, room.Y+40) {
						t.Fatal("opaque content hides part of the filled caret")
					}
				}
				gapY := room.Y + 0.5
				if tc.shape == CursorUnderline {
					gapY = room.Y + 79.5
				}
				if red(room.X-0.5, gapY) || left < ed.r.X+ed.gutterW {
					t.Fatal("the caret does not fit entirely in the margin before the content")
				}
			})
		}
	}
}

// A stand-in's fixed viewport x is not a text-column goal. Adding a
// caret on either side starts at column 0, regardless of horizontal scroll.
func TestBlocksStandInAddCursorGoal(t *testing.T) {
	for _, cmd := range []Command{AddCursorAbove, AddCursorBelow} {
		for _, scroll := range []float32{37, 83, 0} {
			ct := newEditorTest(t, "", strings.Repeat("x", 200)+"\ntable\nsource\n"+strings.Repeat("y", 200))
			r := hide(ct.st.Buffer.Offset(Pos{Line: 1}), ct.st.Buffer.Offset(Pos{Line: 3}))
			ct.st.SetReplacements([]Replacement{r})
			ct.st.SetBlocks([]Block{tall(1, false, 80, "Table")})
			ct.st.Select(r.Start, r.Start)
			ct.settle()
			ed := &ct.st.ed
			ed.scrollX = scroll
			ct.tt.Frame()
			if ed.scrollX != scroll {
				t.Fatal("the source did not keep the requested horizontal scroll")
			}
			ct.run(cmd)
			want := r.End
			if cmd == AddCursorAbove {
				want = 0
			}
			sels := ct.st.Selections()
			if len(sels) != 2 || ed.primary().head != want || ed.primary().goal != 0 {
				t.Fatalf("command %v, scroll %v: selections %+v, primary %+v, want new caret %d with goal 0", cmd, scroll, sels, ed.primary(), want)
			}
		}
	}
}

// Ordinary carets keep their text column when adding a scrolled caret.
func TestAddCursorScrolledTextGoal(t *testing.T) {
	for _, cmd := range []Command{AddCursorAbove, AddCursorBelow} {
		ct := newEditorTest(t, "", strings.Repeat(strings.Repeat("x", 200)+"\n", 3))
		ct.st.Select(ct.st.Buffer.Offset(Pos{Line: 1, Col: 7}), ct.st.Buffer.Offset(Pos{Line: 1, Col: 7}))
		ct.settle()
		ct.st.ed.scrollX = 83
		ct.tt.Frame()
		ct.run(cmd)
		line := 2
		if cmd == AddCursorAbove {
			line = 0
		}
		if want := ct.st.Buffer.Offset(Pos{Line: line, Col: 7}); len(ct.st.Selections()) != 2 || ct.st.ed.primary().head != want {
			t.Fatalf("command %v changed the ordinary text column: %+v, want %d", cmd, ct.st.Selections(), want)
		}
	}
}

// Revealing and centering a stand-in use the whole block, not a source
// row that is hidden behind it.
func TestBlocksStandInCaretReveal(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("top\n", 20)+"table\nsource\n"+strings.Repeat("end\n", 100))
	ct.st.ScrollMargin = -1
	r := hide(ct.st.Buffer.Offset(Pos{Line: 20}), ct.st.Buffer.Offset(Pos{Line: 22}))
	ct.st.SetReplacements([]Replacement{r})
	ct.st.SetBlocks([]Block{tall(20, false, 120, "Table")})
	ct.settle()
	ed := &ct.st.ed
	if b := ed.view.blocks[0]; !b.measured || b.height != 120 {
		t.Fatalf("the stand-in is not measured before reveal: %+v", b)
	}
	ct.st.Select(r.Start, r.Start)
	ct.settle()
	top := float64(20 * ed.view.lineHeight)
	if want := top + 120 - float64(ed.viewH); ed.scrollY != want {
		t.Fatalf("revealed at %v, want %v with the whole stand-in visible", ed.scrollY, want)
	}
	want := top - float64(ed.viewH-120)/2
	ct.run(ScrollCursorCenter)
	if ed.scrollY != want {
		t.Fatalf("center command scrolled to %v, want %v", ed.scrollY, want)
	}
	ct.st.SetScrollY(0)
	ed.reveal = revealCenter
	ct.settle()
	if ed.scrollY != want {
		t.Fatalf("center reveal scrolled to %v, want %v", ed.scrollY, want)
	}
}
