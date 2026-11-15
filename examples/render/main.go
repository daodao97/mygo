// Render makes a video of native UI with the render plugin: a six second
// promo at 1920×1080 and 30 frames a second, with sound, whose frames
// draw on the GPU, as a window's do, and encode with the system's encoder.
//
//	go run ./examples/render                    # promo.mp4
//	go run ./examples/render -o promo.mov -codec prores
//	go run ./examples/render -o frames/%04d.png # PNGs
//	go run ./examples/render -renderer cpu      # on the CPU
//	go run ./examples/render -preview           # plays it in a window
//
// The view is package ui's: text, boxes and real widgets, a switch, a
// slider and a progress bar, moved by the frame's number (Interpolate,
// Spring) and by ui's own animations, which follow the video's clock. The
// sound is made here too: plucks of a sine, played where the scenes
// start.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/render"
	"github.com/egoist/mygo/ui"
)

const (
	fps    = 30
	frames = 6 * fps
)

var (
	accent = ui.Hex("#22d3ee")
	ink    = ui.Hex("#0b1120")
	muted  = ui.Hex("#94a3b8")
)

// promo is the composition: a title, a card of widgets, and where its
// frames are drawn.
func promo(pluck *render.Sound) render.Composition {
	return render.Composition{
		Width: 1920, Height: 1080, FPS: fps, Frames: frames, Scale: 2,
		Dark: true, Background: ink,
		View: func(c *ui.Context, f render.Frame) {
			f.Sequence(c, 0, 75, func(f render.Frame) { title(c, f, pluck) })
			f.Sequence(c, 60, 75, func(f render.Frame) { card(c, f, pluck) })
			f.Sequence(c, 125, 0, func(f render.Frame) { outro(c, f, pluck) })
		},
	}
}

// title springs the name in, and fades the scene out at its end.
func title(c *ui.Context, f render.Frame, pluck *render.Sound) {
	f.Audio(pluck)
	in := f.Spring(ui.Bouncy.Spring)
	out := f.Interpolate([]float64{60, 74}, []float64{1, 0})
	tag, _ := f.Shift(12, 0)
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(14).Opacity(float32(out)).Children(func() {
		scale := float32(0.6 + 0.4*in)
		ui.Text(c, "MyGo").FontSize(120).Bold().LetterSpacing(-3).Scale(scale, scale).Opacity(float32(min(in*1.5, 1)))
		ui.Text(c, "Native UI, rendered to video").FontSize(30).TextColor(muted).
			Opacity(float32(tag.Interpolate([]float64{0, 12}, []float64{0, 1}))).
			Translate(0, float32(tag.Interpolate([]float64{0, 12}, []float64{24, 0}, render.Interpolation{Easing: ui.EaseOut})))
	})
}

// card shows widgets at work: the slider and the bar follow the frame,
// and the switch turns on halfway, with ui's own animation.
func card(c *ui.Context, f render.Frame, pluck *render.Sound) {
	f.Audio(pluck, render.AudioOptions{Volume: 0.7})
	rise := f.Spring(ui.Snappy.Spring)
	out := f.Interpolate([]float64{60, 74}, []float64{1, 0})
	level := f.Interpolate([]float64{8, 50}, []float64{0.1, 0.85}, render.Interpolation{Easing: ui.EaseInOut})
	on := f.Index >= 30
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Opacity(float32(out)).Children(func() {
		ui.Column(c).Width(460).Padding(28).Gap(20).Radius(24).Background(ui.Hex("#111827")).
			Border(1, ui.Hex("#1f2937")).Shadow(0, 24, 60, 0, ui.RGBA(0, 0, 0, 0.5)).
			Translate(0, float32(80*(1-rise))).Opacity(float32(min(rise*2, 1))).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Export").FontSize(26).Bold()
				ui.Spacer(c)
				ui.Switch(c, &on)
			})
			ui.Text(c, fmt.Sprintf("Quality %.0f%%", level*100)).TextColor(muted)
			ui.Slider(c, &level, 0, 1)
			done := f.Interpolate([]float64{20, 70}, []float64{0, 1})
			ui.Progress(c, done)
			ui.Row(c).Gap(10).Children(func() {
				ui.Badge(c, fmt.Sprintf("frame %d", f.Index))
				ui.Spacer(c)
				ui.Button(c, "Cancel")
				ui.PrimaryButton(c, "Render")
			})
		})
	})
}

