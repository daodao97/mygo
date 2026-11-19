package richtext

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// longDoc returns a document of n paragraphs, each long enough to wrap in
// a narrow editor.
func longDoc(n int) *Document {
	paras := make([]Paragraph, n)
	for i := range paras {
		paras[i] = para(fmt.Sprintf("Paragraph %d, long enough to wrap in a window this narrow at times.", i))
	}
	return NewDocument(TextOf(paras...))
}

// boxOf returns the box of paragraph i among boxes.
func boxOf(t *testing.T, boxes []ParagraphBox, i int) ParagraphBox {
	t.Helper()
	for _, b := range boxes {
		if b.Index == i {
			return b
		}
	}
	t.Fatalf("paragraph %d is not in view: %v", i, boxes)
	return ParagraphBox{}
}

// near reports whether a and b are the same to a hundredth of a DIP.
func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// openTester is ui.NewTester for a test that reads the document after its
// last frame (CaretRect, VisibleParagraphs): the window stays open until
// the test ends, as an app's does while it shows the editor. A document
// holds its editor weakly, so that it doesn't keep a closed window's
// alive, and once nothing uses the Tester the collector may take the
// editor, after which the document reads as shown in none.
func openTester(t *testing.T, view func(c *ui.Context), width, height int) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(view, width, height)
	t.Cleanup(func() { runtime.KeepAlive(tt) })
	return tt
}

// ScrollBy scrolls the editor by as much as asked, as far as its text
// goes, and reports whether it moved; before an editor showed the
// document, it does nothing.
func TestScrollBy(t *testing.T) {
	doc := longDoc(200)
	if doc.ScrollBy(100) {
		t.Fatal("no editor shows the document, and it scrolled")
	}
	tt, boxes := proseTester(t, doc, &Theme{ParagraphSpacing: 0.5}, 400)
	doc.SetSelection(0, 0)
	tt.Frame()
	if (*boxes)[0].Index != 0 || (*boxes)[0].Top != 0 {
		t.Fatalf("the first frame shows %v", (*boxes)[0])
	}
	// A paragraph that stays in view moves up by as much.
	b := (*boxes)[len(*boxes)/2]
	if !doc.ScrollBy(b.Top / 2) {
		t.Fatal("ScrollBy did not scroll")
	}
	if !tt.FramePending() {
		t.Fatal("ScrollBy asked for no frame")
	}
	tt.Frame()
	if got := boxOf(t, *boxes, b.Index); !near(got.Top, b.Top/2) {
		t.Fatalf("scrolled by %v, paragraph %d went from %v to %v", b.Top/2, b.Index, b.Top, got.Top)
	}
	// The wheel goes as far as the text does, and so does ScrollBy.
	if !doc.ScrollBy(1e9) {
		t.Fatal("ScrollBy to the end did not scroll")
	}
	tt.Frame()
	last := (*boxes)[len(*boxes)-1]
	if last.Index != 199 || !near(last.Top+last.Height, 400) {
		t.Fatalf("at the end, the last box is %+v", last)
	}
	if doc.ScrollBy(10) {
		t.Fatal("ScrollBy scrolled past the end")
	}
	if !doc.ScrollBy(-1e9) {
		t.Fatal("ScrollBy to the start did not scroll")
	}
	tt.Frame()
	if first := (*boxes)[0]; first.Index != 0 || first.Top != 0 {
		t.Fatalf("at the start, the first box is %+v", first)
	}
	if doc.ScrollBy(-10) {
		t.Fatal("ScrollBy scrolled past the start")
	}
	// Frames that change nothing ask for none more.
	tt.Frame()
	if tt.FramePending() {
		t.Fatal("the frames do not settle")
	}
}

