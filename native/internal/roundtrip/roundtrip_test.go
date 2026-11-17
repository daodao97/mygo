package roundtrip_test

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/native/internal/roundtrip"
	"github.com/egoist/mygo/native/internal/roundtrip/check"
)

func echo(t testing.TB) roundtrip.Echo { return check.Echo(t) }

func TestScalars(t *testing.T) { check.Scalars(t) }

func TestText(t *testing.T) { check.Text(t) }

func TestRecord(t *testing.T) { check.Record(t) }

func TestLists(t *testing.T) { check.Lists(t) }

func TestMaybe(t *testing.T) { check.Maybe(t) }

func TestTimes(t *testing.T) { check.Times(t) }

func TestKinds(t *testing.T) { check.Kinds(t) }

func TestErrors(t *testing.T) { check.Errors(t) }

func TestAsync(t *testing.T) { check.Async(t) }

func TestStreams(t *testing.T) { check.Streams(t) }

func TestObjects(t *testing.T) { check.Objects(t) }

func TestCallbacks(t *testing.T) { check.Callbacks(t) }

func TestTicks(t *testing.T) { check.Ticks(t) }

func TestGoImplements(t *testing.T) { check.GoImplements(t) }

func TestBuffer(t *testing.T) { check.Buffer(t) }

func TestMore(t *testing.T) { check.More(t) }

// The cost of a call of each shape, which the encoding of values decides.
func BenchmarkAdd(b *testing.B) {
	e := echo(b)
	for b.Loop() {
		e.Add(1, 2)
	}
}

func BenchmarkText(b *testing.B) {
	e := echo(b)
	s := strings.Repeat("x", 100)
	for b.Loop() {
		e.Text(s, nil)
	}
}

func BenchmarkRecord(b *testing.B) {
	e := echo(b)
	r := roundtrip.Record{Name: "r", Counts: []int{1, 2, 3, 4}, Tags: map[string]string{"a": "b"}, Next: &roundtrip.Inner{}, Kind: roundtrip.KindA, Data: make([]byte, 64)}
	for b.Loop() {
		if _, err := e.Record(r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStream(b *testing.B) {
	e := echo(b)
	for b.Loop() {
		for _, err := range e.Count(context.Background(), 100, -1) {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
