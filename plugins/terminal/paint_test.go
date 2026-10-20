package terminal

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestMobileCursorBlinksWithoutKeyboardFocus(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), Theme: LightTheme(), ActiveCursor: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 320, 240)
	v := term.v
	if v.focused {
		t.Fatal("mobile cursor acquired keyboard focus")
	}
	v.blinkStart = time.Now()
	on, _, _, block := v.cursorRect(0, 0, 8, 16)
	v.blinkStart = time.Now().Add(-blinkPeriod - 50*time.Millisecond)
	off, _, _, _ := v.cursorRect(0, 0, 8, 16)
	if on.W == 0 || off.W != 0 || !block {
		t.Fatalf("cursor did not blink without focus: on=%+v off=%+v block=%v", on, off, block)
	}
}

func TestRowBackgroundReachesRightEdge(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), Theme: LightTheme(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 301, 100)
	for _, scale := range []float32{1, 2} {
		tt.SetScale(scale)
		sawRemainder := false
		for _, width := range []int{301, 304} {
			tt.SetSize(width, 100)
			// The first row has background through its last column; the
			// second has only a short colored run at its start.
			term.Feed([]byte("\x1b[?25l\x1b[1;1H\x1b[48;2;80;90;100m\x1b[2K\x1b[0m\x1b[2;1H\x1b[2K\x1b[48;2;80;90;100m   \x1b[0m"))
			tt.Frame()
			v, img := term.v, tt.Image()
			right := img.Bounds().Max.X - 1
			if right >= v.ox+v.cols*v.cellW {
				sawRemainder = true
			}
			for row, want := range []ui.Color{ui.RGB(80, 90, 100), LightTheme().Background} {
				got := img.RGBAAt(right, v.oy+row*v.cellH+v.cellH/2)
				if got.R != want.R || got.G != want.G || got.B != want.B {
					t.Fatalf("width %d, scale %g, row %d: right edge = %v, want %v", width, scale, row, got, want)
				}
			}
			if col, _ := v.cellAt(float32(width)-0.1, float32(v.oy+v.cellH/2)/scale); col != v.cols-1 {
				t.Fatalf("right-edge mouse position maps to column %d, want %d", col, v.cols-1)
			}
		}
		if !sawRemainder {
			t.Fatalf("scale %g did not exercise a partial character column", scale)
		}
	}
}

