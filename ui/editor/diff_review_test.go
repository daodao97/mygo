package editor

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Regressions found by reviewing the diff viewer.

// One version changing several places, as typing at several carets or
// reverting hunks at once, re-diffs right.
func TestReviewDiffMultiChangeVersion(t *testing.T) {
	var lines []string
	for i := range 50 {
		lines = append(lines, fmt.Sprint("l", i))
	}
	oldS := strings.Join(lines, "\n") + "\n"
	lines[40] = "Z"
	newS := strings.Join(lines, "\n") + "\n"
	old, nw := NewBuffer(oldS), NewBuffer(newS)
	var d diffModel
	d.set(old, nw, false)
	d.update()
	r, _ := nw.snapshot()
	nw.Edit(Edit{Start: r.LineStart(1), End: r.LineStart(21)}, Edit{Start: r.LineStart(25), End: r.LineStart(25), Text: "\n"})
	d.update()
	checkHunks(t, oldS, nw.String(), d.hunks)

	for seed := range uint64(500) {
		rnd := rand.New(rand.NewPCG(seed, 3))
		var a []string
		for i := range 40 {
			a = append(a, fmt.Sprint("line ", i%13))
		}
		oldS := strings.Join(a, "\n") + "\n"
		old, nw := NewBuffer(oldS), NewBuffer(oldS)
		var d diffModel
		d.set(old, nw, false)
		d.update()
		for range 6 {
			t, _ := nw.snapshot()
			var edits []Edit
			at := 0
			for range 1 + rnd.IntN(3) {
				start := min(t.Len(), at+rnd.IntN(40))
				end := min(t.Len(), start+rnd.IntN(12))
				edits = append(edits, Edit{Start: start, End: end, Text: []string{"", "\n", "x\n", "line 3\n", "q"}[rnd.IntN(5)]})
				at = end + 1
			}
			nw.Edit(edits...)
			d.update()
		}
		t.Run(fmt.Sprint(seed), func(t *testing.T) { checkHunks(t, oldS, nw.String(), d.hunks) })
	}
}

// New becoming the same buffer as Old shows no changes, rather than the
// blocks of those there were.
func TestReviewDiffSameBuffer(t *testing.T) {
	a, b := NewBuffer("a\nb\nc\n"), NewBuffer("a\nX\nc\n")
	d := &DiffState{Old: a, New: b, Layout: DiffUnified, ExpandUnchanged: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
	tt.Frame()
	d.New = a
	tt.Frame()
	if len(d.Hunks()) != 0 {
		t.Fatalf("hunks %v of a buffer with itself", d.Hunks())
	}
}

// A patch shows its own - and + runs, not a diff of its lines' texts.
func TestReviewSetPatchHunks(t *testing.T) {
	files, err := ParsePatch("--- a/x\n+++ b/x\n@@ -1,5 +1,2 @@\n c1\n-X\n-Y\n-Z\n c2\n@@ -40,2 +37,5 @@\n c3\n+X\n+Y\n+Z\n c4\n")
	if err != nil {
		t.Fatal(err)
	}
	d := &DiffState{}
	d.SetPatch(files[0])
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 400)
	tt.Frame()
	if got, want := d.Hunks(), []DiffHunk{{1, 4, 1, 1}, {6, 6, 3, 6}}; !slices.Equal(got, want) {
		t.Fatalf("hunks %v, want %v", got, want)
	}
	// A new file deletes nothing.
	files, _ = ParsePatch("--- /dev/null\n+++ b/n.md\n@@ -0,0 +1,2 @@\n+# Title\n+text\n")
	d = &DiffState{}
	d.SetPatch(files[0])
	if a, del := d.Stats(); a != 2 || del != 0 {
		t.Fatalf("a new file: +%d -%d", a, del)
	}
	d = &DiffState{Old: NewBuffer(""), New: NewBuffer("x")}
	if a, del := d.Stats(); a != 1 || del != 0 {
		t.Fatalf("from nothing: +%d -%d", a, del)
	}
}

// The header of a last hunk that only adds lines goes after the old
// side's last line, level with the new side's.
func TestReviewSetPatchLateHeader(t *testing.T) {
	files, _ := ParsePatch("--- a/x\n+++ b/x\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n@@ -10,0 +11,2 @@\n+new1\n+new2\n")
	d := &DiffState{}
	d.SetPatch(files[0])
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 400)
	tt.Frame()
	tt.Frame()
	a, b := &d.eds[0].view.heights, &d.eds[1].view.heights
	if ya, yb := a.geometry(2).rows, b.geometry(2).rows; ya != yb {
		t.Fatalf("line c at %v on the old side, %v on the new", ya, yb)
	}
}

