package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/render"
	"github.com/egoist/mygo/ui"
)

// The promo's scenes draw where they should, and its preview plays it.
func TestPromo(t *testing.T) {
	comp := promo(newPluck())
	bg := [4]uint8{ink.R, ink.G, ink.B, 255}
	for _, c := range []struct {
		frame int
		x, y  int
		drawn bool
	}{
		{0, 960, 540, false},  // the title, not in yet
		{30, 960, 500, true},  // the title
		{100, 960, 540, true}, // the card
		{179, 960, 480, true}, // the outro
	} {
		img, err := render.Still(comp, c.frame, render.Options{Renderer: render.CPU})
		if err != nil {
			t.Fatal(err)
		}
		changed := false
		for dy := -40; dy <= 40 && !changed; dy++ {
			for dx := -200; dx <= 200; dx += 4 {
				i := img.PixOffset(c.x+dx, c.y+dy)
				if [4]uint8(img.Pix[i:i+4]) != bg {
					changed = true
					break
				}
			}
		}
		if changed != c.drawn {
			t.Errorf("frame %d: something drawn %v, want %v", c.frame, changed, c.drawn)
		}
	}

	player, err := render.NewPlayer(comp)
	if err != nil {
		t.Fatal(err)
	}
	defer player.Close()
	tt := ui.NewTester(func(c *ui.Context) { previewView(c, player) }, 800, 520)
	if err := tt.Click("Pause"); err != nil {
		t.Fatal(err)
	}
	if player.Playing() {
		t.Error("playing after Pause")
	}
	player.Seek(3 * time.Second)
	tt.Frame()
	if !tt.HasText("frame 90") {
		t.Errorf("after seeking to 3 s: %v", tt.Texts())
	}
}
