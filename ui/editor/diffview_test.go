package editor

import (
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/diff"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// checkHunks checks that hunks turn old's lines into new's.
func checkHunks(t *testing.T, old, nw string, hunks []diff.Hunk) {
	t.Helper()
	a, b := strings.Split(old, "\n"), strings.Split(nw, "\n")
	var out []string
	i, j := 0, 0
	for _, h := range hunks {
		if h.A0 < i || h.B0 < j || h.A0-i != h.B0-j {
			t.Fatalf("hunk %+v out of order after %d, %d: %+v", h, i, j, hunks)
		}
		for ; i < h.A0; i, j = i+1, j+1 {
			if a[i] != b[j] {
				t.Fatalf("unchanged old line %d %q is %q in new: %+v", i, a[i], b[j], hunks)
			}
			out = append(out, a[i])
		}
		out = append(out, b[h.B0:h.B1]...)
		i, j = h.A1, h.B1
	}
	if len(a)-i != len(b)-j {
		t.Fatalf("tails differ: %d, %d: %+v", len(a)-i, len(b)-j, hunks)
	}
	out = append(out, a[i:]...)
	if got := strings.Join(out, "\n"); got != nw {
		t.Fatalf("hunks make %q, want %q", got, nw)
	}
}

// Edits of the new text compare again only around them, and the hunks
// stay those of the texts.
func TestCodeDiffIncremental(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	pieces := []string{"x", "\n", "", "line\n", "a\nb\n", "{", "}\n", "\n\n"}
	for round := range 300 {
		var sb strings.Builder
		for i := range 30 + r.IntN(30) {
			sb.WriteString([]string{"func a() {", "\treturn", "}", "", "x := 1", "y"}[i%6])
			sb.WriteByte('\n')
		}
		old := NewBuffer(sb.String())
		nw := NewBuffer(sb.String())
		var d diffModel
		d.set(old, nw, false)
		d.update()
		for range 20 {
			text := nw.String()
			at := r.IntN(len(text) + 1)
			end := min(len(text), at+r.IntN(8))
			nw.Edit(Edit{Start: at, End: end, Text: pieces[r.IntN(len(pieces))]})
			if r.IntN(3) == 0 {
				d.update()
				checkHunks(t, old.String(), nw.String(), d.hunks)
			}
		}
		d.update()
		checkHunks(t, old.String(), nw.String(), d.hunks)
		if round%10 == 0 {
			// A change of the old text compares the whole.
			old.Edit(Edit{Start: 0, End: 0, Text: "// new first line\n"})
			d.update()
			checkHunks(t, old.String(), nw.String(), d.hunks)
		}
	}
}

func TestCodeDiffRevertAccept(t *testing.T) {
	for _, c := range []struct{ old, new string }{
		{"a\nb\nc", "a\nx\nc"},
		{"a\nb\nc", "a\nc"},
		{"a\nc", "a\nb\nc"},
		{"a\nb", "a"},
		{"a", "a\nb"},
		{"b\na", "a"},
		{"", "a\nb"},
		{"a\nb", ""},
		{"a\nb\n", "a\nb\nc\n"},
	} {
		old, nw := NewBuffer(c.old), NewBuffer(c.new)
		var d diffModel
		d.set(old, nw, false)
		d.update()
		for i := 0; len(d.hunks) > 0 && i < 10; i++ {
			d.revert(0)
			d.update()
		}
		if nw.String() != c.old {
			t.Errorf("reverting %q to %q gave %q", c.new, c.old, nw.String())
		}
		old, nw = NewBuffer(c.old), NewBuffer(c.new)
		d = diffModel{}
		d.set(old, nw, false)
		d.update()
		for i := 0; len(d.hunks) > 0 && i < 10; i++ {
			d.accept(0)
			d.update()
		}
		if old.String() != c.new {
			t.Errorf("accepting %q over %q gave %q", c.new, c.old, old.String())
		}
	}
}

func TestDiffViewer(t *testing.T) {
	var oldText, newText strings.Builder
	for i := range 200 {
		line := "line " + strconv.Itoa(i)
		oldText.WriteString(line + "\n")
		if i == 100 {
			newText.WriteString("changed\n")
			continue
		}
		newText.WriteString(line + "\n")
	}
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer(oldText.String()), New: NewBuffer(newText.String()), Layout: layout, Editable: true}
		tt := ui.NewTester(func(c *ui.Context) {
			DiffView(c, d).Fill()
		}, 900, 500)
		tt.Frame()
		if hs := d.Hunks(); len(hs) != 1 || hs[0] != (DiffHunk{100, 101, 100, 101}) {
			t.Fatalf("hunks %+v", hs)
		}
		if a, del := d.Stats(); a != 1 || del != 1 {
			t.Fatalf("stats %d, %d", a, del)
		}
		// Unchanged lines collapse but 3 around the change: 97 before, 97
		// after, with the empty line after the last newline.
		gaps := d.gapList
		if len(gaps) != 2 || gaps[0].hidden != 97 || gaps[1].hidden != 97 {
			t.Fatalf("gaps %+v", gaps)
		}
		ed := &d.eds[1]
		total := ed.view.heights.total()
		lh := float64(ed.view.lineHeight)
		want := float64(3+1+3)*lh + 2*diffSeparatorHeight
		if layout == DiffUnified {
			want += lh
		}
		if total != want {
			t.Fatalf("%v: height %v, want %v", layout, total, want)
		}
		// Expanding shows the lines.
		d.expand(gaps[0].a, 1, true)
		tt.Frame()
		if d.gapList[0].hidden != 0 {
			t.Fatalf("expanded gap %+v", d.gapList[0])
		}
		// Editing the new side follows: reverting the change by hand.
		ed.sync()
		r := &ed.view.text
		st := &State{}
		_ = st
		a, z := r.LineStart(100), r.LineEnd(100)
		d.New.Edit(Edit{Start: a, End: z, Text: "line 100"})
		tt.Frame()
		if hs := d.Hunks(); len(hs) != 0 {
			t.Fatalf("hunks after reverting by hand %+v", hs)
		}
		d.New.Undo()
		tt.Frame()
		d.Revert(0)
		tt.Frame()
		if d.New.String() != d.Old.String() {
			t.Fatal("Revert left the texts different")
		}
	}
}

// A split view's sides are as high as each other, line for line.
func TestDiffViewerAlign(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\n"
	nw := "a\nB\nB2\nB3\nc\nf\ng\n"
	for _, wrap := range []bool{false, true} {
		d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw), ExpandUnchanged: true, Wrap: wrap,
			Annotations:      []DiffAnnotation{{Side: DiffNew, Line: 2, Height: 40}, {Side: DiffOld, Line: 0, Height: 25}},
			RenderAnnotation: func(c *ui.Context, a DiffAnnotation) { ui.Text(c, "note") }}
		tt := ui.NewTester(func(c *ui.Context) {
			DiffView(c, d).Fill()
		}, 600, 400)
		tt.Frame()
		ha, hb := &d.eds[0].view.heights, &d.eds[1].view.heights
		if ha.total() != hb.total() {
			t.Fatalf("wrap %v: heights %v and %v", wrap, ha.total(), hb.total())
		}
		// Unchanged lines line up.
		for _, p := range [][2]int{{0, 0}, {2, 4}, {5, 5}} {
			if ga, gb := ha.geometry(p[0]).rows, hb.geometry(p[1]).rows; ga != gb {
				t.Fatalf("wrap %v: old line %d at %v, new line %d at %v", wrap, p[0], ga, p[1], gb)
			}
		}
		if !tt.HasText("note") {
			t.Fatal("no annotation")
		}
	}
}

// An annotation of a collapsed line goes with it, on both sides of a
// split view: the other side leaves it no room, and the lines after it
// stay in line.
func TestDiffViewerAnnotationCollapsed(t *testing.T) {
	var lines []string
	for i := range 40 {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	old := strings.Join(lines, "\n") + "\n"
	lines[30] = "changed"
	nw := strings.Join(lines, "\n") + "\n"
	for _, side := range []DiffSide{DiffOld, DiffNew} {
		// Lines 5 and 39 are collapsed, above the change and below it.
		for _, line := range []int{5, 39} {
			d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw),
				Annotations:      []DiffAnnotation{{Side: side, Line: line, Height: 40}},
				RenderAnnotation: func(c *ui.Context, a DiffAnnotation) { ui.Text(c, "note") }}
			tt := ui.NewTester(func(c *ui.Context) {
				DiffView(c, d).Fill()
			}, 600, 1000)
			tt.Frame()
			if tt.HasText("note") {
				t.Fatalf("side %d, line %d: the annotation of a collapsed line shows", side, line)
			}
			ha, hb := &d.eds[0].view.heights, &d.eds[1].view.heights
			if ha.total() != hb.total() {
				t.Errorf("side %d, line %d: heights %v and %v", side, line, ha.total(), hb.total())
			}
			if ga, gb := ha.geometry(30).rows, hb.geometry(30).rows; ga != gb {
				t.Errorf("side %d, line %d: the changed line at %v and %v", side, line, ga, gb)
			}
			// Expanded, it shows, with its room on the other side.
			d.ExpandAll()
			tt.Frame()
			if !tt.HasText("note") {
				t.Fatalf("side %d, line %d: no annotation once expanded", side, line)
			}
			if ha.total() != hb.total() {
				t.Errorf("side %d, line %d: expanded heights %v and %v", side, line, ha.total(), hb.total())
			}
		}
	}
}

