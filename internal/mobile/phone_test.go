package mobile

import (
	"bytes"
	"sync"
	"testing"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/wait"
)

// textClient is a document of a text input, without its layout.
type textClient struct {
	text        string
	sel, marked platform.TextRange
	isMarked    bool
}

func (c *textClient) TextForRange(r platform.TextRange) (string, platform.TextRange) {
	a, z := min(r.Start, len(c.text)), min(r.End, len(c.text))
	return c.text[a:z], platform.TextRange{Start: a, End: z}
}
func (c *textClient) Selection() platform.TextSelection { return platform.TextSelection{Range: c.sel} }
func (c *textClient) MarkedRange() (platform.TextRange, bool) {
	return c.marked, c.isMarked
}
func (c *textClient) ReplaceText(r *platform.TextRange, s string) {
	to := c.sel
	if r != nil {
		to = *r
	} else if c.isMarked {
		to = c.marked
	}
	c.text = c.text[:to.Start] + s + c.text[to.End:]
	c.sel = platform.TextRange{Start: to.Start + len(s), End: to.Start + len(s)}
	c.isMarked = false
}
func (c *textClient) SetMarkedText(r *platform.TextRange, s string, sel platform.TextRange) {
	c.ReplaceText(r, s)
	c.marked, c.isMarked = platform.TextRange{Start: c.sel.Start - len(s), End: c.sel.Start}, true
}
func (c *textClient) UnmarkText() { c.isMarked = false }
func (c *textClient) BoundsForRange(r platform.TextRange) (platform.RectF, platform.TextRange, bool) {
	return platform.RectF{X: float64(r.Start) * 10, Y: 20, W: 2, H: 16}, r, true
}
func (c *textClient) IndexForPoint(x, y float64) (int, bool) { return int(x) / 10, true }

// surfaceEvents hears what a surface sends its content.
type surfaceEvents struct {
	platform.WindowHandler
	mu     sync.Mutex
	events []platform.SurfaceEvent
}

func (w *surfaceEvents) SurfaceEvent(ev platform.SurfaceEvent) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, ev)
	return false
}

func (w *surfaceEvents) kinds(k platform.SurfaceEventKind) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, e := range w.events {
		if e.Kind == k {
			n++
		}
	}
	return n
}

// surfaceWindow makes a window of native UI, with a surface to draw on.
func surfaceWindow(t *testing.T, b *Backend, h *fakeHost) (*window, *surfaceEvents) {
	t.Helper()
	wh := &surfaceEvents{}
	var w *window
	onMain(b, func() {
		pw, err := b.NewWindow(&platform.WindowOptions{Surface: true}, wh)
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*window)
		w.s.handle = 1
		w.width, w.height = 400, 800
	})
	return w, wh
}

