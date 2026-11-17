package mobile

import (
	"sync"

	"github.com/egoist/mygo/internal/mobile/wire"
)

// A host is the native side of the app: the Swift host on iOS, the Kotlin
// host on Android. It runs calls on the UI thread, where the system's
// views live, and sends events. Go runs on a thread of its own, its main
// thread (the one the core calls the main thread), so that it can wait
// (Step) while the UI thread goes on.
type host interface {
	// call runs an encoded call and returns its encoded reply, nil for a
	// call not waited for. UI thread only.
	call(msg []byte) []byte
	// post asks the UI thread to call transport.drain soon. Any thread.
	post()
}

// transport carries calls from Go's threads to the UI thread, which runs
// them in order: when it next drains them (post), or at once while it
// waits for Go to answer an event, so that neither side waits for the
// other: an event handler the UI thread waits for may make calls.
type transport struct {
	h host

	mu      sync.Mutex
	pending []request
	posted  bool
	// waiting counts the events the UI thread waits in, nested; kick
	// wakes it to run calls meanwhile.
	waiting int
	kick    chan struct{}
}

type request struct {
	msg []byte
	// fn, when set, runs on the UI thread in place of a call of the host:
	// a native module's main-thread method, or its view.
	fn    func()
	reply chan []byte // nil for a call not waited for
}

func newTransport(h host) *transport {
	return &transport{h: h, kick: make(chan struct{}, 1)}
}

// message is a call of package wire.
type message interface {
	Encode(w *wire.Writer)
	Wait() bool
}

// call sends m to the UI thread, and when m is waited for, decodes its
// reply into reply. Any thread but the UI thread.
func (t *transport) call(m message, reply interface{ Decode(*wire.Reader) }) {
	var w wire.Writer
	m.Encode(&w)
	req := request{msg: w.Buf}
	if m.Wait() {
		req.reply = make(chan []byte, 1)
	}
	b := t.send(req)
	if b != nil && reply != nil {
		reply.Decode(&wire.Reader{Buf: *b})
	}
}

// run runs fn on the UI thread, in order with the calls, and when wait,
// waits for it. Any thread but the UI thread when wait.
func (t *transport) run(fn func(), wait bool) {
	req := request{fn: fn}
	if wait {
		req.reply = make(chan []byte, 1)
	}
	t.send(req)
}

// send queues req for the UI thread and waits for its reply, if it has
// one to wait for.
func (t *transport) send(req request) *[]byte {
	t.mu.Lock()
	t.pending = append(t.pending, req)
	post := false
	if t.waiting > 0 {
		select {
		case t.kick <- struct{}{}:
		default:
		}
	} else if !t.posted {
		t.posted, post = true, true
	}
	t.mu.Unlock()
	if post {
		t.h.post()
	}
	if req.reply == nil {
		return nil
	}
	b := <-req.reply
	return &b
}

// drain runs the calls sent so far. UI thread only.
func (t *transport) drain() {
	for {
		// One at a time: a call may wait for Go (wait), which runs the calls
		// after it meanwhile, one of which Go may be waiting for.
		t.mu.Lock()
		if len(t.pending) == 0 {
			t.posted = false
			t.mu.Unlock()
			return
		}
		r := t.pending[0]
		t.pending = t.pending[1:]
		t.mu.Unlock()
		var b []byte
		if r.fn != nil {
			r.fn()
		} else {
			b = t.h.call(r.msg)
		}
		if r.reply != nil {
			r.reply <- b
		}
	}
}

// wait waits for done, an answer of Go's main thread to an event of the
// UI thread, running the calls Go makes meanwhile. UI thread only.
func (t *transport) wait(done chan []byte) []byte {
	t.mu.Lock()
	t.waiting++
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.waiting--
		// A call that came as it stopped waiting only kicked it.
		post := t.waiting == 0 && len(t.pending) > 0 && !t.posted
		if post {
			t.posted = true
		}
		t.mu.Unlock()
		if post {
			t.h.post()
		}
	}()
	for {
		t.drain()
		select {
		case b := <-done:
			// Calls sent before the answer run before the UI thread goes on.
			t.drain()
			return b
		case <-t.kick:
		}
	}
}
