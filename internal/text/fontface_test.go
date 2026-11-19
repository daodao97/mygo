package text

import (
	"encoding/binary"
	"math/rand/v2"
	"testing"
)

// sfntOf builds a font file of tables, by tag, sorted as given.
func sfntOf(tags []string, tables map[string][]byte) []byte {
	be := binary.BigEndian
	out := be.AppendUint32(nil, 0x00010000)
	out = be.AppendUint16(out, uint16(len(tags)))
	out = append(out, make([]byte, 6)...)
	off := 12 + 16*len(tags)
	for _, tag := range tags {
		out = append(out, tag...)
		out = be.AppendUint32(out, 0)
		out = be.AppendUint32(out, uint32(off))
		out = be.AppendUint32(out, uint32(len(tables[tag])))
		off += len(tables[tag])
	}
	for _, tag := range tags {
		out = append(out, tables[tag]...)
	}
	return out
}

// os2Of returns an OS/2 table (version 4) of a weight and width class
// and fsSelection.
func os2Of(weight, width, selection uint16) []byte {
	t := make([]byte, 96)
	binary.BigEndian.PutUint16(t[0:], 4)
	binary.BigEndian.PutUint16(t[4:], weight)
	binary.BigEndian.PutUint16(t[6:], width)
	binary.BigEndian.PutUint16(t[62:], selection)
	return t
}

// headOf returns a head table of a macStyle.
func headOf(macStyle uint16) []byte {
	t := make([]byte, 54)
	binary.BigEndian.PutUint16(t[0:], 1)
	binary.BigEndian.PutUint16(t[44:], macStyle)
	return t
}

type axis struct {
	tag         string
	lo, def, hi float64
}

// fvarOf returns an fvar table of axes.
func fvarOf(axes ...axis) []byte {
	be := binary.BigEndian
	t := be.AppendUint16(nil, 1)
	t = be.AppendUint16(t, 0)
	t = be.AppendUint16(t, 16) // axes' offset
	t = be.AppendUint16(t, 2)
	t = be.AppendUint16(t, uint16(len(axes)))
	t = be.AppendUint16(t, 20)
	t = be.AppendUint16(t, 0)
	t = be.AppendUint16(t, 0)
	for _, a := range axes {
		t = append(t, a.tag...)
		for _, v := range []float64{a.lo, a.def, a.hi} {
			t = be.AppendUint32(t, uint32(int32(v*65536)))
		}
		t = be.AppendUint16(t, 0)
		t = be.AppendUint16(t, 256)
	}
	return t
}