// The keyboard edits the document through its client, by UTF-16 offsets, and
// gets the state after each edit in the answer.
func TestKeyboardEdits(t *testing.T) {
	b, h := startBackend(t)
	w, _ := surfaceWindow(t, b, h)
	c := &textClient{text: "hello", sel: platform.TextRange{Start: 5, End: 5}}
	onMain(b, func() {
		w.s.SetTextInput(platform.TextInputState{Active: true, Client: c,
			Traits: platform.TextTraits{Keyboard: platform.KeyboardEmail, Secure: true}})
	})
	wait.Until(t, "the host to hear of the input", func() bool { return h.called(wire.CallSetTextInput) })
	ask := func(f func(w *wire.Writer)) wire.TextStateReply {
		var reply []byte
		h.onUI(func() { reply = deliver(encodeEvent(f), true) })
		r := wire.Reader{Buf: reply}
		return wire.TextStateReply{Text: r.String(), Base: r.Int(), Length: r.Int(), SelStart: r.Int(), SelEnd: r.Int(), MarkStart: r.Int(), MarkEnd: r.Int()}
	}
	st := ask(func(wr *wire.Writer) {
		wr.Uvarint(wire.EventTextReplace)
		wr.Int(w.id)
		wr.Bool(false)
		wr.Int(0)
		wr.Int(0)
		wr.String(" big")
	})
	if c.text != "hello big" || st.Text != "hello big" || st.SelStart != 9 || st.Length != 9 || st.MarkStart != -1 {
		t.Errorf("typed: %q, state %+v", c.text, st)
	}
	st = ask(func(wr *wire.Writer) {
		wr.Uvarint(wire.EventTextMark)
		wr.Int(w.id)
		wr.Bool(false)
		wr.Int(0)
		wr.Int(0)
		wr.String("xy")
		wr.Int(2)
		wr.Int(2)
	})
	if st.MarkStart != 9 || st.MarkEnd != 11 {
		t.Errorf("composing: state %+v", st)
	}
	st = ask(func(wr *wire.Writer) {
		wr.Uvarint(wire.EventTextReplace)
		wr.Int(w.id)
		wr.Bool(false)
		wr.Int(0)
		wr.Int(0)
		wr.String("!")
	})
	if c.text != "hello big!" || st.MarkStart != -1 {
		t.Errorf("committed: %q, state %+v", c.text, st)
	}
	// A return typed is the key, not text.
	st = ask(func(wr *wire.Writer) {
		wr.Uvarint(wire.EventTextReplace)
		wr.Int(w.id)
		wr.Bool(false)
		wr.Int(0)
		wr.Int(0)
		wr.String("\n")
	})
	if c.text != "hello big!" {
		t.Errorf("return: %q", c.text)
	}
	if wh := w.h.(*surfaceEvents); wh.kinds(platform.KeyPressed) != 1 {
		t.Errorf("return: %d keys", wh.kinds(platform.KeyPressed))
	}
}

// A password's document does not show: what the keyboard deletes goes as keys.
func TestSecureDeletesByKeys(t *testing.T) {
	b, h := startBackend(t)
	w, wh := surfaceWindow(t, b, h)
	onMain(b, func() {
		w.s.SetTextInput(platform.TextInputState{Active: true, Client: &textClient{}, Traits: platform.TextTraits{Secure: true}})
	})
	h.onUI(func() {
		deliver(encodeEvent(func(wr *wire.Writer) {
			wr.Uvarint(wire.EventTextDelete)
			wr.Int(w.id)
			wr.Int(2)
			wr.Int(0)
		}), true)
	})
	if n := wh.kinds(platform.KeyPressed); n != 2 {
		t.Errorf("%d keys", n)
	}
}

// Frames stop in the background, where iOS ends apps that use the GPU, and the
// one held back draws as the app comes back.
func TestNoFramesInBackground(t *testing.T) {
	b, h := startBackend(t)
	w, wh := surfaceWindow(t, b, h)
	frame := func() {
		h.onUI(func() {
			deliver(encodeEvent(func(wr *wire.Writer) {
				wr.Uvarint(wire.EventFrame)
				wr.Int(w.id)
			}), true)
		})
		onMain(b, func() {})
	}
	bg := func(on bool) {
		h.onUI(func() {
			deliver(encodeEvent(func(wr *wire.Writer) {
				wr.Uvarint(wire.EventBackground)
				wr.Bool(on)
			}), true)
		})
	}
	frame()
	if n := wh.kinds(platform.SurfaceFrame); n != 1 {
		t.Fatalf("%d frames", n)
	}
	bg(true)
	frame()
	if n := wh.kinds(platform.SurfaceFrame); n != 1 {
		t.Fatalf("in the background: %d frames", n)
	}
	h.mu.Lock()
	h.calls = nil
	h.mu.Unlock()
	bg(false)
	wait.Until(t, "the frame to be asked for", func() bool { return h.called(wire.CallRequestFrame) })
}

