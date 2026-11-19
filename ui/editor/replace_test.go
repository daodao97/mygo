package editor

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/ui"
)

// hide is a replacement showing nothing.
func hide(start, end int) Replacement { return Replacement{Start: start, End: end} }

// box builds an element that draws nothing, named label.
func box(start, end int, width float32, label string) Replacement {
	return Replacement{Start: start, End: end, Width: width, Label: label,
		Content: func(*ui.Context, ui.Element, Replacement) {}}
}

// caretX returns where the caret at off shows on its row.
func (ct *editorTest) caretX(off int) float32 {
	ct.t.Helper()
	cr, ok := ct.st.ed.caretRect(off)
	if !ok {
		ct.t.Fatalf("offset %d doesn't show", off)
	}
	return cr.X
}

func (ct *editorTest) caretTo(off int) {
	ct.st.Select(off, off)
	ct.tt.Frame()
}

func (ct *editorTest) wantCursor(off int) {
	ct.t.Helper()
	if got := ct.st.Cursor(); got != off {
		ct.t.Fatalf("caret at %d, want %d in %q", got, off, ct.st.Buffer.String())
	}
}

// A range left out of the line takes no room, and the text stays the
// buffer's.
func TestReplacementsHide(t *testing.T) {
	src := "a **b** c\nz"
	ct := newEditorTest(t, "", src)
	ct.st.SetReplacements([]Replacement{hide(2, 4), hide(5, 7)})
	ct.tt.Frame()
	ed := &ct.st.ed
	if got := ed.view.line(0).disp; got != "a b c" {
		t.Fatalf("displayed %q", got)
	}
	if a, b := ct.caretX(2), ct.caretX(4); a != b {
		t.Fatalf("the ends of a hidden range show at %v and %v", a, b)
	}
	if got, want := ct.caretX(8), 4*ed.view.charWidth; got < want-1 || got > want+1 {
		t.Fatalf("c shows at %v, want %v", got, want)
	}
	if got := ct.st.Buffer.String(); got != src {
		t.Fatalf("the buffer holds %q", got)
	}
	ct.st.Select(0, len(src))
	if got := ct.st.SelectedText(); got != src {
		t.Fatalf("selected %q", got)
	}
	var n platform.AccessNode
	ed.accessibility(&n)
	if n.Value != src {
		t.Fatalf("assistive technology reads %q", n.Value)
	}
	if count, _ := func() (int, int) { ct.st.Find("*"); return ct.st.FindMatches() }(); count != 4 {
		t.Fatalf("found %d marks in the text", count)
	}
}

// Text shows in the range's place, in its Color.
func TestReplacementsText(t *testing.T) {
	ct := newEditorTest(t, "", "- item\nz")
	red := ui.RGB(255, 0, 0)
	ct.st.SetReplacements([]Replacement{{Start: 0, End: 1, Text: "•", Color: red}})
	ct.tt.Frame()
	v := &ct.st.ed.view
	l := v.line(0)
	if l.disp != "• item" {
		t.Fatalf("displayed %q", l.disp)
	}
	var cc runColors
	v.colorsOf(l, &cc)
	if got := cc.at(0, ui.Color{}); got != red {
		t.Fatalf("the text shows in %v", got)
	}
}

// Carets step over what a replacement shows as one character, select it
// whole, and deleting next to it deletes all of it.
func TestReplacementsCaretsStepOver(t *testing.T) {
	ct := newEditorTest(t, "", "abXXXXcd")
	set := func() { ct.st.SetReplacements([]Replacement{{Start: 2, End: 6, Text: "…"}}); ct.tt.Frame() }
	set()
	ct.caretTo(2)
	ct.key(0, ui.KeyRight)
	ct.wantCursor(6)
	ct.key(0, ui.KeyRight)
	ct.wantCursor(7)
	ct.key(0, ui.KeyLeft)
	ct.wantCursor(6)
	ct.key(0, ui.KeyLeft)
	ct.wantCursor(2)
	ct.key(ui.Shift, ui.KeyRight)
	if s := ct.st.Selection(); s.Start() != 2 || s.End() != 6 {
		t.Fatalf("selected %+v", s)
	}
	ct.caretTo(6)
	ct.key(0, ui.KeyBackspace)
	if got := ct.st.Buffer.String(); got != "abcd" {
		t.Fatalf("backspace left %q", got)
	}
	if got := ct.st.Replacements(); len(got) != 0 {
		t.Fatalf("a replacement of deleted text stayed: %+v", got)
	}
	ct.run(Undo)
	set()
	ct.caretTo(2)
	ct.key(0, ui.KeyDelete)
	if got := ct.st.Buffer.String(); got != "abcd" {
		t.Fatalf("delete left %q", got)
	}
}