// With no context lines, deletions and separators still show.
func TestReviewDiffNoContext(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n"
	nw := strings.Replace(old, "f\n", "", 1)
	for _, layout := range []DiffLayout{DiffUnified, DiffSplit} {
		d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw), ContextLines: -1, Layout: layout}
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 400)
		tt.Frame()
		tt.Frame()
		if h := d.eds[1].view.heights.total(); h == 0 {
			t.Fatalf("layout %d: nothing shows of the new side", layout)
		}
		if layout == DiffSplit && d.eds[0].view.heights.total() != d.eds[1].view.heights.total() {
			t.Fatalf("sides of %v and %v", d.eds[0].view.heights.total(), d.eds[1].view.heights.total())
		}
	}
}

// Sticky deleted lines and spacers on collapsed lines keep the diff's
// line lookup. They are not app blocks standing in for hidden source.
func TestReviewDiffNoContextHitTest(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n"
	nw := strings.Replace(old, "f\n", "", 1)
	for _, layout := range []DiffLayout{DiffUnified, DiffSplit} {
		t.Run(fmt.Sprint(layout), func(t *testing.T) {
			d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw), ContextLines: -1, Layout: layout}
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 400)
			tt.Frame()
			tt.Frame()
			ed := &d.eds[1]
			h := &ed.view.heights
			kind := blockDeleted
			if layout == DiffSplit {
				kind = blockSpacer
			}
			seen := 0
			ed.eachBlock(0, h.n-1, func(b *block, top float64) {
				if b.kind != kind {
					return
				}
				seen++
				if !b.sticky || !h.isHidden(b.line) || b.line == 0 {
					t.Fatalf("expected a sticky diff block on a collapsed line: %+v", b)
				}
				for _, y := range []float64{top, top + float64(b.height)/2} {
					// With every unchanged line collapsed, lookup falls
					// back to line 0, not the block's hidden line.
					if got := h.lineAt(y); got != 0 {
						t.Errorf("kind %d at height %v: line %d, want 0", b.kind, y, got)
					}
					if got, want := ed.offsetAt(0, y), ed.view.text.LineEnd(0); got != want {
						t.Errorf("kind %d at height %v: offset %d, want %d", b.kind, y, got, want)
					}
				}
			})
			if seen == 0 {
				t.Fatalf("no diff block of kind %d was tested", kind)
			}
		})
	}
}

// Both sides of a split view wrap alike where their line counts have
// different numbers of digits.
func TestReviewDiffWrapGutters(t *testing.T) {
	var a []string
	for i := range 995 {
		a = append(a, strings.Repeat("w", i%300)+" "+strings.Repeat("ab ", i%7))
	}
	b := slices.Insert(slices.Clone(a), 1, slices.Repeat([]string{"added"}, 10)...)
	d := &DiffState{Old: NewBuffer(strings.Join(a, "\n")), New: NewBuffer(strings.Join(b, "\n")), Wrap: true, ExpandUnchanged: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 700, 400)
	tt.Frame()
	tt.Frame()
	if ga, gb := d.eds[0].gutterW, d.eds[1].gutterW; ga != gb {
		t.Fatalf("gutters %v and %v", ga, gb)
	}
}

// Lines expanded stay expanded as the new text is edited among them.
func TestReviewDiffExpansionKept(t *testing.T) {
	var lines []string
	for i := range 100 {
		lines = append(lines, fmt.Sprint("line ", i))
	}
	old := strings.Join(lines, "\n") + "\n"
	lines[10] = "changed"
	nw := strings.Join(lines, "\n") + "\n"
	for _, at := range []int{50, 11} {
		d := &DiffState{Old: NewBuffer(old), New: NewBuffer(nw)}
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 400)
		tt.Frame()
		tt.Frame()
		d.expand(d.gapList[1].a, 1, true)
		tt.Frame()
		r, _ := d.New.snapshot()
		d.New.Edit(Edit{Start: r.LineEnd(at), End: r.LineEnd(at), Text: "x"})
		tt.Frame()
		tt.Frame()
		for _, g := range d.gapList[1:] {
			if g.hidden > 0 {
				t.Fatalf("editing line %d collapsed %d lines again: %+v", at, g.hidden, d.gapList)
			}
		}
	}
}

