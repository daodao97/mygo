//go:build !js

package e2e

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
)

// A page the web view kept as it was left, and shows again on Back
// without loading it, runs no script again. It hears events as before,
// and the call it left unanswered fails, as Go canceled it.
func TestPageShownAgainFromCache(t *testing.T) {
	w := newWindow(t, mygo.WindowOptions{Title: "Page cache", Width: 400, Height: 300})
	p := w.Page()
	var ready atomic.Int32
	p.OnDOMReady(func() { ready.Add(1) })
	if err := p.LoadURL("app://localhost/"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, "window.run")
	mustEval(t, w, `(() => {
		addEventListener("pageshow", (e) => { window.shownAgain = e.persisted; });
		window.left = mygo.call("Streams.Forever", mygo.channel()).then(() => "returned", (e) => "failed: " + e.message);
	})()`)

	if err := p.LoadURL("app://localhost/capture"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, `location.pathname === "/capture" && document.readyState === "complete"`)
	wait.For(t, "the call of the page left to be canceled", streams.stopped)
	wait.Until(t, "the DOM of the second page ready", func() bool { return ready.Load() == 2 })

	p.GoBack()
	// A page loaded again has neither the listener nor window.left: only
	// the page kept hears pageshow.
	waitFor(t, w, `location.pathname === "/" && document.readyState === "complete" && (window.shownAgain !== undefined || window.left === undefined)`)
	if mustEval(t, w, "window.shownAgain === true") != true {
		t.Skip("the engine loaded the page again instead of keeping it")
	}
	wait.Until(t, "the DOM of the page shown again ready", func() bool { return ready.Load() == 3 })
	if err := ticked.Emit(w, Tick{N: 9}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, "results.includes('tick9')")
	if got := mustEval(t, w, "window.left"); got != "failed: the page was left before the call returned" {
		t.Errorf("the call the page left unanswered: %v", got)
	}
	if got := mustEval(t, w, `mygo.call("Greeter.Greet", "again")`); got != "Hello, again!" {
		t.Errorf("a call of the page shown again = %v", got)
	}
}

// Evals that run in a page as the navigation away from it commits answer,
// although the web view keeps the page frozen: WebKit runs what they
// queued to answer before it freezes the page (the bridge's pagehide
// listener), not when Back shows it again. Before, the poll of waitFor
// that ran then in TestPageShownAgainFromCache waited for good.
func TestEvalAsPageIsCached(t *testing.T) {
	w := newWindow(t, mygo.WindowOptions{Title: "Eval as the page is cached", Width: 400, Height: 300})
	p := w.Page()
	if err := p.LoadURL("app://localhost/"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, "window.run")
	// The page streams from Go as it is left, as in
	// TestPageShownAgainFromCache, whose waitFor met this.
	mustEval(t, w, `(() => {
		addEventListener("pageshow", (e) => { window.shownAgain = e.persisted; });
		mygo.call("Streams.Forever", mygo.channel()).catch(() => {});
	})()`)
	var committed atomic.Bool
	off := p.OnDidNavigate(func(string) { committed.Store(true) })
	defer off()
	if err := p.LoadURL("app://localhost/capture"); err != nil {
		t.Fatal(err)
	}
	// Evals, one after the other, until Go hears the navigation commit and
	// a few more: some run in the page left, the last ones as it is left.
	ctx := wait.Context(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed []string
	for i, after := 0, 0; after < 5; i++ {
		if committed.Load() {
			after++
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			// An engine may fail an eval of a page it destroys: it must
			// not wait for good.
			if _, err := p.EvalContext(ctx, "location.pathname"); errors.Is(err, context.DeadlineExceeded) {
				mu.Lock()
				failed = append(failed, fmt.Sprintf("eval %d never answered", i))
				mu.Unlock()
			}
		}()
		time.Sleep(time.Millisecond) // spreads them over the navigation
	}
	wg.Wait()
	for _, f := range failed {
		t.Error(f)
	}
	wait.For(t, "the call of the page left to be canceled", streams.stopped)
	waitFor(t, w, `location.pathname === "/capture" && document.readyState === "complete"`)
	p.GoBack()
	waitFor(t, w, `location.pathname === "/" && document.readyState === "complete"`)
	if mustEval(t, w, "window.shownAgain === true") != true {
		t.Skip("the engine loaded the page again instead of keeping it")
	}
}
