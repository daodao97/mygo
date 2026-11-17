// Package check holds the checks of the round trip of every kind of value
// through the module roundtrip, which its tests run under go test, and
// its app (roundtrip/app) on phones, where go test does not reach.
package check

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/native"
	"github.com/egoist/mygo/native/internal/roundtrip"
	"github.com/egoist/mygo/ui"
)

// T is what the checks use of a test: testing.T's methods, which the app
// implements too.
type T interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Skip(args ...any)
	Context() context.Context
	Cleanup(func())
}

// ctx is a context for a check, which ends with it, and after a while.
func ctx(t T) context.Context {
	c, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	return c
}

// Echo makes the module's implementation, skipping where it is not built:
// mygo native build -target host native/internal/roundtrip, with Swift.
func Echo(t T) roundtrip.Echo {
	t.Helper()
	e, err := roundtrip.NewEcho()
	if errors.Is(err, errors.ErrUnsupported) {
		t.Skip("no implementation on this platform")
	}
	if err != nil && strings.Contains(err.Error(), "mygo native build") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func Scalars(t T) {
	e := Echo(t)
	got := e.Scalars(-5, 127, 65535, -7, 1<<63, 1.5, -2.25, true)
	want := roundtrip.Scalars{I: -4, I8: -128, U16: 0, I32: -6, U64: 1<<63 + 1, F32: 2.5, F: -1.25, B: false}
	if got != want {
		t.Fatalf("Scalars = %+v, want %+v", got, want)
	}
	if n := e.Add(40, 2); n != 42 {
		t.Fatalf("Add = %d", n)
	}
}

func Text(t T) {
	e := Echo(t)
	got := e.Text("héllo, 世界\x00!", []byte{1, 2, 3})
	if got.S != "héllo, 世界\x00!héllo, 世界\x00!" || !bytes.Equal(got.B, []byte{3, 2, 1}) {
		t.Fatalf("Text = %+v", got)
	}
	if got := e.Text("", nil); got.S != "" || len(got.B) != 0 {
		t.Fatalf("Text of nothing = %+v", got)
	}
}

func Record(t T) {
	e := Echo(t)
	in := roundtrip.Record{
		Name:   "r",
		Counts: []int{1, -2, 3},
		Tags:   map[string]string{"a": "1", "b": ""},
		Inner:  roundtrip.Inner{X: 1.5, Y: -2},
		Next:   &roundtrip.Inner{X: 3},
		Kind:   roundtrip.KindB,
		Data:   []byte("data"),
	}
	got, err := e.Record(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "r" || !slices.Equal(got.Counts, []int{2, -4, 6}) || len(got.Tags) != 2 || got.Tags["a"] != "1" ||
		got.Inner != in.Inner || got.Next == nil || *got.Next != *in.Next || got.Kind != roundtrip.KindB || string(got.Data) != "data" {
		t.Fatalf("Record = %+v", got)
	}
	got, err = e.Record(roundtrip.Record{Kind: roundtrip.KindA})
	if err != nil || got.Next != nil || len(got.Counts) != 0 || len(got.Tags) != 0 {
		t.Fatalf("Record of zeros = %+v, %v", got, err)
	}
}

func Lists(t T) {
	e := Echo(t)
	got, err := e.Lists([]string{"a", "bb", "世"}, [][]int{{1, 2}, {}, {3}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got["a"] != 1 || got["bb"] != 2 || got["世"] != 3 || got["nested"] != 6 {
		t.Fatalf("Lists = %v", got)
	}
}

func Maybe(t T) {
	e := Echo(t)
	n := 41
	if got := e.Maybe(&n); got == nil || *got != 42 {
		t.Fatalf("Maybe(41) = %v", got)
	}
	if got := e.Maybe(nil); got != nil {
		t.Fatalf("Maybe(nil) = %v", *got)
	}
}

func Times(t T) {
	e := Echo(t)
	at := time.Date(2026, 11, 7, 12, 30, 0, 125_000_000, time.UTC)
	got, err := e.Times(at, 1500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if d := got.At.Sub(at.Add(1500 * time.Millisecond)); d < -time.Millisecond || d > time.Millisecond || got.Twice != 3*time.Second {
		t.Fatalf("Times = %+v", got)
	}
}

func Kinds(t T) {
	e := Echo(t)
	got, err := e.Kind(roundtrip.KindC, roundtrip.Low)
	if err != nil || got.Kind != roundtrip.KindA || got.Level != roundtrip.High {
		t.Fatalf("Kind = %+v, %v", got, err)
	}
}

func Errors(t T) {
	e := Echo(t)
	if err := e.Fail(""); err != nil {
		t.Fatal(err)
	}
	if err := e.Fail("named"); !errors.Is(err, roundtrip.ErrNamed) {
		t.Fatalf("Fail(named) = %v", err)
	}
	if err := e.Fail("unsupported"); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Fail(unsupported) = %v", err)
	}
	var ne *native.Error
	if err := e.Fail("other"); !errors.As(err, &ne) || ne.Domain != "RoundtripDomain" || ne.Number != 7 || ne.Message != "other" {
		t.Fatalf("Fail(other) = %#v", err)
	}
}

func Async(t T) {
	e := Echo(t)
	s, err := e.Wait(ctx(t), time.Millisecond, "done")
	if err != nil || s != "done" {
		t.Fatalf("Wait = %q, %v", s, err)
	}
	ctx, cancel := context.WithTimeout(ctx(t), 20*time.Millisecond)
	defer cancel()
	if _, err := e.Wait(ctx, time.Minute, "never"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait canceled = %v", err)
	}
}

func Streams(t T) {
	e := Echo(t)
	var got []int
	for n, err := range e.Count(ctx(t), 5, -1) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, n)
	}
	if !slices.Equal(got, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("Count = %v", got)
	}
	got = nil
	var last error
	for n, err := range e.Count(ctx(t), 5, 2) {
		if err != nil {
			last = err
			break
		}
		got = append(got, n)
	}
	if !slices.Equal(got, []int{0, 1}) || !errors.Is(last, roundtrip.ErrNamed) {
		t.Fatalf("Count failing = %v, %v", got, last)
	}
	// Breaking out early stops the stream.
	for n := range e.Count(ctx(t), 1000, -1) {
		if n == 3 {
			break
		}
	}
	var names []string
	for r, err := range e.Records(ctx(t), 3) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, r.Name+":"+r.Tags["i"])
	}
	if !slices.Equal(names, []string{"r0:0", "r1:1", "r2:2"}) {
		t.Fatalf("Records = %v", names)
	}
}

