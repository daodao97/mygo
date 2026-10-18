package backgroundtask

import (
	"context"
	"errors"
	"github.com/egoist/mygo/internal/wait"
	"testing"
	"time"
)

func TestExpirationBeforeRunAndReuse(t *testing.T) {
	calls := 0
	w := New(time.Hour, func(context.Context) error { calls++; return nil })
	if !errors.Is(w.Run(t.Context()), ErrUnprepared) {
		t.Fatal("unprepared accepted")
	}
	_ = w.Prepare()
	w.Expire()
	if !errors.Is(w.Run(t.Context()), context.Canceled) || calls != 0 {
		t.Fatal("expired work ran")
	}
	_ = w.Prepare()
	if err := w.Run(t.Context()); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestExpirationDuringRunAndNoOverlap(t *testing.T) {
	entered := make(chan struct{})
	done := make(chan error, 1)
	w := New(time.Hour, func(ctx context.Context) error { close(entered); <-ctx.Done(); return nil })
	_ = w.Prepare()
	go func() { done <- w.Run(t.Context()) }()
	wait.For(t, "worker starts", entered)
	if !errors.Is(w.Prepare(), ErrRunning) || !errors.Is(w.Run(t.Context()), ErrRunning) {
		t.Fatal("overlap accepted")
	}
	w.Expire()
	if err := wait.For(t, "worker canceled", done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