func TestDiffViewerLineSelection(t *testing.T) {
	var got DiffLineRange
	d := &DiffState{Old: NewBuffer("a\nb\nc\n"), New: NewBuffer("a\nx\nc\n"), Layout: DiffUnified, ExpandUnchanged: true,
		LineSelection: true, OnLineSelection: func(r DiffLineRange) { got = r }}
	tt := ui.NewTester(func(c *ui.Context) {
		DiffView(c, d).Fill()
	}, 600, 400)
	tt.Frame()
	ed := &d.eds[1]
	lh := ed.view.lineHeight
	// The number of new line 2, below the deleted line.
	y := float32(ed.view.heights.geometry(1).rows) + lh/2
	tt.ClickAt(ed.gutterW-8, y)
	if r, ok := d.SelectedLines(); !ok || r != (DiffLineRange{DiffNew, 1, 1}) || got != r {
		t.Fatalf("selected %+v %v, reported %+v", r, ok, got)
	}
	tt.ClickAtWith(ui.Shift, ed.gutterW-8, float32(ed.view.heights.geometry(2).rows)+lh/2)
	if r, _ := d.SelectedLines(); r != (DiffLineRange{DiffNew, 1, 2}) {
		t.Fatalf("extended %+v", r)
	}
}

// A press on the divider focuses it where the user scrolled the page: a
// diff the page's scroll view cuts stays put.
func TestDiffDividerPressKeepsThePageScroll(t *testing.T) {
	d := &DiffState{Old: NewBuffer("a\nb\nc\n"), New: NewBuffer("a\nx\nc\n"), Layout: DiffSplit}
	var page ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c.Key("page")).Fill().TrackScroll(&page).Children(func() {
			ui.Box(c).Height(100).Shrink(0)
			// 100 to 360 of a window 300 high.
			DiffView(c, d).Height(260).Shrink(0)
			ui.Box(c).Height(400).Shrink(0)
		})
	}, 900, 300)
	tt.Frame()
	tt.Frame()
	div, ok := tt.AccessByRole(ui.RoleSplitter, "")
	if !ok {
		t.Fatal("the diff has no divider")
	}
	x := div.Bounds.X + div.Bounds.W/2
	tt.Press(x, 200)
	tt.Frame()
	tt.Frame()
	if div, _ := tt.AccessByRole(ui.RoleSplitter, ""); !div.Focused {
		t.Fatal("the press didn't focus the divider")
	}
	if page.Y != 0 {
		t.Errorf("the press scrolled the page to %v", page.Y)
	}
	tt.Release(x, 200)
}

// The app runs commands and reads the caret in the side that has the
// keyboard, or had it last: the new side's until the old one has it.
func TestDiffViewerActiveSide(t *testing.T) {
	var oldText, newText strings.Builder
	for i := range 40 {
		line := "line " + strconv.Itoa(i)
		oldText.WriteString(line + "\n")
		switch i {
		case 5, 30:
			newText.WriteString("changed " + strconv.Itoa(i) + "\n")
		default:
			newText.WriteString(line + "\n")
		}
	}
	d := &DiffState{Old: NewBuffer(oldText.String()), New: NewBuffer(newText.String()), ExpandUnchanged: true}
	type caret struct {
		side   DiffSide
		line   int
		hunk   int
		inside bool
	}
	at := func() caret {
		h, in := d.CurrentHunk()
		return caret{d.ActiveSide(), d.CursorPos().Line, h, in}
	}
	// Before the viewer is built, the new side's caret is at the start.
	if c := at(); c != (caret{DiffNew, 0, -1, false}) || d.Cursor() != 0 {
		t.Fatalf("before building: %+v, offset %d", c, d.Cursor())
	}
	tt := ui.NewTester(func(c *ui.Context) {
		DiffView(c, d).Fill()
	}, 900, 500)
	tt.Frame()
	d.GoToHunk(1)
	tt.Frame()
	if c := at(); c != (caret{DiffNew, 30, 1, true}) || d.Cursor() != d.New.Offset(Pos{Line: 30}) {
		t.Fatalf("after GoToHunk(1): %+v, offset %d", c, d.Cursor())
	}
	// Between changes, the current one is the one before the caret: the
	// view shows the second change in its middle, lines of the first
	// above it.
	lh := float32(d.eds[1].view.lineHeight)
	tt.ClickAt(200, 2.5*lh)
	tt.Frame()
	if c := at(); c.side != DiffOld || c.line <= 5 || c.line >= 30 || c.hunk != 0 || c.inside {
		t.Fatalf("clicked in the old side: %+v", c)
	}
	// The old side stays active as the window loses the keyboard.
	tt.SetFocused(false)
	tt.Frame()
	if side := d.ActiveSide(); side != DiffOld {
		t.Fatalf("unfocused: side %v", side)
	}
	// GoToHunk moves the caret CursorPos reads, so an app's Next Change
	// steps from it.
	h, _ := d.CurrentHunk()
	d.GoToHunk(h + 1)
	tt.Frame()
	if c := at(); c != (caret{DiffOld, 30, 1, true}) {
		t.Fatalf("after GoToHunk in the old side: %+v", c)
	}
	tt.SetFocused(true)
	tt.Frame()
	if !d.Run(Find) {
		t.Fatal("Run(Find) did nothing")
	}
	tt.Frame()
	if !d.eds[0].find.open || d.eds[1].find.open {
		t.Fatalf("find bars open: old %v, new %v", d.eds[0].find.open, d.eds[1].find.open)
	}
	// Back in the new side, between the changes, Find selects the match
	// after the caret.
	tt.ClickAt(650, 2.5*lh)
	tt.Frame()
	d.Find("changed")
	tt.Frame()
	if c := at(); c.side != DiffNew || c.line != 30 || !d.eds[1].find.open {
		t.Fatalf("Find in the new side: %+v, open %v", c, d.eds[1].find.open)
	}
	// A unified view has only the new side's editor.
	d.Layout = DiffUnified
	tt.Frame()
	if side := d.ActiveSide(); side != DiffNew {
		t.Fatalf("unified: side %v", side)
	}
}

// The caret is in a hunk on the lines it changes, and in a deletion on
// the line of the side without its lines where they were; after the last
// line of a hunk, it is after it.
func TestDiffViewerCurrentHunk(t *testing.T) {
	d := &DiffState{Old: NewBuffer("a\nb\nc\nd\ne\n"), New: NewBuffer("a\nB\nc\ne\n"), ExpandUnchanged: true}
	if n := len(d.Hunks()); n != 2 {
		t.Fatalf("%d hunks, want 2", n)
	}
	ed := d.editorOf(DiffNew)
	for _, c := range []struct {
		line, hunk int
		inside     bool
	}{{0, -1, false}, {1, 0, true}, {2, 0, false}, {3, 1, true}, {4, 1, false}} {
		off := d.New.Offset(Pos{Line: c.line})
		ed.setSels([]sel{ed.newSel(off, off)}, revealNone)
		if h, in := d.CurrentHunk(); h != c.hunk || in != c.inside {
			t.Errorf("new line %d: hunk %d inside %v, want %d %v", c.line, h, in, c.hunk, c.inside)
		}
	}
}

// The wheel over what the diff shows over its lines, as a comment under a
// line, scrolls the diff, as it does over the lines.
func TestDiffScrollsUnderAnnotation(t *testing.T) {
	var a []string
	for i := range 300 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	b[1] = "changed"
	d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), Layout: DiffUnified, ExpandUnchanged: true}
	d.Annotations = []DiffAnnotation{{Side: DiffNew, Line: 1, Height: 300, Key: "comment"}}
	d.RenderAnnotation = func(c *ui.Context, an DiffAnnotation) { ui.Column(c).Fill().Children(func() { ui.Text(c, "comment") }) }
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 400)
	tt.Frame()
	r, ok := tt.Find("comment")
	if !ok {
		t.Fatal("no comment shown")
	}
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, 40)
	tt.Frame()
	if y := d.eds[1].scrollY; y != 40 {
		t.Fatalf("scrolled to %v over the comment, want 40", y)
	}
}

// The wheel over a separator, a hunk's header or the bar expanding the
// unmodified lines, scrolls the diff, as it does over the lines, in both
// layouts.
func TestDiffScrollsOverSeparator(t *testing.T) {
	var patch strings.Builder
	patch.WriteString("--- a/x\n+++ b/x\n")
	for h := range 12 {
		n := strconv.Itoa(1 + h*100)
		patch.WriteString("@@ -" + n + ",6 +" + n + ",6 @@ hunk " + strconv.Itoa(h) + "\n")
		for i := range 6 {
			patch.WriteString("-old " + strconv.Itoa(i) + "\n+new " + strconv.Itoa(i) + "\n")
		}
	}
	files, err := ParsePatch(patch.String())
	if err != nil {
		t.Fatal(err)
	}
	var a []string
	for i := range 600 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	for i := 0; i < len(b); i += 50 {
		b[i] = "changed"
	}
	for _, tc := range []struct {
		name, find string
		diff       func() *DiffState
	}{
		{"header", "@@ -201,6 +201,6 @@ hunk 2", func() *DiffState {
			d := &DiffState{}
			d.SetPatch(files[0])
			return d
		}},
		{"separator", "unmodified lines", func() *DiffState {
			return &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n"))}
		}},
	} {
		for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
			d := tc.diff()
			d.Layout = layout
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
			tt.Frame()
			tt.Frame()
			var r ui.Rect
			found := false
			for _, n := range tt.Texts() {
				if strings.Contains(n, tc.find) {
					r, found = tt.Find(n)
					break
				}
			}
			if !found {
				t.Fatalf("%s, layout %v: no %q in %q", tc.name, layout, tc.find, tt.Texts())
			}
			tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, 40)
			tt.Frame()
			if y := d.eds[1].scrollY; y != 40 {
				t.Errorf("%s, layout %v: scrolled to %v over the %s, want 40", tc.name, layout, y, tc.name)
			}
		}
	}
}

