// Package golden compares what a ui.Tester draws with images kept beside the
// tests, as golden files do, on the platform it runs on: text differs from one
// to another. Run the tests with MYGO_UPDATE_GOLDEN=1 to write them.
//
//	func TestCard(t *testing.T) {
//		tt := ui.NewTester(card, 320, 200)
//		golden.Match(t, tt, "testdata/card.png")
//	}
package golden

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Tolerance is how far a channel of a pixel may be from the golden's, of 255,
// and how many pixels may be further, out of a thousand.
type Tolerance struct {
	Channel  int
	PerMille int
}

// Exact asks for the same pixels.
var Exact = Tolerance{}

// Match fails t where the tester's frame differs from the image at path. A
// failure writes the frame beside it, as name.got.png, to compare.
func Match(t testing.TB, tt *ui.Tester, path string, tol ...Tolerance) {
	t.Helper()
	tt.Frame()
	img := tt.Image()
	if os.Getenv("MYGO_UPDATE_GOLDEN") != "" {
		if err := write(path, img); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v: run with MYGO_UPDATE_GOLDEN=1 to write it", err)
	}
	want, err := png.Decode(bytes.NewReader(f))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	limit := Exact
	if len(tol) > 0 {
		limit = tol[0]
	}
	if n, total := Diff(want, img, limit.Channel); n*1000 > total*limit.PerMille {
		got := path[:len(path)-len(filepath.Ext(path))] + ".got.png"
		if err := write(got, img); err != nil {
			t.Error(err)
		}
		t.Errorf("%s: %d of %d pixels differ; what it drew is in %s", path, n, total, got)
	}
}

// Diff counts the pixels that differ by more than channel in a channel, and
// the pixels of the larger image: those only one has differ.
func Diff(a, b image.Image, channel int) (n, total int) {
	ab, bb := a.Bounds(), b.Bounds()
	w, h := max(ab.Dx(), bb.Dx()), max(ab.Dy(), bb.Dy())
	total = w * h
	for y := range h {
		for x := range w {
			pa := image.Pt(ab.Min.X+x, ab.Min.Y+y)
			pb := image.Pt(bb.Min.X+x, bb.Min.Y+y)
			if !pa.In(ab) || !pb.In(bb) {
				n++
				continue
			}
			if differs(a.At(pa.X, pa.Y), b.At(pb.X, pb.Y), channel) {
				n++
			}
		}
	}
	return n, total
}

func differs(a, b color.Color, limit int) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	far := func(x, y uint32) bool {
		d := int(x>>8) - int(y>>8)
		if d < 0 {
			d = -d
		}
		return d > limit
	}
	return far(ar, br) || far(ag, bg) || far(ab, bb) || far(aa, ba)
}

func write(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}
