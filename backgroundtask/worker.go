// Package backgroundtask coordinates cancellable work with native background
// expiration. Scheduling remains the operating system's decision.
package backgroundtask

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrRunning = errors.New("backgroundtask: work already running")
var ErrUnprepared = errors.New("backgroundtask: call Prepare before Run")

type Worker struct {
	mu                         sync.Mutex
	timeout                    time.Duration
	work                       func(context.Context) error
	generation                 uint64
	prepared, expired, running bool
	cancel                     context.CancelFunc
}

func New(timeout time.Duration, work func(context.Context) error) *Worker {
	if timeout <= 0 || work == nil {
		panic("backgroundtask: invalid worker")
	}
	return &Worker{timeout: timeout, work: work}
}

// Prepare must precede each native launch and expiration-handler installation.
// Overlapping launches are rejected until the prior Run returns.
func (w *Worker) Prepare() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return ErrRunning
	}
	w.generation++
	w.prepared = true
	w.expired = false
	return nil
}
func (w *Worker) Expire() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.expired = true
	if w.cancel != nil {
		w.cancel()
	}
}
func (w *Worker) Run(parent context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return ErrRunning
	}
	if !w.prepared {
		w.mu.Unlock()
		return ErrUnprepared
	}
	w.prepared = false
	w.running = true
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	w.cancel = cancel
	if w.expired {
		cancel()
	}
	w.mu.Unlock()
	defer func() { cancel(); w.mu.Lock(); w.running = false; w.cancel = nil; w.mu.Unlock() }()
	if e := ctx.Err(); e != nil {
		return e
	}
	e := w.work(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return e
}
