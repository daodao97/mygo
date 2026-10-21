package terminal

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestProgramCachedPanelThemeChanges(t *testing.T) {
	loadLib(t)
	for _, cachedDark := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached-dark-%t", cachedDark), func(t *testing.T) {
			light, dark := LightTheme(), DarkTheme()
			light.Background = ui.RGB(251, 251, 251)
			dark.Background = ui.RGB(17, 19, 21)
			term, err := New(Options{Conn: newPipe(), Theme: light, DarkTheme: dark, AdaptiveColors: true, NoBlink: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { term.Close() })
			tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 700, 220)
			tt.SetDark(cachedDark)
			tt.Frame()
			bg, fg := 240, 225
			if cachedDark {
				bg, fg = 42, 70
			}
			// Feed once: Codex keeps these startup RGB colors when the app
			// changes appearance. Erased cells cover the entire input panel.
			term.Feed([]byte(fmt.Sprintf("\x1b[?25l\x1b[H\x1b[2J\x1b[48;2;%d;%d;%d;38;2;%d;%d;%dm\x1b[2KAsk Codex to do anything\x1b[0m\r\n"+
				"\x1b[48;5;255m\x1b[2K256 color panel\x1b[0m\r\n"+
				"\x1b[107m\x1b[2KANSI white\x1b[0m\r\n"+
				"\x1b[48;2;240;100;100m\x1b[2KColored panel\x1b[0m", bg, bg, bg, fg, fg, fg)))
			before := term.Text()
			background := func(row, col int) ui.Color {
				for _, span := range term.v.lines[row].bgs {
					if col >= span.x0 && col < span.x1 {
						return span.c
					}
				}
				return color(term.v.colors.Background)
			}
			for _, targetDark := range []bool{cachedDark, !cachedDark, cachedDark} {
				tt.SetDark(targetDark)
				tt.Frame()
				panel := background(0, 0)
				original := ui.RGB(uint8(bg), uint8(bg), uint8(bg))
				if targetDark == cachedDark && panel != original {
					t.Fatalf("matching appearance changed panel: %v", panel)
				}
				if targetDark != cachedDark && (colorLuminance(panel) < .18) != targetDark {
					t.Fatalf("stale panel did not adapt: dark=%v panel=%v", targetDark, panel)
				}
				cols, _ := term.Size()
				if background(0, cols-1) != panel {
					t.Fatal("panel padding kept the cached color")
				}
				for _, run := range term.v.lines[0].runs {
					if ratio := contrastRatio(run.c, panel); ratio < 4.5 {
						t.Fatalf("unreadable placeholder: %.2f", ratio)
					}
				}
				if got := background(1, 0); targetDark && colorLuminance(got) >= .18 {
					t.Fatalf("extended gray panel did not adapt: %v", got)
				}
				if background(2, 0) != color(term.v.colors.Palette[15]) {
					t.Fatal("ordinary ANSI background was remapped")
				}
				if background(3, 0) != ui.RGB(240, 100, 100) {
					t.Fatal("colored panel was remapped")
				}
				save(t, tt, fmt.Sprintf("codex-cached-%t-theme-%t", cachedDark, targetDark))
			}
			tt.SetDark(!cachedDark)
			term.SetAdaptiveColors(false)
			tt.Frame()
			if background(0, 0) != ui.RGB(uint8(bg), uint8(bg), uint8(bg)) {
				t.Fatal("disabling Program adaptation reused adjusted rows")
			}
			if term.Text() != before {
				t.Fatal("theme adaptation changed terminal contents")
			}
		})
	}
}

