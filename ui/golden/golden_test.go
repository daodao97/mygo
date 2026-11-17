package golden

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDiff(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 2, 2))
	b := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if n, total := Diff(a, b, 0); n != 0 || total != 4 {
		t.Errorf("same: %d of %d", n, total)
	}
	b.Set(1, 1, color.RGBA{10, 0, 0, 0})
	if n, _ := Diff(a, b, 0); n != 1 {
		t.Errorf("one pixel: %d", n)
	}
	if n, _ := Diff(a, b, 10); n != 0 {
		t.Errorf("within tolerance: %d", n)
	}
	c := image.NewRGBA(image.Rect(0, 0, 3, 2))
	if n, total := Diff(a, c, 0); n != 2 || total != 6 {
		t.Errorf("sizes: %d of %d", n, total)
	}
}

func TestMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "box.png")
	view := func(c *ui.Context) { ui.Box(c).Fill().Background(ui.RGB(10, 20, 30)) }
	t.Setenv("MYGO_UPDATE_GOLDEN", "1")
	Match(t, ui.NewTester(view, 40, 40), path)
	t.Setenv("MYGO_UPDATE_GOLDEN", "")
	Match(t, ui.NewTester(view, 40, 40), path)
}