// A diff's find matches the lines it shows: the lines a gap collapses
// hold no matches, and the matches come and go as the user expands and
// collapses the gaps, in both layouts.
func TestDiffFindMatchesTheShownLines(t *testing.T) {
	var a []string
	for i := range 600 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	b[0] = "changed"
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n"))}
		d.Layout = layout
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
		tt.Frame()
		tt.Frame()
		d.Find("line 500")
		tt.Frame()
		if ed := &d.eds[1]; len(ed.find.matches) != 0 {
			t.Fatalf("%v: matches in the collapsed lines: %v", layout, ed.find.matches)
		}
		d.ExpandAll()
		tt.Frame()
		tt.Frame()
		ed := &d.eds[1]
		if len(ed.find.matches) == 0 || ed.find.matches[0].start != ed.view.text.LineStart(500) {
			t.Fatalf("%v: no match in the lines the expand shows: %v", layout, ed.find.matches)
		}
		d.CollapseAll()
		tt.Frame()
		tt.Frame()
		if ed := &d.eds[1]; len(ed.find.matches) != 0 {
			t.Fatalf("%v: matches stayed over the collapsed lines: %v", layout, ed.find.matches)
		}
		// The shown lines keep matching: the change, in view from the start.
		d.Find("changed")
		tt.Frame()
		if ed := &d.eds[1]; len(ed.find.matches) != 1 {
			t.Fatalf("%v: the shown change matched %v, want once", layout, ed.find.matches)
		}
		// A regular expression's anchors hold at the shown lines' own
		// ends: three line starts hold "line", one line ends in "3", and
		// the collapsed lines' worth of both hold nothing.
		d.eds[1].find.regex = true
		d.Find(`^line`)
		tt.Frame()
		ed = &d.eds[1]
		if len(ed.find.matches) != 3 || ed.find.matches[0].start != ed.view.text.LineStart(1) {
			t.Fatalf("%v: ^line matched %v, want the three shown line starts", layout, ed.find.matches)
		}
		d.Find(`3$`)
		tt.Frame()
		if ed := &d.eds[1]; len(ed.find.matches) != 1 {
			t.Fatalf("%v: 3$ matched %v, want the shown line's own end", layout, ed.find.matches)
		}
		d.Find(`a*`)
		tt.Frame()
		// a* holds the one a of "changed" and skips its empties.
		ed = &d.eds[1]
		if len(ed.find.matches) != 1 || ed.find.matches[0].start != ed.view.text.LineStart(0)+2 {
			t.Fatalf("%v: a* matched %v, want the shown change's a", layout, ed.find.matches)
		}
		d.Find(`\bline\b`)
		tt.Frame()
		// \b holds at the line ends: three whole words of line.
		if len(ed.find.matches) != 3 {
			t.Fatalf("%v: \\bline\\b matched %v, want the three shown lines", layout, ed.find.matches)
		}
	}
	// The old side finds by its own hidden lines: the same query over the
	// same gap of its text (split).
	d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n"))}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
	tt.Frame()
	tt.Frame()
	d.oldActive = true
	d.Find("line 500")
	tt.Frame()
	if ed := d.editorOf(DiffOld); len(ed.find.matches) != 0 {
		t.Fatalf("the old side matched the collapsed lines: %v", ed.find.matches)
	}
	d.ExpandAll()
	tt.Frame()
	tt.Frame()
	if ed := d.editorOf(DiffOld); len(ed.find.matches) == 0 {
		t.Fatal("the old side found nothing in the lines the expand shows")
	}
}

// Typing in a shown line of an editable diff, with gaps before and after
// it, searches the touched lines by their own: no panic, and the matches
// are those of a search of the whole text.
func TestDiffSearchLinesOfAnEdit(t *testing.T) {
	var a []string
	for i := range 600 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	b[300] = "changed"
	d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), Editable: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
	tt.Frame()
	tt.Frame()
	d.Find("line 30")
	tt.Frame()
	// Type in a shown line: 301, a line of context the gaps leave shown
	// before and after.
	off := d.eds[1].view.text.LineStart(301)
	d.New.Edit(Edit{Start: off, End: off + 1, Text: "x"})
	tt.Frame()
	ed := &d.eds[1]
	f := &ed.find
	// Searching only the lines the edit touched (searchLines) holds the
	// same matches as the whole text has: the edited line is dirty, of
	// the version the view is at, and the gaps around it lie past the
	// line's window — the path where the hidden spans search a slice of
	// the text.
	f.version = ed.view.version
	f.hiddenVer = ed.o.decor.(*diffSide).hideV
	f.dirty = []lineSpan{{301, 302}}
	tt.Frame()
	got := slices.Clone(f.matches)
	// The matches are those of a search of the whole text. The edit also
	// widens the hunk around it, and the context's own matches with it.
	f.stale = true
	tt.Frame()
	if !slices.Equal(got, f.matches) {
		t.Fatalf("the touched lines matched %v, a full search %v", got, f.matches)
	}
}

// A filler's stripes are those in sight, however high it is, at -45°
// every 8 DIPs from its bottom-left corner, which they keep to as it
// scrolls.
func TestFillerStripes(t *testing.T) {
	v := ui.Rect{X: 100, Y: 0, W: 600, H: 800}
	// pieces returns the stripes in v of a filler whose bottom-left corner
	// is at (100, bottom), where x+y is each at its bottom.
	pieces := func(v ui.Rect, bottom float64) []float64 {
		var out []float64
		fillerStripes(v, [2]float64{100, bottom}, func(a, b [2]float32) {
			if a[1] != v.Y+v.H || b[1] != v.Y || math.Abs(float64(b[0]-a[0]-v.H)) > 1e-3 {
				t.Fatalf("stripe from %v to %v, want from the bottom of %v to its top at -45°", a, b, v)
			}
			if a[0] >= v.X+v.W || b[0] <= v.X {
				t.Fatalf("stripe from %v to %v misses %v", a, b, v)
			}
			out = append(out, float64(a[0])+float64(a[1]))
		})
		return out
	}
	// 12M DIPs high, 700k lines, scrolled to its middle.
	bottom := 6e6 + 0.3
	got := pieces(v, bottom)
	if want := int(v.W+v.H) / 8; len(got) < want-1 || len(got) > want+1 {
		t.Fatalf("%d stripes, want about %d", len(got), want)
	}
	for i, s := range got {
		if d := math.Mod(s-(100+bottom), 8); math.Abs(d) > 1e-3 && math.Abs(math.Abs(d)-8) > 1e-3 {
			t.Fatalf("stripe %d at %v, off the corner's phase by %v", i, s, d)
		}
		if i > 0 && math.Abs(s-got[i-1]-8) > 1e-3 {
			t.Fatalf("stripes at %v and %v, want 8 DIPs apart", got[i-1], s)
		}
	}
	// Scrolled by 13.25 DIPs, the stripes move with the filler.
	moved := pieces(v, bottom-13.25)
	for _, s := range moved {
		if d := math.Mod(s+13.25-(100+bottom), 8); math.Abs(d) > 1e-3 && math.Abs(math.Abs(d)-8) > 1e-3 {
			t.Fatalf("scrolled, a stripe at %v, off the filler's phase by %v", s, d)
		}
	}
	if got := pieces(ui.Rect{X: 100, Y: 50}, bottom); len(got) != 0 {
		t.Fatalf("%d stripes in an empty rectangle", len(got))
	}
}