// Lines deleted from the end of a text without a final newline are a
// change of its last line, which reverts and shows in the gutter.
func TestReviewDiffDeletionAtEnd(t *testing.T) {
	st := &State{Buffer: NewBuffer("a\nb"), DiffBase: NewBuffer("a\nb\nc")}
	tt := ui.NewTester(func(c *ui.Context) { View(c, st).Fill().AutoFocus() }, 400, 200)
	tt.Frame()
	tt.Frame()
	if g := st.ed.gitDiff; g == nil || g.status(1) != 3 {
		t.Fatal("no mark of the lines deleted after the last line")
	}
	st.Select(3, 3)
	st.Run(RevertHunk)
	if got := st.Buffer.String(); got != "a\nb\nc" {
		t.Fatalf("reverted into %q", got)
	}
}

// Annotations on one line go one after the other, with keys or without.
func TestReviewDiffAnnotationsStack(t *testing.T) {
	d := &DiffState{Old: NewBuffer("a\nb\n"), New: NewBuffer("a\nB\n"), Annotations: []DiffAnnotation{
		{Side: DiffNew, Line: 0, Height: 30}, {Side: DiffNew, Line: 0, Height: 40}}}
	d.RenderAnnotation = func(c *ui.Context, a DiffAnnotation) { ui.Text(c, fmt.Sprint("note ", a.Height)) }
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 400)
	tt.Frame()
	tt.Frame()
	r1, ok1 := tt.Find("note 30")
	r2, ok2 := tt.Find("note 40")
	if !ok1 || !ok2 || r2.Y-r1.Y < 29 {
		t.Fatalf("notes at %v and %v", r1, r2)
	}
}

// An annotation on the last line shown goes above the separator after it.
func TestReviewDiffAnnotationBeforeSeparator(t *testing.T) {
	var lines []string
	for i := range 30 {
		lines = append(lines, fmt.Sprint("line ", i))
	}
	old := strings.Join(lines, "\n") + "\n"
	lines[5] = "changed"
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer(old), New: NewBuffer(strings.Join(lines, "\n") + "\n"), Layout: layout,
			Annotations: []DiffAnnotation{{Side: DiffNew, Line: 8, Height: 30}}}
		d.RenderAnnotation = func(c *ui.Context, a DiffAnnotation) { ui.Text(c, "the note") }
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 600)
		tt.Frame()
		tt.Frame()
		note, _ := tt.Find("the note")
		sep, ok := tt.Find("Expand 21 unmodified lines")
		if !ok || note.Y > sep.Y {
			t.Fatalf("layout %d: the note at %v, the separator at %v (%v)", layout, note, sep, ok)
		}
	}
}

// The unified view shows the old text's annotations of unchanged lines.
func TestReviewDiffUnifiedOldAnnotation(t *testing.T) {
	d := &DiffState{Old: NewBuffer("a\nb\nc"), New: NewBuffer("a\nX\nc"), Layout: DiffUnified,
		Annotations: []DiffAnnotation{{Side: DiffOld, Line: 2, Height: 30}}}
	d.RenderAnnotation = func(c *ui.Context, a DiffAnnotation) { ui.Text(c, "old note") }
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 600, 300)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("old note") {
		t.Fatal("the old side's annotation is missing")
	}
}

// Clicking the number of a wrapped deleted line selects that line.
func TestReviewDiffDeletedLineClick(t *testing.T) {
	old := "a\n" + strings.Repeat("word ", 60) + "\nsecond\nc\n"
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer("a\nc\n"), Layout: DiffUnified, Wrap: true, LineSelection: true}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 500, 400)
	tt.Frame()
	tt.Frame()
	ed := &d.eds[1]
	above, _ := ed.view.heights.blocksOf(1)
	if len(above) == 0 || above[0].kind != blockDeleted {
		t.Fatalf("no deleted block: %+v", above)
	}
	top := float32(ed.view.heights.geometry(1).top - ed.scrollY)
	tt.ClickAt(10, top+ed.view.lineHeight*1.5) // the deleted line's second row
	if r, ok := d.SelectedLines(); !ok || r != (DiffLineRange{Side: DiffOld, Start: 1, End: 1}) {
		t.Fatalf("selected %+v %v", r, ok)
	}
}

