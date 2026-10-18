package chart

import (
	"github.com/egoist/mygo/ui"
)

// SparkSeries uses normalized x/y fractions, like Line. NaN/Inf breaks a line.
// Compute one common y scale for all series before calling Sparkline.
type SparkSeries struct {
	X, Y  []float32
	Color ui.Color
}
type SparkOptions struct {
	Curve                              Curve
	LineWidth, FillOpacity, LastRadius float32
}

// Sparkline is an axis-free multi-series plot, styled by its caller. It reuses
// chart's gap handling and paths. Dense data can first be reduced with MinMax.
func Sparkline(c *ui.Context, series []SparkSeries, options SparkOptions) ui.Element {
	return Plot(c, Axes{}, func(area ui.Element) {
		area.Draw(func(p *ui.Painter, r ui.Rect) { DrawSparkline(p, r, series, options) })
	})
}

// DrawSparkline draws inside a caller-owned chart area, for overlays and custom
// margins. Coordinates are normalized, with a shared scale supplied by callers.
func DrawSparkline(p *ui.Painter, r ui.Rect, series []SparkSeries, options SparkOptions) {
	width := options.LineWidth
	if width <= 0 {
		width = 1.6
	}
	for _, s := range series {
		if options.FillOpacity > 0 {
			p.FillPathGradient(Area(r, s.X, nil, s.Y, options.Curve), ui.LinearGradient{From: s.Color.Alpha(min(1, options.FillOpacity)), To: s.Color.Alpha(0), Angle: 180})
		}
		p.StrokePath(Line(r, s.X, s.Y, options.Curve), width, s.Color)
		n := min(len(s.X), len(s.Y))
		if n > 0 && options.LastRadius > 0 && defined(s.X[n-1], s.Y[n-1]) {
			p.FillPath((&ui.Path{}).Circle(X(r, s.X[n-1]), Y(r, s.Y[n-1]), options.LastRadius), s.Color)
		}
	}
}
