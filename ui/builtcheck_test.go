package ui

import (
	"bytes"
	"fmt"
	"hash/maphash"
	"os"
	"reflect"
	"slices"
	"sort"
	"unsafe"
)

// The build check: after each phase of a frame (checkBuilt), the fields of
// node that the view builds, with the journal rolled back, are what the
// view built. The fields below are what the frame computes in fields of
// their own, which it leaves alone: the boxes layout gives, what commit
// and painting find, and the tree's links, which a List's rows join as it
// lays out.
var derivedFields = map[string]bool{
	"x": true, "y": true, "w": true, "h": true, "contentW": true, "contentH": true,
	"world": true, "pinWorld": true, "ink": true, "noCull": true, "tl": true, "measured": true,
	"lay": true, "laid": true, "placed": true, "flips": true, "frags": true,
	"poseM": true, "poseFade": true, "reposed": true,
	"parent": true, "first": true, "last": true, "next": true, "nchild": true,
	// What a measuring pass kept for the commit pass, as "measured" is.
	"hold": true,
}

// builtField is a field the check compares: its bytes, and a slice's
// elements too.
type builtField struct {
	name      string
	off, size uintptr
	slice     bool
	elem      uintptr
	typ       reflect.Type
}

var builtFields = func() (fs []builtField) {
	t := reflect.TypeOf(node{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if derivedFields[f.Name] {
			continue
		}
		b := builtField{name: f.Name, off: f.Offset, size: f.Type.Size(), typ: f.Type}
		if f.Type.Kind() == reflect.Slice {
			b.slice, b.elem = true, f.Type.Elem().Size()
		}
		fs = append(fs, b)
	}
	return fs
}()

var nslices = func() (n int) {
	for _, f := range builtFields {
		if f.slice {
			n++
		}
	}
	return
}()

var (
	builtFails   = map[string]int{}
	builtCollect = os.Getenv("MYGO_BUILT_COLLECT") != ""
	builtSeed    = maphash.MakeSeed()
)

func nodeBytes(e *node) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(e)), nodeSize)
}

// sliceHash hashes the elements of the slice whose header is at p.
func sliceHash(p unsafe.Pointer, elem uintptr) uint64 {
	s := *(*[]byte)(p) // its length counts elements
	if len(s) == 0 {
		return 0
	}
	return maphash.Bytes(builtSeed, unsafe.Slice(unsafe.SliceData(s), uintptr(len(s))*elem))
}

func hashSlices(s *builtSnapshot, b []byte) {
	for _, f := range builtFields {
		if f.slice {
			s.hashes = append(s.hashes, sliceHash(unsafe.Pointer(&b[f.off]), f.elem))
		}
	}
}

