//go:build !js

package mygo

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo/internal/wait"
)

// cancelService is a service whose method waits for its context to be
// canceled.
type cancelService struct {
	started chan struct{}
	done    chan error
}

// Wait waits until the call is canceled.
func (s *cancelService) Wait(ctx context.Context) error {
	close(s.started)
	<-ctx.Done()
	s.done <- ctx.Err()
	return ctx.Err()
}

// A page may cancel a call: the method's context is canceled.
func TestCancelCall(t *testing.T) {
	svc := &cancelService{started: make(chan struct{}), done: make(chan error, 1)}
	Bind(svc)
	w := &Window{}
	raw := `{"id":1,"k":"tok","m":"cancelService.Wait","a":[],"cancel":true}`
	go handleCall(w, context.Background(), raw, true)
	wait.For(t, "the call to start", svc.started)
	w.cancelCall(1, "tok")
	if err := wait.For(t, "the call to be canceled", svc.done); !errors.Is(err, context.Canceled) {
		t.Errorf("the call ended with %v, want it canceled", err)
	}
}

// A page may cancel a call before its goroutine made it, as it aborts
// what it just started: the call is canceled as soon as it makes it,
// rather than running to its end under a context nobody holds.
func TestCancelCallBeforeItStarts(t *testing.T) {
	svc := &cancelService{started: make(chan struct{}), done: make(chan error, 1)}
	BindAs("cancelEarly", svc)
	w := &Window{}
	// The cancel is handled on the main thread, the call on a goroutine of
	// its own: the page that aborts at once gets here first.
	w.cancelCall(1, "tok")
	go handleCall(w, context.Background(), `{"id":1,"k":"tok","m":"cancelEarly.Wait","a":[],"cancel":true}`, true)
	if err := wait.For(t, "the call to be canceled", svc.done); !errors.Is(err, context.Canceled) {
		t.Errorf("the call ended with %v, want it canceled", err)
	}
}

// Only the page that made a call may cancel it.
func TestCancelCallOfAnotherPage(t *testing.T) {
	w := &Window{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.addCancel(1, "tok", cancel)
	// Another page's cancel does not cancel the call, and neither does
	// the cancel of another call.
	w.cancelCall(1, "other")
	w.cancelCall(2, "tok")
	select {
	case <-ctx.Done():
		t.Fatal("the call was canceled")
	default:
	}
	w.cancelCall(1, "tok")
	select {
	case <-ctx.Done():
	default:
		t.Fatal("the call was not canceled")
	}
	// A call that is over is not canceled again.
	w.addCancel(3, "tok", cancel)
	w.removeCancel(3, "tok")
	w.cancelCall(3, "tok")
}

// A page that aborts after the call is over but before the reply reaches
// it: the cancel finds the id of a call that ran, and is dropped, rather
// than waiting for the page to reload. A page that lives long would fill
// the waits of canceledEarly, and its early cancels would be forgotten.
func TestCancelCallOfACallThatRan(t *testing.T) {
	w := &Window{}
	// More cancels of calls that ran than canceledEarly holds.
	for id := int64(1); id <= maxCanceledEarly+1; id++ {
		w.addCancel(id, "tok", func() {})
		w.removeCancel(id, "tok")
		w.cancelCall(id, "tok")
	}
	if len(w.canceledEarly) != 0 {
		t.Errorf("the window waits for %d cancels of calls that ran", len(w.canceledEarly))
	}
	// An early cancel still works: the call is canceled as it makes it.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.cancelCall(maxCanceledEarly+2, "tok")
	w.addCancel(maxCanceledEarly+2, "tok", cancel)
	select {
	case <-ctx.Done():
	default:
		t.Fatal("the call was not canceled")
	}
}

// A page that navigates while a cancelable call runs: the call ends after
// the new page's bookkeeping began, and must not write to it.
func TestCancelCallPageReloaded(t *testing.T) {
	svc := &cancelService{started: make(chan struct{}), done: make(chan error, 1)}
	BindAs("cancelReload", svc)
	w := &Window{}
	w.resetPage() // the page loaded
	go handleCall(w, w.pageContext(), `{"id":1,"k":"tok","m":"cancelReload.Wait","a":[],"cancel":true}`, true)
	wait.For(t, "the call to start", svc.started)
	// The page navigates away: the call ends with the page that made it,
	// and the new page's bookkeeping holds nothing of it.
	w.resetPage()
	if err := wait.For(t, "the call to end with the page", svc.done); !errors.Is(err, context.Canceled) {
		t.Errorf("the call ended with %v, want it canceled with the page", err)
	}
	// Ending after the reload, the call finds no cancel of its own to end,
	// and a cancel of it finds nothing to cancel.
	w.removeCancel(1, "tok")
	w.cancelCall(1, "tok")
	if len(w.cancels) != 0 {
		t.Errorf("the new page holds %d cancels of the page that went away", len(w.cancels))
	}
}