// A range showing nothing costs no step: each key moves the caret where
// it shows, and deleting takes the character beyond it, not hidden text.
func TestReplacementsHiddenCostNoStep(t *testing.T) {
	src := "a **b** c"
	ct := newEditorTest(t, "", src)
	set := func() { ct.st.SetReplacements([]Replacement{hide(2, 4), hide(5, 7)}); ct.tt.Frame() }
	set()
	ct.caretTo(1)
	x := ct.caretX(1)
	for _, step := range []struct {
		key ui.Key
		to  int
	}{{ui.KeyRight, 4}, {ui.KeyRight, 7}, {ui.KeyRight, 8}, {ui.KeyLeft, 5}, {ui.KeyLeft, 2}, {ui.KeyLeft, 1}} {
		ct.key(0, step.key)
		ct.wantCursor(step.to)
		now := ct.caretX(step.to)
		if now == x {
			t.Fatalf("the step to %d left the caret where it showed", step.to)
		}
		x = now
	}
	ct.caretTo(4)
	ct.key(0, ui.KeyBackspace)
	if got := ct.st.Buffer.String(); got != "a**b** c" {
		t.Fatalf("backspace left %q", got)
	}
	ct.run(Undo)
	set()
	ct.caretTo(5)
	ct.key(0, ui.KeyDelete)
	if got := ct.st.Buffer.String(); got != "a **b**c" {
		t.Fatalf("delete left %q", got)
	}
}

// Replacements move with edits, keeping out what is inserted at their
// edges, and go when their text does.
func TestReplacementsMoveWithEdits(t *testing.T) {
	ct := newEditorTest(t, "", "ab**cd")
	ct.st.SetReplacements([]Replacement{hide(2, 4)})
	ct.tt.Frame()
	want := func(start, end int) {
		t.Helper()
		ct.tt.Frame()
		got := ct.st.Replacements()
		if len(got) != 1 || got[0].Start != start || got[0].End != end {
			t.Fatalf("replacements %+v, want %d to %d", got, start, end)
		}
	}
	b := ct.st.Buffer
	b.Edit(Edit{Start: 0, End: 0, Text: "xx"})
	want(4, 6)
	b.Edit(Edit{Start: 4, End: 4, Text: "y"}) // at its start
	want(5, 7)
	b.Edit(Edit{Start: 7, End: 7, Text: "y"}) // at its end
	want(5, 7)
	b.Edit(Edit{Start: 6, End: 6, Text: "z"}) // inside
	want(5, 8)
	b.Edit(Edit{Start: 5, End: 8})
	ct.tt.Frame()
	if got := ct.st.Replacements(); len(got) != 0 {
		t.Fatalf("replacements %+v after deleting their text", got)
	}
}

// Ranges are cut to the text, kept sorted, and left out when empty or
// overlapping one before them.
func TestReplacementsNormalized(t *testing.T) {
	ct := newEditorTest(t, "", "abcdefgh")
	ct.st.SetReplacements([]Replacement{hide(6, 100), hide(3, 3), hide(1, 4), hide(2, 5), hide(-5, 1)})
	got := ct.st.Replacements()
	want := [][2]int{{0, 1}, {1, 4}, {6, 8}}
	if len(got) != len(want) {
		t.Fatalf("replacements %+v", got)
	}
	for i, w := range want {
		if got[i].Start != w[0] || got[i].End != w[1] {
			t.Fatalf("replacement %d is %+v, want %v", i, got[i], w)
		}
	}
}

