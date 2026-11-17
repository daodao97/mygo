package background

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo/internal/wait"
)

type result struct {
	run int64
	ok  bool
}

func handlers(t *testing.T, m map[string]func(context.Context) error) {
	t.Helper()
	tasks.Lock()
	defer tasks.Unlock()
	tasks.handlers, tasks.running = m, map[int64]context.CancelFunc{}
}

// A task runs its handler and says what came of it; one the app no longer has
// finishes, so that the system stops asking; a panic stays in its task.
func TestRun(t *testing.T) {
	handlers(t, map[string]func(context.Context) error{
		"fine":   func(context.Context) error { return nil },
		"fails":  func(context.Context) error { return errors.New("no network") },
		"panics": func(context.Context) error { panic("oh") },
	})
	done := make(chan result, 1)
	report := func(run int64, ok bool) { done <- result{run, ok} }
	for i, c := range []struct {
		name string
		ok   bool
	}{{"fine", true}, {"fails", false}, {"panics", false}, {"gone", true}} {
		run(runEvent{Run: int64(i), Name: c.name}, report)
		if got := wait.For(t, c.name, done); got != (result{int64(i), c.ok}) {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}

// The system's time running out ends the handler's context.
func TestStop(t *testing.T) {
	started := make(chan struct{})
	handlers(t, map[string]func(context.Context) error{
		"long": func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	})
	done := make(chan result, 1)
	run(runEvent{Run: 7, Name: "long"}, func(run int64, ok bool) { done <- result{run, ok} })
	wait.For(t, "it to start", started)
	stop(7)
	if got := wait.For(t, "it to end", done); got != (result{7, false}) {
		t.Errorf("stopped: %+v", got)
	}
}