// ScrollToParagraph lines a paragraph's lines up with the editor's top,
// middle or bottom, or scrolls as little as shows them, as far as the
// text goes.
func TestScrollToParagraph(t *testing.T) {
	doc := longDoc(200)
	doc.ScrollToParagraph(100, ui.ScrollStart) // no editor yet: nothing
	tt, boxes := proseTester(t, doc, &Theme{ParagraphSpacing: 0.5}, 400)
	doc.SetSelection(0, 0)
	tt.Frame()
	to := func(i int, align ui.ScrollAlign) ParagraphBox {
		t.Helper()
		doc.ScrollToParagraph(i, align)
		tt.Frame()
		return boxOf(t, *boxes, i)
	}
	if b := to(100, ui.ScrollStart); !near(b.Top, 0) {
		t.Fatalf("paragraph 100 at the start: %+v", b)
	}
	if b := to(150, ui.ScrollCenter); !near(b.Top+b.Height/2, 200) {
		t.Fatalf("paragraph 150 in the middle: %+v", b)
	}
	if b := to(60, ui.ScrollEnd); !near(b.Top+b.Height, 400) {
		t.Fatalf("paragraph 60 at the end: %+v", b)
	}
	// Nearest: in view, nothing moves; below, it comes up to the bottom;
	// above, down to the top.
	in := (*boxes)[len(*boxes)/2]
	if b := to(in.Index, ui.ScrollNearest); b != in {
		t.Fatalf("paragraph %d in view went from %+v to %+v", in.Index, in, b)
	}
	if b := to(90, ui.ScrollNearest); !near(b.Top+b.Height, 400) {
		t.Fatalf("paragraph 90 below, nearest: %+v", b)
	}
	if b := to(20, ui.ScrollNearest); !near(b.Top, 0) {
		t.Fatalf("paragraph 20 above, nearest: %+v", b)
	}
	// Past the ends: the first and the last paragraphs, as far as the text
	// goes.
	doc.ScrollToParagraph(-5, ui.ScrollStart)
	tt.Frame()
	if b := (*boxes)[0]; b.Index != 0 || b.Top != 0 {
		t.Fatalf("paragraph -5: %+v", b)
	}
	doc.ScrollToParagraph(1000, ui.ScrollStart)
	tt.Frame()
	if b := (*boxes)[len(*boxes)-1]; b.Index != 199 || !near(b.Top+b.Height, 400) {
		t.Fatalf("paragraph 1000: %+v", b)
	}
	defer func() {
		if recover() == nil {
			t.Error("an alignment out of range did not panic")
		}
	}()
	doc.ScrollToParagraph(0, ui.ScrollAlign(9))
}

// anchorTester builds an EditorBase of doc 300×400 that leaves room
// above paragraph *at of *room DIPs, the boxes it reads as it builds, and
// whether to focus it. It scrolls to paragraph 50 and puts the caret at
// the start of the paragraph in the middle of the view.
func anchorTester(t *testing.T, doc *Document, focus bool) (tt *ui.Tester, boxes *[]ParagraphBox, at *int, room *float32) {
	t.Helper()
	boxes, at, room = new([]ParagraphBox), new(int), new(float32)
	tt = openTester(t, func(c *ui.Context) {
		*boxes = doc.VisibleParagraphs()
		EditorBase(c, doc, EditorOptions{Font: ui.Font{Size: 10}, Space: func(p int) float32 {
			if p == *at {
				return *room
			}
			return 0
		}}).Fill()
	}, 300, 400)
	if focus {
		tt.ClickAt(150, 5)
		if !tt.Focused("Rich text") {
			t.Fatal("the editor did not take the focus")
		}
	}
	doc.ScrollToParagraph(50, ui.ScrollStart)
	tt.Frame()
	mid := (*boxes)[len(*boxes)/2]
	doc.SetSelection(mid.Start, mid.Start)
	tt.Frame()
	if (*boxes)[0].Index != 50 {
		t.Fatalf("the view starts at paragraph %d", (*boxes)[0].Index)
	}
	return tt, boxes, at, room
}

// caretRect returns where the editor shows the caret, which is in view.
func caretRect(t *testing.T, doc *Document) ui.Rect {
	t.Helper()
	r, ok := doc.CaretRect()
	if !ok {
		t.Fatal("the caret is out of view")
	}
	return r
}

