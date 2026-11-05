package terminal

import "github.com/egoist/mygo/ui"

type contrastPair struct{ fg, bg ui.Color }

// adaptiveContrast is the contrast AdaptiveColors keeps text at with the
// panels it moves; Options.MinimumContrast is the terminal's own
// (contrasted, in resolve).
const adaptiveContrast = 4.5

// readableForeground keeps text readable with AdaptiveColors. Faint text is
// flattened onto its background first, so the check measures the color that
// shows. Pairs are cached across rows, bounded for long sessions with many
// RGB colors; adjustment happens when changed rows are built, never when
// they are drawn.
func (v *view) readableForeground(fg, bg ui.Color) ui.Color {
	if !v.adaptiveColors {
		return fg
	}
	key := contrastPair{fg, bg}
	if c, ok := v.contrastCache[key]; ok {
		return c
	}
	visible := fg
	if fg.A < 255 {
		visible = bg.Mix(ui.RGB(fg.R, fg.G, fg.B), float32(fg.A)/255)
	}
	c := fg
	if contrast(luminance(visible), luminance(bg)) < adaptiveContrast {
		c = contrasted(visible, bg, adaptiveContrast)
	}
	if v.contrastCache == nil {
		v.contrastCache = make(map[contrastPair]ui.Color)
	} else if len(v.contrastCache) >= 512 {
		clear(v.contrastCache)
	}
	v.contrastCache[key] = c
	return c
}

// Programs can retain a light RGB panel after the surrounding
// terminal becomes dark, or vice versa. Mirror its offset from the previous
// background onto the current one. Only neutral colors with the opposite
// light/dark polarity qualify; colored panels and matching colors stay put.
func (v *view) adaptiveBackground(bg ui.Color) ui.Color {
	if !v.adaptiveColors || max(bg.R, bg.G, bg.B)-min(bg.R, bg.G, bg.B) > 12 {
		return bg
	}
	current := color(v.colors.Background)
	lb, lc := luminance(bg), luminance(current)
	if !(lb >= .5 && lc < .18 || lb < .18 && lc >= .5) {
		return bg
	}
	previous := v.oppositeBackground
	channel := func(value, from, to uint8) uint8 {
		delta := min(max(int(value)-int(from), -64), 64)
		return uint8(min(max(int(to)-delta, 0), 255))
	}
	return ui.RGB(channel(bg.R, previous.R, current.R), channel(bg.G, previous.G, current.G), channel(bg.B, previous.B, current.B))
}
