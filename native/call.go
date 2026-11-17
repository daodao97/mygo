package native

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/nativehost"
	"github.com/egoist/mygo/internal/texture"
)

// The status of a result, in the block that carries it both ways.
const (
	statusOK      = 0
	statusError   = 1
	statusPending = 2 // an asynchronous call or a stream started
	statusEnd     = 3 // a stream finished
)

// What the platform tells Go through the one callback every module shares.
const (
	kindResult      = 1 // the result of an asynchronous call: id is the call
	kindItem        = 2 // a value of a stream, its error or its end
	kindNotify      = 3 // a callback without results: id is the function
	kindAsk         = 4 // a callback with results, answered at once
	kindRelease     = 5 // the platform let go of a function or an object of Go's
	kindInvoke      = 6 // a method of an object Go implements, answered at once
	kindInvokeAsync = 7 // one taking a context, answered with resume
	kindCancel      = 8 // the platform canceled what kindInvokeAsync started
	kindPresent     = 9 // a frame for a texture: id is the texture, status the IOSurfaceRef
)

type reply struct {
	status int64
	data   []byte
	// block is the platform's memory data is in, freed with free.
	block uintptr
}

func (r reply) free() {
	if r.block != 0 {
		freeBlock(r.block)
	}
}

// pending holds the calls and streams waiting for the platform.
var pending sync.Map // uint64 → chan reply

var errNotMain = errors.New("native: the method runs on the main thread, which is not running the app's event loop: call it once the app runs (App.Run), or from the main goroutine")

// onMain runs fn on the main thread when flags say so.
func onMain(flags Flags, fn func()) error {
	if flags&MainThread == 0 {
		fn()
		return nil
	}
	if ui := nativehost.OnUI; ui != nil {
		// On phones Go's main thread is its own, and the system's views and
		// Swift's main actor live on the UI thread.
		ui(fn)
		return nil
	}
	if mygo.App.IsReady() {
		ran := false
		mygo.RunOnMain(func() { fn(); ran = true })
		if !ran {
			return errors.New("native: the app has quit")
		}
		return nil
	}
	if isMainThread() {
		fn()
		return nil
	}
	return errNotMain
}

// call calls a function of the implementation that returns at once, with
// the arguments args, which it releases.
func (m *Module) call(ctx context.Context, h uint64, sym string, flags Flags, args *Encoder) (*Decoder, error) {
	lib, err := m.load()
	if err != nil {
		args.release()
		return nil, err
	}
	var r reply
	if flags&MainThread == 0 {
		r = lib.invoke(m, sym, h, args.bytes(), 0)
	} else {
		err = onMain(flags, func() { r = lib.invoke(m, sym, h, args.bytes(), 0) })
	}
	args.release()
	if err != nil {
		return nil, err
	}
	return m.result(ctx, sym, r)
}

// callNone calls a function that returns nothing.
func (m *Module) callNone(h uint64, sym string, flags Flags) error {
	d, err := m.call(context.Background(), h, sym, flags, nil)
	if err != nil {
		return err
	}
	return d.Close()
}

// direct calls the C function of a method of numbers.
func (m *Module) direct(h uint64, sym string, args []uintptr) uintptr {
	lib, err := m.load()
	if err != nil {
		return 0
	}
	return lib.directSym(m, sym, h, args)
}

func (m *Module) result(ctx context.Context, sym string, r reply) (*Decoder, error) {
	switch r.status {
	case statusOK:
		return newDecoder(r.data, r.block), nil
	case statusError:
		err := m.decodeError(ctx, r.data)
		r.free()
		return nil, err
	}
	r.free()
	return nil, fmt.Errorf("native: %s answered with status %d", sym, r.status)
}

// callAsync calls an asynchronous function of the implementation: it
// starts a task, which ctx cancels, and reports through the callback.
// Main-thread methods start their task from any thread: it runs on the
// main actor.
func (m *Module) callAsync(ctx context.Context, h uint64, sym string, args *Encoder) (*Decoder, error) {
	lib, err := m.load()
	if err != nil {
		args.release()
		return nil, err
	}
	id := nextID.Add(1)
	ch := make(chan reply, 1)
	pending.Store(id, ch)
	defer pending.Delete(id)
	r := lib.invoke(m, sym, h, args.bytes(), id)
	args.release()
	if r.status != statusPending {
		return m.result(ctx, sym, r)
	}
	r.free()
	select {
	case r = <-ch:
	case <-ctx.Done():
		lib.cancel(m, id)
		return nil, ctx.Err()
	}
	return m.result(ctx, sym, r)
}

type stream struct {
	m   *Module
	lib *library
	sym string
	id  uint64
	ch  chan reply
}

func (m *Module) startStream(h uint64, sym string, flags Flags, args *Encoder) (*stream, error) {
	lib, err := m.load()
	if err != nil {
		args.release()
		return nil, err
	}
	s := &stream{m: m, lib: lib, sym: sym, id: nextID.Add(1), ch: make(chan reply, 1)}
	pending.Store(s.id, s.ch)
	var r reply
	err = onMain(flags, func() { r = lib.invoke(m, sym, h, args.bytes(), s.id) })
	args.release()
	if err != nil {
		pending.Delete(s.id)
		return nil, err
	}
	if r.status != statusPending {
		pending.Delete(s.id)
		d, err := m.result(context.Background(), sym, r)
		if err != nil {
			return nil, err
		}
		d.Close()
		return nil, fmt.Errorf("native: %s did not start its stream", sym)
	}
	r.free()
	return s, nil
}