// While the editor has the keyboard, what grows between the top of the
// view and the caret moves the text above it up, and the line being typed
// stays, as browsers keep the focused editable element in place.
func TestScrollAnchorCaret(t *testing.T) {
	doc := longDoc(200)
	tt, boxes, at, room := anchorTester(t, doc, true)
	caret := caretRect(t, doc)
	first, second := (*boxes)[0], (*boxes)[1]
	_, at0 := doc.Selection()
	cp := doc.Text().paraAt(at0)

	// Room opens above a paragraph between the top of the view and the
	// caret's.
	*at, *room = first.Index+2, 40
	tt.Frame()
	if got := caretRect(t, doc); got != caret {
		t.Fatalf("room above paragraph %d moved the caret from %v to %v", *at, caret, got)
	}
	if got := boxOf(t, *boxes, second.Index); !near(got.Top, second.Top-40) {
		t.Fatalf("room above paragraph %d: paragraph %d went from %v to %v, want 40 up", *at, second.Index, second.Top, got.Top)
	}
	// It grows, and goes.
	*room = 90
	tt.Frame()
	if got := caretRect(t, doc); got != caret {
		t.Fatalf("more room moved the caret from %v to %v", caret, got)
	}
	*room = 0
	tt.Frame()
	if got := caretRect(t, doc); got != caret {
		t.Fatalf("the room gone moved the caret from %v to %v", caret, got)
	}
	// Room above the caret's own paragraph keeps its line too.
	*at, *room = cp, 30
	tt.Frame()
	if got := caretRect(t, doc); got != caret {
		t.Fatalf("room above the caret's paragraph moved the caret from %v to %v", caret, got)
	}
	// Room below the caret moves nothing above it.
	*at = cp + 2
	tt.Frame()
	if got := caretRect(t, doc); got != caret {
		t.Fatalf("room below the caret moved it from %v to %v", caret, got)
	}
	// Typing keeps its line, and so does an edit above it in view.
	tt.Type("abc")
	typed := caretRect(t, doc)
	if typed.Y != caret.Y || typed.X <= caret.X {
		t.Fatalf("typed from %v, the caret is at %v", caret, typed)
	}
	start := (*boxes)[1].Start
	doc.Edit(func(e *Edit) { e.ReplaceString(start, start, "Inserted\n", Attributes{}) })
	tt.Frame()
	if got := caretRect(t, doc); got.Y != typed.Y {
		t.Fatalf("a paragraph inserted above moved the caret from %v to %v", typed, got)
	}
	// Joining the caret's paragraph to the one above keeps the view.
	before := (*boxes)[0]
	tt.Key(ui.Cmd, ui.KeyLeft)
	tt.Key(0, ui.KeyBackspace)
	if got := boxOf(t, *boxes, before.Index); got.Top != before.Top {
		t.Fatalf("joining paragraphs moved the first in view from %+v to %+v", before, got)
	}
	// Frames that change nothing ask for none more.
	tt.Frame()
	if tt.FramePending() {
		t.Fatal("the frames do not settle")
	}
}

// Without the keyboard, the editor keeps its place by the first paragraph
// in view, as before: what grows above the caret moves the caret down.
func TestScrollAnchorUnfocused(t *testing.T) {
	doc := longDoc(200)
	tt, boxes, at, room := anchorTester(t, doc, false)
	if tt.Focused("Rich text") {
		t.Fatal("the editor has the focus")
	}
	caret := caretRect(t, doc)
	first := (*boxes)[0]
	*at, *room = first.Index+2, 40
	tt.Frame()
	if got := caretRect(t, doc); !near(got.Y, caret.Y+40) {
		t.Fatalf("room above paragraph %d moved the caret from %v to %v, want 40 down", *at, caret.Y, got.Y)
	}
	if got := (*boxes)[0]; got != first {
		t.Fatalf("the first paragraph in view went from %+v to %+v", first, got)
	}
}

// An editor scrolled to its top stays there while focused, as one that
// shows all of its text does: room above the caret moves it down.
func TestScrollAnchorAtTop(t *testing.T) {
	doc := longDoc(200)
	var boxes []ParagraphBox
	var room float32
	tt := openTester(t, func(c *ui.Context) {
		boxes = doc.VisibleParagraphs()
		EditorBase(c, doc, EditorOptions{Font: ui.Font{Size: 10}, Space: func(p int) float32 {
			if p == 1 {
				return room
			}
			return 0
		}}).Fill()
	}, 300, 400)
	doc.SetSelection(0, 0)
	tt.Frame()
	tt.ClickAt(150, 200)
	if !tt.Focused("Rich text") {
		t.Fatal("the editor did not take the focus")
	}
	caret := caretRect(t, doc)
	room = 40
	tt.Frame()
	if got := caretRect(t, doc); !near(got.Y, caret.Y+40) {
		t.Fatalf("at the top, room above the caret moved it from %v to %v, want 40 down", caret.Y, got.Y)
	}
	if first := boxes[0]; first.Index != 0 || first.Top != 0 {
		t.Fatalf("at the top, the first box is %+v", first)
	}
}

