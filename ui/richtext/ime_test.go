package richtext

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBoundsForRangePlain(t *testing.T) {
	doc := NewDocument(NewText("hello world"))
	richTester(t, doc)
	v := doc.shownIn.Value()

	one, _, ok := v.BoundsForRange(ui.TextInputRange{Start: 0, End: 1})
	if !ok {
		t.Fatal("single-rune bounds not visible")
	}
	endCaret, _, _ := v.BoundsForRange(ui.TextInputRange{Start: 5, End: 5})
	multi, actual, ok := v.BoundsForRange(ui.TextInputRange{Start: 0, End: 5})
	if !ok || actual != (ui.TextInputRange{Start: 0, End: 5}) {
		t.Fatalf("multi-rune bounds: rect=%+v actual=%+v ok=%v", multi, actual, ok)
	}
	if wantW := endCaret.X - one.X; multi.W != wantW || multi.W <= one.W {
		t.Fatalf("multi-rune rect=%+v, want W=%v (> %v)", multi, wantW, one.W)
	}
	rev, revActual, ok := v.BoundsForRange(ui.TextInputRange{Start: 5, End: 0})
	if !ok || revActual != actual || rev != multi {
		t.Fatalf("reversed range: rect=%+v actual=%+v, want rect=%+v actual=%+v", rev, revActual, multi, actual)
	}
}

func TestBoundsForRangeComposition(t *testing.T) {
	doc := NewDocument(NewText("ab"))
	tt, _ := richTester(t, doc)
	doc.SetSelection(1, 1)
	tt.Frame()

	v := doc.shownIn.Value()
	v.SetMarkedText(nil, "nihao", ui.TextInputRange{Start: 5, End: 5})

	// Virtual text is "anihaob"; query subrange "iha" inside the composition.
	startCaret, _, _ := v.BoundsForRange(ui.TextInputRange{Start: 2, End: 2})
	endCaret, _, _ := v.BoundsForRange(ui.TextInputRange{Start: 5, End: 5})
	b, actual, ok := v.BoundsForRange(ui.TextInputRange{Start: 2, End: 5})
	if !ok || actual != (ui.TextInputRange{Start: 2, End: 5}) {
		t.Fatalf("composition bounds: rect=%+v actual=%+v ok=%v", b, actual, ok)
	}
	if wantW := endCaret.X - startCaret.X; b.X != startCaret.X || b.W != wantW || b.W <= 1 {
		t.Fatalf("composition rect=%+v, want X=%v W=%v", b, startCaret.X, wantW)
	}
	full, fullActual, ok := v.BoundsForRange(ui.TextInputRange{Start: 0, End: 7})
	if !ok || fullActual != (ui.TextInputRange{Start: 0, End: 7}) || full.W <= b.W {
		t.Fatalf("full range with composition: rect=%+v actual=%+v ok=%v (innerW=%v)", full, fullActual, ok, b.W)
	}
}

func TestBoundsForRangeLineBreak(t *testing.T) {
	doc := NewDocument(NewText("hi\nhello world"))
	richTester(t, doc)
	v := doc.shownIn.Value()

	startCaret, _, _ := v.BoundsForRange(ui.TextInputRange{Start: 1, End: 1})
	cross, actual, ok := v.BoundsForRange(ui.TextInputRange{Start: 1, End: 10})
	if !ok || actual != (ui.TextInputRange{Start: 1, End: 1}) || cross != startCaret {
		t.Fatalf("newline cross: rect=%+v actual=%+v ok=%v, want rect=%+v", cross, actual, ok, startCaret)
	}

	const wrapped = "aa bbbbbb"
	wrapDoc := NewDocument(NewText(wrapped))
	tt := ui.NewTester(func(c *ui.Context) {
		Editor(c, wrapDoc).Width(80).Height(200)
	}, 100, 220)
	tt.Press(10, 10)
	tt.Release(10, 10)
	wv := wrapDoc.shownIn.Value()

	line0, _, _ := wv.BoundsForRange(ui.TextInputRange{Start: 0, End: 0})
	wrapAt := -1
	for i := 1; i <= len(wrapped); i++ {
		if c, _, _ := wv.BoundsForRange(ui.TextInputRange{Start: i, End: i}); c.Y > line0.Y {
			wrapAt = i
			break
		}
	}
	if wrapAt <= 0 {
		t.Fatal("text did not soft-wrap")
	}

	firstLine, firstActual, ok := wv.BoundsForRange(ui.TextInputRange{Start: 0, End: wrapAt})
	if !ok || firstActual != (ui.TextInputRange{Start: 0, End: wrapAt}) || firstLine.Y != line0.Y || firstLine.W <= 1 {
		t.Fatalf("first wrapped line: rect=%+v actual=%+v ok=%v (line0Y=%v)", firstLine, firstActual, ok, line0.Y)
	}
	crossWrap, crossActual, ok := wv.BoundsForRange(ui.TextInputRange{Start: 0, End: len(wrapped)})
	if !ok || crossActual != (ui.TextInputRange{Start: 0, End: 0}) || crossWrap != line0 {
		t.Fatalf("cross wrap: rect=%+v actual=%+v ok=%v, want rect=%+v", crossWrap, crossActual, ok, line0)
	}

	wv.caret = wrapAt
	wv.affinity = ui.TextUpstream
	secondLine, secondActual, ok := wv.BoundsForRange(ui.TextInputRange{Start: wrapAt, End: len(wrapped)})
	if !ok || secondActual != (ui.TextInputRange{Start: wrapAt, End: len(wrapped)}) || secondLine.Y <= line0.Y || secondLine.W <= 1 {
		t.Fatalf("second wrapped line: rect=%+v actual=%+v ok=%v (line0Y=%v)", secondLine, secondActual, ok, line0.Y)
	}
	atWrapCaret, _, _ := wv.BoundsForRange(ui.TextInputRange{Start: wrapAt, End: wrapAt})
	if atWrapCaret.Y != line0.Y || atWrapCaret.W != 1 {
		t.Fatalf("collapsed caret at wrap with upstream affinity: rect=%+v, want on line 0 (Y=%v)", atWrapCaret, line0.Y)
	}
}
