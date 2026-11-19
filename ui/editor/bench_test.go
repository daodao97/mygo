package editor

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// bigGo is a Go file of about n lines.
func bigGo(n int) string {
	var b strings.Builder
	b.WriteString("package big\n\nimport \"fmt\"\n\n")
	for i := 0; b.Len() < n*40; i++ {
		b.WriteString("// handle" + strconv.Itoa(i) + " handles the request, with \"quotes\" and 0x1f.\n")
		b.WriteString("func handle" + strconv.Itoa(i) + "(w io.Writer, r *Request) error {\n")
		b.WriteString("\tif err := r.Check(\"x\\n\", 42); err != nil {\n\t\treturn fmt.Errorf(`bad: %w`, err)\n\t}\n")
		b.WriteString("\tfor i, v := range r.Values { // each\n\t\tfmt.Fprintln(w, i, v)\n\t}\n\treturn nil\n}\n\n")
	}
	return b.String()
}

func newBenchEditor(b *testing.B, text string, wrap bool) (*State, *ui.Tester) {
	st := &State{Buffer: NewBuffer(text), SoftWrap: wrap}
	st.Buffer.SetLanguage(LanguageNamed("go"))
	tt := ui.NewTester(func(c *ui.Context) {
		View(c, st).Fill().AutoFocus()
	}, 1200, 800)
	tt.SetScale(2)
	// Highlight the whole text first, as an idle editor has.
	st.ed.view.advanceSyntax(st.ed.view.text.LineCount(), time.Hour)
	tt.Frame()
	tt.Move(600, 400)
	return st, tt
}

// BenchmarkCodeEditorScroll renders the frames of scrolling a Go file of
// 100,000 lines a few lines at a time: building, laying out the rows that
// come into view, painting and drawing on the CPU.
func BenchmarkCodeEditorScroll(b *testing.B) {
	_, tt := newBenchEditor(b, bigGo(100_000), false)
	b.ReportAllocs()
	for b.Loop() {
		tt.Scroll(600, 400, 0, 61)
	}
}

// BenchmarkCodeEditorScrollWrap scrolls with soft wraps.
func BenchmarkCodeEditorScrollWrap(b *testing.B) {
	_, tt := newBenchEditor(b, bigGo(100_000), true)
	b.ReportAllocs()
	for b.Loop() {
		tt.Scroll(600, 400, 0, 61)
	}
}

// BenchmarkCodeEditorType types a character in the middle of a Go file
// of 100,000 lines and renders the frame.
func BenchmarkCodeEditorType(b *testing.B) {
	st, tt := newBenchEditor(b, bigGo(100_000), false)
	mid := st.Buffer.Offset(Pos{Line: 50_000})
	st.Select(mid, mid)
	tt.Frame()
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		if i%2 == 0 {
			tt.Type("x")
		} else {
			tt.Key(0, ui.KeyBackspace)
		}
	}
}

// BenchmarkCodeEditorOpen makes an editor of a Go file of 100,000 lines
// and renders its first frame.
func BenchmarkCodeEditorOpen(b *testing.B) {
	text := bigGo(100_000)
	b.ReportAllocs()
	for b.Loop() {
		st := &State{Buffer: NewBuffer(text)}
		st.Buffer.SetLanguage(LanguageNamed("go"))
		tt := ui.NewTester(func(c *ui.Context) {
			View(c, st).Fill()
		}, 1200, 800)
		tt.SetScale(2)
		tt.Frame()
	}
}

// BenchmarkDiffViewerScroll scrolls a split diff of two versions of a Go
// file of 20,000 lines, a change every 50 lines, all lines shown.
func BenchmarkDiffViewerScroll(b *testing.B) {
	old := bigGo(20_000)
	lines := strings.Split(old, "\n")
	for i := 10; i < len(lines); i += 50 {
		lines[i] = strings.Replace(lines[i], "err", "e", 1) + " // changed"
	}
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(strings.Join(lines, "\n")), ExpandUnchanged: true}
	d.Old.SetLanguage(LanguageNamed("go"))
	d.New.SetLanguage(LanguageNamed("go"))
	tt := ui.NewTester(func(c *ui.Context) {
		DiffView(c, d).Fill()
	}, 1400, 800)
	tt.SetScale(2)
	tt.Frame()
	tt.Move(700, 400)
	b.ReportAllocs()
	for b.Loop() {
		tt.Scroll(700, 400, 0, 61)
	}
}

// BenchmarkDiffViewerType types in a diff of 20,000 lines, which compares
// again around the edit.
func BenchmarkDiffViewerType(b *testing.B) {
	old := bigGo(20_000)
	lines := strings.Split(old, "\n")
	for i := 10; i < len(lines); i += 50 {
		lines[i] += " // changed"
	}
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(strings.Join(lines, "\n")), Editable: true}
	tt := ui.NewTester(func(c *ui.Context) {
		DiffView(c, d).Fill()
	}, 1400, 800)
	tt.SetScale(2)
	tt.Frame()
	ed := &d.eds[1]
	off := d.New.Offset(Pos{Line: 10_010})
	ed.sels = []sel{{anchor: off, head: off, goal: -1}}
	ed.reveal = revealCenter
	tt.ClickAt(1000, 400)
	ed.sels = []sel{{anchor: off, head: off, goal: -1}}
	tt.Frame()
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		if i%2 == 0 {
			tt.Type("x")
		} else {
			tt.Key(0, ui.KeyBackspace)
		}
	}
}