// outro names the renderers, one after another.
func outro(c *ui.Context, f render.Frame, pluck *render.Sound) {
	f.Audio(pluck, render.AudioOptions{Volume: 0.8})
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(18).Children(func() {
		ui.Text(c, "Drawn on the GPU").FontSize(56).Bold().Opacity(float32(f.Interpolate([]float64{0, 10}, []float64{0, 1})))
		ui.Row(c).Gap(16).Children(func() {
			for i, name := range []string{"Metal", "Direct3D 11", "OpenGL", "CPU"} {
				g, _ := f.Shift(8+5*i, 0)
				s := g.Spring(ui.Snappy.Spring)
				ui.Text(c, name).FontSize(26).TextColor(accent).Padding(10, 18).Radius(14).
					Background(ui.RGBA(34, 211, 238, 0.12)).Opacity(float32(min(s*2, 1))).Translate(0, float32(30*(1-s)))
			}
		})
	})
}

// newPluck makes a pluck: a sine of A4 with its octave, dying away.
func newPluck() *render.Sound {
	const rate = 48000
	pcm := make([]float32, rate*6/10)
	for i := range pcm {
		t := float64(i) / rate
		env := math.Exp(-7*t) * min(t/0.004, 1)
		pcm[i] = float32(0.3 * env * (math.Sin(2*math.Pi*440*t) + 0.4*math.Sin(2*math.Pi*880*t)))
	}
	return render.SoundOf(rate, 1, pcm)
}

func main() {
	out := flag.String("o", "promo.mp4", "the video (.mp4, .mov, .webm), or images with a verb for the frame's number (frames/%04d.png)")
	codec := flag.String("codec", "", "h264, hevc, prores, prores4444 or vp9")
	renderer := flag.String("renderer", "auto", "auto, gpu or cpu")
	preview := flag.Bool("preview", false, "play the video in a window")
	ffmpeg := flag.Bool("ffmpeg", false, "encode with ffmpeg")
	flag.Parse()

	comp := promo(newPluck())
	if *preview {
		previewWindow(comp)
		return
	}
	o := render.Options{
		Renderer: map[string]render.Renderer{"auto": render.Auto, "gpu": render.GPU, "cpu": render.CPU}[*renderer],
		Codec: map[string]render.Codec{"h264": render.H264, "hevc": render.HEVC, "prores": render.ProRes,
			"prores4444": render.ProRes4444, "vp9": render.VP9}[*codec],
		Progress: func(p render.Progress) {
			if p.Done%fps == 0 || p.Done == p.Frames {
				fmt.Printf("\r%s: %d/%d frames, %.0f fps", p.Renderer, p.Done, p.Frames, float64(p.Done)/p.Elapsed.Seconds())
			}
		},
	}
	if *ffmpeg {
		o.Encoder = render.FFmpeg
	}
	start := time.Now()
	var err error
	if strings.Contains(*out, "%") {
		err = render.Images(context.Background(), comp, *out, o)
	} else {
		err = render.Video(context.Background(), comp, *out, o)
	}
	fmt.Println()
	if err != nil {
		log.Fatal(err)
	}
	abs, _ := filepath.Abs(*out)
	fmt.Printf("%s in %v\n", abs, time.Since(start).Round(time.Millisecond))
}

// previewWindow plays the composition in a window, with controls to pause
// it and seek.
func previewWindow(comp render.Composition) {
	player, err := render.NewPlayer(comp)
	if err != nil {
		log.Fatal(err)
	}
	player.SetLoop(true)
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title: "Render preview", Width: 1000, Height: 640,
			Content: ui.View(func(c *ui.Context) { previewView(c, player) }),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}

func previewView(c *ui.Context, p *ui.VideoPlayer) {
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Video(c, p).Grow(1).FillWidth()
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			label := "Pause"
			if !p.Playing() {
				label = "Play"
			}
			if ui.Button(c, label).Clicked() {
				if p.Playing() {
					p.Pause()
				} else {
					p.Play()
				}
			}
			at := p.Position().Seconds()
			if ui.Slider(c, &at, 0, p.Duration().Seconds()).Grow(1).Changed() {
				p.Seek(time.Duration(at * float64(time.Second)))
			}
			ui.Text(c, fmt.Sprintf("frame %d", int(p.Position().Seconds()*fps))).TextColor(muted).Width(90)
		})
	})
}
