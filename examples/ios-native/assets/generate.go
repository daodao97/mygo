//go:build ignore

// Run from examples/ios-native: go run ./assets/generate.go.
// WebP encoding uses ffmpeg as development tooling only.
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func write(name string, b *bytes.Buffer) { must(os.WriteFile("assets/"+name, b.Bytes(), 0644)) }

func main() {
	generateBrand()
	img := image.NewRGBA(image.Rect(0, 0, 240, 120))
	colors := []color.RGBA{{232, 91, 85, 255}, {54, 124, 206, 255}, {243, 198, 78, 255}, {50, 165, 130, 255}}
	for y := 0; y < 120; y++ {
		for x := 0; x < 240; x++ {
			img.SetRGBA(x, y, colors[(y/60)*2+x/120])
		}
	}
	var b bytes.Buffer
	must(png.Encode(&b, img))
	write("pattern.png", &b)
	b.Reset()
	must(jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}))
	write("pattern.jpg", &b)
	// Stored portrait pixels, EXIF orientation 6: the decoder turns them
	// clockwise into the same upright landscape as the PNG.
	portrait := image.NewRGBA(image.Rect(0, 0, 120, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 120; x++ {
			portrait.Set(x, y, img.At(239-y, x))
		}
	}
	b.Reset()
	must(jpeg.Encode(&b, portrait, &jpeg.Options{Quality: 95}))
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	data := b.Bytes()
	out := append([]byte{}, data[:2]...)
	out = append(out, 0xff, 0xe1, 0, byte(len(exif)+2))
	out = append(out, exif...)
	out = append(out, data[2:]...)
	must(os.WriteFile("assets/oriented.jpg", out, 0644))
	pal := color.Palette{colors[0], colors[1], colors[2], colors[3]}
	frame := image.NewPaletted(img.Bounds(), pal)
	for y := 0; y < 120; y++ {
		for x := 0; x < 240; x++ {
			frame.SetColorIndex(x, y, uint8((y/60)*2+x/120))
		}
	}
	b.Reset()
	second := image.NewPaletted(img.Bounds(), pal)
	for i, v := range frame.Pix {
		second.Pix[i] = (v + 1) % 4
	}
	// Deliberately animated source: ui.DecodeBitmap displays its first frame.
	must(gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{frame, second}, Delay: []int{10, 10}, LoopCount: 0}))
	write("pattern.gif", &b)
	alpha := image.NewNRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			dx, dy := x-60, y-60
			if dx*dx+dy*dy < 52*52 {
				alpha.SetNRGBA(x, y, color.NRGBA{54, 124, 206, uint8(60 + x)})
			}
		}
	}
	b.Reset()
	must(png.Encode(&b, alpha))
	write("transparent.png", &b)
	must(exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", "assets/pattern.png", "-c:v", "libwebp", "-lossless", "1", "assets/pattern.webp").Run())
}

// Match the CLI template's blue-violet gradient and white ring. The home
// icon fills its square; iOS supplies the corners. The launch mark retains
// the template's transparent inset and rounded shape.
func generateBrand() {
	must(os.MkdirAll("resources", 0755))
	for _, launch := range []bool{false, true} {
		size := 1024
		if launch {
			size = 512
		}
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		for y := range size {
			for x := range size {
				fx, fy := (float64(x)+0.5)*1024/float64(size), (float64(y)+0.5)*1024/float64(size)
				alpha := 1.0
				if launch {
					qx, qy := math.Abs(fx-512)-227, math.Abs(fy-512)-227
					d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - 185
					alpha = min(max(0.5-d, 0), 1)
				}
				t := (fx + fy) / 2048
				ring := min(max(0.5-math.Abs(math.Hypot(fx-512, fy-512)-230)+48, 0), 1)
				channel := func(from, to float64) uint8 { return uint8((from+(to-from)*t)*(1-ring) + 255*ring) }
				img.SetNRGBA(x, y, color.NRGBA{channel(59, 168), channel(130, 85), channel(246, 247), uint8(alpha * 255)})
			}
		}
		var b bytes.Buffer
		must(png.Encode(&b, img))
		if launch {
			write("launch-logo.png", &b)
		} else {
			must(os.WriteFile("resources/icon.png", b.Bytes(), 0644))
		}
	}
}