func checkBuiltFields(rt *engine, phase builtPhase) {
	if phase == builtDropped {
		rt.builtSnap = nil
		return
	}
	root := rt.c.root
	if root == nil {
		return
	}
	if phase == builtBuilt {
		s := rt.builtSnap
		if s == nil {
			s = &builtSnapshot{}
			rt.builtSnap = s
		}
		s.nodes, s.bytes, s.hashes = s.nodes[:0], s.bytes[:0], s.hashes[:0]
		// Depth first, without a closure, so that checking allocates
		// nothing for the tests that count allocations.
		for e := root; e != nil; {
			s.nodes = append(s.nodes, e)
			b := nodeBytes(e)
			s.bytes = append(s.bytes, b...)
			hashSlices(s, b)
			switch {
			case e.first != nil:
				e = e.first
			default:
				for e != nil && e.next == nil && e != root {
					e = e.parent
				}
				if e == nil || e == root {
					e = nil
				} else {
					e = e.next
				}
			}
		}
		s.order = s.order[:0]
		for i := range s.nodes {
			s.order = append(s.order, i)
		}
		slices.SortFunc(s.order, s.byAddress)
		return
	}
	if !builtCollect && (phase == builtLaidOut || phase == builtCommitted) {
		// What changed by then shows as the frame painted, the check
		// costing half: MYGO_BUILT_COLLECT checks each phase, to tell
		// which wrote.
		return
	}
	s := rt.builtSnap
	if s == nil || len(s.nodes) == 0 {
		return
	}
	// Each node's bytes now, with the journal rolled back on them.
	if cap(s.scratch) < len(s.bytes) {
		s.scratch = make([]byte, len(s.bytes))
	}
	now := s.scratch[:len(s.bytes)]
	for i, e := range s.nodes {
		copy(now[i*nodeSize:], nodeBytes(e))
	}
	j := &rt.journal
	for k := len(j.entries) - 1; k >= 0; k-- {
		en := j.entries[k]
		p := uintptr(en.p)
		at, ok := slices.BinarySearchFunc(s.order, p, s.holds)
		if !ok {
			continue // not a node the view built
		}
		i := s.order[at]
		if en.size == 0 {
			kept := j.nodes[en.off]
			copy(now[i*nodeSize:], nodeBytes(&kept))
			continue
		}
		off := int(p - uintptr(unsafe.Pointer(s.nodes[i])))
		copy(now[i*nodeSize+off:], j.data[en.off:en.off+en.size])
	}
	for i, e := range s.nodes {
		was, cur := s.bytes[i*nodeSize:(i+1)*nodeSize], now[i*nodeSize:(i+1)*nodeSize]
		if e.epoch != *(*uint64)(unsafe.Pointer(&was[unsafe.Offsetof(e.epoch)])) {
			continue // recycled since
		}
		hashes := s.hashes[i*nslices:]
		k := 0
		for _, f := range builtFields {
			a, b := was[f.off:f.off+f.size], cur[f.off:f.off+f.size]
			changed := !bytes.Equal(a, b)
			if f.slice {
				changed = changed || hashes[k] != sliceHash(unsafe.Pointer(&b[0]), f.elem)
				k++
			}
			if !changed {
				continue
			}
			key := phaseName(phase) + ": " + f.name + innerField(f, a, b)
			if builtCollect {
				builtFails[key]++
				continue
			}
			panic("ui: a built field changed after the build and is not in the journal, at " + key + " of " + describeBuilt(e))
		}
	}
}

// innerField names the field of a struct that differs, as ".width.v".
func innerField(f builtField, was, now []byte) string {
	t, path := f.typ, ""
	for t.Kind() == reflect.Struct {
		found := false
		for i := 0; i < t.NumField(); i++ {
			g := t.Field(i)
			a, b := was[g.Offset:g.Offset+g.Type.Size()], now[g.Offset:g.Offset+g.Type.Size()]
			if !bytes.Equal(a, b) {
				path += "." + g.Name
				was, now, t = a, b, g.Type
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	if f.slice && path == "" {
		path = " (elements)"
	}
	return path
}

// reportBuilt prints what MYGO_BUILT_COLLECT collected.
func reportBuilt() {
	if !builtCollect {
		return
	}
	var keys []string
	for k := range builtFails {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "BUILT %6d %s\n", builtFails[k], k)
	}
}

func phaseName(p builtPhase) string {
	return [...]string{"built", "laid out", "committed", "painted", "repainted", "dropped"}[p]
}

func describeBuilt(e *node) string {
	w := e.widget
	if w == "" {
		w = fmt.Sprintf("kind %d", e.kind)
	}
	if e.text != "" {
		w += fmt.Sprintf(" %q", e.text)
	}
	return w
}

// shownBackground returns the background e painted with in the last
// frame: a Transition moving its colors shows another than the one the
// view built, which painting puts back.
func shownBackground(e *node) Color {
	if r := e.st.trec; r != nil && r.colorSet && r.colorFrame == e.c.rt.frame {
		return r.colorShown[0]
	}
	return e.bg
}

// shownShadows returns the shadows e painted with in the last frame, as
// shownBackground its background.
func shownShadows(e *node) []shadow {
	if r := e.st.trec; r != nil && r.shadowSet && r.shadowFrame == e.c.rt.frame {
		return r.shadowShown
	}
	return e.shadows
}