// Names git quotes read as the names, in renames and new files.
func TestReviewPatchQuotedNames(t *testing.T) {
	files, err := ParsePatch("diff --git \"a/rn/old/\\346\\226\\207.txt\" \"b/rn/new/\\346\\226\\207.txt\"\nsimilarity index 100%\nrename from \"rn/old/\\346\\226\\207.txt\"\nrename to \"rn/new/\\346\\226\\207.txt\"\n" +
		"diff --git \"a/my file.txt\" \"b/my file.txt\"\nnew file mode 100644\nindex 0000000..e69de29\n")
	if err != nil {
		t.Fatal(err)
	}
	if files[0].OldName != "rn/old/文.txt" || files[0].NewName != "rn/new/文.txt" {
		t.Fatalf("rename %q -> %q", files[0].OldName, files[0].NewName)
	}
	if files[1].NewName != "my file.txt" || files[1].OldName != "" {
		t.Fatalf("new file %q -> %q", files[1].OldName, files[1].NewName)
	}
}

// A patch of a file with \r\n endings applies to it.
func TestReviewPatchCRLF(t *testing.T) {
	files, err := ParsePatch("--- a/x\r\n+++ b/x\r\n@@ -1,3 +1,3 @@\r\n a\r\n-b\r\n+x\r\n c\r\n")
	if err != nil {
		t.Fatal(err)
	}
	got, err := files[0].Apply("a\r\nb\r\nc\r\n")
	if err != nil || got != "a\r\nx\r\nc\r\n" {
		t.Fatalf("applied %q, %v", got, err)
	}
}

// A hunk with more lines than its header counts is an error.
func TestReviewPatchOvercount(t *testing.T) {
	if _, err := ParsePatch("--- a/x\n+++ b/x\n@@ -1,1 +1,3 @@\n a\n b\n+c\n"); err == nil {
		t.Fatal("a hunk longer than it counts parsed")
	}
}

// The annotations of deleted lines go under their rows, after those of the
// lines before, where LineTop puts the lines, and a press on a line's
// number below them selects it.
func TestReviewDiffDeletedAnnotations(t *testing.T) {
	old := "a\nfirst\n" + strings.Repeat("word ", 60) + "\nthird\nc\n"
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer("a\nc\n"), Layout: DiffUnified, Wrap: true, LineSelection: true,
		Annotations: []DiffAnnotation{{Side: DiffOld, Line: 1, Height: 30, Key: "one"}, {Side: DiffOld, Line: 2, Height: 40, Key: "two"}}}
	d.RenderAnnotation = func(c *ui.Context, a DiffAnnotation) { ui.Text(c, fmt.Sprint("note ", a.Key)) }
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 500, 400)
	tt.Frame()
	tt.Frame()
	ed := &d.eds[1]
	lh := float64(ed.view.lineHeight)
	at := func(line int) float32 { return ed.r.Y + float32(d.LineTop(DiffOld, line)-ed.scrollY) }
	rows := d.oldView.heights.wraps.rows(2)
	if rows < 2 {
		t.Fatalf("the long line wraps into %d rows", rows)
	}
	if got, want := at(2)-at(1), float32(lh+30); got != want {
		t.Fatalf("line 2 is %v below line 1, want %v", got, want)
	}
	if got, want := at(3)-at(2), float32(float64(rows)*lh+40); got != want {
		t.Fatalf("line 3 is %v below line 2, want %v", got, want)
	}
	for _, c := range []struct {
		key  string
		line int
	}{{"one", 1}, {"two", 2}} {
		r, ok := tt.Find("note " + c.key)
		if !ok {
			t.Fatalf("note %s missing", c.key)
		}
		under := at(c.line) + float32(float64(d.oldView.heights.wraps.rows(c.line))*lh)
		if r.Y < under || r.Y > under+5 {
			t.Fatalf("note %s at %v, line %d's rows end at %v", c.key, r.Y, c.line, under)
		}
	}
	tt.ClickAt(ed.r.X+ed.gutterW-8, at(3)+float32(lh/2))
	if r, ok := d.SelectedLines(); !ok || r.Side != DiffOld || r.Start != 3 || r.End != 3 {
		t.Fatalf("selected %+v (%v), want old line 3", r, ok)
	}
}
