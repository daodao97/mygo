// Package async manages latest-wins work independently of a UI backend.
package async

import (
	"context"
	"sync"
	"time"
)

type Phase string

const (
	Idle      Phase = "idle"
	Running   Phase = "running"
	Succeeded Phase = "succeeded"
	Failed    Phase = "failed"
	Canceled  Phase = "canceled"
)

type State[T any] struct {
	Phase      Phase
	Value      T
	HasValue   bool
	Err        error
	Generation uint64
	sequence   uint64
}
type Work[T any] func(context.Context, func(T)) error

// Task dispatches all notifications through Dispatch. Dispatch must serialize
// callbacks on a queue, never execute them inline (e.g. a UI queue). Values must be immutable after Publish. Replacing
// or canceling work suppresses even results already queued for delivery.
type Task[T any] struct {
	mu             sync.Mutex
	dispatch       func(func())
	notify         func(State[T])
	cancel         context.CancelFunc
	state          State[T]
	paused, closed bool
	cleanup        func()
	delivered      uint64
}

func New[T any](dispatch func(func()), notify func(State[T])) *Task[T] {
	if dispatch == nil {
		panic("async: missing dispatcher")
	}
	return &Task[T]{dispatch: dispatch, notify: notify, state: State[T]{Phase: Idle}}
}
func (t *Task[T]) Snapshot() State[T] { t.mu.Lock(); defer t.mu.Unlock(); return t.state }
func (t *Task[T]) deliver(state State[T]) {
	t.dispatch(func() {
		t.mu.Lock()
		valid := !t.closed && state.Generation == t.state.Generation && state.sequence > t.delivered
		if valid {
			t.delivered = state.sequence
		}
		fn := t.notify
		t.mu.Unlock()
		if valid && fn != nil {
			fn(state)
		}
	})
}
func (t *Task[T]) Start(parent context.Context, work Work[T]) bool {
	if parent == nil || work == nil {
		panic("async: missing context/work")
	}
	t.mu.Lock()
	if t.closed || t.paused {
		t.mu.Unlock()
		return false
	}
	if t.cancel != nil {
		t.cancel()
	}
	ctx, cancel := context.WithCancel(parent)
	t.cancel = cancel
	t.delivered = 0
	t.state = State[T]{Phase: Running, Generation: t.state.Generation + 1, sequence: 1}
	initial := t.state
	t.mu.Unlock()
	t.deliver(initial)
	go func() {
		defer cancel()
		publish := func(value T) {
			t.mu.Lock()
			if t.closed || ctx.Err() != nil || t.state.Generation != initial.Generation || t.state.Phase != Running {
				t.mu.Unlock()
				return
			}
			t.state.sequence++
			t.state.Value = value
			t.state.HasValue = true
			state := t.state
			t.mu.Unlock()
			t.deliver(state)
		}
		err := ctx.Err()
		if err == nil {
			err = work(ctx, publish)
		}
		t.mu.Lock()
		if t.closed || t.state.Generation != initial.Generation || t.state.Phase != Running {
			t.mu.Unlock()
			return
		}
		if ctx.Err() != nil {
			t.state.Phase = Canceled
			t.state.Err = ctx.Err()
		} else if err != nil {
			t.state.Phase = Failed
			t.state.Err = err
		} else {
			t.state.Phase = Succeeded
		}
		t.cancel = nil
		t.state.sequence++
		state := t.state
		t.mu.Unlock()
		t.deliver(state)
	}()
	return true
}
func (t *Task[T]) cancelLocked() {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.state.Generation++
	t.state.sequence = 1
	t.delivered = 0
	t.state.Phase = Canceled
	t.state.Err = context.Canceled
}
func (t *Task[T]) Cancel() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.cancelLocked()
	s := t.state
	t.mu.Unlock()
	t.deliver(s)
}
func (t *Task[T]) Pause() {
	t.mu.Lock()
	if t.closed || t.paused {
		t.mu.Unlock()
		return
	}
	t.paused = true
	t.cancelLocked()
	s := t.state
	t.mu.Unlock()
	t.deliver(s)
}
func (t *Task[T]) Resume() { t.mu.Lock(); t.paused = false; t.mu.Unlock() }

// SetCleanup installs a lifecycle unsubscribe, invoked exactly once by Close.
func (t *Task[T]) SetCleanup(fn func()) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		if fn != nil {
			fn()
		}
		return
	}
	old := t.cleanup
	t.cleanup = fn
	t.mu.Unlock()
	if old != nil {
		old()
	}
}
func (t *Task[T]) Close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.cancelLocked()
	t.closed = true
	fn := t.cleanup
	t.cleanup = nil
	t.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Poller combines view visibility and foreground state. Inactive pollers have
// no ticker and cancel work. Tick is also an explicit refresh/test entry point.
// Each activation starts an immediate tick; overlapping work is skipped.
type Poller[T any] struct {
	mu                          sync.Mutex
	task                        *Task[T]
	work                        Work[T]
	interval                    time.Duration
	visible, foreground, closed bool
	cancel                      context.CancelFunc
}

func NewPoller[T any](task *Task[T], interval time.Duration, work Work[T]) *Poller[T] {
	if task == nil || work == nil || interval <= 0 {
		panic("async: invalid poller")
	}
	return &Poller[T]{task: task, work: work, interval: interval, foreground: true}
}
func (p *Poller[T]) update() {
	active := p.visible && p.foreground && !p.closed
	if !active {
		if p.cancel != nil {
			p.cancel()
			p.cancel = nil
		}
		p.task.Pause()
		return
	}
	if p.cancel != nil {
		return
	}
	p.task.Resume()
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go func() {
		p.Tick()
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.Tick()
			}
		}
	}()
}
func (p *Poller[T]) SetVisible(v bool)    { p.mu.Lock(); p.visible = v; p.update(); p.mu.Unlock() }
func (p *Poller[T]) SetForeground(v bool) { p.mu.Lock(); p.foreground = v; p.update(); p.mu.Unlock() }
func (p *Poller[T]) Tick() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !p.visible || !p.foreground || p.task.Snapshot().Phase == Running {
		return
	}
	p.task.Start(context.Background(), p.work)
}
func (p *Poller[T]) Close() { p.mu.Lock(); p.closed = true; p.update(); p.mu.Unlock(); p.task.Close() }