func TestMinimumContrastRendering(t *testing.T) {
	loadLib(t)
	light := LightTheme()
	light.MinimumContrast = 4.5
	dark := DarkTheme()
	term, err := New(Options{Conn: newPipe(), Theme: light, DarkTheme: dark, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 700, 220)
	tt.SetDark(false)
	// Bright ANSI, the extended palette, RGB and faint text all commonly
	// appear in Program output. The fifth row uses its own dark background.
	term.Feed([]byte("\x1b[?25l\x1b[H\x1b[2J" +
		"\x1b[92mgreen \x1b[93myellow \x1b[97mwhite\x1b[0m\r\n" +
		"\x1b[38;5;154mextended green \x1b[38;5;250mgray\x1b[0m\r\n" +
		"\x1b[38;2;210;220;235mRGB text\x1b[0m\r\n" +
		"\x1b[2;38;2;130;140;150mfaint text\x1b[0m\r\n" +
		"\x1b[48;2;20;24;28;38;2;45;50;55mtext on dark background\x1b[0m\r\n" +
		"\x1b[7;38;2;210;220;235minverse text\x1b[0m\r\n" +
		"\x1b[38;2;210;220;235m█\x1b[0m\r\n" +
		"\x1b[8mconcealed\x1b[0m"))
	before := term.Text()
	tt.Frame()
	v := term.v
	for row := range 6 {
		line := v.lines[row]
		if len(line.runs) == 0 {
			t.Fatalf("row %d has no text", row)
		}
		for _, run := range line.runs {
			bg := color(v.colors.Background)
			for _, span := range line.bgs {
				if int(run.cols[0]) >= span.x0 && int(run.cols[0]) < span.x1 {
					bg = span.c
					break
				}
			}
			if ratio := contrastRatio(run.c, bg); ratio < light.MinimumContrast {
				t.Errorf("row %d: text %v on %v has contrast %.2f", row, run.c, bg, ratio)
			}
		}
	}
	if boxes := v.lines[6].boxes; len(boxes) != 1 || boxes[0].c != ui.RGB(210, 220, 235) {
		t.Fatalf("contrast correction changed block artwork: %+v", boxes)
	}
	if len(v.lines[7].runs) != 0 {
		t.Fatal("contrast correction revealed concealed text")
	}
	if term.Text() != before {
		t.Fatal("contrast correction changed copied terminal content")
	}
	tt.SetDark(true)
	tt.Frame()
	if got := v.lines[2].runs[0].c; got != ui.RGB(210, 220, 235) {
		t.Fatalf("dark theme changed RGB text: %v", got)
	}
	if got := v.lines[3].runs[0].c; got != ui.RGB(130, 140, 150).Alpha(.5) {
		t.Fatalf("dark theme changed faint text: %v", got)
	}
	tt.SetDark(false)
	tt.Frame()
	if ratio := contrastRatio(v.lines[2].runs[0].c, light.Background); ratio < 4.5 {
		t.Fatalf("switching back reused dark text: contrast %.2f", ratio)
	}
	save(t, tt, "terminal-contrast-light")
}

func TestMinimumContrastPreservesReadableColorsAndBoundsCache(t *testing.T) {
	v := &view{theme: &Theme{MinimumContrast: 4.5}}
	bg := ui.RGB(251, 251, 251)
	for _, fg := range []ui.Color{ui.RGB(30, 40, 50), ui.RGB(0, 70, 160), ui.RGB(0, 0, 0).Alpha(.8)} {
		if got := v.readableForeground(fg, bg); got != fg {
			t.Fatalf("already readable color %v changed to %v", fg, got)
		}
	}
	if got := v.readableForeground(ui.RGB(0, 0, 0).Alpha(.5), bg); got.A != 255 || contrastRatio(got, bg) < 4.5 {
		t.Fatalf("faint black text remained unreadable: %v", got)
	}
	for i := range 1024 {
		v.readableForeground(ui.RGB(uint8(i), uint8(i/256), 180), bg)
	}
	if len(v.contrastCache) > 512 {
		t.Fatalf("unbounded contrast cache: %d", len(v.contrastCache))
	}
}

func TestContrasted(t *testing.T) {
	white, black := ui.RGB(255, 255, 255), ui.RGB(0, 0, 0)
	for _, c := range []struct {
		fg, bg ui.Color
		ratio  float32
	}{
		{ui.RGB(240, 240, 240), white, 3},  // a dark theme's gray on a light one
		{ui.RGB(30, 30, 30), black, 3},     // a light theme's on a dark one
		{ui.RGB(229, 229, 16), white, 4.5}, // bright yellow
	} {
		got := contrasted(c.fg, c.bg, c.ratio)
		r := contrast(luminance(got), luminance(c.bg))
		if r < c.ratio || r > c.ratio*1.1 {
			t.Errorf("contrasted(%v, %v, %v) = %v, a ratio of %v", c.fg, c.bg, c.ratio, got, r)
		}
	}
	if got := contrasted(black, white, 4.5); got != black {
		t.Errorf("black on white changed to %v", got)
	}
	// Short of the ratio even at black or white, the text takes the one
	// that contrasts more.
	if got := contrasted(ui.RGB(100, 100, 100), ui.RGB(128, 128, 128), 21); got != black {
		t.Errorf("gray on gray at 21 = %v, want black", got)
	}
}

// Shapes keep their colors under the minimum contrast, as in Ghostty, and
// text takes it: Copilot CLI's input box, a light gray edged with half
// blocks of that gray, showed dark bars on a white background.
func TestMinimumContrastShapes(t *testing.T) {
	loadLib(t)
	light := &Theme{Foreground: ui.RGB(0x20, 0x20, 0x20), Background: ui.RGB(255, 255, 255)}
	term, err := New(Options{Conn: newPipe(), Theme: light, MinimumContrast: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 200)
	gray := ui.RGB(232, 233, 235)
	term.Feed([]byte("\x1b[38;2;232;233;235m▄▀█\U0001fb82A"))
	tt.Frame()
	term.mu.Lock()
	line := term.v.lines[0]
	term.mu.Unlock()
	for _, b := range line.boxes {
		if b.c != gray {
			t.Errorf("%q drawn in %v, want %v", b.r, b.c, gray)
		}
	}
	if len(line.boxes) != 4 {
		t.Errorf("drew %d shapes, want 4", len(line.boxes))
	}
	var text []ui.Color
	for _, r := range line.runs {
		text = append(text, r.c)
	}
	if len(text) == 0 || text[len(text)-1] == gray {
		t.Errorf("text in %v: not contrasted from %v", text, gray)
	}
}