// newFitBench shows a diff of two versions of a Go file of about n lines,
// a change every 50 lines, as high as its content in a scrolling list
// under a header, as an app listing a change's files shows it.
func newFitBench(n int, layout DiffLayout, wrap bool) *ui.Tester {
	old := bigGo(n)
	lines := strings.Split(old, "\n")
	for i := 10; i < len(lines); i += 50 {
		lines[i] = strings.Replace(lines[i], "err", "e", 1) + " // changed, and a few more words to wrap on narrow windows"
	}
	d := &DiffState{Old: NewBuffer(old), New: NewBuffer(strings.Join(lines, "\n")), Layout: layout, Wrap: wrap, FitContent: true}
	d.Old.SetLanguage(LanguageNamed("go"))
	d.New.SetLanguage(LanguageNamed("go"))
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() {
			ui.Box(c).Height(200)
			DiffView(c, d).FillWidth()
		})
	}, 1400, 800)
	tt.SetScale(2)
	for range 4 {
		tt.Frame()
	}
	tt.Move(700, 400)
	return tt
}

func benchFitScroll(b *testing.B, layout DiffLayout) {
	tt := newFitBench(20_000, layout, false)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		dy := float32(61)
		if i/200%2 == 1 {
			dy = -61
		}
		tt.Scroll(700, 400, 0, dy)
	}
}

// BenchmarkDiffFitScroll scrolls a split diff of 20,000 lines as high as
// its content, down and back up, in the list it is in.
func BenchmarkDiffFitScroll(b *testing.B) { benchFitScroll(b, DiffSplit) }

// BenchmarkDiffFitScrollUnified scrolls a unified one.
func BenchmarkDiffFitScrollUnified(b *testing.B) { benchFitScroll(b, DiffUnified) }

func benchFitResize(b *testing.B, layout DiffLayout, wrap bool) {
	tt := newFitBench(20_000, layout, wrap)
	widths := []int{1400, 1380, 1360, 1340, 1320, 1300, 1320, 1340, 1360, 1380}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		tt.SetSize(widths[i%len(widths)], 800)
		tt.Frame()
	}
}

// BenchmarkDiffFitResize renders the frames of a window resized through
// widths over a split diff of 20,000 lines as high as its content.
func BenchmarkDiffFitResize(b *testing.B) { benchFitResize(b, DiffSplit, false) }

// BenchmarkDiffFitResizeWrap resizes with soft wraps, which each width
// measures again.
func BenchmarkDiffFitResizeWrap(b *testing.B) { benchFitResize(b, DiffSplit, true) }

// BenchmarkDiffFitResizeUnifiedWrap resizes a unified one with soft wraps.
func BenchmarkDiffFitResizeUnifiedWrap(b *testing.B) { benchFitResize(b, DiffUnified, true) }

// BenchmarkDiffFitFrame renders a frame of a split diff of 20,000 lines
// as high as its content in which nothing changed, as the pointer moving
// over it asks for.
func BenchmarkDiffFitFrame(b *testing.B) {
	tt := newFitBench(20_000, DiffSplit, false)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		tt.Move(700, float32(400+i%2))
	}
}

// BenchmarkDiffDeletedFileScroll scrolls near the end of a unified diff of
// a deleted Go file of 20,000 lines: one run of deleted lines, most of
// it above the view.
func BenchmarkDiffDeletedFileScroll(b *testing.B) {
	d := &DiffState{Old: NewBuffer(bigGo(20_000)), New: NewBuffer(""), Layout: DiffUnified}
	d.Old.SetLanguage(LanguageNamed("go"))
	tt := ui.NewTester(func(c *ui.Context) {
		DiffView(c, d).Fill()
	}, 1400, 800)
	tt.SetScale(2)
	tt.Frame()
	d.SetScrollY(math.MaxFloat64)
	tt.Frame()
	tt.Move(700, 400)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		dy := float32(-61)
		if i%2 == 1 {
			dy = 61
		}
		tt.Scroll(700, 400, 0, dy)
	}
}

// bigChinese is Chinese prose of n lines, with Latin words, numbers and
// punctuation, each a few rows wide.
func bigChinese(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("这是第" + strconv.Itoa(i) + "行，用于测试软换行的中文文本：每一行都含有若干标点符号、English words 和数字 42。逗号、顿号和句号在中文排版中占一个全角字宽；分号与冒号同样如此。\n")
	}
	return b.String()
}