// A link the app is opened with, sent before the app made its backend, reaches
// it.
func TestEarlyEvents(t *testing.T) {
	current.Store(nil)
	deliver(encodeEvent(func(w *wire.Writer) {
		w.Uvarint(wire.EventOpenURLs)
		w.Strings([]string{"myapp://first"})
	}), true)
	b := New()
	if len(b.events) != 1 {
		t.Fatalf("%d events wait", len(b.events))
	}
	if !bytes.Contains(b.events[0].msg, []byte("myapp://first")) {
		t.Errorf("event %q", b.events[0].msg)
	}
	early.events = nil
}

// A plugin's method answers through its request, or with its error.
func TestPluginCall(t *testing.T) {
	b, h := startBackend(t)
	type res struct {
		data []byte
		err  error
	}
	got := make(chan res, 2)
	call := func() {
		onMain(b, func() {
			b.PluginCall("location", "current", []byte("{}"), func(d []byte, err error) { got <- res{d, err} })
		})
	}
	call()
	wait.Until(t, "the call", func() bool { return h.called(wire.CallPluginCall) })
	h.onUI(func() {
		deliver(encodeEvent(func(w *wire.Writer) {
			w.Uvarint(wire.EventPluginResult)
			w.Int(1)
			w.String(`{"a":1}`)
			w.String("")
			w.String("")
		}), true)
	})
	r := wait.For(t, "the answer", got)
	if string(r.data) != `{"a":1}` || r.err != nil {
		t.Errorf("answer %q, %v", r.data, r.err)
	}
	call()
	h.onUI(func() {
		deliver(encodeEvent(func(w *wire.Writer) {
			w.Uvarint(wire.EventPluginResult)
			w.Int(2)
			w.String("")
			w.String("no")
			w.String("Denied")
		}), true)
	})
	r = wait.For(t, "the error", got)
	if pe, ok := r.err.(*platform.PluginError); !ok || pe.Code != "Denied" {
		t.Errorf("error %v", r.err)
	}
}

// Web views close with the window that holds them, and hear nothing of it.
func TestHeldViews(t *testing.T) {
	b, h := startBackend(t)
	w, _ := surfaceWindow(t, b, h)
	var v platform.WebView
	onMain(b, func() {
		var err error
		v, err = w.s.NewWebView(&platform.WindowOptions{}, &winHandler{})
		if err != nil {
			t.Error(err)
		}
		w.s.PlaceWebViews([]platform.WebViewPlacement{{WebView: v, Clip: platform.RectF{W: 10, H: 10}, Frame: platform.RectF{W: 10, H: 10}}})
	})
	wait.Until(t, "the host to hear where it goes", func() bool { return h.called(wire.CallPlaceViews) })
	child := v.(*window)
	onMain(b, func() { v.Close() })
	if !child.closed || len(w.children) != 0 {
		t.Errorf("closed %v, children %d", child.closed, len(w.children))
	}
}

// Four fingers open the inspector where developer tools are on, and do nothing
// in a release, whose apps may take them.
func TestFourFingers(t *testing.T) {
	b, h := startBackend(t)
	w, wh := surfaceWindow(t, b, h)
	touch := func(pointer int) {
		onMain(b, func() {
			w.s.touch(&wire.TouchEvent{ID: w.id, Pointer: pointer, Phase: "down", Kind: "touch", X: float64(pointer) * 40, Y: 100})
		})
	}
	lift := func(pointer int) {
		onMain(b, func() {
			w.s.touch(&wire.TouchEvent{ID: w.id, Pointer: pointer, Phase: "up", Kind: "touch", X: float64(pointer) * 40, Y: 100})
		})
	}
	for p := 1; p <= 4; p++ {
		touch(p)
	}
	if n := wh.kinds(platform.KeyPressed); n != 0 {
		t.Errorf("without developer tools, %d keys", n)
	}
	for p := 1; p <= 4; p++ {
		lift(p)
	}
	onMain(b, func() { w.devTools = true })
	for p := 1; p <= 4; p++ {
		touch(p)
	}
	if n := wh.kinds(platform.KeyPressed); n != 1 {
		t.Errorf("with them, %d keys", n)
	}
}
