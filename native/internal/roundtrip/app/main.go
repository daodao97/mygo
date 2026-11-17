// App runs the round trip's checks where go test does not reach, and the
// SQLite plugin's: in an app on a phone, whose modules are their Swift on
// iOS and their Kotlin on Android. It logs each check, then "roundtrip: PASS" or "roundtrip:
// FAIL", which CI waits for in the system's log:
//
//	go run ./cmd/mygo build -platform android/arm64 native/internal/roundtrip/app
package main

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/native/internal/roundtrip/check"
	"github.com/egoist/mygo/ui"
)

func main() {
	result := "running"
	mygo.App.WhenReady(func() {
		var w *mygo.Window
		w = mygo.NewWindow(mygo.WindowOptions{Title: "Round trip", Content: ui.View(func(c *ui.Context) {
			ui.Column(c).Fill().Padding(24).Children(func() {
				ui.Text(c, "roundtrip: "+result).FontSize(20)
			})
		})})
		go func() {
			failed := run()
			mygo.RunOnMain(func() {
				result = map[bool]string{true: "FAIL", false: "PASS"}[failed]
				w.Invalidate()
			})
			log.Printf("roundtrip: %s", map[bool]string{true: "FAIL", false: "PASS"}[failed])
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// run runs the checks, each on a goroutine of its own, as a test is, and
// reports whether one failed.
func run() bool {
	failed := false
	checks := append(check.All, struct {
		Name string
		Run  func(check.T)
	}{"SQLite", checkSQLite})
	for _, c := range checks {
		t := &tester{name: c.Name}
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					t.failed = true
					t.log(fmt.Sprint("panic: ", r))
				}
			}()
			c.Run(t)
		}()
		<-done
		for i := len(t.cleanups) - 1; i >= 0; i-- {
			t.cleanups[i]()
		}
		switch {
		case t.failed:
			failed = true
			log.Printf("roundtrip: --- FAIL: %s", c.Name)
		case t.skipped:
			// A check that skips here fails: the app is for where the module
			// runs.
			failed = true
			log.Printf("roundtrip: --- SKIP: %s", c.Name)
		default:
			log.Printf("roundtrip: --- ok: %s", c.Name)
		}
	}
	return failed
}

// tester is a check's T: Fatal and Skip end its goroutine, as testing.T's
// do.
type tester struct {
	name            string
	failed, skipped bool
	cleanups        []func()
}

func (t *tester) Helper() {}

func (t *tester) log(msg string) { log.Printf("roundtrip: %s: %s", t.name, msg) }

func (t *tester) Fatal(args ...any) {
	t.failed = true
	t.log(fmt.Sprint(args...))
	runtime.Goexit()
}

func (t *tester) Fatalf(format string, args ...any) { t.Fatal(fmt.Sprintf(format, args...)) }

func (t *tester) Skip(args ...any) {
	t.skipped = true
	t.log(fmt.Sprint(args...))
	runtime.Goexit()
}

func (t *tester) Context() context.Context { return context.Background() }

func (t *tester) Cleanup(f func()) { t.cleanups = append(t.cleanups, f) }
