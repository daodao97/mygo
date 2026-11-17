package mobile

import (
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/wait"
)

// rawCall is a call by its first byte.
type rawCall struct {
	id    byte
	reply bool
}

func (c rawCall) Encode(w *wire.Writer) { w.Buf = append(w.Buf, c.id) }
func (c rawCall) Wait() bool            { return c.reply }

// loopHost hears the calls the test's UI thread runs.
type loopHost struct {
	onCall func(id byte)
}

func (h *loopHost) call(msg []byte) []byte {
	h.onCall(msg[0])
	return nil
}

func (h *loopHost) post() {}

// A call that waits for Go runs the calls after it meanwhile, among which may
// be the one Go waits for before it answers.
func TestWaitRunsCallsBehindIt(t *testing.T) {
	var tr *transport
	done := make(chan []byte, 1)
	h := &loopHost{}
	h.onCall = func(id byte) {
		if id == 1 {
			tr.wait(done) // as closing a window asks Go something
		}
	}
	tr = newTransport(h)
	var wg sync.WaitGroup
	wg.Add(1)
	tr.call(rawCall{id: 1}, nil)
	queued := make(chan struct{})
	go func() {
		defer wg.Done()
		close(queued)
		tr.call(rawCall{id: 2, reply: true}, nil)
		done <- nil
	}()
	<-queued
	// The UI thread: both calls may be queued by now, in one batch or not.
	ran := make(chan struct{})
	go func() {
		tr.drain()
		close(ran)
	}()
	wait.For(t, "the calls to run", ran)
	wg.Wait()
}

// Once the core's loop is gone, the host's events are answered at once: no one
// is left to, and the UI thread would hang.
func TestEventsAfterQuit(t *testing.T) {
	b, h := startBackend(t)
	var held []byte
	stopped := make(chan struct{})
	h.onUI(func() {
		b.Quit()
		wait.Until(t, "the loop to stop", func() bool {
			b.mu.Lock()
			defer b.mu.Unlock()
			return b.stopped
		})
		held = deliver(encodeEvent(func(w *wire.Writer) { w.Uvarint(wire.EventBackground) }), true)
		close(stopped)
	})
	wait.For(t, "the event to be answered", stopped)
	if held != nil {
		t.Errorf("answered %v", held)
	}
}

// What a keyboard composes counts UTF-16 units, its caret in runes on our side.
func TestRunesInUTF16(t *testing.T) {
	for _, c := range []struct {
		s     string
		units int
		want  int
	}{{"abc", 2, 2}, {"a😀b", 3, 2}, {"a😀b", 4, 3}, {"中文", 1, 1}, {"", 3, 0}} {
		if got := platform.RunesInUTF16(c.s, c.units); got != c.want {
			t.Errorf("%q, %d: %d, want %d", c.s, c.units, got, c.want)
		}
	}
}

// The phone's zone is time.Local, and what Go doesn't know of leaves it.
func TestTimeZone(t *testing.T) {
	defer func(l *time.Location) { time.Local = l }(time.Local)
	setTimeZone("Asia/Tokyo")
	if name, off := time.Now().Zone(); name != "JST" || off != 9*3600 {
		t.Errorf("in Tokyo: %s %d", name, off)
	}
	setTimeZone("Nowhere/Land")
	setTimeZone("")
	if time.Local.String() != "Asia/Tokyo" {
		t.Errorf("after an unknown zone: %v", time.Local)
	}
}