// A split view hatches the filler of rows a side lacks with diagonal
// stripes, as many as show, however many lines it stands for: not a
// line across its top.
func TestDiffViewerFillerHatched(t *testing.T) {
	var nw strings.Builder
	nw.WriteString("a\n")
	const added = 200_000
	for range added {
		nw.WriteString("0x4d, 0x5a, 0x90, 0x00,\n")
	}
	nw.WriteString("z\n")
	d := &DiffState{Old: NewBuffer("a\nz\n"), New: NewBuffer(nw.String()), Layout: DiffSplit}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 900, 500)
	waitCompared(t, tt, d)
	tt.SetScale(2)
	tt.Frame()
	ed := &d.eds[0]
	lh := ed.view.lineHeight
	if h := ed.view.heights.total(); h < added*float64(lh) {
		t.Fatalf("the old side is %v DIPs high, want a filler of %d lines", h, added)
	}
	bg := ed.theme.Background
	// stripes returns where, in DIPs across the old side's text, the row
	// at y in the window is darker than the background: the middle of
	// each run of such pixels.
	stripes := func(y float32) []float32 {
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 900
		var out []float32
		from := -1
		x0, x1 := int((ed.r.X+ed.gutterW+2)*s), int((ed.r.X+ed.r.W-scrollbarWidth-2)*s)
		for x := x0; x <= x1; x++ {
			c := img.RGBAAt(x, int(y*s))
			dark := x < x1 && math.Abs(float64(c.R)-float64(bg.R))+math.Abs(float64(c.G)-float64(bg.G))+math.Abs(float64(c.B)-float64(bg.B)) > 3
			switch {
			case dark && from < 0:
				from = x
			case !dark && from >= 0:
				out = append(out, float32(from+x-1)/2/s)
				from = -1
			}
		}
		return out
	}
	// In the filler's top row of pixels, a stripe every 8 DIPs, not a
	// line across.
	top := ed.r.Y + lh + 0.25
	n := (ed.r.W - ed.gutterW - scrollbarWidth - 4) / 8
	if got := stripes(top); float32(len(got)) < n-2 || float32(len(got)) > n+2 {
		t.Fatalf("at the filler's top, the row is dark in %d runs %v, want a stripe every 8 DIPs (%v)", len(got), got, n)
	}
	// Halfway down, the stripes go up to the right, at -45°.
	tt.Scroll(ed.r.X+ed.r.W/2, ed.r.Y+ed.r.H/2, 0, float32(added/2)*lh+0.5)
	tt.Frame()
	if ed.scrollY < float64(added/4)*float64(lh) {
		t.Fatalf("scrolled to %v", ed.scrollY)
	}
	y := ed.r.Y + ed.r.H/2
	a, b := stripes(y), stripes(y+3)
	if float32(len(a)) < n-2 || float32(len(a)) > n+2 {
		t.Fatalf("halfway down the filler, the row is dark in %d runs %v, want a stripe every 8 DIPs (%v)", len(a), a, n)
	}
	for i := 1; i < len(a)-1; i++ {
		// 3 DIPs lower, a stripe is 3 DIPs to the left.
		found := false
		for _, x := range b {
			if math.Abs(float64(x-(a[i]-3))) < 1 {
				found = true
			}
		}
		if !found {
			t.Fatalf("a stripe at %v, and 3 DIPs lower at %v, want one at %v", a[i], b, a[i]-3)
		}
	}
}

// A diff as high as its lines, in a scroll container, shows a horizontal
// scroll bar at the bottom of the part of it the window shows, on both
// sides of a split diff, and dragging it scrolls the lines sideways.
func TestFitContentDiffHorizontalScrollbar(t *testing.T) {
	var a []string
	for i := range 200 {
		a = append(a, "line "+strconv.Itoa(i)+" "+strings.Repeat("x", 200))
	}
	b := append([]string(nil), a...)
	b[1] = "changed " + strings.Repeat("y", 200)
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), Layout: layout, FitContent: true, ExpandUnchanged: true}
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Scroll(c).Fill().Children(func() { DiffView(c, d).FillWidth() })
		}, 900, 400)
		tt.Frame()
		tt.Frame()
		eds := []*editor{&d.eds[1]}
		if layout == DiffSplit {
			eds = append(eds, &d.eds[0])
		}
		for _, ed := range eds {
			if ed.r.H <= 400 {
				t.Fatalf("%v: editor %v high, want its lines", layout, ed.r.H)
			}
			if _, h := ed.scrollbars(); !h {
				t.Fatalf("%v: no horizontal scroll bar over lines wider than the view", layout)
			}
			if want := 400 - ed.r.Y - scrollbarWidth; ed.hbarY != want {
				t.Fatalf("%v: horizontal bar at %v, want %v, at the window's bottom", layout, ed.hbarY, want)
			}
		}
		ed := eds[0]
		y := ed.r.Y + ed.hbarY + scrollbarWidth/2
		x := ed.r.X + ed.textX + 2
		tt.Move(x, y)
		tt.Press(x, y)
		tt.Move(x+200, y)
		tt.Release(x+200, y)
		tt.Frame()
		if ed.scrollX <= 0 {
			t.Fatalf("%v: dragging the bar scrolled to %v", layout, ed.scrollX)
		}
	}
}

// A diff's scroll position, the height it shows and where its lines are
// read as the last frame laid them out, however it scrolled: the wheel,
// the keys, the scroll bar's thumb, GoToHunk and SetScrollY; and the view
// built after a scroll reads it, as a minimap beside the diff does.
func TestDiffScrollPosition(t *testing.T) {
	var a []string
	for i := range 600 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	for i := 25; i < len(b); i += 50 {
		b[i] = "changed"
	}
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), Layout: layout, Editable: true}
		var built float64
		tt := ui.NewTester(func(c *ui.Context) {
			built = d.ScrollY()
			DiffView(c, d).Fill()
		}, 800, 600)
		tt.Frame()
		tt.Frame()
		check := func(how string) {
			t.Helper()
			tt.Frame()
			y := d.ScrollY()
			if y != d.eds[1].scrollY || layout == DiffSplit && y != d.eds[0].scrollY {
				t.Fatalf("layout %v, %s: ScrollY %v, the sides %v and %v", layout, how, y, d.eds[0].scrollY, d.eds[1].scrollY)
			}
			if built != y {
				t.Fatalf("layout %v, %s: the view was last built at %v, ScrollY %v", layout, how, built, y)
			}
		}
		if h := d.ViewportHeight(); h != 600 {
			t.Fatalf("layout %v: ViewportHeight %v, want 600", layout, h)
		}
		total := d.LineTop(DiffNew, d.New.LineCount())
		if total != d.eds[1].view.heights.total() || total <= 600 {
			t.Fatalf("layout %v: the diff is %v high, its rows %v", layout, total, d.eds[1].view.heights.total())
		}
		// The collapsed lines take no room: far less than 600 lines.
		if lh := float64(d.eds[1].view.lineHeight); total >= 600*lh {
			t.Fatalf("layout %v: the diff is %v high, as all its lines", layout, total)
		}
		if old := d.LineTop(DiffOld, d.Old.LineCount()); old != total {
			t.Errorf("layout %v: the old side is %v high, the new %v", layout, old, total)
		}
		// The wheel.
		tt.Scroll(600, 300, 0, 40)
		check("the wheel")
		if d.ScrollY() != 40 {
			t.Fatalf("layout %v: ScrollY %v after the wheel, want 40", layout, d.ScrollY())
		}
		// The keys, in the new side's editor.
		tt.ClickAt(700, 300)
		before := d.ScrollY()
		for range 3 {
			tt.Key(0, ui.KeyPageDown)
		}
		check("the keys")
		if d.ScrollY() <= before {
			t.Fatalf("layout %v: Page Down left ScrollY at %v", layout, d.ScrollY())
		}
		// The thumb.
		ed := &d.eds[1]
		t0, n := ed.bar(false).Thumb()
		x, y := ed.r.X+ed.r.W-scrollbarWidth/2, ed.r.Y+t0+n/2
		before = d.ScrollY()
		// Up, as Page Down went to the end.
		tt.Press(x, y)
		tt.Move(x, y-100)
		tt.Release(x, y-100)
		check("the thumb")
		if d.ScrollY() >= before {
			t.Fatalf("layout %v: dragging the thumb left ScrollY at %v", layout, d.ScrollY())
		}
		// GoToHunk, which scrolls as the frame lays the view out.
		last := len(d.Hunks()) - 1
		d.GoToHunk(last)
		check("GoToHunk")
		top := d.LineTop(DiffNew, d.Hunks()[last].NewStart)
		if y := d.ScrollY(); top < y || top >= y+d.ViewportHeight() {
			t.Fatalf("layout %v: GoToHunk showed %v to %v, the hunk at %v", layout, y, y+d.ViewportHeight(), top)
		}
		// SetScrollY, as far as the diff scrolls.
		// Line 323, the first after the lines collapsed under line 275's
		// change.
		d.SetScrollY(d.LineTop(DiffNew, 323))
		check("SetScrollY")
		if d.ScrollY() != d.LineTop(DiffNew, 323) {
			t.Fatalf("layout %v: SetScrollY scrolled to %v, want %v", layout, d.ScrollY(), d.LineTop(DiffNew, 323))
		}
		if first := d.eds[1].shown[0]; first != 323 {
			t.Errorf("layout %v: SetScrollY to line 323 shows line %d first", layout, first)
		}
		d.SetScrollY(-5)
		check("SetScrollY above the top")
		if d.ScrollY() != 0 {
			t.Fatalf("layout %v: SetScrollY(-5) scrolled to %v", layout, d.ScrollY())
		}
		d.SetScrollY(math.MaxFloat64)
		check("SetScrollY past the end")
		if y := d.ScrollY(); y != d.eds[1].scrollMax() || y <= 0 {
			t.Fatalf("layout %v: SetScrollY past the end scrolled to %v, want %v", layout, y, d.eds[1].scrollMax())
		}
	}
}

