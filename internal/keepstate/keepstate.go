// Package keepstate turns a program's values into JSON and back, unexported
// fields and all, so that an app comes back as it was after a restart, without
// tags or methods on its types: a struct of its own, as main packages keep theirs,
// just works.
//
// It keeps data, not the world around it. What a function, a channel, an interface
// or a pointer to another package's type holds stays as the program made it, and so
// does a slice or a map of elements that hold any of those, which couldn't be made
// whole. A field whose type changed since the value was saved, as a change of the
// code may have done, stays as it is too. A pointer met a second time, as a list's
// link back, comes back nil where it was: data, not a graph.
package keepstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// maxDepth bounds how deep it goes, and maxNodes how much it writes down: a value
// this large is no state to write every second.
const (
	maxDepth = 4096
	maxNodes = 1 << 20
)

var timeType = reflect.TypeFor[time.Time]()

// ownModule is this framework's: a type of it is the world around an app, what
// its pointers lead to, as a window or its elements, however close by.
var ownModule = "github.com/egoist/mygo"

// walk is one Encode or Decode.
type walk struct {
	// pkg is the package of the value, and first the start of its path, which
	// tells an app's other packages from the standard library's whatever its
	// module is called.
	pkg, first string
	// seen are the pointers met.
	seen  map[unsafe.Pointer]bool
	nodes int
	// fits remembers keepable.
	fits map[reflect.Type]bool
}

func newWalk(t reflect.Type) *walk {
	pkg := pkgOf(t)
	first, _, _ := strings.Cut(pkg, "/")
	return &walk{pkg: pkg, first: first, seen: map[unsafe.Pointer]bool{}, fits: map[reflect.Type]bool{}}
}

// Encode returns v, a pointer, as JSON. Values it can't walk it leaves out. What
// goes wrong, as a value too large, is an error.
func Encode(v any) (data []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			data, err = nil, fmt.Errorf("keepstate: %v", r)
		}
	}()
	r := reflect.ValueOf(v)
	if r.Kind() != reflect.Pointer || r.IsNil() {
		return nil, nil
	}
	w := newWalk(r.Elem().Type())
	tree, _ := w.encode(r.Elem(), 0)
	if w.nodes > maxNodes {
		return nil, fmt.Errorf("keepstate: more than %d values to keep", maxNodes)
	}
	return json.Marshal(tree)
}

// Decode sets what data holds in v, a pointer, as far as the two still agree.
func Decode(data []byte, v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("keepstate: %v", r)
		}
	}()
	r := reflect.ValueOf(v)
	if r.Kind() != reflect.Pointer || r.IsNil() {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // integers whole
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return err
	}
	newWalk(r.Elem().Type()).decode(r.Elem(), tree, 0)
	return nil
}

// pkgOf returns the package a value belongs to: of its element, for a slice or a
// map of its structs.
func pkgOf(t reflect.Type) string {
	for {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			t = t.Elem()
		default:
			return t.PkgPath()
		}
	}
}

// walkable says whether to go into a struct: an app's, not the standard
// library's, whose locks and buffers restart as they are.
func (w *walk) walkable(t reflect.Type) bool {
	path := t.PkgPath()
	if path == "" || path == "main" || path == w.pkg {
		return true
	}
	first, _, _ := strings.Cut(path, "/")
	return first == w.first || strings.Contains(first, ".")
}

// followed says whether to go through a pointer to t: to data of the value's own
// package, not to the windows, cameras and bitmaps of others, nor through the
// framework's own types, which lead to all of a window's elements.
func (w *walk) followed(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Struct:
		return t.PkgPath() == w.pkg && !isFramework(t.PkgPath())
	case reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Interface:
		return false
	}
	return true
}

func isFramework(path string) bool {
	if !strings.HasPrefix(path, ownModule) {
		return false
	}
	return !strings.HasPrefix(path, ownModule+"/examples/")
}

// keepable says whether a value of t comes back whole, with nothing in it left to
// what the program made: what a new element of a collection must be.
func (w *walk) keepable(t reflect.Type) bool {
	if ok, seen := w.fits[t]; seen {
		return ok
	}
	w.fits[t] = true // a type that holds itself rests on the rest of it
	ok := true
	switch t.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Interface:
		ok = false
	case reflect.Pointer:
		ok = w.followed(t.Elem()) && w.keepable(t.Elem())
	case reflect.Slice, reflect.Array:
		ok = w.keepable(t.Elem())
	case reflect.Map:
		ok = validKey(t.Key()) && w.keepable(t.Elem())
	case reflect.Struct:
		if t == timeType {
			break
		}
		ok = w.walkable(t)
		for i := 0; ok && i < t.NumField(); i++ {
			ok = w.keepable(t.Field(i).Type)
		}
	}
	w.fits[t] = ok
	return ok
}