func TestFaceDescriptors(t *testing.T) {
	regular := sfntOf([]string{"OS/2", "head"}, map[string][]byte{"OS/2": os2Of(400, 5, 0x40), "head": headOf(0)})
	bold := sfntOf([]string{"OS/2", "head"}, map[string][]byte{"OS/2": os2Of(700, 5, 0x20), "head": headOf(1)})
	// A collection of the bold one alone.
	ttc := append([]byte("ttcf\x00\x01\x00\x00\x00\x00\x00\x01\x00\x00\x00\x10"), bold...)
	for i := range 2 {
		// Its tables' offsets are from the collection's start.
		rec := 16 + 12 + 16*i
		binary.BigEndian.PutUint32(ttc[rec+8:], binary.BigEndian.Uint32(ttc[rec+8:])+16)
	}
	for _, c := range []struct {
		name string
		data []byte
		want faceDescriptors
	}{
		{"regular", regular, faceDescriptors{"400", "100%", ""}},
		{"bold", bold, faceDescriptors{"700", "100%", ""}},
		{"collection", ttc, faceDescriptors{"700", "100%", ""}},
		{"italic", sfntOf([]string{"OS/2"}, map[string][]byte{"OS/2": os2Of(400, 5, 1)}), faceDescriptors{"400", "100%", "italic"}},
		{"italic by head", sfntOf([]string{"head", "OS/2"}, map[string][]byte{"OS/2": os2Of(300, 3, 0), "head": headOf(2)}), faceDescriptors{"300", "75%", "italic"}},
		{"oblique", sfntOf([]string{"OS/2"}, map[string][]byte{"OS/2": os2Of(400, 9, 1<<9)}), faceDescriptors{"400", "200%", "oblique"}},
		{"weight in hundreds", sfntOf([]string{"OS/2"}, map[string][]byte{"OS/2": os2Of(7, 0, 0)}), faceDescriptors{"700", "", ""}},
		{"out of range", sfntOf([]string{"OS/2"}, map[string][]byte{"OS/2": os2Of(5000, 12, 0)}), faceDescriptors{}},
		{"short OS/2", sfntOf([]string{"OS/2"}, map[string][]byte{"OS/2": os2Of(700, 5, 1)[:10]}), faceDescriptors{"700", "100%", ""}},
		{"no tables", sfntOf(nil, nil), faceDescriptors{}},
		// Archivo's axes.
		{"variable", sfntOf([]string{"OS/2", "fvar", "head"}, map[string][]byte{
			"OS/2": os2Of(400, 5, 0x40), "head": headOf(0),
			"fvar": fvarOf(axis{"wght", 100, 400, 900}, axis{"wdth", 62, 100, 125}),
		}), faceDescriptors{"100 900", "62% 125%", ""}},
		{"variable weight", sfntOf([]string{"OS/2", "fvar"}, map[string][]byte{
			"OS/2": os2Of(400, 3, 0),
			"fvar": fvarOf(axis{"opsz", 8, 14, 144}, axis{"wght", 1, 400, 1000}),
		}), faceDescriptors{"1 1000", "75%", ""}},
		{"variable italic", sfntOf([]string{"OS/2", "fvar"}, map[string][]byte{
			"OS/2": os2Of(400, 5, 1),
			"fvar": fvarOf(axis{"wght", 200, 400, 800}, axis{"ital", 1, 1, 1}),
		}), faceDescriptors{"200 800", "100%", "italic"}},
		{"variable upright and italic", sfntOf([]string{"OS/2", "fvar"}, map[string][]byte{
			"OS/2": os2Of(400, 5, 0),
			"fvar": fvarOf(axis{"wght", 200, 400, 800}, axis{"ital", 0, 0, 1}),
		}), faceDescriptors{"200 800", "100%", ""}},
		{"variable slant", sfntOf([]string{"fvar"}, map[string][]byte{
			"fvar": fvarOf(axis{"slnt", -10, 0, 0}, axis{"wdth", 25, 100, 151.5}),
		}), faceDescriptors{"", "25% 151.5%", "oblique 0deg 10deg"}},
		// An axis whose default is outside its range is ignored.
		{"bad axis", sfntOf([]string{"OS/2", "fvar"}, map[string][]byte{
			"OS/2": os2Of(500, 5, 0),
			"fvar": fvarOf(axis{"wght", 900, 400, 100}),
		}), faceDescriptors{"500", "100%", ""}},
	} {
		if got := faceDescriptorsOf(c.data); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// Malformed data gives no descriptors, or what of them is whole, and
// never panics.
func TestFaceDescriptorsMalformed(t *testing.T) {
	variable := sfntOf([]string{"OS/2", "fvar", "head"}, map[string][]byte{
		"OS/2": os2Of(400, 5, 0), "head": headOf(0),
		"fvar": fvarOf(axis{"wght", 100, 400, 900}, axis{"wdth", 62, 100, 125}, axis{"slnt", -12, 0, 0}),
	})
	for _, data := range [][]byte{nil, {}, []byte("OTTO"), []byte("wOF2 not a font at all"),
		// A collection that holds itself.
		[]byte("ttcf\x00\x01\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00"),
		[]byte("ttcf\x00\x01\x00\x00\x00\x00\x00\x01\xff\xff\xff\xff"),
	} {
		if got := faceDescriptorsOf(data); got != (faceDescriptors{}) {
			t.Errorf("%q: %+v, want none", data, got)
		}
	}
	// Cut short anywhere.
	for n := range len(variable) {
		faceDescriptorsOf(variable[:n])
	}
	// Tables past the end, and an axis size too small.
	bad := sfntOf([]string{"OS/2"}, map[string][]byte{"OS/2": os2Of(700, 5, 0)})
	binary.BigEndian.PutUint32(bad[12+8:], 1<<31)
	if got := faceDescriptorsOf(bad); got != (faceDescriptors{}) {
		t.Errorf("table past the end: %+v", got)
	}
	small := fvarOf(axis{"wght", 100, 400, 900})
	binary.BigEndian.PutUint16(small[10:], 4)
	if got := faceDescriptorsOf(sfntOf([]string{"fvar"}, map[string][]byte{"fvar": small})); got != (faceDescriptors{}) {
		t.Errorf("axes of 4 bytes: %+v", got)
	}
	// Bytes changed at random.
	r := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		b := append([]byte(nil), variable...)
		for range 1 + r.IntN(8) {
			b[r.IntN(len(b))] = byte(r.Uint32())
		}
		faceDescriptorsOf(b)
	}
}