// LineTop places the lines of both sides where they show: side by side,
// a changed line across from its new version; one over the other, the
// deleted line above the new one, and an unchanged line of the old text
// where it is in the new.
func TestDiffLineTop(t *testing.T) {
	a := []string{"a", "b", "c", "d", "e", "f"}
	b := []string{"a", "B", "c", "d", "e", "f", "g"}
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer(strings.Join(a, "\n") + "\n"), New: NewBuffer(strings.Join(b, "\n") + "\n"), Layout: layout, ExpandUnchanged: true}
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
		tt.Frame()
		tt.Frame()
		lh := float64(d.eds[1].view.lineHeight)
		for line := range b {
			if got, want := d.LineTop(DiffNew, line), float64(line)*lh; layout == DiffSplit && got != want {
				t.Errorf("split: new line %d at %v, want %v", line, got, want)
			}
		}
		switch layout {
		case DiffSplit:
			if got, want := d.LineTop(DiffOld, 1), d.LineTop(DiffNew, 1); got != want {
				t.Errorf("split: old line 1 at %v, its new version at %v", got, want)
			}
		case DiffUnified:
			if got, want := d.LineTop(DiffOld, 1), 1*lh; got != want {
				t.Errorf("unified: deleted line 1 at %v, want %v", got, want)
			}
			if got, want := d.LineTop(DiffNew, 1), 2*lh; got != want {
				t.Errorf("unified: new line 1 at %v, want %v below the deleted one", got, want)
			}
			for _, line := range []int{0, 2, 5} {
				if got, want := d.LineTop(DiffOld, line), d.LineTop(DiffNew, line); got != want {
					t.Errorf("unified: unchanged old line %d at %v, the new at %v", line, got, want)
				}
			}
			if got, want := d.LineTop(DiffNew, len(b)), float64(len(b)+1)*lh; got != want {
				t.Errorf("unified: the diff is %v high, want %v", got, want)
			}
		}
		if got, want := d.LineTop(DiffOld, len(a)), d.LineTop(DiffNew, len(b)); got != want {
			t.Errorf("layout %v: the old side ends at %v, the new at %v", layout, got, want)
		}
	}
}

// KeepScrollbars shows the diff's thumb with the pointer away, and
// NoScrollbars takes it away, with its room, on both sides.
func TestDiffScrollbarsKeptAndHidden(t *testing.T) {
	var a []string
	for i := range 200 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	b[100] = "changed"
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), Layout: layout, ExpandUnchanged: true}
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
		tt.Move(-10, -10)
		tt.Frame()
		column := func() []uint32 {
			img := tt.Image()
			s := img.Bounds().Dx() / 800
			var px []uint32
			for y := 0; y < 600*s; y++ {
				for x := (800 - scrollbarWidth) * s; x < 800*s; x++ {
					c := img.RGBAAt(x, y)
					px = append(px, uint32(c.R)<<24|uint32(c.G)<<16|uint32(c.B)<<8|uint32(c.A))
				}
			}
			return px
		}
		none := column()
		d.KeepScrollbars = true
		tt.Frame()
		if slices.Equal(column(), none) {
			t.Errorf("layout %v: a kept thumb doesn't show with the pointer away", layout)
		}
		d.KeepScrollbars, d.NoScrollbars = false, true
		tt.Move(400, 300)
		tt.Frame()
		for i := range d.eds {
			if i == 0 && layout == DiffUnified {
				continue
			}
			ed := &d.eds[i]
			if v, h := ed.scrollbars(); v || h || ed.trackWidth() != 0 {
				t.Errorf("layout %v, side %d: NoScrollbars left bars %v %v, room %v", layout, i, v, h, ed.trackWidth())
			}
		}
		if layout == DiffSplit && d.eds[0].view.wrapWidth != d.eds[1].view.wrapWidth {
			t.Errorf("NoScrollbars: the sides wrap at %v and %v", d.eds[0].view.wrapWidth, d.eds[1].view.wrapWidth)
		}
	}
}

// A minimap beside a diff, as its guide builds one, shows where the view
// is and scrolls the diff where it is pressed and dragged, and follows
// the diff as it scrolls.
func TestDiffMinimap(t *testing.T) {
	var a []string
	for i := range 600 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	for i := 25; i < len(b); i += 50 {
		b[i] = "changed"
	}
	d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), KeepScrollbars: true}
	var box ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		var scale float64
		ui.Row(c).Fill().Children(func() {
			DiffView(c, d).Grow(1).FillHeight()
			mini := ui.Box(c).Width(18).FillHeight()
			mini.Draw(func(p *ui.Painter, r ui.Rect) {
				scale = float64(r.H) / max(d.LineTop(DiffNew, d.New.LineCount()), 1)
				at := func(y float64) float32 { return r.Y + float32(y*scale) }
				for _, h := range d.Hunks() {
					top := at(d.LineTop(DiffNew, h.NewStart))
					bottom := at(d.LineTop(DiffNew, max(h.NewEnd, h.NewStart+1)))
					p.Fill(ui.Rect{X: r.X, Y: top, W: r.W, H: max(bottom-top, 2)}, ui.RGB(46, 160, 67), 0)
				}
				box = ui.Rect{X: r.X, Y: at(d.ScrollY()), W: r.W, H: float32(d.ViewportHeight() * scale)}
				p.Stroke(box, c.Theme().Border, 0, 1)
			})
			mini.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputPointerDown || ev.Kind == ui.InputPointerMove && ev.Button == 0 {
					d.SetScrollY(float64(ev.Y)/max(scale, 1e-9) - d.ViewportHeight()/2)
					return true
				}
				return ev.Kind == ui.InputPointerUp
			})
		})
	}, 800, 600)
	tt.Frame()
	tt.Frame()
	total := d.LineTop(DiffNew, d.New.LineCount())
	if box.Y != 0 || math.Abs(float64(box.H)-600*600/total) > 0.01 {
		t.Fatalf("the view's box at %v, want at the top, %v high", box, 600*600/total)
	}
	tt.Press(791, 300)
	tt.Move(791, 400)
	tt.Release(791, 400)
	tt.Frame()
	want := 400*total/600 - 300
	if want <= 0 || want >= d.eds[1].scrollMax() {
		t.Fatalf("the fixture scrolls to %v, out of 0 to %v", want, d.eds[1].scrollMax())
	}
	if math.Abs(d.ScrollY()-want) > 0.01 {
		t.Fatalf("dragging the minimap to 400 scrolled to %v, want %v", d.ScrollY(), want)
	}
	if mid := float64(box.Y + box.H/2); math.Abs(mid-400) > 0.01 {
		t.Fatalf("the view's box is centered at %v, want 400", mid)
	}
	// The wheel over the diff moves the box.
	before := box.Y
	tt.Scroll(600, 300, 0, -100)
	tt.Frame()
	if box.Y >= before {
		t.Fatalf("the view's box stayed at %v as the diff scrolled up", box.Y)
	}
}

// Unchanged lines collapsed into a separator are at the separator's top,
// before the first change, between changes and after the last, on both
// sides of both layouts; past the last line is the height of the diff.
func TestDiffLineTopCollapsed(t *testing.T) {
	var a []string
	for i := range 100 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	b[20], b[60] = "changed", "changed"
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), Layout: layout}
		seps := map[int]float64{} // by the old line the gap starts at
		d.RenderSeparator = func(c *ui.Context, sep ui.Element, s *DiffSeparator) {
			seps[s.a] = float64(sep.Bounds().Y)
			DefaultSeparator(c, sep, s)
		}
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 2000)
		tt.Frame()
		tt.Frame()
		// The gaps: lines 0 to 16, 24 to 56 and 64 to 99 collapse.
		for _, gap := range []struct{ start, line int }{{0, 5}, {21, 40}, {61, 80}} {
			top, ok := seps[gap.start]
			if !ok {
				t.Fatalf("layout %v: no separator of the gap at %d: %v", layout, gap.start, seps)
			}
			for _, side := range []DiffSide{DiffOld, DiffNew} {
				if got := d.LineTop(side, gap.line); got != top {
					t.Errorf("layout %v, side %v: collapsed line %d at %v, its separator's top %v", layout, side, gap.line, got, top)
				}
			}
		}
		total := d.eds[1].view.heights.total()
		for _, side := range []DiffSide{DiffOld, DiffNew} {
			if got := d.LineTop(side, 100); got != total {
				t.Errorf("layout %v, side %v: past the last line at %v, want the height %v", layout, side, got, total)
			}
		}
	}
}

// A view built after the diff or the editor was resized reads the height
// it shows now, though the scroll and the content stay.
func TestViewportHeightAfterResize(t *testing.T) {
	d := &DiffState{Old: NewBuffer("a\nb\nc"), New: NewBuffer("a\nB\nc")}
	st := &State{Buffer: NewBuffer("a\nb\nc")}
	var diffH, editorH float64
	tt := ui.NewTester(func(c *ui.Context) {
		diffH, editorH = d.ViewportHeight(), st.ViewportHeight()
		ui.Column(c).Fill().Children(func() {
			DiffView(c, d).Grow(1).FillWidth()
			View(c, st).Grow(1).FillWidth()
		})
	}, 800, 600)
	tt.Frame()
	if diffH != 300 || editorH != 300 {
		t.Fatalf("the view read heights %v and %v, want 300", diffH, editorH)
	}
	// The frame of the resize builds before it lays out: the view is
	// built again, with no other frame asked for.
	tt.SetSize(800, 400)
	if d.ViewportHeight() != 200 || diffH != 200 || editorH != 200 {
		t.Fatalf("after a resize the view read heights %v and %v, the diff's %v, want 200", diffH, editorH, d.ViewportHeight())
	}
}