// An edit that moves a paragraph across the top of the view in one step
// of undo, as a block editor's drag by a handle does, keeps the view: the
// paragraphs in view that did not move stay where they were, and the one
// moved goes where it was put. The editor follows the edit's replacements
// one by one, as undo and redo do, rather than one region from the first
// to the last, which held the first paragraph in view and sent the view to
// the region's start (#1723). With the keyboard, the caret's line stays.
func TestAMoveAcrossTheTopOfTheViewKeepsIt(t *testing.T) {
	for _, focus := range []bool{false, true} {
		for _, kind := range []string{"into the view", "into the view from just above it", "out to above it"} {
			into := kind != "out to above it"
			name := kind
			if focus {
				name += ", focused"
			}
			t.Run(name, func(t *testing.T) {
				doc := longDoc(200)
				tt, boxes, _, _ := anchorTester(t, doc, focus)
				doc.ScrollBy(13) // the first paragraph in view partly out
				tt.Frame()
				before := placed(doc, *boxes)
				caret := caretRect(t, doc)

				// Into the view: paragraph 10, or the one right above the
				// view, goes before the one next to last in view, below the
				// caret's. Out of it: the second in view goes before
				// paragraph 10.
				from, to := 10, (*boxes)[len(*boxes)-2].Index
				switch kind {
				case "into the view from just above it":
					from = (*boxes)[0].Index - 1
				case "out to above it":
					from, to = (*boxes)[1].Index, 10
				}
				moving := paraName(from)
				text := doc.Text()
				fs, fe := text.paraStart(from), text.paraStart(from+1)
				moved := text.Slice(fs, fe)
				ts := text.paraStart(to)
				doc.Edit(func(e *Edit) {
					e.Replace(fs, fe, Text{})
					if ts > fs {
						ts -= fe - fs
					}
					e.Replace(ts, ts, moved)
				})
				tt.Frame()
				after := placed(doc, *boxes)

				switch {
				case focus:
					if got := caretRect(t, doc); got != caret {
						t.Fatalf("the move took the caret from %v to %v", caret, got)
					}
				case into:
					// What's above where it went stays, and it goes where
					// the paragraph it went before was.
					for p, top := range before {
						if num(p) < to && !near(after[p], top) {
							t.Errorf("the move took %s, above it, from %v to %v", p, top, after[p])
						}
					}
					if !near(after[moving], before[paraName(to)]) {
						t.Errorf("%s landed at %v; %s, which it went before, was at %v", moving, after[moving], paraName(to), before[paraName(to)])
					}
				default:
					// What's above where it was stays, and what was below
					// it moves up into its place.
					for p, top := range before {
						if num(p) < from && !near(after[p], top) {
							t.Errorf("the move took %s, above it, from %v to %v", p, top, after[p])
						}
					}
					if _, ok := after[moving]; ok {
						t.Errorf("%s is still in view", moving)
					}
					if !near(after[paraName(from+1)], before[moving]) {
						t.Errorf("%s went to %v, not up to %v where %s was", paraName(from+1), after[paraName(from+1)], before[moving], moving)
					}
				}

				// Undo puts the view back as it was, and redo as the move
				// left it.
				doc.Undo()
				tt.Frame()
				samePlaces(t, "undone", before, placed(doc, *boxes))
				doc.Redo()
				tt.Frame()
				samePlaces(t, "redone", after, placed(doc, *boxes))
			})
		}
	}
}

// placed returns where the paragraphs in view are, by their names
// ("Paragraph 12").
func placed(doc *Document, boxes []ParagraphBox) map[string]float32 {
	text := doc.Text()
	out := map[string]float32{}
	for _, b := range boxes {
		s := text.Slice(b.Start, text.paraStart(b.Index+1)).String()
		out[strings.SplitN(s, ",", 2)[0]] = b.Top
	}
	return out
}