// next asks for the stream's next value; done reports its end.
func (s *stream) next(ctx context.Context) (data []byte, err error, done bool) {
	s.lib.next(s.m, s.id)
	select {
	case r := <-s.ch:
		switch r.status {
		case statusOK:
			return r.data, nil, false
		case statusEnd:
			return nil, nil, true
		default:
			return nil, s.m.decodeError(ctx, r.data), false
		}
	case <-ctx.Done():
		return nil, ctx.Err(), false
	}
}

// stop cancels the stream's task, which runs its onTermination.
func (s *stream) stop() {
	s.lib.cancel(s.m, s.id)
	pending.Delete(s.id)
}

// The notifications of callbacks run in order, on the main thread when the
// app runs.
var notes struct {
	mu      sync.Mutex
	queue   []func()
	running bool
}

func notify(fn func()) {
	notes.mu.Lock()
	notes.queue = append(notes.queue, fn)
	if notes.running {
		notes.mu.Unlock()
		return
	}
	notes.running = true
	notes.mu.Unlock()
	go func() {
		for {
			notes.mu.Lock()
			if len(notes.queue) == 0 {
				notes.running = false
				notes.mu.Unlock()
				return
			}
			fn := notes.queue[0]
			notes.queue[0] = nil
			notes.queue = notes.queue[1:]
			notes.mu.Unlock()
			if mygo.App.IsReady() {
				mygo.RunOnMain(fn)
			} else {
				fn()
			}
		}
	}()
}

// asyncs are the calls of Go's methods taking a context that the platform
// is waiting for, by its continuation. A continuation's id is unique in
// the process: its high bits are its module's index (Module.index), which
// Go gives each module as it loads it.
var asyncs sync.Map // uint64 → context.CancelFunc

// contBits is where a continuation's module index starts.
const contBits = 48

// modules are the modules loaded, by index.
var modules sync.Map // uint64 → *Module

// moduleOf returns the module of a continuation, or nil.
func moduleOf(cont uint64) *Module {
	m, ok := modules.Load(cont >> contBits)
	if !ok {
		return nil
	}
	return m.(*Module)
}

// handle answers the platform; the encoder it returns, if any, holds the
// answer of a question.
func handle(kind, id uint64, status int64, data []byte) (int64, *Encoder, bool) {
	switch kind {
	case kindResult, kindItem:
		if ch, ok := pending.Load(id); ok {
			select {
			case ch.(chan reply) <- reply{status: status, data: data}:
			default:
			}
		}
	case kindNotify:
		v := lookupGo(id)
		if v == nil || v.dead.Load() {
			return 0, nil, false
		}
		d := &Decoder{b: data}
		notify(func() {
			if !v.dead.Load() {
				_ = v.fn(d, nil)
			}
		})
	case kindAsk:
		v := lookupGo(id)
		if v == nil || v.dead.Load() {
			return answer(nil, ErrClosed)
		}
		e := NewEncoder()
		return answer(e, v.fn(&Decoder{b: data}, e))
	case kindRelease:
		goValues.Delete(id)
	case kindInvoke:
		v := lookupGo(id)
		if v == nil {
			return answer(nil, ErrClosed)
		}
		d := &Decoder{b: data}
		method := int(d.Int())
		e := NewEncoder()
		return answer(e, v.dispatch(context.Background(), method, d, e))
	case kindInvokeAsync:
		// status is the continuation, which waits for an answer even
		// when there is no method to call.
		cont := uint64(status)
		m := moduleOf(cont)
		v := lookupGo(id)
		if v == nil {
			if m != nil {
				go resume(m, cont, statusError, errorBlock(ErrClosed))
			}
			return 0, nil, false
		}
		d := &Decoder{b: data}
		method := int(d.Int())
		ctx, cancel := context.WithCancel(context.Background())
		asyncs.Store(cont, cancel)
		go func() {
			defer cancel()
			e := NewEncoder()
			st, out, _ := answer(e, v.dispatch(ctx, method, d, e))
			asyncs.Delete(cont)
			resume(v.m, cont, st, out.b)
			out.release()
		}()
	case kindPresent:
		// The surface comes retained, for the texture to keep.
		if t := texture.Lookup(id); t != nil {
			t.Present(uintptr(status))
		} else {
			releaseSurface(uintptr(status))
		}
	case kindCancel:
		if c, ok := asyncs.Load(id); ok {
			c.(context.CancelFunc)()
		}
	}
	return 0, nil, false
}

// resume answers the continuation cont of module m.
func resume(m *Module, cont uint64, status int64, data []byte) {
	if lib, err := m.load(); err == nil {
		lib.resume(m, cont, status, data)
	}
}

// answer is the answer of a question: what e holds, or err.
func answer(e *Encoder, err error) (int64, *Encoder, bool) {
	if err != nil {
		if e == nil {
			e = NewEncoder()
		}
		e.b = e.b[:0]
		encodeError(e, err)
		return statusError, e, true
	}
	return statusOK, e, true
}

// Flush waits until the notifications of callbacks the platform has made
// so far have run, for tools such as mygo native call that exit next.
func Flush() {
	for {
		notes.mu.Lock()
		idle := !notes.running && len(notes.queue) == 0
		notes.mu.Unlock()
		if idle {
			return
		}
		runtime.Gosched()
	}
}