// splitDiff returns a split diff of lines, long ones when long, whose
// line 20 changed.
func splitDiff(long bool) *DiffState {
	var a, b []string
	for i := range 40 {
		line := "line " + strconv.Itoa(i)
		if long {
			line += strings.Repeat(" word", 30)
		}
		a = append(a, line)
		if i == 20 {
			line = "changed " + line
		}
		b = append(b, line)
	}
	return &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), ExpandUnchanged: true}
}

// sideWidths returns the widths the sides of a split diff were drawn at,
// and the left of the new one.
func sideWidths(d *DiffState) (old, nw, newX float32) {
	return d.eds[0].r.W, d.eds[1].r.W, d.eds[1].r.X
}

// A split diff's sides share its width by SplitPercent, as two halves
// with none, the line between them taking a DIP.
func TestDiffSplitShare(t *testing.T) {
	d := splitDiff(false)
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 901, 400)
	tt.Frame()
	if a, b, x := sideWidths(d); a != 450 || b != 450 || x != 451 {
		t.Fatalf("with no share, the sides are %v and %v wide, the new one at %v, want halves", a, b, x)
	}
	if tt.FramePending() {
		t.Fatal("the halves ask for frames")
	}
	d.SplitPercent = 30
	tt.Frame()
	if a, b, x := sideWidths(d); a != 270 || b != 630 || x != 271 {
		t.Fatalf("at 30%%, the sides are %v and %v wide, the new one at %v", a, b, x)
	}
	// They keep the share as the view grows and shrinks.
	tt.SetSize(1201, 400)
	tt.Frame()
	if a, b, _ := sideWidths(d); a != 360 || b != 840 {
		t.Fatalf("at 30%% of a wider view, the sides are %v and %v wide", a, b)
	}
	if tt.FramePending() {
		t.Fatal("the share asks for frames")
	}
}

// The pointer drags the line between the sides, from a few DIPs either
// side of it, which focuses it; SplitPercent follows.
func TestDiffSplitDrag(t *testing.T) {
	d := splitDiff(false)
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 901, 400)
	tt.Frame()
	x, y := float32(450.5), float32(200)
	tt.Press(x, y)
	if !tt.Focused("Divider") {
		t.Fatal("a press on the line does not focus it")
	}
	if tt.Cursor() != ui.CursorResizeEW {
		t.Fatalf("the cursor over the line is %v", tt.Cursor())
	}
	tt.Move(x+90, y)
	tt.Release(x+90, y)
	if a, b, nx := sideWidths(d); a != 540 || b != 360 || nx != 541 {
		t.Fatalf("dragged 90 DIPs right, the sides are %v and %v wide, the new one at %v", a, b, nx)
	}
	if d.SplitPercent != 60 {
		t.Fatalf("dragged to 540 of 900 DIPs, SplitPercent is %v", d.SplitPercent)
	}
	// From over the old side as from over the new, and with the
	// release's own move.
	tt.Press(538, y)
	tt.Move(448, y)
	tt.Release(358, y)
	if a, _, _ := sideWidths(d); a != 360 || d.SplitPercent != 40 {
		t.Fatalf("dragged 180 DIPs left, the old side is %v wide, at %v%%", a, d.SplitPercent)
	}
	// A press without a move keeps the share, as the editors' text.
	tt.Press(360.5, y)
	tt.Release(360.5, y)
	if d.SplitPercent != 40 || d.Old.String() != splitDiff(false).Old.String() {
		t.Fatalf("a press moved the line to %v%%", d.SplitPercent)
	}
}

// The line between the sides crosses the separators, which span both:
// it is drawn over them, and dragged from over them as from over the
// lines.
func TestDiffSplitLineCrossesSeparators(t *testing.T) {
	var old strings.Builder
	for i := range 40 {
		old.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	nw := strings.Replace(strings.Replace(old.String(), "line 2\n", "line two\n", 1), "line 37\n", "line thirty-seven\n", 1)
	d := &DiffState{Old: NewBuffer(old.String()), New: NewBuffer(nw), Layout: DiffSplit}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 901, 600)
	waitCompared(t, tt, d)
	var sep ui.AccessInfo
	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleGroup && strings.HasSuffix(a.Name, "unmodified lines") {
			sep = a
		}
	}
	if sep.Bounds.H == 0 {
		t.Fatalf("no separator in %q", tt.Texts())
	}
	img := tt.Image()
	line := img.RGBAAt(450, int(d.eds[0].r.Y+d.eds[0].view.lineHeight/2))
	y := sep.Bounds.Y + sep.Bounds.H/2
	if got := img.RGBAAt(450, int(y)); got != line {
		t.Fatalf("over the separator the line is drawn %v, over the lines %v", got, line)
	}
	tt.Press(450.5, y)
	tt.Move(540.5, y)
	tt.Release(540.5, y)
	if d.SplitPercent != 60 {
		t.Fatalf("dragged from over the separator to 540 of 900 DIPs, SplitPercent is %v", d.SplitPercent)
	}
}

// Tab goes from the old side to the line between the sides and on to the
// new side, and the arrows move the line.
func TestDiffSplitKeys(t *testing.T) {
	d := splitDiff(false)
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 901, 400)
	tt.Frame()
	tt.ClickAt(200, 10)
	if d.ActiveSide() != DiffOld {
		t.Fatal("a click on the old side does not give it the keyboard")
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Divider") {
		t.Fatal("Tab from the old side does not focus the line")
	}
	// By 10 DIPs, as ui.Split's; the share is a percentage, which the
	// sides' widths come back from as near as floats are.
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }
	tt.Key(0, ui.KeyRight)
	if a, b, _ := sideWidths(d); !near(a, 460) || !near(b, 440) {
		t.Fatalf("Right on the line: the sides are %v and %v wide", a, b)
	}
	tt.Key(0, ui.KeyLeft)
	tt.Key(0, ui.KeyLeft)
	if a, _, _ := sideWidths(d); !near(a, 440) || !near(d.SplitPercent, 440*100/float32(900)) {
		t.Fatalf("Left twice: the old side is %v wide, at %v%%", a, d.SplitPercent)
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("New text") {
		t.Fatal("Tab from the line does not go to the new side")
	}
}

// Neither side is dragged narrower than its gutter and 12 columns, and a
// view too narrow for both shows halves, keeping the share for when it is
// wide enough again.
func TestDiffSplitMinimum(t *testing.T) {
	d := splitDiff(false)
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 901, 400)
	tt.Frame()
	ed := &d.eds[0]
	least := ed.textX + diffMinColumns*ed.view.charWidth + ed.trackWidth() + 4
	if least < 100 {
		t.Fatalf("a side keeps only %v DIPs", least)
	}
	tt.Press(450.5, 200)
	tt.Move(0, 200)
	tt.Release(0, 200)
	if a, b, _ := sideWidths(d); a != least || b != 900-least {
		t.Fatalf("dragged to the left edge, the sides are %v and %v wide, want the old one %v", a, b, least)
	}
	tt.Press(least+0.5, 200)
	tt.Move(900, 200)
	tt.Release(900, 200)
	if a, b, _ := sideWidths(d); b != least || a != 900-least {
		t.Fatalf("dragged to the right edge, the sides are %v and %v wide, want the new one %v", a, b, least)
	}
	// The arrows stop there too.
	tt.Key(0, ui.KeyRight)
	if _, b, _ := sideWidths(d); b != least {
		t.Fatalf("Right at the edge: the new side is %v wide", b)
	}
	// A share the app sets past them shows at them.
	d.SplitPercent = 1
	tt.Frame()
	if a, _, _ := sideWidths(d); a != least {
		t.Fatalf("at 1%%, the old side is %v wide", a)
	}
	// Too narrow for both: halves, the share kept.
	tt.SetSize(201, 400)
	tt.Frame()
	if a, b, x := sideWidths(d); a != 100 || b != 100 || x != 101 {
		t.Fatalf("in 201 DIPs, the sides are %v and %v wide, the new one at %v", a, b, x)
	}
	tt.Press(100.5, 200)
	tt.Move(150, 200)
	tt.Release(150, 200)
	if d.SplitPercent != 1 {
		t.Fatalf("a drag where the sides do not fit set the share to %v", d.SplitPercent)
	}
	tt.SetSize(901, 400)
	tt.Frame()
	if a, _, _ := sideWidths(d); a != least {
		t.Fatalf("wide again, the old side is %v wide", a)
	}
}