func Objects(t T) {
	e := Echo(t)
	c, err := e.Counter(10)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := c.Next(), c.Next(); a != 10 || b != 11 {
		t.Fatalf("Next = %d, %d", a, b)
	}
	d := c.Same()
	if n := d.Next(); n != 12 {
		t.Fatalf("Same().Next() = %d", n)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); !errors.Is(err, native.ErrClosed) {
		t.Fatalf("second Close = %v", err)
	}
}

func Callbacks(t T) {
	e := Echo(t)
	var mu sync.Mutex
	var seen []string
	n := e.Each([]string{"a", "b", "c"}, func(word string, i int) {
		mu.Lock()
		seen = append(seen, word+string(rune('0'+i)))
		mu.Unlock()
	})
	native.Flush()
	mu.Lock()
	defer mu.Unlock()
	if n != 3 || !slices.Equal(seen, []string{"a0", "b1", "c2"}) {
		t.Fatalf("Each = %d, %v", n, seen)
	}
	sum, err := e.Ask([]int{1, 2, 3}, func(n int) (int, error) { return 2 * n, nil })
	if err != nil || sum != 12 {
		t.Fatalf("Ask = %d, %v", sum, err)
	}
	boom := errors.New("boom")
	if _, err := e.Ask([]int{1}, func(int) (int, error) { return 0, boom }); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Ask failing = %v", err)
	}
}

func Ticks(t T) {
	e := Echo(t)
	var n atomic.Int64
	c := e.Ticks(time.Millisecond, func(i int) { n.Store(int64(i)) })
	// f runs on Go's main thread, which an Android emulator keeps busy for
	// seconds as the app draws its first frames.
	deadline := time.Now().Add(30 * time.Second)
	for n.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if n.Load() < 3 {
		t.Fatalf("ticked %d times", n.Load())
	}
}

type scorer struct{}

func (scorer) Score(word string) float64 { return float64(len(word)) }

func (scorer) Explain(ctx context.Context, word string) (string, error) {
	select {
	case <-time.After(time.Millisecond):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return "because " + word, nil
}

func GoImplements(t T) {
	e := Echo(t)
	got, err := e.Use(ctx(t), scorer{}, []string{"ab", "cde"})
	if err != nil || got != 5+float64(len("because ab")) {
		t.Fatalf("Use = %v, %v", got, err)
	}
}

func Buffer(t T) {
	e := Echo(t)
	buf := native.NewBuffer(300)
	n, err := e.Fill(buf, 0x55)
	if err != nil || n != 300 {
		t.Fatalf("Fill = %d, %v", n, err)
	}
	for i, b := range buf.Bytes() {
		if b != byte(i^0x55) {
			t.Fatalf("byte %d = %#x", i, b)
		}
	}
}

func More(t T) {
	e := Echo(t)
	if got := e.Mix(1.5, 2.25, true, -3); got != 1.75 {
		t.Fatalf("Mix = %v", got)
	}
	if got := e.Mix(0, -0.5, false, 127); got != 126.5 {
		t.Fatalf("Mix = %v", got)
	}
	got, err := e.Box(ui.Rect{X: 1, Y: 2, W: 3, H: 4.5})
	if err != nil || got != (ui.Rect{X: 4, Y: 6.5, W: 3, H: 4.5}) {
		t.Fatalf("Box = %+v, %v", got, err)
	}
	if n := e.Nothing(make([]roundtrip.Empty, 3)); n != 3 {
		t.Fatalf("Nothing = %d", n)
	}
	// Main-thread methods wait for the app, which tests do not run.
	if !mygo.App.IsReady() {
		if _, err := e.OnMain(roundtrip.Record{}, nil); err == nil {
			t.Fatal("OnMain ran off the main thread")
		}
	} else {
		// f, which returns nothing, runs later on Go's main thread.
		in := make(chan roundtrip.Inner, 1)
		got, err := e.OnMain(roundtrip.Record{Name: "main", Kind: roundtrip.KindA, Inner: roundtrip.Inner{X: 2}}, func(i roundtrip.Inner) { in <- i })
		if err != nil || got.S != "main" {
			t.Fatalf("OnMain = %+v, %v", got, err)
		}
		if i := <-in; i.X != 2 {
			t.Fatalf("OnMain's f got %+v", i)
		}
	}
}

// All are the checks, by name.
var All = []struct {
	Name string
	Run  func(T)
}{
	{"Scalars", Scalars},
	{"Text", Text},
	{"Record", Record},
	{"Lists", Lists},
	{"Maybe", Maybe},
	{"Times", Times},
	{"Kinds", Kinds},
	{"Errors", Errors},
	{"Async", Async},
	{"Streams", Streams},
	{"Objects", Objects},
	{"Callbacks", Callbacks},
	{"Ticks", Ticks},
	{"GoImplements", GoImplements},
	{"Buffer", Buffer},
	{"More", More},
}
