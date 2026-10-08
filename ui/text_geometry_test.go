package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/platform"
)

type selectionTestHost struct{ *headless }

func (*selectionTestHost) nativeTextSelection() bool { return true }

func TestReadOnlyNativeSelection(t *testing.T) {
	const text = "Read-only text: Hello MyGo 👋 中文"
	tt := snapshotTester(func(c *Context) {
		Text(c, text).Selectable().Label("sample").Width(280)
	}, 320, 100)
	tt.rt.host = &selectionTestHost{tt.h}
	tt.Click("sample")
	state := tt.h.ime
	if !state.Active || !state.ReadOnly || state.ID == 0 || state.Text != text {
		t.Fatalf("missing read-only selection proxy: %+v", state)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: 16, To: 21})
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
	if tt.h.clipboard != "Hello" {
		t.Fatalf("native read-only selection was not retained: %q", tt.h.clipboard)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "selectAll"})
	g := tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "selection", End: utf8.RuneCountInString(text)})
	if !g.Valid || len(g.Rects) == 0 {
		t.Fatal("read-only native selection lost Go geometry")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
	if tt.h.clipboard != text {
		t.Fatal("read-only selection did not copy the full Go text")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "cut"})
	tt.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "replacement"})
	if tt.h.ime.Text != text {
		t.Fatal("read-only proxy accepted an edit")
	}
}

func TestTextGeometryUsesVisibleGoLayout(t *testing.T) {
	for _, multiline := range []bool{false, true} {
		t.Run(map[bool]string{false: "line", true: "area"}[multiline], func(t *testing.T) {
			value := "Hello 中文👋 world"
			if multiline {
				value += "\nSecond line with emoji 👋\nThird line"
			}
			tt := snapshotTester(func(c *Context) {
				Column(c).Padding(17).Children(func() {
					var e *Element
					if multiline {
						e = TextArea(c, &value).Height(160)
					} else {
						e = TextInput(c, &value).Height(50)
					}
					e.Label("Editor").FontSize(21).Width(250).AutoFocus()
				})
			}, 320, 240)
			state := tt.h.ime
			if state.ID == 0 || state.Bounds.X != 17 || state.Bounds.Y != 17 || state.Bounds.W != 250 {
				t.Fatalf("field identity/bounds: %+v", state)
			}
			for _, i := range []int{0, 6, 8, 9, utf8.RuneCountInString(value)} {
				g := tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "caret", Start: i})
				if !g.Valid || g.Caret.H <= 0 || g.Caret.W != 1 {
					t.Fatalf("missing caret %d: %+v", i, g)
				}
				hit := tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "hit", X: g.Caret.X, Y: g.Caret.Y + g.Caret.H/2})
				if !hit.Valid || hit.Index != i {
					t.Fatalf("caret/hit mismatch at rune %d: %+v", i, hit)
				}
			}
			g := tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "selection", Start: 6, End: utf8.RuneCountInString(value)})
			if !g.Valid || len(g.Rects) == 0 || multiline && len(g.Rects) < 3 {
				t.Fatalf("missing selection: %+v", g)
			}
			empty := tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "selection", Start: 0, End: 0})
			if !empty.Valid || len(empty.Rects) != 0 {
				t.Fatalf("empty selection: %+v", empty)
			}
			for _, r := range g.Rects {
				if r.X < state.Bounds.X || r.Y < state.Bounds.Y || r.X+r.W > state.Bounds.X+state.Bounds.W || r.Y+r.H > state.Bounds.Y+state.Bounds.H {
					t.Fatalf("selection escaped clipping: %+v", r)
				}
			}
			if tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID + 1, Kind: "caret"}).Valid {
				t.Fatal("accepted a stale field identity")
			}
			tt.send(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
			if tt.rt.textGeometry(platform.TextGeometryQuery{ID: state.ID, Kind: "caret"}).Valid {
				t.Fatal("accepted a blurred editor")
			}
		})
	}
}

