//go:build !js

package mygo

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

func watchClipboard(t *testing.T, fn func()) func() {
	t.Helper()
	off := Clipboard.OnChanged(fn)
	t.Cleanup(off)
	return off
}

func drainClipboardChanges() { onMain(func() { loop.drain() }) }

func TestClipboardChangesSharedWatchAndNoReads(t *testing.T) {
	t.Cleanup(Clipboard.Clear)
	var reads, calls atomic.Int32
	if err := Clipboard.Write(transfer.New(transfer.NewItem(transfer.Lazy(transfer.Text, func() ([]byte, error) {
		reads.Add(1)
		return []byte("private"), nil
	})))); err != nil {
		t.Fatal(err)
	}
	starts := onMainValue(func() int { return fb.ClipboardWatchStarts })
	stops := onMainValue(func() int { return fb.ClipboardWatchStops })
	notify := func() {
		if !isMainThread() {
			t.Error("callback outside main thread")
		}
		calls.Add(1)
	}
	one, two := watchClipboard(t, notify), watchClipboard(t, notify)
	drainClipboardChanges()
	if calls.Load() != 0 || reads.Load() != 0 {
		t.Fatal("subscribing read or reported existing contents")
	}
	onMain(func() {
		fb.EmitClipboardChange()
		fb.EmitClipboardChange()
	})
	drainClipboardChanges()
	if calls.Load() != 2 || reads.Load() != 0 {
		t.Fatalf("calls=%d reads=%d", calls.Load(), reads.Load())
	}
	one()
	one()
	onMain(func() { fb.EmitClipboardChange() })
	drainClipboardChanges()
	if calls.Load() != 3 {
		t.Fatalf("remaining listener: %d", calls.Load())
	}
	two()
	onMain(func() {
		if fb.ClipboardWatchStarts-starts != 1 || fb.ClipboardWatchStops-stops != 1 {
			t.Error("listeners did not share one native watch")
		}
		fb.EmitClipboardChange()
	})
	drainClipboardChanges()
	if calls.Load() != 3 || reads.Load() != 0 {
		t.Fatal("removed listener called or provider read")
	}
}

func TestClipboardChangesCanReadAndWriteWithoutReentry(t *testing.T) {
	t.Cleanup(Clipboard.Clear)
	var calls int
	inCallback := false
	watchClipboard(t, func() {
		if inCallback {
			t.Error("nested clipboard callback")
		}
		inCallback = true
		defer func() { inCallback = false }()
		calls++
		want := "first"
		if calls > 1 {
			want = "second"
		}
		b, err := Clipboard.ReadFormat(transfer.Text)
		if err != nil || string(b) != want {
			t.Errorf("read in callback: %q, %v", b, err)
		}
		if calls == 1 {
			Clipboard.WriteText("second")
			loop.drain() // a nested native modal loop must not reenter fn
		}
	})
	// In one call: the callback's nested loop runs what is posted meanwhile.
	onMain(func() {
		Clipboard.WriteText("first")
		loop.drain()
		if calls != 2 {
			t.Errorf("want two settled changes, got %d", calls)
		}
	})
}

func TestClipboardChangesWaitForNativeOperation(t *testing.T) {
	t.Cleanup(Clipboard.Clear)
	var calls int
	watchClipboard(t, func() {
		calls++
		if clipboardOperations != 0 {
			t.Error("callback ran inside a native clipboard operation")
		}
	})
	onMain(func() {
		fb.EmitClipboardChange() // queue before the native operation starts
		withClipboardOperation(func() {
			loop.drain()
			fb.EmitClipboardChange()
			loop.drain()
			if calls != 0 {
				t.Error("callback ran from the nested loop")
			}
		})
	})
	drainClipboardChanges()
	onMain(func() {
		if calls != 1 {
			t.Errorf("deferred changes = %d", calls)
		}
	})
}

func TestClipboardChangesCancelQueuedAndRestart(t *testing.T) {
	var old, current atomic.Int32
	off := watchClipboard(t, func() { old.Add(1) })
	var next func()
	onMain(func() {
		fb.EmitClipboardChange()
		off()
		next = Clipboard.OnChanged(func() { current.Add(1) })
	})
	t.Cleanup(next)
	drainClipboardChanges()
	if old.Load() != 0 || current.Load() != 0 {
		t.Fatal("a stale event crossed a watch restart")
	}
	onMain(func() { fb.EmitClipboardChange() })
	drainClipboardChanges()
	if current.Load() != 1 {
		t.Fatal("restarted watch missed the next change")
	}
}

