package ui

import (
	"fmt"
	"runtime"
	"slices"
	"testing"
	"weak"

	"github.com/egoist/mygo/internal/text"
	"github.com/egoist/mygo/internal/wait"
	"golang.org/x/image/font/gofont/gomono"
)

// TestTwoTestersInParallel runs Testers on different goroutines, as
// t.Parallel does, and fails under go test -race while they share one
// glyph atlas (issue #1722).
func TestTwoTestersInParallel(t *testing.T) {
	for i := range 4 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			frame := 0
			tt := NewTester(func(c *Context) {
				Column(c).Children(func() {
					for j := range 30 {
						r := rune(0x4e00 + i*3000 + frame*30 + j) // new glyphs every frame
						Row(c).Children(func() {
							Text(c, fmt.Sprintf("tester %d frame %d %c", i, frame, r)).FontSize(float32(11 + (frame+j)%9))
						})
					}
				})
			}, 400, 900)
			for range 40 {
				frame++
				tt.Frame()
			}
		})
	}
}

// TestParallelTestersStyledText runs Testers laying out styled text, with
// spans of weights, italics and sizes, underlines and inline objects, on
// different goroutines, and fails under go test -race while their
// engines share state of the process: the frame statistics setting each
// runtime wrote as it started, Core Text's run delegates and glyph
// outlines on macOS, DirectWrite's inline objects on Windows (issue
// #1722).
func TestParallelTestersStyledText(t *testing.T) {
	for i := range 4 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			frame := 0
			tt := NewTester(func(c *Context) {
				Column(c).Children(func() {
					for j := range 20 {
						RichText(c,
							Span{Text: fmt.Sprintf("tester %d frame %d row %d ", i, frame, j)},
							Span{Text: "bold", Weight: 700},
							Span{Text: " italic", Italic: true, Size: float32(11 + j%5)},
							Span{Text: " linked", Underline: true},
							Span{Text: "\uFFFC", ObjectWidth: float32(8 + j%4), ObjectHeight: 10, ObjectDescent: 2},
							Span{Text: " light", Weight: 300, Size: float32(10 + (frame+j)%7)},
						).MaxWidth(240)
					}
				})
			}, 400, 900)
			for range 10 {
				frame++
				tt.Frame()
			}
			if !tt.HasText(fmt.Sprintf("tester %d frame 10 row 0 bold", i)) {
				t.Error("the styled text isn't laid out")
			}
		})
	}
}

// A Tester's window, which shapes text in a system of its own, draws the
// fonts the app registered, as the app's windows do. Go Mono is
// monospaced, so "iiiiiiii" is as wide as "mmmmmmmm" only in it.
func TestATesterDrawsRegisteredFonts(t *testing.T) {
	const family = "MyGo Test Tester Mono"
	if err := RegisterFont(gomono.TTF, family); err != nil {
		t.Fatal(err)
	}
	tt := NewTester(func(c *Context) {
		Column(c).Children(func() {
			for _, s := range []string{"iiiiiiii", "mmmmmmmm"} {
				Row(c).Children(func() { Text(c, s).Font(family).FontSize(20) })
			}
		})
	}, 400, 300)
	i, _ := tt.Find("iiiiiiii")
	m, _ := tt.Find("mmmmmmmm")
	if i.W < m.W-0.5 || i.W > m.W+0.5 {
		t.Fatalf("the registered font isn't drawn: %v wide, %v in Go Mono", i.W, m.W)
	}
}

// The text system of a Tester's window, its own, closes and becomes
// unreachable once the Tester is and the next Tester starts, when the
// Testers keep no more idle (testerTexts): a test binary of a thousand
// Testers otherwise kept a thousand, each with a Pango font map and a
// thread of fontconfig's on Linux (#1902).
func TestTesterTextSystemUnreachable(t *testing.T) {
	// Keep none idle, so that each Tester's goes once it ends.
	testerTexts.mu.Lock()
	max := maxIdleTesterTexts
	maxIdleTesterTexts = func() int { return 0 }
	testerTexts.mu.Unlock()
	t.Cleanup(func() {
		testerTexts.mu.Lock()
		maxIdleTesterTexts = max
		testerTexts.mu.Unlock()
	})
	var systems []weak.Pointer[text.System]
	for i := range 8 {
		tt := NewTester(func(c *Context) {
			Column(c).Children(func() {
				Text(c, fmt.Sprintf("tester %d", i))
				TextInput(c, new(string)).Placeholder("Name")
			})
		}, 300, 200)
		tt.Frame()
		if tt.h.text == nil {
			t.Fatal("the Tester's window has no text system of its own")
		}
		systems = append(systems, weak.Make(tt.h.text))
	}
	wait.Until(t, "the text systems of the Testers dropped idle", func() bool {
		runtime.GC()
		return !slices.ContainsFunc(systems, func(w weak.Pointer[text.System]) bool {
			s := w.Value()
			return s != nil && !testerTextIdle(s)
		})
	})
	next := NewTester(func(c *Context) { Text(c, "next") }, 300, 200)
	wait.Until(t, "the text systems of the Testers dropped unreachable", func() bool {
		runtime.GC()
		return !slices.ContainsFunc(systems, func(w weak.Pointer[text.System]) bool {
			s := w.Value()
			return s != nil && s != next.h.text
		})
	})
	runtime.KeepAlive(next)
}