// BenchmarkCodeEditorWrapChinese measures the soft wraps of all the
// lines of Chinese prose of 20,000 lines, as each width of a resize does.
func BenchmarkCodeEditorWrapChinese(b *testing.B) {
	st, _ := newBenchEditor(b, bigChinese(20_000), true)
	v := &st.ed.view
	width := v.wrapWidth
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		v.setWrap(true, width-float32(i%2*10))
		for !v.measureWraps(0, time.Hour) {
		}
	}
}

// BenchmarkCodeEditorScrollWrapChinese scrolls Chinese prose of 20,000
// lines with soft wraps, whose rows are shaped to fit as they show.
func BenchmarkCodeEditorScrollWrapChinese(b *testing.B) {
	_, tt := newBenchEditor(b, bigChinese(20_000), true)
	b.ReportAllocs()
	for b.Loop() {
		tt.Scroll(600, 400, 0, 61)
	}
}

// benchNote is a Markdown note of n lines, and what its live preview
// replaces: the marks of emphasis on every line, and a link's on every
// fourth.
func benchNote(n int) (string, []Replacement) {
	var b strings.Builder
	var rs []Replacement
	for i := range n {
		start := b.Len()
		b.WriteString("Some **bold** words")
		rs = append(rs, Replacement{Start: start + 5, End: start + 7}, Replacement{Start: start + 11, End: start + 13})
		if i%4 == 0 {
			at := b.Len()
			b.WriteString(" and a [link](https://example.com)")
			rs = append(rs, Replacement{Start: at + 7, End: at + 8}, Replacement{Start: at + 12, End: at + 34})
		}
		b.WriteString(" on line " + strconv.Itoa(i) + ".\n")
	}
	return b.String(), rs
}

// BenchmarkCodeEditorTypeBlocks types in 10,000 lines and renders the
// frame, with blocks moved through edits or set again in every build.
// The folded mode also exercises the unchanged folds on each layout.
// The stand-in mode hides whole lines with empty replacements, sets both
// replacements and blocks every build, and types in adjacent visible text.
func BenchmarkCodeEditorTypeBlocks(b *testing.B) {
	for _, mode := range []string{"source", "moved", "set", "set-folded", "set-stand-in"} {
		b.Run(mode, func(b *testing.B) {
			standIn := mode == "set-stand-in"
			text, _ := benchNote(10_000)
			var bs []Block
			for line := 0; line < 10_000; line += 4 {
				bs = append(bs, tall(line, false, 20, "Block"))
			}
			st := &State{Buffer: NewBuffer(text)}
			if standIn {
				var rs []Replacement
				for _, block := range bs {
					rs = append(rs, hide(st.Buffer.Offset(Pos{Line: block.Line}), st.Buffer.Offset(Pos{Line: block.Line + 1})))
				}
				st.SetReplacements(rs)
			}
			tt := ui.NewTester(func(c *ui.Context) {
				if mode == "set" || mode == "set-folded" || standIn {
					st.SetBlocks(bs)
				}
				if standIn {
					// SetBlocks syncs edits first. Use moved ranges, not stale
					// byte offsets after typing before later replacements.
					st.SetReplacements(st.Replacements())
				}
				View(c, st).Fill().AutoFocus()
			}, 1200, 800)
			tt.SetScale(2)
			tt.Frame()
			if mode != "source" {
				st.SetBlocks(bs)
			}
			if mode == "set-folded" {
				st.ed.folds = []fold{{100, 200}, {8_000, 8_100}}
			}
			pos := Pos{Line: 5_000}
			if standIn {
				pos.Line++
				pos.Col = 1
			}
			mid := st.Buffer.Offset(pos)
			st.Select(mid, mid)
			tt.Frame()
			tt.Frame()
			if standIn {
				if _, ok := st.ed.standIn(pos.Line - 1); !ok || st.ed.view.heights.isHidden(pos.Line) {
					b.Fatal("the benchmark needs a stand-in beside the visible typing line")
				}
			}
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				if i%2 == 0 {
					tt.Type("x")
				} else {
					tt.Key(0, ui.KeyBackspace)
				}
				tt.Frame()
			}
		})
	}
}

// BenchmarkCodeEditorTypeReplacements types a character in the middle of
// a note of 10,000 lines and renders the frame: with no replacements, as
// its source shows; with those of a live preview, which the edits move;
// and with the app setting them again after each key press.
func BenchmarkCodeEditorTypeReplacements(b *testing.B) {
	for _, mode := range []string{"source", "moved", "set"} {
		b.Run(mode, func(b *testing.B) {
			text, rs := benchNote(10_000)
			st := &State{Buffer: NewBuffer(text)}
			tt := ui.NewTester(func(c *ui.Context) {
				View(c, st).Fill().AutoFocus()
			}, 1200, 800)
			tt.SetScale(2)
			tt.Frame()
			if mode != "source" {
				st.SetReplacements(rs)
			}
			mid := st.Buffer.Offset(Pos{Line: 5_000})
			st.Select(mid, mid)
			tt.Frame()
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				if i%2 == 0 {
					tt.Type("x")
				} else {
					tt.Key(0, ui.KeyBackspace)
				}
				if mode == "set" {
					st.SetReplacements(st.Replacements())
				}
				tt.Frame()
			}
		})
	}
}