// Lines covered through their newline take no rows and carets go past
// them; a line showing the replacement keeps its row.
func TestReplacementsAcrossLines(t *testing.T) {
	ct := newEditorTest(t, "", "a\nb\nc\nd") // lines start at 0, 2, 4, 6
	ed := &ct.st.ed
	h := &ed.view.heights
	lh := float64(ed.view.lineHeight)
	before := h.total()
	ct.st.SetReplacements([]Replacement{hide(2, 6)})
	ct.tt.Frame()
	if got := h.total(); got != before-2*lh {
		t.Fatalf("height %v of %v", got, before)
	}
	if !h.isHidden(1) || !h.isHidden(2) || h.isHidden(3) {
		t.Fatalf("hidden lines %+v", h.hidden)
	}
	ct.caretTo(0)
	ct.key(0, ui.KeyDown)
	if p := ct.st.CursorPos(); p.Line != 3 {
		t.Fatalf("down went to line %d", p.Line)
	}
	ct.caretTo(1)
	ct.key(0, ui.KeyRight)
	ct.wantCursor(6)
	ct.key(0, ui.KeyLeft)
	ct.wantCursor(1)

	// Showing something, the first line keeps its row.
	ct.st.SetReplacements([]Replacement{{Start: 2, End: 6, Text: "⋯"}})
	ct.tt.Frame()
	if got := h.total(); got != before-lh {
		t.Fatalf("with text, height %v of %v", got, before)
	}
	if got := ed.view.line(1).disp; got != "⋯" {
		t.Fatalf("displayed %q", got)
	}

	// From inside a line into another, which shows what follows it.
	ct = newEditorTest(t, "", "ab\ncd\nef")
	ed = &ct.st.ed
	ct.st.SetReplacements([]Replacement{hide(1, 7)})
	ct.tt.Frame()
	if a, c := ed.view.line(0).disp, ed.view.line(2).disp; a != "a" || c != "f" {
		t.Fatalf("displayed %q and %q", a, c)
	}
	if !ed.view.heights.isHidden(1) {
		t.Fatal("the line between shows")
	}
	ct.caretTo(1)
	ct.key(0, ui.KeyRight)
	ct.wantCursor(7)
	ct.key(0, ui.KeyLeft)
	ct.wantCursor(1)

	// All the first lines hidden, the caret stays at the first shown.
	ct = newEditorTest(t, "", "a\nb\nc")
	ct.st.SetReplacements([]Replacement{hide(0, 4)})
	ct.tt.Frame()
	ct.caretTo(4)
	ct.key(0, ui.KeyLeft)
	ct.wantCursor(4)
	if _, ok := ct.st.ed.caretRect(4); !ok {
		t.Fatal("the first shown stop has no caret rectangle")
	}
	ct.caretTo(0)
	ct.key(0, ui.KeyRight)
	ct.wantCursor(4)
}

// A hidden prefix can end inside the first shown line. Its source stop
// is the replacement's end, not the hidden line start or buffer zero.
func TestReplacementsHiddenPrefixEndsWithinLine(t *testing.T) {
	for _, prefix := range []string{"a\n", "\na\n"} {
		for _, cmd := range []Command{MoveLeft, MoveUp, MovePageUp} {
			t.Run(prefix+"/"+cmd.String(), func(t *testing.T) {
				src := prefix + "bc"
				want := len(prefix) + len("b")
				ct := newEditorTest(t, "", src)
				ct.st.SetReplacements([]Replacement{hide(0, want)})
				ct.settle()
				ct.caretTo(len(src))
				for range 2 {
					ct.run(cmd)
					ct.wantCursor(want)
					if _, ok := ct.st.ed.caretRect(ct.st.Cursor()); !ok {
						t.Fatal("the backward boundary has no caret rectangle")
					}
				}
				ct.caretTo(0)
				ct.run(MoveRight)
				ct.wantCursor(want)
				ct.run(MoveRight)
				ct.wantCursor(len(src))
			})
		}
	}
}

