// Package wire encodes the messages the backend of phones (internal/mobile)
// and its hosts exchange: the Swift host on iOS, the Kotlin host on
// Android. A message is its number and then its fields, in the order the
// spec (gen/spec.go) lists them; replies are their fields alone.
//
// Integers are zigzag varints, floats 8 bytes little endian, booleans a
// byte, strings and bytes a uvarint length and their bytes, lists of
// strings a uvarint count and the strings, lists of floats a count and
// the floats. msgs.go, Wire.swift and
// Wire.kt are generated from the spec (go generate), so the three sides
// agree on every message.
package wire

import (
	"encoding/binary"
	"errors"
	"math"
)

//go:generate go run ./gen

// ErrShort is the error of a Reader that ran out of bytes.
var ErrShort = errors.New("wire: message too short")

// Writer appends encoded values to its buffer.
type Writer struct{ Buf []byte }

// Reset empties the buffer, keeping its memory.
func (w *Writer) Reset() { w.Buf = w.Buf[:0] }

func (w *Writer) Uvarint(v uint64) { w.Buf = binary.AppendUvarint(w.Buf, v) }

func (w *Writer) Int(v int) { w.Buf = binary.AppendVarint(w.Buf, int64(v)) }

func (w *Writer) Bool(v bool) {
	if v {
		w.Buf = append(w.Buf, 1)
	} else {
		w.Buf = append(w.Buf, 0)
	}
}

func (w *Writer) Float(v float64) {
	w.Buf = binary.LittleEndian.AppendUint64(w.Buf, math.Float64bits(v))
}

func (w *Writer) String(s string) {
	w.Uvarint(uint64(len(s)))
	w.Buf = append(w.Buf, s...)
}

func (w *Writer) Bytes(b []byte) {
	w.Uvarint(uint64(len(b)))
	w.Buf = append(w.Buf, b...)
}

func (w *Writer) Floats(f []float64) {
	w.Uvarint(uint64(len(f)))
	for _, v := range f {
		w.Float(v)
	}
}

func (w *Writer) Strings(s []string) {
	w.Uvarint(uint64(len(s)))
	for _, v := range s {
		w.String(v)
	}
}

// Reader reads values from a message. A value past its end reads as zero
// and sets Err, which the caller checks once it read the message.
type Reader struct {
	Buf []byte
	Err error
}

func (r *Reader) fail() {
	if r.Err == nil {
		r.Err = ErrShort
	}
	r.Buf = nil
}

func (r *Reader) Uvarint() uint64 {
	v, n := binary.Uvarint(r.Buf)
	if n <= 0 {
		r.fail()
		return 0
	}
	r.Buf = r.Buf[n:]
	return v
}

func (r *Reader) Int() int {
	v, n := binary.Varint(r.Buf)
	if n <= 0 {
		r.fail()
		return 0
	}
	r.Buf = r.Buf[n:]
	return int(v)
}

func (r *Reader) Bool() bool {
	if len(r.Buf) < 1 {
		r.fail()
		return false
	}
	v := r.Buf[0] != 0
	r.Buf = r.Buf[1:]
	return v
}

func (r *Reader) Float() float64 {
	if len(r.Buf) < 8 {
		r.fail()
		return 0
	}
	v := math.Float64frombits(binary.LittleEndian.Uint64(r.Buf))
	r.Buf = r.Buf[8:]
	return v
}

// raw returns the next n bytes, aliasing the message.
func (r *Reader) raw() []byte {
	n := r.Uvarint()
	if uint64(len(r.Buf)) < n {
		r.fail()
		return nil
	}
	b := r.Buf[:n:n]
	r.Buf = r.Buf[n:]
	return b
}

func (r *Reader) String() string { return string(r.raw()) }

// Bytes returns a copy of the next bytes, which outlives the message.
func (r *Reader) Bytes() []byte {
	b := r.raw()
	if b == nil {
		return nil
	}
	return append([]byte(nil), b...)
}

func (r *Reader) Strings() []string {
	n := r.Uvarint()
	if n > uint64(len(r.Buf)) {
		r.fail()
		return nil
	}
	s := make([]string, 0, n)
	for range n {
		s = append(s, r.String())
	}
	return s
}

func (r *Reader) Floats() []float64 {
	n := r.Uvarint()
	if n > uint64(len(r.Buf))/8 {
		r.fail()
		return nil
	}
	f := make([]float64, n)
	for i := range f {
		f[i] = r.Float()
	}
	return f
}
