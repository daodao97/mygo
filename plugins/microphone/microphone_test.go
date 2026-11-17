package microphone

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/egoist/mygo"
)

type stopCall struct {
	token  int64
	cancel bool
}

func fakeNative(t *testing.T) *[]stopCall {
	var calls []stopCall
	old := stopNative
	stopNative = func(token int64, cancel bool) { calls = append(calls, stopCall{token, cancel}) }
	t.Cleanup(func() { stopNative = old })
	return &calls
}

func audio(t *testing.T, token int64, pcm []byte, end bool, failure string) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"token": token, "pcm": pcm, "end": end, "error": failure})
	if err != nil {
		t.Fatal(err)
	}
	onAudio(raw)
}

func TestCaptureStopDeliversThenEnds(t *testing.T) {
	calls := fakeNative(t)
	var got []Chunk
	c := track(func(ch Chunk) { got = append(got, ch) })
	audio(t, c.token, []byte{1, 2}, false, "")
	c.Stop()
	c.Stop()
	audio(t, c.token, []byte{3, 4}, false, "")
	audio(t, c.token, nil, true, "")
	audio(t, c.token, []byte{5, 6}, false, "") // after the end: dropped
	if len(got) != 3 || string(got[0].PCM) != "\x01\x02" || string(got[1].PCM) != "\x03\x04" || !got[2].End || got[2].Err != nil {
		t.Fatalf("chunks = %+v", got)
	}
	if len(*calls) != 1 || (*calls)[0] != (stopCall{c.token, false}) {
		t.Fatalf("native stops = %+v", *calls)
	}
	if _, ok := captures.m[c.token]; ok {
		t.Fatal("an ended capture stayed tracked")
	}
}

func TestCaptureCancelAndFailure(t *testing.T) {
	calls := fakeNative(t)
	var got []Chunk
	c := track(func(ch Chunk) { got = append(got, ch) })
	c.Cancel()
	audio(t, c.token, []byte{1}, false, "")
	if len(got) != 0 || len(*calls) != 1 || !(*calls)[0].cancel {
		t.Fatalf("canceled: chunks %+v, stops %+v", got, *calls)
	}
	d := track(func(ch Chunk) { got = append(got, ch) })
	audio(t, d.token, nil, true, "Recording interrupted")
	if len(got) != 1 || !got[0].End || got[0].Err == nil {
		t.Fatalf("failure: %+v", got)
	}
	e := track(func(ch Chunk) { got = append(got, ch) })
	e.end(errors.New("ctx done"))
	if last := got[len(got)-1]; !last.End || last.Err == nil || (*calls)[len(*calls)-1] != (stopCall{e.token, true}) {
		t.Fatalf("end: %+v, stops %+v", got, *calls)
	}
}

func TestClassify(t *testing.T) {
	if !errors.Is(classify(&mygo.NativeError{Code: "Denied"}), ErrDenied) || !errors.Is(classify(&mygo.NativeError{Code: "Busy"}), ErrBusy) {
		t.Fatal("codes not classified")
	}
}
