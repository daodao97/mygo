package text

import (
	"encoding/binary"
	"strconv"
)

// faceDescriptors are the CSS descriptors of the face of a font file, as
// a browser's FontFace takes them (weight, stretch and style): what the
// file covers, so that the faces registered under one family are told
// apart and a variable font gives every weight of its axis. An empty one
// leaves the browser's default (400, 100%, normal).
type faceDescriptors struct {
	weight, stretch, style string
}

// faceDescriptorsOf reads the descriptors of a TrueType or OpenType font,
// or of the first font of a collection, from its tables: the ranges of
// its "wght" and "wdth" axes, and its "ital" and "slnt" ones, for a
// variable font; the weight and width classes and italic bits of OS/2
// and head otherwise, and for the axes it lacks. Malformed data gives
// none.
func faceDescriptorsOf(data []byte) faceDescriptors {
	font := sfntFont(data)
	if font == nil {
		return faceDescriptors{}
	}
	table := func(tag string) []byte {
		n := int(sfntU16(font, 4))
		for i := range n {
			rec := sfntSub(font, 12+16*i, 16)
			if rec == nil {
				return nil
			}
			if string(rec[:4]) == tag {
				return sfntSub(data, int(binary.BigEndian.Uint32(rec[8:])), int(binary.BigEndian.Uint32(rec[12:])))
			}
		}
		return nil
	}
	var d faceDescriptors
	italic, oblique := false, false
	if os2 := table("OS/2"); len(os2) >= 8 {
		if w := int(sfntU16(os2, 4)); w >= 1 && w <= 9 {
			// Some old fonts give the class's hundreds.
			d.weight = strconv.Itoa(w * 100)
		} else if w >= 1 && w <= 1000 {
			d.weight = strconv.Itoa(w)
		}
		if w := int(sfntU16(os2, 6)); w >= 1 && w <= 9 {
			d.stretch = [...]string{"50%", "62.5%", "75%", "87.5%", "100%", "112.5%", "125%", "150%", "200%"}[w-1]
		}
		if len(os2) >= 64 {
			sel := sfntU16(os2, 62)
			italic, oblique = sel&1 != 0, sel&(1<<9) != 0
		}
	}
	if head := table("head"); len(head) >= 46 && sfntU16(head, 44)&2 != 0 {
		italic = true
	}
	switch {
	case italic:
		d.style = "italic"
	case oblique:
		d.style = "oblique"
	}
	fvar := table("fvar")
	if len(fvar) < 16 || sfntU16(fvar, 0) != 1 {
		return d
	}
	axes, count, size := int(sfntU16(fvar, 4)), int(sfntU16(fvar, 8)), int(sfntU16(fvar, 10))
	if size < 20 {
		return d
	}
	for i := range count {
		a := sfntSub(fvar, axes+i*size, 20)
		if a == nil {
			break
		}
		lo, def, hi := sfntFixed(a[4:]), sfntFixed(a[8:]), sfntFixed(a[12:])
		if lo > def || def > hi {
			continue
		}
		switch string(a[:4]) {
		case "wght":
			lo, hi = min(max(lo, 1), 1000), min(max(hi, 1), 1000)
			if lo <= hi {
				d.weight = cssNumber(lo) + " " + cssNumber(hi)
			}
		case "wdth":
			if lo > 0 {
				d.stretch = cssNumber(lo) + "% " + cssNumber(hi) + "%"
			}
		case "ital":
			// An axis from upright to italic leaves the style to the
			// file's default; one that is italic throughout is italic.
			if lo >= 1 {
				d.style = "italic"
			}
		case "slnt":
			// A slant is counterclockwise, an oblique angle clockwise.
			if d.style != "italic" && (lo != 0 || hi != 0) && lo >= -90 && hi <= 90 {
				d.style = "oblique " + cssNumber(-hi) + "deg " + cssNumber(-lo) + "deg"
			}
		}
	}
	return d
}

// sfntFont returns data from the table directory of its font, the first
// of a collection, or nil when it holds none.
func sfntFont(data []byte) []byte {
	if len(data) < 12 {
		return nil
	}
	switch string(data[:4]) {
	case "ttcf":
		if sfntU32(data, 8) == 0 {
			return nil
		}
		if off := sfntU32(data, 12); off >= 12 && off <= uint32(len(data)) {
			return sfntFont(data[off:])
		}
		return nil
	case "\x00\x01\x00\x00", "OTTO", "true":
		return data
	}
	return nil
}

// sfntSub returns n bytes of b from off, or nil if they are not all there.
func sfntSub(b []byte, off, n int) []byte {
	if off < 0 || n < 0 || off > len(b) || n > len(b)-off {
		return nil
	}
	return b[off : off+n]
}

func sfntU16(b []byte, off int) uint16 {
	if s := sfntSub(b, off, 2); s != nil {
		return binary.BigEndian.Uint16(s)
	}
	return 0
}

func sfntU32(b []byte, off int) uint32 {
	if s := sfntSub(b, off, 4); s != nil {
		return binary.BigEndian.Uint32(s)
	}
	return 0
}

// sfntFixed reads an OpenType Fixed, a signed 16.16 number.
func sfntFixed(b []byte) float64 { return float64(int32(binary.BigEndian.Uint32(b))) / 65536 }

// cssNumber formats a number for CSS, with at most three decimals.
func cssNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	for s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	if s == "-0" {
		s = "0"
	}
	return s
}
