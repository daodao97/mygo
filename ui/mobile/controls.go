package mobile

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Switch shows a switch of the system's size and color: UISwitch's on
// Cupertino, green when on, Material's on Material. Changed reports a change.
func Switch(c *ui.Context, on *bool) ui.Element {
	t := c.Theme()
	w, h := float32(51), float32(31)
	if Current() == Material {
		w, h = 52, 32
	}
	sw := ui.SwitchBase(c, on).Size(w, h).Radius(h / 2)
	if sw.Changed() {
		mygo.Haptics.Light()
	}
	pos := sw.Animate("knob", onFloat(*on), 180*time.Millisecond)
	bg := t.Border
	onColor := t.Accent
	if Current() == Cupertino {
		onColor = ui.Hex("#34C759")
	}
	sw.Background(bg.Mix(onColor, pos))
	sw.Draw(func(p *ui.Painter, r ui.Rect) {
		in := float32(2)
		d := r.H - 2*in
		if Current() == Material {
			// Grows as it turns on.
			d = 16 + 8*pos
			in = (r.H - d) / 2
		}
		k := ui.Rect{X: r.X + in + pos*(r.W-d-2*in), Y: r.Y + in, W: d, H: d}
		p.Shadow(k, d/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.25))
		p.Fill(k, ui.RGB(255, 255, 255), d/2)
	})
	return sw
}

func onFloat(on bool) float32 {
	if on {
		return 1
	}
	return 0
}