func validKey(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// encode returns what to write of v, and whether to write anything: a nil slice
// is written, as nil, and a function isn't.
func (w *walk) encode(v reflect.Value, depth int) (any, bool) {
	if depth > maxDepth {
		return nil, false
	}
	if w.nodes++; w.nodes > maxNodes {
		return nil, false
	}
	v = reachable(v)
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint(), true
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false // JSON has none of those
		}
		return f, true
	case reflect.String:
		return v.String(), true
	case reflect.Slice:
		if !w.keepable(v.Type().Elem()) {
			return nil, false
		}
		if v.IsNil() {
			return nil, true
		}
		fallthrough
	case reflect.Array:
		out := make([]any, v.Len())
		for i := range out {
			out[i], _ = w.encode(v.Index(i), depth+1)
		}
		return out, true
	case reflect.Map:
		if !validKey(v.Type().Key()) || !w.keepable(v.Type().Elem()) {
			return nil, false
		}
		if v.IsNil() {
			return nil, true
		}
		out := make(map[string]any, v.Len())
		for it := v.MapRange(); it.Next(); {
			k, ok := keyString(it.Key())
			if !ok {
				continue
			}
			// A map's values can't be addressed, as reading unexported fields
			// asks: a copy can.
			e := reflect.New(v.Type().Elem()).Elem()
			e.Set(it.Value())
			out[k], _ = w.encode(e, depth+1)
		}
		return out, true
	case reflect.Pointer:
		if !w.followed(v.Type().Elem()) {
			return nil, false
		}
		if v.IsNil() {
			return nil, true
		}
		if w.seen[v.UnsafePointer()] {
			return nil, false
		}
		w.seen[v.UnsafePointer()] = true
		return w.encode(v.Elem(), depth+1)
	case reflect.Struct:
		if v.Type() == timeType {
			if b, err := v.Interface().(time.Time).MarshalText(); err == nil {
				return string(b), true
			}
			return nil, false
		}
		if !w.walkable(v.Type()) {
			return nil, false
		}
		out := make(map[string]any, v.NumField())
		t := v.Type()
		for i := range t.NumField() {
			if f, ok := w.encode(v.Field(i), depth+1); ok {
				out[t.Field(i).Name] = f
			}
		}
		return out, true
	}
	return nil, false
}

func keyString(k reflect.Value) (string, bool) {
	switch k.Kind() {
	case reflect.String:
		return k.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(k.Uint(), 10), true
	}
	return "", false
}

func keyValue(t reflect.Type, s string) (reflect.Value, bool) {
	k := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.String:
		k.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, t.Bits())
		if err != nil {
			return k, false
		}
		k.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, t.Bits())
		if err != nil {
			return k, false
		}
		k.SetUint(n)
	default:
		return k, false
	}
	return k, true
}

// reachable returns v without the read-only mark of an unexported field. Only an
// addressable value can lose it: every path here keeps values addressable.
func reachable(v reflect.Value) reflect.Value {
	if v.CanSet() || !v.CanAddr() {
		return v
	}
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
}

func (w *walk) decode(v reflect.Value, tree any, depth int) {
	if depth > maxDepth {
		return
	}
	v = reachable(v)
	if !v.CanSet() {
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		if b, ok := tree.(bool); ok {
			v.SetBool(b)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, ok := tree.(json.Number); ok {
			if i, err := n.Int64(); err == nil && !v.OverflowInt(i) {
				v.SetInt(i)
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if n, ok := tree.(json.Number); ok {
			if i, err := strconv.ParseUint(n.String(), 10, 64); err == nil && !v.OverflowUint(i) {
				v.SetUint(i)
			}
		}
	case reflect.Float32, reflect.Float64:
		if n, ok := tree.(json.Number); ok {
			if f, err := n.Float64(); err == nil && !v.OverflowFloat(f) {
				v.SetFloat(f)
			}
		}
	case reflect.String:
		if s, ok := tree.(string); ok {
			v.SetString(s)
		}
	case reflect.Slice:
		if !w.keepable(v.Type().Elem()) {
			return // the whole collection stays as made
		}
		if tree == nil {
			v.SetZero()
			return
		}
		if list, ok := tree.([]any); ok {
			out := reflect.MakeSlice(v.Type(), len(list), len(list))
			for i, x := range list {
				w.decode(out.Index(i), x, depth+1)
			}
			v.Set(out)
		}
	case reflect.Array:
		if list, ok := tree.([]any); ok {
			for i, x := range list {
				if i < v.Len() {
					w.decode(v.Index(i), x, depth+1)
				}
			}
		}
	case reflect.Map:
		if !validKey(v.Type().Key()) || !w.keepable(v.Type().Elem()) {
			return
		}
		if tree == nil {
			v.SetZero()
			return
		}
		if m, ok := tree.(map[string]any); ok {
			out := reflect.MakeMapWithSize(v.Type(), len(m))
			for s, x := range m {
				k, ok := keyValue(v.Type().Key(), s)
				if !ok {
					continue
				}
				e := reflect.New(v.Type().Elem()).Elem()
				w.decode(e, x, depth+1)
				out.SetMapIndex(k, e)
			}
			v.Set(out)
		}
	case reflect.Pointer:
		if !w.followed(v.Type().Elem()) {
			return
		}
		if tree == nil {
			if w.keepable(v.Type().Elem()) {
				v.SetZero()
			}
			return
		}
		if v.IsNil() {
			if !w.keepable(v.Type().Elem()) {
				return // what it holds that can't be kept stays as made: nothing
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		w.decode(v.Elem(), tree, depth+1)
	case reflect.Struct:
		if v.Type() == timeType {
			if s, ok := tree.(string); ok {
				var t time.Time
				if t.UnmarshalText([]byte(s)) == nil {
					v.Set(reflect.ValueOf(t))
				}
			}
			return
		}
		m, ok := tree.(map[string]any)
		if !ok || !w.walkable(v.Type()) {
			return
		}
		t := v.Type()
		for i := range t.NumField() {
			if x, ok := m[t.Field(i).Name]; ok {
				w.decode(v.Field(i), x, depth+1)
			}
		}
	}
}