// paraName returns the name of longDoc's paragraph i, and num the number
// in a name.
func paraName(i int) string { return fmt.Sprintf("Paragraph %d", i) }

func num(name string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(name, "Paragraph "))
	return n
}

// samePlaces fails unless the same paragraphs are in view, at the same
// places.
func samePlaces(t *testing.T, what string, want, got map[string]float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d paragraphs in view, want %d: %v, want %v", what, len(got), len(want), got, want)
	}
	for p, top := range want {
		if g, ok := got[p]; !ok || !near(g, top) {
			t.Fatalf("%s: %s is at %v (in view: %v), want %v", what, p, g, ok, top)
		}
	}
}

// An edit of one replacement that puts a paragraph right where the first
// paragraph in view starts shows it at the top of the view, as it always
// has. One of several, as a paragraph moved there from above, puts it
// above the view: what's in view stays, as undoing such a move needs. At
// the start of the caret's paragraph, a Return moves the line being typed
// down.
func TestWhatIsPutAtTheStartOfTheAnchor(t *testing.T) {
	t.Run("one replacement", func(t *testing.T) {
		doc := longDoc(200)
		tt, boxes, _, _ := anchorTester(t, doc, false)
		doc.ScrollBy(13)
		tt.Frame()
		first := (*boxes)[0]
		before := placed(doc, *boxes)
		doc.Edit(func(e *Edit) {
			e.ReplaceString(first.Start, first.Start, "Inserted, long enough to wrap in a window this narrow, and then some more.\n", Attributes{})
		})
		tt.Frame()
		after := placed(doc, *boxes)
		if top, ok := after["Inserted"]; !ok || !near(top, first.Top) {
			t.Errorf("the paragraph put there is at %v (in view: %v), want %v, where %s was", top, ok, first.Top, paraName(first.Index))
		}
		if p := paraName(first.Index); after[p] <= before[p] {
			t.Errorf("%s stayed at %v", p, after[p])
		}
	})
	t.Run("several replacements", func(t *testing.T) {
		doc := longDoc(200)
		tt, boxes, _, _ := anchorTester(t, doc, false)
		doc.ScrollBy(13)
		tt.Frame()
		before := placed(doc, *boxes)
		text := doc.Text()
		fs, fe := text.paraStart(10), text.paraStart(11)
		moved := text.Slice(fs, fe)
		at := (*boxes)[0].Start
		doc.Edit(func(e *Edit) {
			e.Replace(fs, fe, Text{})
			e.Replace(at-(fe-fs), at-(fe-fs), moved)
		})
		tt.Frame()
		samePlaces(t, "a paragraph moved to its start", before, placed(doc, *boxes))
	})
	t.Run("the caret's paragraph", func(t *testing.T) {
		doc := longDoc(200)
		tt, boxes, _, _ := anchorTester(t, doc, true)
		first := (*boxes)[0]
		caret := caretRect(t, doc)
		tt.Key(0, ui.KeyEnter)
		if got := caretRect(t, doc); got.Y <= caret.Y {
			t.Fatalf("a Return at the start of the caret's line left it at %v from %v", got.Y, caret.Y)
		}
		if got := (*boxes)[0]; got != first {
			t.Fatalf("a Return moved the first paragraph in view from %+v to %+v", first, got)
		}
	})
}