// firstShownCaret finds the backward endpoint from display eligibility,
// not from navigation: the first stand-in or the first shown text line.
// A replacement can end within that line, so its hidden prefix is skipped.
func firstShownCaret(t *testing.T, ed *editor) int {
	t.Helper()
	v := &ed.view
	for line := range v.text.LineCount() {
		if v.heights.isHidden(line) {
			if r, ok := ed.standIn(line); ok {
				return r.Start
			}
			continue
		}
		off := v.text.LineStart(line)
		for _, r := range v.repls {
			if r.Start < off && off < r.End {
				off = r.End
			}
		}
		return off
	}
	t.Fatal("the buffer has no shown caret stop")
	return 0
}

// Walking ends only at the buffer's end going forward, or its first
// shown caret stop going back. An arbitrary no-progress step is a failure.
func TestReplacementsWalkEnds(t *testing.T) {
	rng := rand.New(rand.NewPCG(1651, 1))
	for range 200 {
		var b strings.Builder
		for range 1 + rng.IntN(40) {
			b.WriteString([]string{"a", " ", "\t", "\n", "語", "**"}[rng.IntN(6)])
		}
		src := b.String()
		ct := newEditorTest(t, "", src)
		var rs []Replacement
		for range rng.IntN(6) {
			start := rng.IntN(len(src) + 1)
			end := min(len(src), start+rng.IntN(8))
			for start > 0 && start < len(src) && src[start]&0xc0 == 0x80 {
				start--
			}
			for end < len(src) && src[end]&0xc0 == 0x80 {
				end++
			}
			r := hide(start, end)
			switch rng.IntN(3) {
			case 1:
				r.Text = "…"
			case 2:
				r.Content, r.Width = func(*ui.Context, ui.Element, Replacement) {}, 20
			}
			rs = append(rs, r)
		}
		ct.st.SetReplacements(rs)
		ct.st.SoftWrap = rng.IntN(2) == 0
		ct.tt.Frame()
		ed := &ct.st.ed
		for _, step := range []struct {
			cmd Command
			fwd bool
		}{{MoveRight, true}, {MoveLeft, false}} {
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
					t.Fatalf("%q with %+v: %v from %d left an invisible caret at %d", src, rs, step.cmd, at, to)
				}
				if to == at {
					if to != want {
						t.Fatalf("%q with %+v: %v stopped at %d, want endpoint %d, step %d", src, rs, step.cmd, to, want, n)
					}
					break
				}
				if (to > at) != step.fwd || n > len(src)+1 {
					t.Fatalf("%q with %+v: %v from %d went to %d, step %d", src, rs, step.cmd, at, to, n)
				}
			}
		}
	}
}

// An element takes its Width in the row, wraps as a character that wide,
// and shows at the place of its range, moving as the text scrolls.
func TestReplacementsElements(t *testing.T) {
	ct := newEditorTest(t, "", "abXXXcd\nz")
	ct.st.SetReplacements([]Replacement{box(2, 5, 40, "Done")})
	ct.tt.Frame()
	ct.tt.Frame()
	ed := &ct.st.ed
	cw := ed.view.charWidth
	if got, want := ct.caretX(5), 2*cw+40; got < want-1 || got > want+1 {
		t.Fatalf("the text after it shows at %v, want %v", got, want)
	}
	r, ok := ct.tt.Find("Done")
	if !ok {
		t.Fatal("the element isn't built")
	}
	if got, want := r.X, ed.r.X+ed.textX+ct.caretX(2); got < want-1 || got > want+1 {
		t.Fatalf("the element is at %v, want %v", got, want)
	}
	if r.W != 40 || r.H != ed.view.lineHeight {
		t.Fatalf("the element is %v by %v", r.W, r.H)
	}
	if _, ok := ct.tt.AccessByRole(ui.RoleImage, "Done"); !ok {
		t.Fatal("assistive technology doesn't see the element's label")
	}
	// As high as a row, and square, without a width.
	ct.st.SetReplacements([]Replacement{box(2, 5, 0, "Done")})
	ct.tt.Frame()
	ct.tt.Frame()
	if r, _ := ct.tt.Find("Done"); r.W != ed.view.lineHeight {
		t.Fatalf("the square element is %v wide", r.W)
	}
	// A press on an element taking none goes to the text.
	r, _ = ct.tt.Find("Done")
	ct.tt.ClickAt(r.X+r.W-1, r.Y+r.H/2)
	ct.wantCursor(5)
	ct.tt.ClickAt(r.X+1, r.Y+r.H/2)
	ct.wantCursor(2)
}

