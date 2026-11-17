package keepstate

import (
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/scene"
)

func init() {
	ownModule = "this test's types are an app's" // not the framework's world
}

type inner struct {
	Name  string
	count int
}

type thing struct {
	tab     int
	name    string
	on      bool
	ratio   float64
	big     int64
	rows    []string
	byName  map[string]int
	byID    map[int]inner
	in      inner
	when    time.Time
	took    time.Duration
	next    *inner
	empty   []int
	arr     [2]int
	mu      sync.Mutex
	fn      func()
	ch      chan int
	foreign *scene.Image
	any     any
	nan     float64
}

// A struct of an app's comes back whole, unexported fields and all.
func TestRoundTrip(t *testing.T) {
	when := time.Date(2026, 11, 2, 12, 0, 0, 5, time.UTC)
	a := &thing{
		tab: 3, name: "中文 😀", on: true, ratio: 0.25, big: math.MaxInt64 - 1,
		rows: []string{"a", "b"}, byName: map[string]int{"x": 1}, byID: map[int]inner{7: {"seven", 7}},
		in: inner{"in", 2}, when: when, took: time.Minute, next: &inner{"next", 9}, arr: [2]int{4, 5},
		nan: math.NaN(),
	}
	data, err := Encode(a)
	if err != nil {
		t.Fatal(err)
	}
	var b thing
	if err := Decode(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.tab != 3 || b.name != a.name || !b.on || b.ratio != 0.25 || b.big != a.big {
		t.Errorf("scalars: %d %q %v", b.tab, b.name, b.ratio)
	}
	if !reflect.DeepEqual(b.rows, a.rows) || !reflect.DeepEqual(b.byName, a.byName) || !reflect.DeepEqual(b.byID, a.byID) {
		t.Errorf("collections: %v %v %v", b.rows, b.byName, b.byID)
	}
	if b.in != a.in || !b.when.Equal(when) || b.took != time.Minute || b.next == nil || *b.next != *a.next || b.arr != a.arr {
		t.Errorf("structs: %+v, %v", b.in, b.next)
	}
}

// What it doesn't keep stays as the program made it.
func TestLeavesTheWorld(t *testing.T) {
	made := func() {}
	ch := make(chan int)
	img := &scene.Image{W: 1}
	a := &thing{tab: 1, foreign: &scene.Image{W: 9}, any: 5}
	data, err := Encode(a)
	if err != nil {
		t.Fatal(err)
	}
	b := thing{fn: made, ch: ch, foreign: img, any: "made", ratio: 2}
	if err := Decode(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.fn == nil || b.ch != ch || b.foreign != img || img.W != 1 || b.any != "made" {
		t.Errorf("the world changed: %v %v", b.foreign, b.any)
	}
	if b.tab != 1 {
		t.Errorf("tab: %d", b.tab)
	}
}

// A change of the code keeps what still fits.
func TestCodeChanged(t *testing.T) {
	type before struct {
		tab   string
		kept  int
		gone  int
		rows  []int
		byKey map[string]int
	}
	type after struct {
		tab   int // another type: stays as made
		kept  int
		added string
		rows  []string // another type of element: its elements stay empty
		byKey map[int]int
	}
	data, err := Encode(&before{tab: "x", kept: 5, gone: 9, rows: []int{1}, byKey: map[string]int{"a": 1}})
	if err != nil {
		t.Fatal(err)
	}
	b := after{tab: 7, added: "made"}
	if err := Decode(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.tab != 7 || b.kept != 5 || b.added != "made" {
		t.Errorf("%+v", b)
	}
}

// A pointer of the framework leads nowhere, whatever it holds: a widget's state
// may hold a handle that reaches all of a window's elements.
func TestFrameworkPointers(t *testing.T) {
	defer func(m string) { ownModule = m }(ownModule)
	ownModule = "github.com/egoist/mygo"
	a := &inner{Name: "x"}
	type holder struct {
		in  *inner
		img *scene.Image
	}
	data, err := Encode(&holder{in: a, img: &scene.Image{W: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Errorf("wrote %s", data)
	}
}

// A cycle ends.
func TestCycle(t *testing.T) {
	type node struct {
		next *node
		n    int
	}
	a := &node{n: 1}
	a.next = a
	data, err := Encode(a)
	if err != nil {
		t.Fatal(err)
	}
	var b node
	if err := Decode(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.n != 1 {
		t.Errorf("n: %d", b.n)
	}
}

// Numbers that overflow what they go into stay out.
func TestOverflow(t *testing.T) {
	type big struct{ n int64 }
	type small struct{ n int8 }
	data, err := Encode(&big{n: 1000})
	if err != nil {
		t.Fatal(err)
	}
	b := small{n: 1}
	if err := Decode(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.n != 1 {
		t.Errorf("n: %d", b.n)
	}
}

// Encoding a plain scalar, as a widget's local state is.
func TestScalar(t *testing.T) {
	n := 42
	data, err := Encode(&n)
	if err != nil {
		t.Fatal(err)
	}
	var m int
	if err := Decode(data, &m); err != nil || m != 42 {
		t.Errorf("%d, %v", m, err)
	}
}

// What holds many ways to the same values stays quick and small.
func TestSharedPointers(t *testing.T) {
	type node struct {
		prev, next *node
		n          int
	}
	var first, last *node
	for i := range 200 {
		x := &node{n: i, prev: last}
		if last != nil {
			last.next = x
		} else {
			first = x
		}
		last = x
	}
	data, err := Encode(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 200*64 {
		t.Errorf("%d bytes for 200 nodes", len(data))
	}
	var back node
	if err := Decode(data, &back); err != nil {
		t.Fatal(err)
	}
	n := 0
	for x := &back; x != nil; x = x.next {
		if x.n != n {
			t.Fatalf("node %d says %d", n, x.n)
		}
		n++
	}
	if n != 200 {
		t.Errorf("%d of 200 nodes", n)
	}
}

// A time inside a map's struct, whose values can't be addressed.
func TestTimeInMap(t *testing.T) {
	type todo struct{ created time.Time }
	type state struct {
		exported map[string]todo
		ptr      map[string]*todo
	}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	a := state{exported: map[string]todo{"a": {when}}, ptr: map[string]*todo{"b": {when}}}
	data, err := Encode(&a)
	if err != nil {
		t.Fatal(err)
	}
	var b state
	if err := Decode(data, &b); err != nil {
		t.Fatal(err)
	}
	if !b.exported["a"].created.Equal(when) || b.ptr["b"] == nil || !b.ptr["b"].created.Equal(when) {
		t.Errorf("%v %v", b.exported, b.ptr)
	}
}

// A collection of what can't come back whole stays as the program made it, rather
// than coming back with holes.
func TestCollectionsStayWhole(t *testing.T) {
	type row struct {
		name string
		fn   func() int
	}
	type state struct {
		rows    []row
		byName  map[string]row
		any     []any
		foreign []*scene.Image
		kept    []string
	}
	img := &scene.Image{W: 3}
	fn := func() int { return 7 }
	a := state{rows: []row{{"a", fn}, {"b", fn}}, kept: []string{"x"}}
	data, err := Encode(&a)
	if err != nil {
		t.Fatal(err)
	}
	b := state{
		rows: []row{{"made", fn}}, byName: map[string]row{"m": {"made", fn}},
		any: []any{1}, foreign: []*scene.Image{img},
	}
	if err := Decode(data, &b); err != nil {
		t.Fatal(err)
	}
	if len(b.rows) != 1 || b.rows[0].name != "made" || b.rows[0].fn() != 7 || b.byName["m"].fn == nil {
		t.Errorf("rows: %+v", b.rows)
	}
	if len(b.any) != 1 || b.foreign[0] != img {
		t.Errorf("any %v, foreign %v", b.any, b.foreign)
	}
	if len(b.kept) != 1 || b.kept[0] != "x" {
		t.Errorf("kept: %v", b.kept)
	}
}

// Nil is a value: what the app emptied stays empty, whatever it starts with.
func TestNilComesBack(t *testing.T) {
	type state struct {
		items []string
		names map[string]int
		next  *int
	}
	data, err := Encode(&state{})
	if err != nil {
		t.Fatal(err)
	}
	one := 1
	b := state{items: []string{"seed"}, names: map[string]int{"seed": 1}, next: &one}
	if err := Decode(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.items != nil || b.names != nil || b.next != nil {
		t.Errorf("%+v", b)
	}
}

// Too much to write every second is an error, not a stall.
func TestTooLarge(t *testing.T) {
	big := make([]int, maxNodes+10)
	if _, err := Encode(&big); err == nil {
		t.Error("no error")
	}
}