func TestCustomTextCaretDoesNotEnableNativeSelection(t *testing.T) {
	tt := snapshotTester(func(c *Context) {
		Box(c).Size(200, 60).Focusable().AutoFocus().HandleInput(func(InputEvent) bool { return true }).TextCaret(Rect{10, 10, 1, 20})
	}, 300, 100)
	if !tt.h.ime.Active || tt.h.ime.ID != 0 || tt.h.ime.Bounds != (platform.RectF{}) {
		t.Fatalf("custom input was exposed to native selection: %+v", tt.h.ime)
	}
}

func TestNativeSelectionGeometryKeepsLargeAreaVirtual(t *testing.T) {
	value := strings.Repeat("A long document with 中文 👋\n", 2000)
	tt := snapshotTester(func(c *Context) { TextArea(c, &value).Size(260, 120).AutoFocus() }, 300, 180)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "selectAll"})
	ed := tt.rt.states[tt.rt.focused].editor
	before := ed.area.laid
	g := tt.rt.textGeometry(platform.TextGeometryQuery{ID: tt.h.ime.ID, Kind: "selection", End: utf8.RuneCountInString(value)})
	if !g.Valid || len(g.Rects) == 0 || len(g.Rects) > 20 || ed.area.laid != before {
		t.Fatalf("selection shaped invisible document paragraphs: rects=%d laid=%d -> %d", len(g.Rects), before, ed.area.laid)
	}
	// A native handle moving below the viewport should reveal the endpoint
	// through Go scrolling, without scrolling TextKit's invisible document.
	tt.send(platform.SurfaceEvent{Kind: platform.TextSelectionChanged, From: 0, To: 400})
	if ed.area.scroll <= 0 {
		t.Fatal("native selection did not reveal the endpoint")
	}
}

func TestTextSelectionGeometryDirection(t *testing.T) {
	value := "مرحبا بالعالم"
	tt := snapshotTester(func(c *Context) { TextInput(c, &value).Width(260).AutoFocus() }, 300, 100)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "selectAll"})
	g := tt.rt.textGeometry(platform.TextGeometryQuery{ID: tt.h.ime.ID, Kind: "selection", End: utf8.RuneCountInString(value)})
	if !g.Valid || len(g.Rects) == 0 {
		t.Fatal("missing RTL selection")
	}
	for _, r := range g.Rects {
		if !r.RTL {
			t.Fatal("RTL direction lost at the native boundary")
		}
	}
}

func TestTextGeometrySurroundingWindowAndPassword(t *testing.T) {
	value, password := strings.Repeat("中文👋 ", 400), false
	tt := snapshotTester(func(c *Context) {
		e := TextInput(c, &value).Label("Editor").Width(260).AutoFocus()
		if password {
			e.Password()
		}
	}, 300, 100)
	tt.Key(Ctrl, KeyEnd)
	s := tt.rt.states[tt.rt.focused]
	g := tt.rt.textGeometry(platform.TextGeometryQuery{ID: tt.h.ime.ID, Kind: "caret", Start: tt.h.ime.End})
	if tt.rt.ime.base == 0 || !g.Valid || g.Caret != platformRect(s.editor.caretRect(s)) {
		t.Fatalf("surrounding-text base was lost: base=%d geometry=%+v", tt.rt.ime.base, g)
	}
	// Native commands edit the full buffer, not just the keyboard proxy's
	// surrounding window; copy/cut/undo must share the Go editor's history.
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "selectAll"})
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "cut"})
	if value != "" || tt.h.clipboard != strings.Repeat("中文👋 ", 400) {
		t.Fatal("cut only affected the surrounding proxy")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "undo"})
	if value != tt.h.clipboard {
		t.Fatal("native command bypassed Go undo")
	}
	password = true
	tt.Frame()
	if tt.h.ime.Text != "" || tt.rt.textGeometry(platform.TextGeometryQuery{ID: tt.h.ime.ID, Kind: "caret"}).Valid {
		t.Fatal("password exposed text or geometry")
	}
}

type snapshotTestHost struct{ *headless }

func (*snapshotTestHost) snapshotTextInput() bool { return true }
func snapshotTester(view func(*Context), width, height int) *Tester {
	tt := NewTester(view, width, height)
	tt.rt.host = &snapshotTestHost{tt.h}
	tt.Frame()
	return tt
}
