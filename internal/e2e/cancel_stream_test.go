//go:build !js

package e2e

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
)

var untilCanceledStopped = make(chan error, 1)

func (Streams) UntilCanceled(ctx context.Context, ch *mygo.Channel[int]) error {
	if err := ch.Send(1); err != nil {
		return err
	}
	<-ctx.Done()
	untilCanceledStopped <- ctx.Err()
	return ctx.Err()
}

func TestCancelStreamingCall(t *testing.T) {
	w := newWindow(t, mygo.WindowOptions{Hidden: true})
	if err := w.Page().LoadURL("app://localhost/"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, "window.run")
	mustEval(t, w, `(() => {
		const controller = new AbortController();
		const ch = mygo.channel();
		const state = window.stream = { controller, ch, values: [], error: null, finished: false };
		mygo.callWith("Streams.UntilCanceled", [ch], { signal: controller.signal })
			.catch(error => state.error = error);
		void (async () => {
			for await (const value of ch) state.values.push(value);
			state.finished = true;
		})();
		return true;
	})()`)
	waitFor(t, w, "stream.values.length > 0 || stream.error !== null")
	if got := mustEval(t, w, "JSON.stringify({ values: stream.values, error: stream.error && String(stream.error) })"); got != `{"values":[1],"error":null}` {
		t.Fatalf("before abort: %v", got)
	}

	mustEval(t, w, `stream.controller.abort("stop")`)
	if err := wait.For(t, "the Go stream to stop", untilCanceledStopped); !errors.Is(err, context.Canceled) {
		t.Fatalf("Go stream ended with %v, want context.Canceled", err)
	}
	got := mustEval(t, w, "JSON.stringify({ error: stream.error, closed: stream.ch.closed, finished: stream.finished })")
	if got != `{"error":"stop","closed":true,"finished":true}` {
		t.Fatalf("after abort: %v", got)
	}

	got = mustEval(t, w, `(async () => {
		const ch = mygo.channel();
		const call = mygo.callWith("Streams.Count", [1, ch], { signal: new AbortController().signal });
		const values = [];
		for await (const value of ch) values.push(value);
		await call;
		return JSON.stringify(values);
	})()`)
	if got != "[0]" {
		t.Fatalf("next stream returned %v, want [0]", got)
	}
}