// A hunk's header in a split diff keeps to the old side, cut at the line
// between the sides wherever that is dragged, though its bar spans both;
// a unified diff shows it whole.
func TestDiffSplitHeaderKeepsToOldSide(t *testing.T) {
	const header = "@@ -21,6 +21,7 @@ import jakarta.servlet.AsyncContext;"
	files, err := ParsePatch("--- a/x\n+++ b/x\n" + header + "\n a\n b\n c\n+d\n e\n f\n g\n")
	if err != nil {
		t.Fatal(err)
	}
	// headerBox returns the box of the header's text, not that of the
	// separator labeled with it.
	headerBox := func(tt *ui.Tester) ui.Rect {
		t.Helper()
		a, ok := tt.AccessByRole(ui.RoleText, header)
		if !ok {
			t.Fatalf("no %q in %q", header, tt.Texts())
		}
		return a.Bounds
	}
	d := &DiffState{}
	d.SetPatch(files[0])
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 901, 400)
	tt.Frame()
	tt.Press(450.5, 200)
	tt.Move(0, 200)
	tt.Release(0, 200)
	tt.Frame()
	old, _, _ := sideWidths(d)
	if old > 200 {
		t.Fatalf("dragged to the left edge, the old side is %v wide", old)
	}
	if r := headerBox(tt); r.X+r.W > old {
		t.Fatalf("the header's text spans %v to %v, past the old side's right edge at %v", r.X, r.X+r.W, old)
	}

	// Unified, nothing cuts it: it is as wide as in a view twice as wide.
	d.Layout = DiffUnified
	tt.Frame()
	whole := headerBox(tt)
	tt.SetSize(1801, 400)
	tt.Frame()
	if r := headerBox(tt); whole.W != r.W || whole.W <= old {
		t.Fatalf("unified, the header's text is %v wide, %v in a wider view", whole.W, r.W)
	}
}

// Sides of different widths keep scrolling together and their lines
// aligned, wrapped too, each wrapping at its own width.
func TestDiffSplitAligned(t *testing.T) {
	for _, c := range []struct{ wrap, collapse bool }{{false, false}, {true, false}, {true, true}} {
		wrap := c.wrap
		d := splitDiff(true)
		d.Wrap, d.SplitPercent, d.ExpandUnchanged = wrap, 30, !c.collapse
		d.Annotations = []DiffAnnotation{{Side: DiffNew, Line: 22, Height: 40}}
		d.RenderAnnotation = func(c *ui.Context, a DiffAnnotation) { ui.Text(c, "note") }
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 901, 400)
		tt.Frame()
		tt.Press(270.5, 200)
		tt.Move(200, 200)
		tt.Release(200, 200)
		tt.Frame()
		if tt.FramePending() {
			t.Fatalf("%+v: the sides ask for frames", c)
		}
		ha, hb := &d.eds[0].view.heights, &d.eds[1].view.heights
		if ha.total() != hb.total() {
			t.Fatalf("%+v: heights %v and %v", c, ha.total(), hb.total())
		}
		lines := []int{0, 5, 19, 21, 22, 23, 39}
		if c.collapse {
			// The context around the change, and the separators' tops.
			lines = []int{17, 18, 19, 21, 22, 23, 24, 39}
		}
		for _, l := range lines {
			if ga, gb := rowsTop(ha, l), rowsTop(hb, l); ga != gb {
				t.Fatalf("%+v: old line %d at %v, new line at %v", c, l, ga, gb)
			}
		}
		if wrap && ha.wraps.rows(19) <= hb.wraps.rows(19) {
			t.Fatalf("the narrow old side wraps line 19 in %d rows, the new side in %d", ha.wraps.rows(19), hb.wraps.rows(19))
		}
		if c.collapse {
			continue
		}
		tt.Scroll(600, 200, 0, 120)
		tt.Frame()
		if d.eds[0].scrollY != 120 || d.eds[1].scrollY != 120 {
			t.Fatalf("%+v: scrolled the sides to %v and %v", c, d.eds[0].scrollY, d.eds[1].scrollY)
		}
		if !wrap {
			// Sideways the sides scroll together, as before, each as far
			// as its own lines run past it: the narrow old side further.
			old, nw := &d.eds[0], &d.eds[1]
			tt.Scroll(600, 200, 5000, 0)
			tt.Frame()
			if m := nw.maxScrollX(); m <= 0 || nw.scrollX != m || old.scrollX != m {
				t.Fatalf("over the new side, scrolled sideways to %v and %v, the new side's end %v", old.scrollX, nw.scrollX, m)
			}
			tt.Scroll(100, 200, 5000, 0)
			tt.Frame()
			if m := old.maxScrollX(); m <= nw.maxScrollX() || old.scrollX != m || nw.scrollX != nw.maxScrollX() {
				t.Fatalf("over the old side, scrolled sideways to %v of %v, the new side to %v of %v", old.scrollX, m, nw.scrollX, nw.maxScrollX())
			}
		}
	}
}

// A diff whose state is reset between frames, as an app does while it
// reads the next file's texts, can get a pointer move before the next
// frame lays its editors out. The move must not panic.
func TestDiffViewPointerMoveAfterReset(t *testing.T) {
	st := &DiffState{Old: NewBuffer("a\nb\n"), New: NewBuffer("a\nc\n")}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, st).Fill() }, 600, 300)
	tt.Frame()
	ed := &st.eds[1]
	x, y := ed.r.X+ed.r.W/2, ed.r.Y+ed.r.H/2
	tt.Move(x, y)
	*st = DiffState{}
	tt.Move(x+1, y+1)
}

// An empty side of a diff, as of a file added or deleted, has no lines,
// as git shows it: the line its editor keeps is not numbered, selected
// by its number or commented on.
func TestDiffViewEmptySide(t *testing.T) {
	added, _ := ParsePatch("diff --git a/x.go b/x.go\nnew file mode 100644\n--- /dev/null\n+++ b/x.go\n@@ -0,0 +1,2 @@\n+a\n+b\n")
	for _, c := range []struct {
		name  string
		st    *DiffState
		empty DiffSide
	}{
		{"added", &DiffState{New: NewBuffer("a\nb\n")}, DiffOld},
		{"deleted", &DiffState{Old: NewBuffer("a\nb\n")}, DiffNew},
		{"deleted unified", &DiffState{Old: NewBuffer("a\nb\n"), Layout: DiffUnified}, DiffNew},
		{"emptied", &DiffState{Old: NewBuffer("a\nb\n"), New: NewBuffer("")}, DiffNew},
		{"added patch", func() *DiffState { d := &DiffState{}; d.SetPatch(added[0]); return d }(), DiffOld},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := c.st
			var selected, commented []DiffLineRange
			d.LineSelection = true
			d.OnLineSelection = func(r DiffLineRange) { selected = append(selected, r) }
			d.OnGutterButton = func(r DiffLineRange) { commented = append(commented, r) }
			d.RenderGutterButton = func(c *ui.Context, btn ui.Element, line DiffLineRange) { btn.Label("Comment") }
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
			tt.Frame()
			// inked reports whether the gutter of line 0 of a side shows
			// anything but its background.
			inked := func(side DiffSide) bool {
				ed := &d.eds[side]
				lh := ed.view.lineHeight
				y := ed.r.Y + float32(ed.view.heights.geometry(0).rows-ed.scrollY)
				img := tt.Image()
				s := float32(img.Bounds().Dx()) / 600
				first := img.RGBAAt(int((ed.r.X+1)*s), int((y+1)*s))
				for py := int((y + 1) * s); py < int((y+lh-1)*s); py++ {
					for px := int((ed.r.X + 1) * s); px < int((ed.r.X+ed.gutterW-1)*s); px++ {
						if img.RGBAAt(px, py) != first {
							return true
						}
					}
				}
				return false
			}
			if inked(c.empty) {
				t.Fatal("the empty side's line is numbered")
			}
			if d.Layout == DiffSplit && !inked(1-c.empty) {
				t.Fatal("the other side's first line is not numbered")
			}
			// Its gutter selects nothing and offers no button.
			ed := &d.eds[c.empty]
			y := ed.r.Y + float32(ed.view.heights.geometry(0).rows-ed.scrollY) + ed.view.lineHeight/2
			tt.Move(ed.r.X+ed.gutterW/2, y)
			tt.Frame()
			if tt.HasText("Comment") {
				t.Fatal("the empty side's line has a gutter button")
			}
			tt.ClickAt(ed.r.X+ed.gutterW-8, y)
			if _, ok := d.SelectedLines(); ok || len(selected) > 0 || len(commented) > 0 {
				t.Fatalf("selected %+v, commented %+v on the empty side", selected, commented)
			}
		})
	}
}

// A unified view lays out the deleted lines it shows of a long run, as of
// a file deleted, not those above them, and gives their highlighter the
// frames it asks for.
func TestDiffViewDeletedRunInView(t *testing.T) {
	var lines []string
	for i := range 5000 {
		lines = append(lines, "fn line"+strconv.Itoa(i))
	}
	var hl *parserHighlighter
	lang := &Language{Name: "Parsed", Highlighter: func(text Snapshot, invalidate func()) Highlighter {
		hl = &parserHighlighter{text: text, invalidate: invalidate, pending: 10}
		return hl
	}}
	d := &DiffState{Old: NewBuffer(strings.Join(lines, "\n")), New: NewBuffer(""), Layout: DiffUnified}
	d.Old.SetLanguage(lang)
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
	tt.Frame()
	d.SetScrollY(math.MaxFloat64)
	tt.Frame()
	if n := len(d.oldView.lines.m); n > 100 {
		t.Fatalf("%d deleted lines laid out to show the last few", n)
	}
	if hl == nil || hl.pending > 0 {
		t.Fatal("the deleted lines' highlighter got no frames to finish in")
	}
	if l := d.oldView.line(len(lines) - 1); !l.exact || len(l.toks) == 0 || l.toks[0].Kind != TokenKeyword {
		t.Fatalf("the last deleted line's tokens: %+v, final %v", l.toks, l.exact)
	}
}

