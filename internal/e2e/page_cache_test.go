//go:build !js

package e2e

import (
	"sync/atomic"
	"testing"

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