// What an element does is its own, and the typing stays in the editor.
func TestReplacementsElementTakesClick(t *testing.T) {
	ct := newEditorTest(t, "", "abXXXcd")
	clicks := 0
	var got Replacement
	ct.st.SetReplacements([]Replacement{{Start: 2, End: 5, Width: 30, Interactive: true, Content: func(c *ui.Context, e ui.Element, r Replacement) {
		e.Children(func() {
			b := ui.ButtonBase(c).Fill().KeepFocus().Label("Toggle")
			if b.Clicked() {
				clicks++
				got = r
			}
		})
	}}})
	ct.tt.Frame()
	ct.tt.Frame()
	ct.st.Buffer.Edit(Edit{Start: 0, End: 0, Text: "__"})
	ct.tt.Frame()
	ct.tt.Frame()
	if err := ct.tt.Click("Toggle"); err != nil {
		t.Fatal(err)
	}
	ct.tt.Frame()
	if clicks != 1 {
		t.Fatalf("clicked %d times", clicks)
	}
	if got.Start != 4 || got.End != 7 {
		t.Fatalf("its range is %d to %d, where the edit didn't move it", got.Start, got.End)
	}
	if !ct.st.Focused() {
		t.Fatal("the click took the keyboard from the editor")
	}
	// The wheel over it still scrolls the text.
	ct.st.Buffer.Edit(Edit{Start: ct.st.Buffer.Len(), End: ct.st.Buffer.Len(), Text: strings.Repeat("\nmore", 200)})
	ct.tt.Frame()
	ct.tt.Frame()
	r, ok := ct.tt.Find("Toggle")
	if !ok {
		t.Fatal("the button isn't built")
	}
	ct.tt.Wheel(r.X+r.W/2, r.Y+r.H/2, 0, 60)
	ct.tt.Frame()
	if ct.st.ScrollY() == 0 {
		t.Fatal("the wheel over the element didn't scroll")
	}
}

// Assistive technology reads the buffer's text in the field, and sees
// what is placed over it: an interactive element's controls, and the find
// bar.
func TestReplacementsAccessible(t *testing.T) {
	ct := newEditorTest(t, "", "abXXXcd")
	ct.st.SetReplacements([]Replacement{{Start: 2, End: 5, Width: 30, Label: "Task", Interactive: true,
		Content: func(c *ui.Context, e ui.Element, r Replacement) {
			e.Children(func() { ui.ButtonBase(c).Fill().KeepFocus().Label("Toggle") })
		}}})
	ct.tt.Frame()
	ct.tt.Frame()
	if _, ok := ct.tt.AccessByRole(ui.RoleGroup, "Task"); !ok {
		t.Fatal("the element isn't seen")
	}
	if _, ok := ct.tt.AccessByRole(ui.RoleButton, "Toggle"); !ok {
		t.Fatal("the button in it isn't seen")
	}
	field, ok := ct.tt.AccessByRole(ui.RoleTextField, "")
	if !ok || field.Value != "abXXXcd" {
		t.Fatalf("the field reads %+v", field)
	}
	ct.st.Find("X")
	ct.tt.Frame()
	ct.tt.Frame()
	if _, ok := ct.tt.AccessByRole(ui.RoleToolbar, ""); !ok {
		t.Fatal("the find bar isn't seen")
	}
}

