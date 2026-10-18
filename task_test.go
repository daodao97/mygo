//go:build !js

package mygo

import (
	"context"
	"errors"
	"github.com/egoist/mygo/async"
	"github.com/egoist/mygo/internal/wait"
	"testing"
)

func TestTaskUIAndLifecycle(t *testing.T) {
	old := App.Lifecycle()
	defer onMain(func() { App.mu.Lock(); App.lifecycle = old; App.mu.Unlock() })
	onMain(func() { App.setLifecycle(LifecycleActive) })
	events := make(chan async.State[int], 16)
	task := NewTask(func(s async.State[int]) {
		if !isMainThread() {
			t.Error("callback off UI thread")
		}
		events <- s
	})
	defer task.Close()
	entered := make(chan struct{})
	exited := make(chan struct{})
	task.Start(t.Context(), func(ctx context.Context, publish func(int)) error {
		close(entered)
		<-ctx.Done()
		close(exited)
		publish(99)
		return ctx.Err()
	})
	wait.For(t, "task starts", entered)
	onMain(func() { App.setLifecycle(LifecycleBackground) })
	wait.For(t, "background cancels", exited)
	if task.Start(t.Context(), func(context.Context, func(int)) error { return nil }) {
		t.Fatal("background start accepted")
	}
	onMain(func() { App.setLifecycle(LifecycleActive) })
	task.Start(t.Context(), func(_ context.Context, publish func(int)) error { publish(7); return nil })
	for {
		s := wait.For(t, "UI notification", events)
		if s.Value == 99 {
			t.Fatal("old result delivered")
		}
		if s.Phase == async.Succeeded {
			if s.Value != 7 {
				t.Fatal(s)
			}
			break
		}
	}
}
func TestNetworkPathUnsupported(t *testing.T) {
	if _, err := Network.PathStatus(); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	off, err := Network.OnPathChanged(func(NetworkPathStatus) { t.Error("unsupported callback") })
	if off != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatal("unexpected subscription", off != nil, err)
	}
}