// A step of undo keeps up to maxUndoParts replacements, and its undo
// follows them, as the move's: the view comes back. Undoing a longer one
// tells the editor of one region, from its first replacement to its last,
// as every undo did before: the view goes to the region's start, then to
// the caret the undo puts back.
func TestALongStepUndoesAsARegion(t *testing.T) {
	for _, parts := range []int{maxUndoParts, maxUndoParts + 1} {
		t.Run(fmt.Sprint(parts, " replacements"), func(t *testing.T) {
			doc := longDoc(200)
			tt, boxes, _, _ := anchorTester(t, doc, false)
			doc.ScrollBy(13)
			tt.Frame()
			before := placed(doc, *boxes)
			text := doc.Text()
			fs, fe := text.paraStart(10), text.paraStart(11)
			moved := text.Slice(fs, fe)
			to := text.paraStart((*boxes)[len(*boxes)-2].Index)
			s5 := text.paraStart(5)
			doc.Edit(func(e *Edit) {
				// A letter of paragraph 5 replaced by itself, then the
				// move's two replacements.
				for range parts - 2 {
					e.ReplaceString(s5, s5+1, "P", Attributes{})
				}
				e.Replace(fs, fe, Text{})
				e.Replace(to-(fe-fs), to-(fe-fs), moved)
			})
			tt.Frame()
			if got := placed(doc, *boxes)[paraName(50)]; !near(got, before[paraName(50)]) {
				t.Fatalf("the edit, followed one by one, moved Paragraph 50 from %v to %v", before[paraName(50)], got)
			}
			doc.Undo()
			tt.Frame()
			if doc.Text().String() != text.String() {
				t.Fatal("undo didn't bring the text back")
			}
			back := placed(doc, *boxes)
			came := len(back) == len(before)
			for p, top := range before {
				if g, ok := back[p]; !ok || !near(g, top) {
					came = false
				}
			}
			if want := parts <= maxUndoParts; came != want {
				t.Errorf("undone, the view came back: %v, want %v (%d replacements, %d kept)", came, want, parts, maxUndoParts)
			}
			if _, ok := doc.CaretRect(); !ok {
				t.Error("the caret undo put back is out of view")
			}
		})
	}
}

// A paragraph scrolled partly out of the editor shows only what is inside
// the editor's box: its lines above the box are not painted over what is
// there, as a picture taller than the editor's view would be. In an
// Editor, they are not painted over its border or out of its rounded
// corners either.
func TestScrolledTextStaysInBox(t *testing.T) {
	red := ui.RGB(255, 0, 0)
	for _, styled := range []bool{false, true} {
		name := "EditorBase"
		if styled {
			name = "Editor"
		}
		t.Run(name, func(t *testing.T) {
			doc := NewDocument(NewText(strings.Repeat("overflow text ", 200)))
			doc.SetSelection(doc.Text().Len(), doc.Text().Len())
			var border, radius float32
			tt := openTester(t, func(c *ui.Context) {
				ui.Column(c).Padding(100, 20).Children(func() {
					if styled {
						// Padding of a DIP and large corners bring the text
						// into them; the height is past an Editor's least.
						th := *c.Theme()
						th.Radius = 20
						ui.Themed(c, &th).Children(func() {
							Editor(c, doc, EditorOptions{Theme: &Theme{Text: red, Padding: [2]float32{1, 1}}}).Width(300).Height(120)
						})
						border, radius = editorBorder, th.Radius
					} else {
						EditorBase(c, doc, EditorOptions{Theme: &Theme{Text: red}}).Width(300).Height(120)
					}
				})
			}, 400, 300)
			tt.Frame()
			img := tt.Image()
			s := img.Bounds().Dx() / 400
			// out returns how far the point (x, y) is outside the box the
			// text may show in, in DIPs: inside the border, with the
			// corners inside it rounded.
			x0, y0, x1, y1 := 20+border, 100+border, 320-border, 220-border
			r := max(radius-border, 0)
			out := func(x, y float32) float32 {
				dx := max(x0+r-x, x-(x1-r), 0)
				dy := max(y0+r-y, y-(y1-r), 0)
				if dx > 0 && dy > 0 {
					return float32(math.Hypot(float64(dx), float64(dy))) - r
				}
				return max(x0-x, x-x1, y0-y, y-y1)
			}
			inside, outside := 0, 0
			for y := 0; y < 300*s; y++ {
				for x := 0; x < 400*s; x++ {
					if c := img.RGBAAt(x, y); c.R > 150 && c.G < 100 && c.B < 100 {
						if out((float32(x)+0.5)/float32(s), (float32(y)+0.5)/float32(s)) > 0.5/float32(s) {
							outside++
						} else {
							inside++
						}
					}
				}
			}
			if inside == 0 {
				t.Fatal("no text shows in the editor")
			}
			if outside > 0 {
				t.Fatalf("%d pixels of text outside the box", outside)
			}
		})
	}
}