// Only the lines in view are laid out, and only their elements built.
func TestReplacementsOnlyInView(t *testing.T) {
	const lines = 5000
	ct := newEditorTest(t, "", strings.Repeat("abXXXcd\n", lines))
	built := 0
	content := func(*ui.Context, ui.Element, Replacement) { built++ }
	rs := make([]Replacement, lines)
	for i := range rs {
		rs[i] = Replacement{Start: i*8 + 2, End: i*8 + 5, Width: 20, Content: content}
	}
	ct.st.SetReplacements(rs)
	ct.tt.Frame()
	ct.tt.Frame()
	ed := &ct.st.ed
	built = 0
	ct.tt.Frame()
	inView := int(ed.viewH/ed.view.lineHeight) + 2
	if built == 0 || built > inView {
		t.Fatalf("built %d elements, %d lines in view", built, inView)
	}
	if n := len(ed.view.lines.m); n > 2*inView {
		t.Fatalf("laid out %d lines, %d in view", n, inView)
	}
	// Scrolled far down, those there are built.
	built = 0
	ct.st.ScrollTo(4000)
	ct.tt.Frame()
	if built == 0 || built > inView {
		t.Fatalf("scrolled, built %d elements", built)
	}
	if first, _ := ct.st.VisibleLines(); first != 4000 {
		t.Fatalf("shows line %d", first)
	}
}

// Setting the same replacements lays nothing out again, and changing one
// only its line.
func TestReplacementsKeepLayouts(t *testing.T) {
	ct := newEditorTest(t, "", "ab**\ncd**\nef")
	rs := []Replacement{hide(2, 4), hide(7, 9)}
	ct.st.SetReplacements(rs)
	ct.tt.Frame()
	v := &ct.st.ed.view
	l0, l1 := v.line(0), v.line(1)
	ct.st.SetReplacements([]Replacement{hide(2, 4), hide(7, 9)})
	if v.line(0) != l0 || v.line(1) != l1 {
		t.Fatal("the same replacements laid their lines out again")
	}
	ct.st.SetReplacements([]Replacement{hide(2, 4)})
	if v.line(0) != l0 {
		t.Fatal("a line whose replacements stayed was laid out again")
	}
	if got := v.line(1); got == l1 || got.disp != "cd**" {
		t.Fatalf("the line of the replacement taken away shows %q", got.disp)
	}
}

// Soft wraps count what replacements show, in lines out of view too.
func TestReplacementsWrap(t *testing.T) {
	long := strings.Repeat("x", 150)
	ct := newEditorTest(t, "", long+"\nz")
	ct.st.SoftWrap = true
	ct.tt.Frame()
	ct.tt.Frame()
	v := &ct.st.ed.view
	if rows := len(v.line(0).rows); rows != 3 {
		t.Fatalf("wrapped into %d rows, want 3", rows)
	}
	ct.st.SetReplacements([]Replacement{hide(10, 140)})
	ct.tt.Frame()
	ct.tt.Frame()
	if rows := len(v.line(0).rows); rows != 1 {
		t.Fatalf("with most hidden, %d rows", rows)
	}

	// An element wider than a row, far from the view, measured unlaid.
	const far = 3000
	ct = newEditorTest(t, "", strings.Repeat("\n", far)+"abXXXcd\n")
	ct.st.SoftWrap = true
	ct.tt.Frame()
	ct.tt.Frame()
	v = &ct.st.ed.view
	start := v.text.LineStart(far)
	ct.st.SetReplacements([]Replacement{box(start+2, start+5, 2000, "")})
	for range 50 {
		ct.tt.Frame()
		if v.heights.wraps.extra[far] != wrapUnknown {
			break
		}
	}
	if _, laid := v.lines.m[far]; laid {
		t.Fatal("the line far away was laid out")
	}
	if rows := v.heights.wraps.rows(far); rows != 3 {
		t.Fatalf("measured %d rows out of view, want 3", rows)
	}
}

