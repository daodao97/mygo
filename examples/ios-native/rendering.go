package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"io"
	"net/http"
	"time"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/*.png assets/*.jpg assets/*.webp assets/*.gif
var imageAssets embed.FS

const sampleImageURL = "https://go.dev/doc/gopher/frontpage.png"

var imageFormats = []string{"PNG", "JPEG", "WebP", "EXIF", "GIF"}
var imageFits = []string{"Contain", "Cover", "Stretch", "Scale Down", "Natural"}
var imageSamples = []*ui.Bitmap{
	imageAsset("pattern.png"), imageAsset("pattern.jpg"), imageAsset("pattern.webp"),
	imageAsset("oriented.jpg"), imageAsset("pattern.gif"),
}
var transparentSample = imageAsset("transparent.png")
var colorSample = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 120">
<defs><linearGradient id="sky"><stop stop-color="#367cce"/><stop offset="1" stop-color="#845dd6"/></linearGradient>
<clipPath id="frame"><rect width="240" height="120" rx="18"/></clipPath>
<mask id="fade"><rect width="240" height="120" fill="white"/><circle cx="210" cy="28" r="14" fill="black"/></mask></defs>
<g clip-path="url(#frame)" mask="url(#fade)"><rect width="240" height="120" fill="url(#sky)"/>
<circle cx="52" cy="35" r="17" fill="#f3c64e"/><path d="M0 120 85 55 130 100 180 48 240 120Z" fill="#32a582"/>
<path d="M0 120 95 85 150 120Z" fill="#e85b55" opacity="0.8"/></g></svg>`))

// These controls and decoded textures are transient demo state. Router,
// editor and scroll checkpoints continue to use the app's registered state.
type mediaState struct {
	format, fit              int
	gray, faded              bool
	selected, inlineClicks   int
	grid                     ui.GridState
	decodeResult, linkResult string
	url                      string
	remote                   *ui.Bitmap
	loading                  bool
	remoteResult             string
	remoteSource             string
}

func imageAsset(name string) *ui.Bitmap {
	data, err := imageAssets.ReadFile("assets/" + name)
	if err != nil {
		panic(err)
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		panic(err)
	}
	return b
}

func (s *demo) mediaState() *mediaState {
	if s.media == nil {
		s.media = &mediaState{selected: -1, url: sampleImageURL}
		s.media.grid.Selected = &s.media.selected
		s.media.grid.Label = func(i int) string { return fmt.Sprintf("Thumbnail %03d", i+1) }
	}
	return s.media
}

func (s *demo) imagesView(c *ui.Context, path string) {
	m := s.mediaState()
	ui.Scroll(c).HideScrollbars().TrackScroll(s.pageScroll(path)).Fill().Padding(20).Gap(14).Children(func() {
		ui.Text(c, "Raster Images").FontSize(24).Bold()
		ui.Text(c, "Change the format and fit. All samples have the same four colors.").FontSize(14)
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for i, name := range imageFormats {
				b := ui.Button(c, name).Height(36)
				if i == m.format {
					b.Background(c.Theme().Accent).TextColor(c.Theme().AccentText)
				}
				if b.Clicked() {
					m.format = i
				}
			}
		})
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for i, name := range imageFits {
				b := ui.Button(c, name).Height(36)
				if i == m.fit {
					b.Background(c.Theme().Accent).TextColor(c.Theme().AccentText)
				}
				if b.Clicked() {
					m.fit = i
				}
			}
		})
		ui.Textf(c, "Format: %s · Fit: %s", imageFormats[m.format], imageFits[m.fit]).FontSize(13)
		preview := ui.Image(c, imageSamples[m.format]).Fit(ui.Fit(m.fit)).Height(150).FillWidth().Radius(12).Background(ui.Hex("#e7ebf2")).Label("Image preview")
		if m.gray {
			preview.Grayscale()
		}
		ui.Checkbox(c, &m.gray, "Grayscale")
		ui.Text(c, "EXIF turns portrait pixels upright. GIF shows its first frame.").FontSize(13).TextColor(c.Theme().TextMuted)
		ui.Link(c, "Open thumbnail grid", path+"/grid").Padding(10, 0)
		ui.Text(c, "Transparency & SVG").FontSize(20).Bold()
		ui.Row(c).Gap(14).Children(func() {
			ui.Image(c, transparentSample).Size(100, 100).Background(ui.Hex("#f3c64e")).Radius(12).Label("Transparent PNG")
			ui.Image(c, colorSample).Grow(1).MinWidth(0).Height(100).Fit(ui.Contain).Label("Color SVG")
		})
		ui.Text(c, "Network Image").FontSize(20).Bold()
		address := ui.TextInput(c, &m.url).Label("Remote image URL").InputOptions(ui.InputOptions{Keyboard: ui.KeyboardURL, Return: ui.ReturnDone, Correction: ui.CorrectionOff, Capitalization: ui.CapitalizeNone}).Height(44).FillWidth()
		// Reveal on focus and keyboard/rotation resize, then allow the user
		// to scroll freely while the field remains focused.
		viewport := ui.Local(address, "viewport", func() inputViewport { return inputViewport{} })
		w, h := c.Size()
		focused := address.Focused()
		if focused && (!viewport.focused || viewport.width != w || viewport.height != h) {
			address.ScrollIntoView()
		}
		viewport.focused, viewport.width, viewport.height = focused, w, h
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Load remote image").Height(44).Disabled(m.loading).Clicked() {
				s.loadRemoteImage(m)
			}
			if ui.Button(c, "Clear image").Height(44).Disabled(m.loading).Clicked() {
				m.remote, m.remoteResult = nil, "Image cleared"
			}
		})
		if m.remote != nil {
			ui.Image(c, m.remote).Height(150).FillWidth().Fit(ui.Contain).Radius(12).Label("Remote image")
			if m.remoteSource == sampleImageURL {
				ui.Link(c, "Go gopher by Renée French · CC BY 4.0", "https://go.dev/blog/gopher").FontSize(12)
			}
		} else {
			ui.Box(c).Height(100).FillWidth().Radius(12).Background(c.Theme().Surface).Center().Children(func() {
				label := "Nothing loaded yet"
				if m.loading {
					label = "Loading image…"
				}
				ui.Text(c, label)
			})
		}
		ui.Text(c, m.remoteResult).FontSize(13)
		if ui.Button(c, "Decode invalid image").Height(44).Clicked() {
			if _, err := ui.DecodeBitmap([]byte("invalid image")); err != nil {
				m.decodeResult = "Decode failed: invalid image data"
			}
		}
		ui.Text(c, m.decodeResult).FontSize(13)
	})
}

type inputViewport struct {
	focused       bool
	width, height float32
}

// Fetch and decode off the UI thread; completion runs through Window.Update.
// Hold the media state identity so a test reset cannot receive an old result.
func (s *demo) loadRemoteImage(m *mediaState) {
	if m.loading {
		return
	}
	m.loading, m.remoteResult = true, "Loading image…"
	target := m.url
	go func() {
		b, err := fetchImage(&http.Client{Timeout: 30 * time.Second}, target)
		s.window.Update(func() {
			if s.media != m {
				return
			}
			m.loading = false
			if err != nil {
				m.remoteResult = "Remote image failed: " + err.Error()
			} else {
				m.remote, m.remoteResult = b, "Remote image loaded"
				m.remoteSource = target
			}
		})
	}()
}

func fetchImage(client *http.Client, target string) (*ui.Bitmap, error) {
	resp, err := client.Get(target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	const maxBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("image exceeds 4 MB")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 4_000_000 {
		return nil, fmt.Errorf("image dimensions too large")
	}
	return ui.DecodeBitmap(data)
}

func (s *demo) thumbnailView(c *ui.Context) {
	m := s.mediaState()
	ui.Column(c).Fill().Padding(20).Gap(14).Children(func() {
		ui.Text(c, "Image Grid").FontSize(24).Bold()
		ui.Text(c, "Scroll 200 thumbnails and tap to select.").FontSize(14)
		ui.Textf(c, "Selected thumbnail: %d", m.selected+1)
		ui.GridView(c, &m.grid, 200, 130, 110, func(i int) {
			ui.Image(c, imageSamples[i%len(imageSamples)]).Fit(ui.Cover).Grow(1).FillWidth().Radius(8)
			ui.Textf(c, "Thumbnail %03d", i+1).FontSize(12)
		}).HideScrollbars().Grow(1).MinHeight(0).Gap(10).Label("Thumbnail grid")
	})
}

func (s *demo) linksView(c *ui.Context, path string) {
	m := s.mediaState()
	ui.Scroll(c).HideScrollbars().TrackScroll(s.pageScroll(path)).Fill().Padding(20).Gap(18).Children(func() {
		ui.Text(c, "Links & Text").FontSize(24).Bold()
		appCard(c, func() {
			ui.Text(c, "Inline Links").FontSize(18).Bold()
			ui.RichText(c).FontSize(17).LineHeight(1.6).Children(func() {
				ui.Text(c, "Tap ")
				if ui.Link(c, "this inline action", "").Clicked() {
					m.inlineClicks++
				}
				ui.Text(c, " while retaining its touch target on every line. You can also ")
				ui.Link(c, "open linked detail", "links/detail?source=inline")
				ui.Text(c, " without leaving the app.")
			})
			ui.RichText(c).FontSize(18).LineHeight(1.6).Children(func() {
				if ui.Link(c, "a long inline link that wraps across multiple lines", "").Clicked() {
					m.inlineClicks++
				}
			})
			ui.Textf(c, "Inline actions: %d", m.inlineClicks)
		})
		appCard(c, func() {
			ui.Text(c, "Styled Paragraph").FontSize(18).Bold()
			ui.RichText(c,
				ui.Span{Text: "Bold", Weight: 700}, ui.Span{Text: ", "},
				ui.Span{Text: "italic", Italic: true}, ui.Span{Text: ", "},
				ui.Span{Text: "underline", Underline: true, Color: c.Theme().Accent},
				ui.Span{Text: " and "}, ui.Span{Text: "highlight", Background: ui.Hex("#f3c64e"), Color: ui.Hex("#172238")},
				ui.Span{Text: " share one paragraph.\nEmoji and font fallback: 👋 🌍 ✓"},
			).FontSize(18).LineHeight(1.6)
		})
		ui.Text(c, "External Links").FontSize(18).Bold()
		ui.Link(c, "Open example.com", "https://example.com").Padding(12, 0)
		ui.Text(c, "Opens the system browser. Return to keep this page and its scroll position.").FontSize(14)
		if ui.Button(c, "Try unavailable URL").Height(44).Clicked() {
			c.OpenURLThen("mygo-missing-handler://test", func(err error) {
				if err != nil {
					m.linkResult = "URL open failed"
				} else {
					m.linkResult = "URL opened"
				}
			})
		}
		ui.Text(c, m.linkResult)
	})
}

func (s *demo) linkDetailView(c *ui.Context, path string) {
	ui.Scroll(c).HideScrollbars().TrackScroll(s.pageScroll(path)).Fill().Padding(20).Gap(18).Children(func() {
		ui.Text(c, "Linked Page").FontSize(24).Bold()
		ui.Text(c, "Opened via a relative router link with a query string. Use Back or the left-edge gesture to return.").FontSize(16).LineHeight(1.5)
		ui.Text(c, "Source: "+s.app.Routers[s.app.Tab].Query("source"))
		ui.Link(c, "View image examples", tabRoots[s.app.Tab]+"/images").Padding(12, 0)
	})
}

func (s *demo) renderingView(c *ui.Context, path string) {
	m := s.mediaState()
	ui.Scroll(c).HideScrollbars().TrackScroll(s.pageScroll(path)).Fill().Padding(20).Gap(18).Children(func() {
		ui.Text(c, "Rendering Samples").FontSize(24).Bold()
		ui.Text(c, "Gradient & Shadow").FontSize(18).Bold()
		ui.Box(c).Height(120).FillWidth().Gradient(ui.Hex("#367cce"), ui.Hex("#845dd6"), 90).Radius(18).Shadow(0, 6, 12, 0, ui.Hex("#17223855")).Label("Gradient sample").Role(ui.RoleImage).Center().Children(func() {
			ui.Text(c, "Smooth gradient").FontSize(20).Bold().TextColor(ui.Hex("#ffffff"))
		})
		ui.Text(c, "Group Opacity").FontSize(18).Bold()
		if ui.Button(c, "Toggle opacity").Height(44).Clicked() {
			m.faded = !m.faded
		}
		opacity := float32(1)
		if m.faded {
			opacity = 0.4
		}
		ui.Textf(c, "Opacity: %.1f", opacity)
		ui.Row(c).Padding(16).Gap(12).Radius(16).Background(ui.Hex("#367cce")).Opacity(opacity).Role(ui.RoleImage).Label("Opacity sample").Children(func() {
			ui.Image(c, colorSample).Size(96, 48)
			ui.Text(c, "Text and image fade together").Grow(1).MinWidth(0).TextColor(ui.Hex("#ffffff"))
		})
		ui.Text(c, "Rounded Clip").FontSize(18).Bold()
		ui.Box(c).Height(110).FillWidth().Radius(24).Clip().Background(ui.Hex("#e7ebf2")).Role(ui.RoleImage).Label("Rounded clip sample").Children(func() {
			ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
				p.Image(imageSamples[0], ui.Rect{X: r.X - 25, Y: r.Y - 25, W: r.W + 50, H: r.H + 50}, ui.Cover)
				p.Fill(ui.Rect{X: r.X + r.W - 55, Y: r.Y - 20, W: 90, H: 90}, ui.Hex("#ffffffaa"), 45)
			})
		})
		ui.Text(c, "Custom Drawing").FontSize(18).Bold()
		ui.Box(c).Height(140).FillWidth().Radius(14).Background(c.Theme().Surface).Clip().Role(ui.RoleImage).Label("Drawing sample").Draw(func(p *ui.Painter, r ui.Rect) {
			for i := 0; i < 5; i++ {
				h := float32(20 + i*18)
				p.Fill(ui.Rect{X: r.X + 20 + float32(i)*(r.W-40)/5, Y: r.Y + r.H - 20 - h, W: (r.W - 60) / 5, H: h}, ui.Hex("#32a582"), 5)
			}
			p.Line(r.X+12, r.Y+r.H-16, r.X+r.W-12, r.Y+r.H-16, 1, c.Theme().TextMuted)
		})
	})
}