// ANSI snapshots use absolute source columns and rows. Fitting, rotating and
// zooming a phone must never reflow them or move a wide Unicode cell.
func TestRemoteGridFitsWithoutReflow(t *testing.T) {
	loadLib(t)
	source, err := New(Options{Conn: newPipe(), FixedCols: 100, FixedRows: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	source.Feed([]byte("\x1b[?1049h\x1b[2J\x1b[2;70H中文🙂⣿⣷\x1b[8;90HRIGHT\x1b[31;1H> prompt\x1b[31;9H"))
	phone, err := New(Options{Conn: newPipe(), FixedCols: 100, FixedRows: 32, FitToView: true, Padding: Padding{X: 20, Y: 12}})
	if err != nil {
		t.Fatal(err)
	}
	defer phone.Close()
	// Replay before first paint, as happens when a fast remote attach wins
	// the race against the phone's first frame.
	phone.Feed(source.Snapshot())
	want := source.Text()
	tt := ui.NewTester(func(c *ui.Context) { View(c, phone).Fill() }, 393, 680)
	for _, scale := range []float32{1, 1.25, 2, 3} {
		tt.SetScale(scale)
		for _, size := range [][2]int{{1440, 960}, {720, 960}, {393, 680}, {393, 280}, {780, 180}, {320, 200}} {
			tt.SetSize(size[0], size[1])
			tt.Frame()
			v := phone.v
			if got := phone.Text(); got != want {
				t.Fatalf("%v reflowed source: %q; want %q", size, got, want)
			}
			if v.cols != 100 || v.rows != 32 {
				t.Fatalf("source grid changed: %dx%d", v.cols, v.rows)
			}
			if v.cols*v.cellW > v.viewportW || v.rows*v.cellH > v.viewportH {
				t.Fatalf("%v clips fitted grid: %dx%d cells of %dx%d in %dx%d", size, v.cols, v.rows, v.cellW, v.cellH, v.viewportW, v.viewportH)
			}
			if size == [2]int{1440, 960} && v.font.size <= 13 {
				t.Fatal("larger desktop pane did not enlarge the remote screen")
			}
			// Keep the source aspect ratio and use the largest font that fits.
			// Pango rounds advances to whole DIPs, so at scale 3 the next font
			// can jump several physical pixels rather than filling one at a time.
			// A 1% larger font is beyond the binary search's rounding tolerance.
			larger := v.font
			larger.size *= 1.01
			cw, ch, _ := fontGeometry(larger, v.scale)
			if v.cols*cw <= v.viewportW && v.rows*ch <= v.viewportH {
				t.Fatalf("%v underfits: size %g uses %dx%d cells, but 1%% larger still fits %dx%d in %dx%d", size, v.font.size, v.cellW, v.cellH, cw, ch, v.viewportW, v.viewportH)
			}
			if size[1] == 960 {
				save(t, tt, fmt.Sprintf("remote-grid-desktop-fit-%d", size[0]))
			}
		}
	}
	phone.SetFitToView(false)
	tt.SetSize(393, 280)
	tt.Frame()
	v := phone.v
	if v.font.size != 13 || v.panY == 0 {
		t.Fatalf("zoom did not reveal input at normal size: size=%g panY=%d", v.font.size, v.panY)
	}
	v.scroll(ui.InputEvent{DX: 200, Precise: true})
	tt.Frame()
	if v.panX == 0 {
		t.Fatal("zoomed grid cannot pan horizontally")
	}
	if phone.Text() != want {
		t.Fatal("zoom/pan altered source text")
	}
	save(t, tt, "remote-grid-zoom")
	phone.SetFitToView(true)
	tt.Frame()
	if v.panX != 0 || v.panY != 0 {
		t.Fatal("fit retained zoom offsets")
	}
	save(t, tt, "remote-grid-fit")
}

type screenChunk struct {
	cols, rows int
	data       []byte
}
type screenPipe struct {
	*pipe
	frames  chan screenChunk
	pending screenChunk
}

func (p *screenPipe) ReadScreen(buf []byte) (int, int, int, error) {
	if len(p.pending.data) == 0 {
		select {
		case p.pending = <-p.frames:
		case <-p.closed:
			return 0, 0, 0, io.EOF
		}
	}
	n := copy(buf, p.pending.data)
	cols, rows := p.pending.cols, p.pending.rows
	p.pending.data = p.pending.data[n:]
	return n, cols, rows, nil
}

func TestRemoteScreenResizeBeforeOutput(t *testing.T) {
	loadLib(t)
	conn := &screenPipe{pipe: newPipe(), frames: make(chan screenChunk, 1)}
	phone, err := New(Options{Conn: conn, FixedCols: 80, FixedRows: 24, FitToView: true})
	if err != nil {
		t.Fatal(err)
	}
	defer phone.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, phone).Fill() }, 393, 680)
	for _, size := range [][2]int{{100, 32}, {120, 40}, {60, 20}} {
		source, err := New(Options{Conn: newPipe(), FixedCols: size[0], FixedRows: size[1]})
		if err != nil {
			t.Fatal(err)
		}
		source.Feed([]byte(fmt.Sprintf("\x1b[?1049h\x1b[2J\x1b[%d;%dH中文🙂⣿END", size[1]-1, size[0]-13)))
		want := source.Text()
		conn.frames <- screenChunk{size[0], size[1], source.Snapshot()}
		waitFor(t, tt, "source grid and its snapshot", func() bool {
			cols, rows := phone.Size()
			return cols == size[0] && rows == size[1] && phone.Text() == want
		})
		tt.SetSize(780, 200)
		tt.Frame()
		if phone.Text() != want {
			t.Fatal("orientation reflowed authoritative screen")
		}
		source.Close()
	}
}