// Selections paint over what a replacement shows, an element draws no
// glyph of its own, and find and folds leave it be.
func TestReplacementsPaint(t *testing.T) {
	theme := &Theme{Background: ui.RGB(255, 255, 255), Selection: ui.RGB(0, 0, 255), InactiveSelection: ui.RGB(0, 0, 255)}
	ct := newEditorTest(t, "", "abXXXcd\nz")
	ct.st.Theme, ct.st.NoActiveLine = theme, true
	ct.st.SetReplacements([]Replacement{box(2, 5, 40, "Done")})
	ct.tt.Frame()
	ct.tt.Frame()
	ed := &ct.st.ed
	at := func(x, y float32) (r, g, b uint8) {
		img := ct.tt.Image()
		s := float32(img.Bounds().Dx()) / 600
		c := img.RGBAAt(int(x*s), int(y*s))
		return c.R, c.G, c.B
	}
	room, _ := ct.tt.Find("Done")
	for x := room.X + 2; x < room.X+room.W-2; x += 2 {
		if r, g, b := at(x, room.Y+room.H/2); r != 255 || g != 255 || b != 255 {
			t.Fatalf("something drew at %v in the element's room: %d %d %d", x, r, g, b)
		}
	}
	ct.st.Select(0, 7)
	ct.tt.Frame()
	if r, g, b := at(room.X+room.W/2, room.Y+2); b < 200 || r > 60 || g > 60 {
		t.Fatalf("the selection doesn't cover it: %d %d %d", r, g, b)
	}
	// Whitespace inside a replacement shows no marks.
	if !ed.view.line(0).replaced(3) || ed.view.line(0).replaced(5) {
		t.Fatal("replaced bytes wrong")
	}

	// A folded line's element isn't built.
	ct = newEditorTest(t, "go", "func a() {\n\txx()\n}\nz")
	start := ct.st.Buffer.Offset(Pos{Line: 1, Col: 1})
	ct.st.SetReplacements([]Replacement{box(start, start+2, 20, "Inner")})
	ct.tt.Frame()
	ct.tt.Frame()
	if _, ok := ct.tt.Find("Inner"); !ok {
		t.Fatal("the element isn't built")
	}
	ct.st.Fold(0)
	ct.tt.Frame()
	ct.tt.Frame()
	if _, ok := ct.tt.Find("Inner"); ok {
		t.Fatal("a folded line's element is built")
	}
}

// A frame painting without building, as dragging past an edge scrolls,
// has the elements placed again.
func TestReplacementsFollowScroll(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("abXXXcd\n", 200))
	ct.st.SetReplacements([]Replacement{box(2, 5, 20, "First")})
	ct.tt.Frame()
	ct.tt.Frame()
	ed := &ct.st.ed
	if ed.elementsMoved() {
		t.Fatal("placed elements count as moved")
	}
	if ct.tt.FramePending() {
		t.Fatal("placed elements keep asking for frames")
	}
	ed.scrollY += 10
	if !ed.elementsMoved() {
		t.Fatal("scrolling doesn't count as moving them")
	}
}

// An editor as high as its lines places them too.
func TestReplacementsFitContent(t *testing.T) {
	st := &State{Buffer: NewBuffer("abXXXcd\nz"), FitContent: true}
	st.SetReplacements([]Replacement{box(2, 5, 20, "Done")})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() { View(c, st).FillWidth() })
	}, 600, 400)
	tt.Frame()
	tt.Frame()
	r, ok := tt.Find("Done")
	if !ok {
		t.Fatal("the element isn't built")
	}
	if want := st.ed.r.X + st.ed.textX + 2*st.ed.view.charWidth; r.X < want-1 || r.X > want+1 {
		t.Fatalf("the element is at %v, want %v", r.X, want)
	}
}

// A new buffer forgets the old one's replacements.
func TestReplacementsNewBuffer(t *testing.T) {
	ct := newEditorTest(t, "", "ab**")
	ct.st.SetReplacements([]Replacement{hide(2, 4)})
	ct.tt.Frame()
	ct.st.Buffer = NewBuffer("other")
	ct.tt.Frame()
	if got := ct.st.Replacements(); len(got) != 0 {
		t.Fatalf("replacements %+v of the old buffer", got)
	}
	if got := ct.st.ed.view.line(0).disp; got != "other" {
		t.Fatalf("displayed %q", got)
	}
}

// crossing counts the replacements whose lines an edit would look up.
func crossing(v *view) int {
	n := 0
	for _, c := range v.replCross {
		if c {
			n++
		}
	}
	return n
}

