package editor

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Revealing a caret keeps the configured margin in either direction,
// reducing it when the viewport cannot fit it.
func TestScrollMargin(t *testing.T) {
	for _, tc := range []struct {
		name   string
		margin int
		lines  int
		height int
	}{
		{"default", 0, 3, 400},
		{"none", -1, 0, 400},
		{"five lines", 5, 5, 400},
		{"negative", -2, 0, 400},
		{"large", 1000, 1000, 400},
		{"short viewport", 0, 3, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &State{
				Buffer:       NewBuffer(strings.Repeat("line\n", 100)),
				ScrollMargin: tc.margin,
			}
			tt := ui.NewTester(func(c *ui.Context) {
				View(c, st).Fill().AutoFocus()
			}, 600, tc.height)
			tt.Frame()
			end := st.Buffer.Len()
			st.Select(end, end)
			tt.Frame()
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
			lineHeight := st.LineTop(1) - st.LineTop(0)
			pos := st.CursorPos()
			gap := st.ScrollY() + st.ViewportHeight() - (st.LineTop(pos.Line) + lineHeight)
			want := min(float64(tc.lines)*lineHeight, max(0, (st.ViewportHeight()-lineHeight)/2))
			if math.Abs(gap-want) > 0.5 {
				t.Fatalf("caret line ends %.1fpx above the viewport bottom, want %.1f", gap, want)
			}

			st.SetScrollY(st.LineTop(80))
			off := st.Buffer.Offset(Pos{Line: 50})
			st.Select(off, off)
			tt.Frame()
			gap = st.LineTop(st.CursorPos().Line) - st.ScrollY()
			if math.Abs(gap-want) > 0.5 {
				t.Fatalf("caret line starts %.1fpx below the viewport top, want %.1f", gap, want)
			}

			if tc.lines == 0 {
				wantScroll := st.LineTop(st.Buffer.LineCount()) - lineHeight
				st.SetScrollY(wantScroll)
				tt.Frame()
				if got := st.ScrollY(); got != wantScroll {
					t.Fatalf("scrollY %.1f after scrolling past the end, want %.1f", got, wantScroll)
				}
			}
		})
	}
}

// The value is read each frame, and zero restores the default.
func TestScrollMarginChanges(t *testing.T) {
	ct := newEditorTest(t, "", strings.Repeat("line\n", 100))
	st := ct.st
	for _, tc := range []struct {
		margin int
		lines  int
	}{
		{0, 3},
		{5, 5},
		{1, 1},
		{-2, 0},
		{0, 3},
	} {
		st.ScrollMargin = tc.margin
		st.SetScrollY(0)
		off := st.Buffer.Offset(Pos{Line: 50})
		st.Select(off, off)
		ct.tt.Frame()
		lineHeight := st.LineTop(1) - st.LineTop(0)
		gap := st.ScrollY() + st.ViewportHeight() - st.LineTop(51)
		want := min(float64(tc.lines)*lineHeight, max(0, (st.ViewportHeight()-lineHeight)/2))
		if math.Abs(gap-want) > 0.5 {
			t.Fatalf("margin %d: caret line ends %.1fpx above the viewport bottom, want %.1f", tc.margin, gap, want)
		}
	}
}

// Both diff layouts keep the default margin on their visible sides.
func TestDiffScrollMarginDefault(t *testing.T) {
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		source := strings.Repeat("line\n", 100)
		d := &DiffState{Old: NewBuffer(source), New: NewBuffer(source), Layout: layout, ExpandUnchanged: true}
		tt := ui.NewTester(func(c *ui.Context) {
			DiffView(c, d).Fill()
		}, 600, 400)
		tt.Frame()
		for _, side := range []DiffSide{DiffNew, DiffOld} {
			if layout == DiffUnified && side == DiffOld {
				continue
			}
			d.SetScrollY(0)
			ed := d.editorOf(side)
			off := ed.view.buf.Offset(Pos{Line: 50})
			ed.setSels([]sel{ed.newSel(off, off)}, revealFit)
			tt.Frame()
			gap := ed.scrollY + float64(ed.viewH) - ed.view.heights.top(51)
			if want := 3 * float64(ed.view.lineHeight); math.Abs(gap-want) > 0.5 {
				t.Fatalf("layout %v, side %v: caret line ends %.1fpx above the viewport bottom, want %.1f", layout, side, gap, want)
			}
		}
	}
}
