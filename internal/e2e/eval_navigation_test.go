//go:build !js

package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
)

// pending is an Eval that never settles by itself. The page keeps its
// promise: WebKit answers for a promise nothing holds once the garbage
// collector takes it ("Completion handler for function call is no longer
// reachable"), and a page left for the back-forward cache is kept whole.
const pending = "window.started = true; await (window.pending = new Promise(() => {}))"

// TestEvalAcrossNavigation checks that an Eval pending in a page the
// window leaves returns an error rather than waiting for the window to
// close. To another site, WebKit swaps the web process.
func TestEvalAcrossNavigation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(sessionPage))
	t.Cleanup(server.Close)
	for _, c := range []struct {
		name, first string
		leave       func(*testing.T, *mygo.Window)
	}{
		{"LoadHTML", "app://localhost/session/one", func(t *testing.T, w *mygo.Window) {
			w.Page().LoadHTML("<title>two</title>", "")
		}},
		{"location", "app://localhost/session/one", func(t *testing.T, w *mygo.Window) {
			mustEval(t, w, "location.href = '/session/two'")
		}},
		{"another site", "app://localhost/session/one", func(t *testing.T, w *mygo.Window) {
			if err := w.Page().LoadURL(server.URL + "/session/two"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWindow(t, mygo.WindowOptions{Hidden: true})
			if err := w.Page().LoadURL(c.first); err != nil {
				t.Fatal(err)
			}
			waitFor(t, w, "document.title === 'one'")
			pid := w.Page().ProcessID()
			done := make(chan error, 1)
			go func() {
				_, err := w.Page().Eval(pending)
				done <- err
			}()
			waitFor(t, w, "window.started")
			c.leave(t, w)
			waitFor(t, w, "document.title === 'two'")
			t.Logf("web process %d, then %d", pid, w.Page().ProcessID())
			err := wait.For(t, "the answer of the Eval in the page left", done)
			if err == nil || err.Error() != "mygo: the page navigated away before it answered" {
				t.Errorf("Eval in the page left = %v, want the page navigated away", err)
			}
			// The page shown answers, promises too.
			v, err := w.Page().Eval("new Promise(r => setTimeout(() => r(document.title + !window.started)))")
			if err != nil || v != "twotrue" {
				t.Errorf("Eval in the next page = %v, %v; want twotrue", v, err)
			}
		})
	}

	// pushState and a new fragment stay in the document: an Eval pending
	// there still gets its result.
	t.Run("same document", func(t *testing.T) {
		w := newWindow(t, mygo.WindowOptions{Hidden: true})
		if err := w.Page().LoadURL("app://localhost/session/one"); err != nil {
			t.Fatal(err)
		}
		waitFor(t, w, "document.title === 'one'")
		type result struct {
			v   any
			err error
		}
		done := make(chan result, 1)
		go func() {
			v, err := w.Page().Eval("window.started = true; return await new Promise(r => window.finish = r)")
			done <- result{v, err}
		}()
		waitFor(t, w, "window.started")
		mustEval(t, w, "addEventListener('hashchange', () => window.hashed = true), history.pushState(null, '', '/session/pushed'), location.hash = 'b', 1")
		waitFor(t, w, "window.hashed")
		mustEval(t, w, "finish(location.pathname + location.hash), 1")
		r := wait.For(t, "the answer of the Eval", done)
		if r.err != nil || r.v != "/session/pushed#b" {
			t.Errorf("Eval across pushState and a hash change = %v, %v; want /session/pushed#b", r.v, r.err)
		}
	})

	// Listening for pagehide is best effort: a page that replaced the
	// window's addEventListener and removeEventListener still answers.
	t.Run("page without listeners", func(t *testing.T) {
		w := newWindow(t, mygo.WindowOptions{Hidden: true})
		if err := w.Page().LoadURL("app://localhost/session/one"); err != nil {
			t.Fatal(err)
		}
		waitFor(t, w, "document.title === 'one'")
		mustEval(t, w, "window.addEventListener = window.removeEventListener = null, 1")
		v, err := w.Page().Eval("new Promise(r => setTimeout(() => r(document.title)))")
		if err != nil || v != "one" {
			t.Errorf("Eval in a page without addEventListener = %v, %v; want one", v, err)
		}
	})

	t.Run("closed window", func(t *testing.T) {
		w := newWindow(t, mygo.WindowOptions{Hidden: true})
		if err := w.Page().LoadURL("app://localhost/session/one"); err != nil {
			t.Fatal(err)
		}
		waitFor(t, w, "document.title === 'one'")
		done := make(chan error, 1)
		go func() {
			_, err := w.Page().Eval(pending)
			done <- err
		}()
		waitFor(t, w, "window.started")
		w.Destroy()
		err := wait.For(t, "the answer of the Eval in the closed window", done)
		if err == nil || err.Error() != "mygo: window has been destroyed" {
			t.Errorf("Eval in a closed window = %v, want the window has been destroyed", err)
		}
	})
}
