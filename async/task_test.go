package async

import (
	"context"
	"errors"
	"github.com/egoist/mygo/internal/wait"
	"testing"
	"time"
)

func TestReplacementDropsQueuedAndLateResults(t *testing.T) {
	q := make(chan func(), 32)
	var seen []State[int]
	task := New(func(f func()) { q <- f }, func(s State[int]) { seen = append(seen, s) })
	defer task.Close()
	published := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	task.Start(t.Context(), func(ctx context.Context, publish func(int)) error {
		publish(1)
		close(published)
		<-release
		publish(999)
		close(done)
		return errors.New("old")
	})
	wait.For(t, "old progress queued", published)
	task.Start(t.Context(), func(ctx context.Context, publish func(int)) error { publish(2); return nil })
	close(release)
	wait.For(t, "old work exits", done)
	wait.Until(t, "replacement completes", func() bool { return task.Snapshot().Phase == Succeeded })
	for len(q) > 0 {
		(<-q)()
	}
	if len(seen) == 0 || seen[len(seen)-1].Value != 2 {
		t.Fatal(seen)
	}
	for _, s := range seen {
		if s.Generation != 2 || s.Value == 999 || s.Err != nil {
			t.Fatal(s)
		}
	}
}
func TestPauseCloseAndError(t *testing.T) {
	q := make(chan func(), 32)
	cleanups := 0
	var seen []State[int]
	task := New(func(f func()) { q <- f }, func(s State[int]) { seen = append(seen, s) })
	task.SetCleanup(func() { cleanups++ })
	entered := make(chan struct{})
	exited := make(chan struct{})
	task.Start(t.Context(), func(ctx context.Context, p func(int)) error {
		close(entered)
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	})
	wait.For(t, "work enters", entered)
	task.Pause()
	wait.For(t, "work canceled", exited)
	if task.Start(t.Context(), func(context.Context, func(int)) error { panic("paused") }) {
		t.Fatal("paused start")
	}
	for len(q) > 0 {
		(<-q)()
	}
	if len(seen) != 1 || seen[0].Phase != Canceled {
		t.Fatal(seen)
	}
	task.Resume()
	sentinel := errors.New("failure")
	task.Start(t.Context(), func(context.Context, func(int)) error { return sentinel })
	wait.Until(t, "failure", func() bool { return task.Snapshot().Phase == Failed })
	if !errors.Is(task.Snapshot().Err, sentinel) {
		t.Fatal(task.Snapshot())
	}
	n := len(seen)
	task.Close()
	task.Close()
	for len(q) > 0 {
		(<-q)()
	}
	if len(seen) != n || cleanups != 1 {
		t.Fatal("close delivered/cleanup", seen, cleanups)
	}
}
func TestPollerVisibilityAndForeground(t *testing.T) {
	q := make(chan func(), 64)
	task := New[int](func(f func()) { q <- f }, nil)
	entered := make(chan context.Context, 8)
	exited := make(chan struct{}, 8)
	p := NewPoller(task, time.Hour, func(ctx context.Context, _ func(int)) error {
		entered <- ctx
		<-ctx.Done()
		exited <- struct{}{}
		return ctx.Err()
	})
	defer p.Close()
	p.Tick()
	if task.Snapshot().Phase != Idle {
		t.Fatal("hidden ran")
	}
	p.SetVisible(true)
	ctx := wait.For(t, "visible work", entered)
	p.Tick()
	if len(entered) != 0 {
		t.Fatal("overlap")
	}
	p.SetForeground(false)
	wait.For(t, "background cancels", exited)
	if ctx.Err() == nil {
		t.Fatal("not canceled")
	}
	p.Tick()
	if len(entered) != 0 {
		t.Fatal("background ran")
	}
	p.SetForeground(true)
	wait.For(t, "resume starts", entered)
	p.SetVisible(false)
	wait.For(t, "hidden cancels", exited)
	p.Close()
	p.SetVisible(true)
	p.Tick()
	if len(entered) != 0 {
		t.Fatal("closed ran")
	}
}