// An edit looks up the lines of the replacements it reached inside, not
// of all of them, and setting them again where they are looks up none.
func TestReplacementsLookUpOnlyThoseEdited(t *testing.T) {
	var b strings.Builder
	var rs []Replacement
	for range 500 {
		rs = append(rs, hide(b.Len()+1, b.Len()+3))
		b.WriteString("a**b\n")
	}
	ct := newEditorTest(t, "", b.String())
	ct.st.SetReplacements(rs)
	v := &ct.st.ed.view
	if got := crossing(v); got != len(rs) {
		t.Fatalf("%d replacements to look up at first", got)
	}
	v.replacedLines()
	if got := crossing(v); got != 0 {
		t.Fatalf("%d replacements left to look up", got)
	}
	// Before them all, and at the edges of one: none reached inside.
	ct.st.Buffer.Edit(Edit{Start: 0, End: 0, Text: "x"})
	ct.st.ed.sync()
	if got := crossing(v); got != 0 {
		t.Fatalf("an edit before them has %d to look up", got)
	}
	r := v.repls[10]
	ct.st.Buffer.Edit(Edit{Start: r.End, End: r.End, Text: "y"})
	ct.st.ed.sync()
	if got := crossing(v); got != 0 {
		t.Fatalf("an edit at an end has %d to look up", got)
	}
	r = v.repls[10]
	ct.st.Buffer.Edit(Edit{Start: r.Start + 1, End: r.Start + 1, Text: "z"})
	ct.st.ed.sync()
	if got := crossing(v); got != 1 {
		t.Fatalf("an edit inside one has %d to look up", got)
	}
	v.replacedLines()
	ct.st.SetReplacements(ct.st.Replacements())
	if got := crossing(v); got != 0 {
		t.Fatalf("set again where they are, %d to look up", got)
	}
}

// The lines hidden stay what the text says, as edits add and take away
// newlines inside and around replacements.
func TestReplacementsHiddenFollowEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(1651, 3))
	want := func(v *view) []lineSpan {
		var out []lineSpan
		for i := range v.repls {
			r := &v.repls[i]
			a, b := v.text.LineOf(r.Start), v.text.LineOf(r.End)
			first := a
			if r.Start > v.text.LineStart(a) || r.shown() != "" {
				first++
			}
			if b == a || first > b-1 {
				continue
			}
			if k := len(out); k > 0 && first <= out[k-1].to+1 {
				out[k-1].to = max(out[k-1].to, b-1)
				continue
			}
			out = append(out, lineSpan{first, b - 1})
		}
		return out
	}
	for range 100 {
		var b strings.Builder
		for range 5 + rng.IntN(30) {
			b.WriteString([]string{"a", "bb", "\n", "\n"}[rng.IntN(4)])
		}
		src := b.String()
		ct := newEditorTest(t, "", src)
		var rs []Replacement
		for range 1 + rng.IntN(6) {
			start := rng.IntN(len(src))
			r := hide(start, min(len(src), start+1+rng.IntN(6)))
			if rng.IntN(3) == 0 {
				r.Text = "…"
			}
			rs = append(rs, r)
		}
		ct.st.SetReplacements(rs)
		v := &ct.st.ed.view
		v.replacedLines()
		for range 30 {
			n := ct.st.Buffer.Len()
			start := rng.IntN(n + 1)
			end := min(n, start+rng.IntN(4))
			ct.st.Buffer.Edit(Edit{Start: start, End: end, Text: []string{"", "c", "\n", "d\ne"}[rng.IntN(4)]})
			ct.st.ed.sync()
			if len(v.replCross) != len(v.repls) {
				t.Fatalf("%d flags for %d replacements", len(v.replCross), len(v.repls))
			}
			got, exp := v.replacedLines(), want(v)
			if !slices.Equal(got, exp) && (len(got) != 0 || len(exp) != 0) {
				t.Fatalf("%q with %+v hides %v, want %v", ct.st.Buffer.String(), v.repls, got, exp)
			}
			if rng.IntN(4) == 0 {
				ct.st.SetReplacements(ct.st.Replacements())
			}
		}
	}
}
