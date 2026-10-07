package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestImageFormatRendering(t *testing.T) {
	// Real decoded fixtures, including EXIF orientation, must paint the
	// same corner colors rather than just create an accessible image node.
	for i, format := range imageFormats {
		t.Run(format, func(t *testing.T) {
			w, h := imageSamples[i].Size()
			if w != 240 || h != 120 {
				t.Fatalf("size %dx%d", w, h)
			}
			img := ui.Render(func(c *ui.Context) { ui.Image(c, imageSamples[i]).Size(240, 120) }, 240, 120, 1)
			for _, sample := range []struct {
				x, y int
				want color.RGBA
			}{
				{30, 30, color.RGBA{232, 91, 85, 255}}, {210, 30, color.RGBA{54, 124, 206, 255}},
				{30, 90, color.RGBA{243, 198, 78, 255}}, {210, 90, color.RGBA{50, 165, 130, 255}},
			} {
				got := img.RGBAAt(sample.x, sample.y)
				for channel, v := range []uint8{got.R, got.G, got.B} {
					want := []uint8{sample.want.R, sample.want.G, sample.want.B}[channel]
					if d := int(v) - int(want); d < -4 || d > 4 {
						t.Fatalf("pixel %d,%d: %v, want %v", sample.x, sample.y, got, sample.want)
					}
				}
			}
		})
	}
}

func TestRemoteImageResponses(t *testing.T) {
	data, err := imageAssets.ReadFile("assets/pattern.png")
	if err != nil {
		t.Fatal(err)
	}
	var large bytes.Buffer
	if err := png.Encode(&large, image.NewGray(image.Rect(0, 0, 4097, 1))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		status int
		data   []byte
		want   string
	}{
		{"loaded", 200, data, ""}, {"not-found", 404, data, "HTTP 404"},
		{"invalid", 200, []byte("not an image"), "unknown format"},
		{"too-many-bytes", 200, bytes.Repeat([]byte{0}, (4<<20)+1), "exceeds 4 MB"},
		{"too-many-pixels", 200, large.Bytes(), "dimensions too large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); _, _ = w.Write(tc.data) }))
			defer server.Close()
			b, err := fetchImage(&http.Client{Timeout: time.Second}, server.URL)
			if tc.want == "" {
				if err != nil || b == nil {
					t.Fatalf("load: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
	if _, err := fetchImage(&http.Client{Timeout: time.Second}, "mygo-invalid://image"); err == nil {
		t.Fatal("unsupported scheme accepted")
	}
}

func TestImageURLKeyboardViewport(t *testing.T) {
	s := &demo{}
	s.initNavigation()
	s.app.Routers[0].Transition = ui.TransitionNone
	s.app.Routers[0].Push("/home/images")
	view := ui.NewTester(s.view, 375, 728)
	view.Scroll(8, 300, 0, 420)
	field, ok := view.Find("Remote image URL")
	if !ok {
		t.Fatal("missing URL field")
	}
	view.ClickAt(field.X+field.W-5, field.Y+field.H/2)
	if !view.Focused("Remote image URL") {
		t.Fatal("URL field did not focus")
	}
	view.SetSize(375, 430) // keyboard reduces the available viewport
	field, ok = view.Find("Remote image URL")
	if !ok || field.Y < 54 || field.Y+field.H > 430-66 {
		t.Fatalf("focused URL hidden after resize: %+v", field)
	}
	view.Scroll(8, 200, 0, 60)
	moved, ok := view.Find("Remote image URL")
	if !ok || moved.Y >= field.Y-20 {
		t.Fatalf("focus pinned manual scrolling: before %+v, after %+v", field, moved)
	}
}