func TestClipboardChangesCancelFromCallback(t *testing.T) {
	var first, second atomic.Int32
	var offFirst, offSecond func()
	offFirst = watchClipboard(t, func() {
		first.Add(1)
		offFirst()
		offSecond()
	})
	offSecond = watchClipboard(t, func() { second.Add(1) })
	onMain(func() { fb.EmitClipboardChange() })
	drainClipboardChanges()
	if first.Load() != 1 || second.Load() != 0 {
		t.Fatal("a listener heard a change after the last ones were removed")
	}
}

// A backend that cannot observe the clipboard leaves the listeners
// uncalled, and the next listener asks it again.
func TestClipboardChangesUnobservable(t *testing.T) {
	var calls atomic.Int32
	onMain(func() { fb.ClipboardWatchError = platform.ErrUnsupported })
	starts := onMainValue(func() int { return fb.ClipboardWatchStarts })
	off := Clipboard.OnChanged(func() { calls.Add(1) })
	t.Cleanup(off)
	onMain(func() {
		fb.EmitClipboardChange()
		if fb.ClipboardWatchStarts != starts {
			t.Error("a failed watch counts as started")
		}
		fb.ClipboardWatchError = nil
	})
	drainClipboardChanges()
	if calls.Load() != 0 {
		t.Fatal("a listener heard a clipboard nobody watches")
	}
	watchClipboard(t, func() { calls.Add(1) })
	onMain(func() { fb.EmitClipboardChange() })
	drainClipboardChanges()
	if calls.Load() != 2 {
		t.Fatalf("listeners heard %d changes once the backend watched, want 2", calls.Load())
	}
}

func TestClipboardChangesShutdown(t *testing.T) {
	var calls atomic.Int32
	off := watchClipboard(t, func() { calls.Add(1) })
	stops := onMainValue(func() int { return fb.ClipboardWatchStops })
	onMain(func() {
		fb.EmitClipboardChange()
		clipboardChanges.close() // the cleanup the app runs when it quits (App.OnQuit)
		if fb.ClipboardWatchStops-stops != 1 {
			t.Error("quitting left the native watch running")
		}
	})
	off()
	drainClipboardChanges()
	if calls.Load() != 0 {
		t.Fatal("callback survived shutdown cleanup")
	}
	onMain(func() {
		clipboardStopped = true
		defer func() { clipboardStopped = false }()
		starts := fb.ClipboardWatchStarts
		Clipboard.OnChanged(func() {})()
		if fb.ClipboardWatchStarts != starts {
			t.Error("a listener added after shutdown started a native watch")
		}
	})
}

// Watching touches nothing the clipboard holds, so a provider, which may
// not read or write the clipboard, may add a listener.
func TestClipboardChangesFromProvider(t *testing.T) {
	t.Cleanup(Clipboard.Clear)
	var calls atomic.Int32
	var off func()
	if err := Clipboard.Write(transfer.New(transfer.NewItem(transfer.Lazy(transfer.Text, func() ([]byte, error) {
		off = Clipboard.OnChanged(func() { calls.Add(1) })
		return []byte("text"), nil
	})))); err != nil {
		t.Fatal(err)
	}
	if _, err := Clipboard.ReadFormat(transfer.Text); err != nil {
		t.Fatal(err)
	}
	if off == nil {
		t.Fatal("the provider did not run")
	}
	t.Cleanup(off)
	drainClipboardChanges()
	before := calls.Load()
	onMain(func() { fb.EmitClipboardChange() })
	drainClipboardChanges()
	if calls.Load() != before+1 {
		t.Fatalf("a listener added by a provider heard %d changes, want 1", calls.Load()-before)
	}
}

// A listener added before the app is ready, as main's are before App.Run,
// is watched for once it is.
func TestClipboardChangesBeforeReady(t *testing.T) {
	var calls atomic.Int32
	starts := onMainValue(func() int { return fb.ClipboardWatchStarts })
	App.mu.Lock()
	App.ready = false
	App.mu.Unlock()
	off := Clipboard.OnChanged(func() { calls.Add(1) })
	t.Cleanup(off)
	// What the app does when it finishes launching.
	App.mu.Lock()
	App.ready = true
	pending := App.whenReady
	App.whenReady = nil
	App.mu.Unlock()
	onMain(func() {
		if fb.ClipboardWatchStarts != starts {
			t.Error("the native watch started before the app was ready")
		}
		for _, fn := range pending {
			fn()
		}
		if fb.ClipboardWatchStarts-starts != 1 {
			t.Errorf("the app becoming ready started %d native watches, want 1", fb.ClipboardWatchStarts-starts)
		}
		fb.EmitClipboardChange()
	})
	drainClipboardChanges()
	if calls.Load() != 1 {
		t.Fatalf("a listener added before the app was ready heard %d changes, want 1", calls.Load())
	}
}