// A unified view forgets the deleted lines it laid out, and the rows it
// shaped, once frames stop showing them, as an editor does its own: a
// diff scrolled through keeps no more of them than of a screen or two.
func TestDiffViewForgetsDeletedLines(t *testing.T) {
	var old, nw []string
	for i := range 6000 {
		line := "line " + strconv.Itoa(i)
		old = append(old, line)
		if i%3 != 0 {
			nw = append(nw, line)
		}
	}
	d := &DiffState{Old: NewBuffer(strings.Join(old, "\n")), New: NewBuffer(strings.Join(nw, "\n")), Layout: DiffUnified, ExpandUnchanged: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
	tt.Frame()
	for y := 0.0; y < d.eds[1].view.heights.total(); y += 250 {
		d.SetScrollY(y)
		tt.Frame()
	}
	ov := &d.oldView
	if n := len(ov.lines.m); n > 600 {
		t.Errorf("%d of 2,000 deleted lines still laid out", n)
	}
	if n := len(ov.shaped.cur) + len(ov.shaped.prev); n > 600 {
		t.Errorf("%d shaped rows of 2,000 deleted lines still kept", n)
	}
}

// largeDiffTexts returns texts of n lines each, past what is compared on
// the main thread, every 50th line changed.
func largeDiffTexts(n int) (string, string) {
	var a, b strings.Builder
	for i := range n {
		line := "\tvalue := compute(" + strconv.Itoa(i) + ")\n"
		a.WriteString(line)
		if i%50 == 7 {
			line = "\tvalue := computeAgain(" + strconv.Itoa(i) + ")\n"
		}
		b.WriteString(line)
	}
	return a.String(), b.String()
}

// holdDiffs keeps the comparisons started off the main thread, for the
// test to run when it chooses.
func holdDiffs(t *testing.T) *[]func() {
	var jobs []func()
	prev := startDiff
	startDiff = func(job func()) { jobs = append(jobs, job) }
	t.Cleanup(func() { startDiff = prev })
	return &jobs
}

// waitCompared waits for a comparison of d off the main thread to be
// done and the window to draw it.
func waitCompared(t *testing.T, tt *ui.Tester, d *DiffState) {
	t.Helper()
	for d.Comparing() {
		wait.Until(t, "the window to draw again after the comparison", tt.FramePending)
		tt.Frame()
	}
}

// syncHunks returns the hunks of old and nw compared on the spot, as
// with no window.
func syncHunks(old, nw string, editable bool) []DiffHunk {
	return (&DiffState{Old: NewBuffer(old), New: NewBuffer(nw), Editable: editable}).Hunks()
}

// A large diff is compared off the main thread: its first frame shows the
// texts without changes, and the window draws again with them once the
// comparison is done (#1812).
func TestDiffLargeComparesOffMainThread(t *testing.T) {
	jobs := holdDiffs(t)
	old, nw := largeDiffTexts(diffSyncLines)
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw)}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
	if len(*jobs) != 1 {
		t.Fatalf("%d comparisons started off the main thread, want 1", len(*jobs))
	}
	if !d.Comparing() || len(d.Hunks()) != 0 {
		t.Fatalf("comparing %v with %d hunks before the comparison ran, want true with none", d.Comparing(), len(d.Hunks()))
	}
	// Hidden: only the empty line after each final newline.
	for i, s := range d.sides {
		if h := s.hidden(); len(h) != 1 || h[0].from != diffSyncLines {
			t.Fatalf("side %d hides %v while compared, want the texts whole", i, h)
		}
	}
	go (*jobs)[0]()
	wait.Until(t, "the window to draw again after the comparison", tt.FramePending)
	tt.Frame()
	if d.Comparing() {
		t.Fatal("still comparing after the comparison was done")
	}
	if got, want := d.Hunks(), syncHunks(old, nw, false); !slices.Equal(got, want) || len(got) != diffSyncLines/50 {
		t.Fatalf("hunks %v, want %v", got, want)
	}
	if len(d.sides[1].hidden()) < 2 {
		t.Fatalf("hidden %v after the comparison, want unchanged lines collapsed", d.sides[1].hidden())
	}
	// Small texts are compared at once.
	d.Old, d.New = NewBuffer("a\nb\n"), NewBuffer("a\nc\n")
	tt.Frame()
	if len(*jobs) != 1 || d.Comparing() || len(d.Hunks()) != 1 {
		t.Fatalf("small texts: %d comparisons started, comparing %v, hunks %v", len(*jobs), d.Comparing(), d.Hunks())
	}
}

// Large texts no window shows are compared at once, as for an app's
// Hunks with no view.
func TestDiffLargeWithoutWindow(t *testing.T) {
	jobs := holdDiffs(t)
	old, nw := largeDiffTexts(diffSyncLines)
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw)}
	if n := len(d.Hunks()); n != diffSyncLines/50 || d.Comparing() || len(*jobs) != 0 {
		t.Fatalf("%d hunks, comparing %v, %d comparisons started", n, d.Comparing(), len(*jobs))
	}
}

// Edits while a large diff is compared: of the new text, the result is
// taken and the edits compared again around them; of the old text, the
// result is dropped for a comparison of the old text as it is; of other
// buffers, it is dropped.
func TestDiffLargeEditedWhileComparing(t *testing.T) {
	jobs := holdDiffs(t)
	old, nw := largeDiffTexts(diffSyncLines)
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw), Editable: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
	d.New.Edit(Edit{Start: 0, End: 0, Text: "added\n"})
	tt.Frame()
	d.New.Edit(Edit{Start: d.New.Len(), End: d.New.Len(), Text: "last\n"})
	tt.Frame()
	if len(*jobs) != 1 || !d.Comparing() {
		t.Fatalf("%d comparisons started, comparing %v, want 1, true", len(*jobs), d.Comparing())
	}
	(*jobs)[0]()
	tt.Frame()
	if got, want := d.Hunks(), syncHunks(old, d.New.String(), true); d.Comparing() || !slices.Equal(got, want) {
		t.Fatalf("after editing the new text: comparing %v, hunks %v, want %v", d.Comparing(), got, want)
	}

	// The old text edited: the comparison started before is stale.
	d.Old.Edit(Edit{Start: 0, End: 0, Text: "x\n"}) // compared anew
	tt.Frame()
	d.Old.Edit(Edit{Start: 0, End: 0, Text: "y\n"}) // and anew
	tt.Frame()
	if len(*jobs) != 3 {
		t.Fatalf("%d comparisons started, want 3", len(*jobs))
	}
	(*jobs)[1]()
	tt.Frame()
	if !d.Comparing() || len(d.Hunks()) != 0 {
		t.Fatalf("a stale comparison was taken: comparing %v, hunks %v", d.Comparing(), d.Hunks())
	}
	(*jobs)[2]()
	tt.Frame()
	if got, want := d.Hunks(), syncHunks(d.Old.String(), d.New.String(), true); d.Comparing() || !slices.Equal(got, want) {
		t.Fatalf("after editing the old text: comparing %v, hunks %v, want %v", d.Comparing(), got, want)
	}

	// Other buffers: the comparison of the ones before is dropped.
	d.Old.Edit(Edit{Start: 0, End: 0, Text: "z\n"})
	tt.Frame()
	d.Old, d.New = NewBuffer("a\n"), NewBuffer("b\n")
	tt.Frame()
	(*jobs)[3]()
	tt.Frame()
	if got := d.Hunks(); len(got) != 1 || got[0] != (DiffHunk{0, 1, 0, 1}) {
		t.Fatalf("hunks %v of other buffers, want one", got)
	}
}

// Typing in a large editable diff while it is compared on a goroutine, a
// test for the race detector: the result is of the texts as they are.
func TestDiffLargeTypingWhileComparing(t *testing.T) {
	old, nw := largeDiffTexts(diffSyncLines)
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw), Editable: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
	for i := 0; i < 20 && d.Comparing(); i++ {
		d.New.Edit(Edit{Start: 0, End: 0, Text: strconv.Itoa(i) + "\n"})
		tt.Frame()
	}
	waitCompared(t, tt, d)
	if got, want := d.Hunks(), syncHunks(old, d.New.String(), true); !slices.Equal(got, want) {
		t.Fatalf("hunks %v, want %v", got, want)
	}
}

// With ScrollPastEnd, GoToHunk brings the last change to the middle of the
// view, where without it the view stops at the last line; Focus then gives
// the side its caret's editor the keyboard.
func TestDiffScrollPastEndAndFocus(t *testing.T) {
	var a []string
	for i := range 200 {
		a = append(a, "line "+strconv.Itoa(i))
	}
	b := append([]string(nil), a...)
	b[198] = "changed"
	for _, past := range []bool{false, true} {
		for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
			d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")),
				Layout: layout, ExpandUnchanged: true, ScrollPastEnd: past}
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 600)
			tt.Frame()
			d.GoToHunk(0)
			d.Focus()
			tt.Frame()
			tt.Frame()
			end := d.LineTop(DiffNew, d.New.LineCount()) - d.ViewportHeight()
			if y := d.ScrollY(); past != (y > end) {
				t.Errorf("layout %v, ScrollPastEnd %v: ScrollY %v, the last line's bottom at %v", layout, past, y, end)
			}
			if !d.eds[1].focused {
				t.Errorf("layout %v: Focus left the new side without the keyboard", layout)
			}
		}
	}
}
