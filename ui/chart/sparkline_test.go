package chart

import (
	"github.com/egoist/mygo/ui"
	"math"
	"testing"
)

func TestSparklineColorsAndGaps(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		Sparkline(c, []SparkSeries{
			{X: []float32{0, .25, .5, .75, 1}, Y: []float32{.25, .25, float32(math.NaN()), .25, .25}, Color: ui.RGB(255, 0, 0)},
			{X: []float32{0, 1}, Y: []float32{.75, .75}, Color: ui.RGB(0, 255, 0)},
		}, SparkOptions{LineWidth: 2, LastRadius: 2}).Size(100, 100)
	}, 100, 100)
	if got := pixel(tt, 10, 75); got[0] != 255 || got[1] != 0 {
		t.Fatal("upload color", got)
	}
	if got := pixel(tt, 10, 25); got[1] != 255 || got[0] != 0 {
		t.Fatal("download color", got)
	}
	if got := pixel(tt, 50, 75); got[0] == 255 && got[1] == 0 {
		t.Fatal("interpolated through gap", got)
	}
}
