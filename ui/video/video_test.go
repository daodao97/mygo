package video

import (
	"embed"
	"errors"
	"image/color"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// rgb.mp4 is 1.2 s of H.264 at 10 fps, 64×48, and silent AAC: 0.4 s red,
// then green, then blue. It has no B-frames, so no edit list shifts its
// pictures' times: the MPEG-4 source of Windows on Arm ignores one, and
// would show each picture two frames late.
//
//go:embed testdata/rgb.mp4
var rgbMP4 []byte

// played opens rgb.mp4 as open does and shows it in a Tester, skipping
// the test on a system that has no engine or no H.264 decoder.
func played(t *testing.T, open func() (*ui.VideoPlayer, error)) (*ui.VideoPlayer, *ui.Tester, func() color.RGBA) {
	t.Helper()
	p, err := open()
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	p.SetMuted(true)
	tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(64, 48) }, 64, 48)
	shows := func() color.RGBA { return tt.Image().RGBAAt(32, 24) }
	wait.Until(t, "the first picture", func() bool {
		tt.Frame()
		checkErr(t, p)
		return p.Ready()
	})
	return p, tt, shows
}

// checkErr fails the test on the error that stopped p, or skips it on a
// system that has no H.264 decoder, or no demuxer of the stream's format
// (GStreamer's HLS demuxer, of gst-plugins-bad).
func checkErr(t *testing.T, p *ui.VideoPlayer) {
	t.Helper()
	if err := p.Err(); err != nil {
		if strings.Contains(err.Error(), "missing") || strings.Contains(err.Error(), "decoder") {
			t.Skip("no decoder or demuxer:", err)
		}
		t.Fatal(err)
	}
}

// testdata/hls is rgb.mp4 as an HLS stream, a segment of fragmented MP4
// for each color, made with
//
//	ffmpeg -i rgb.mp4 -c copy -f hls -hls_time 0.4 -hls_segment_type fmp4 \
//		-hls_playlist_type vod -hls_fmp4_init_filename rgb-init.mp4 \
//		-hls_segment_filename 'rgb-%d.m4s' rgb.m3u8
//
// and its EXT-X-TARGETDURATION made 1, as the specification asks, from
// the 0 ffmpeg writes.
//
//go:embed testdata/hls
var hlsFiles embed.FS

// servedStream serves testdata/hls on loopback, for the test, and returns
// the URL of its playlist.
func servedStream(t *testing.T) string {
	t.Helper()
	files, err := fs.Sub(hlsFiles, "testdata/hls")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServerFS(files))
	t.Cleanup(srv.Close)
	return srv.URL + "/rgb.m3u8"
}

// TestWakesForFirstPicture checks that the engine wakes the window for
// the video's first picture: a window that built the video once, before
// it had one, and that builds no frame unless woken, as one showing only
// the video, shows it (#1830).
func TestWakesForFirstPicture(t *testing.T) {
	p, err := OpenData(rgbMP4)
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	p.SetMuted(true)
	tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(64, 48) }, 64, 48)
	for !p.Ready() {
		// Only the frames the window is woken for build: the engine's
		// changes of state, and its first picture.
		wait.Until(t, "a wake, the video not ready", tt.FramePending)
		tt.Frame()
		checkErr(t, p)
	}
	if w, h := p.Size(); w != 64 || h != 48 {
		t.Fatalf("size %d×%d, want 64×48", w, h)
	}
}

// near reports whether c is the color want, give or take what H.264 and
// the conversion to RGB change.
func near(c, want color.RGBA) bool {
	d := func(a, b uint8) bool { return max(a, b)-min(a, b) <= 24 }
	return d(c.R, want.R) && d(c.G, want.G) && d(c.B, want.B) && c.A == 255
}

var (
	red   = color.RGBA{255, 0, 0, 255}
	green = color.RGBA{0, 255, 0, 255}
	blue  = color.RGBA{0, 0, 255, 255}
)

// TestPlays checks that a video in memory plays its frames, as the system
// decodes them, in order and to its end, reporting its length.
func TestPlays(t *testing.T) {
	p, tt, shows := played(t, func() (*ui.VideoPlayer, error) { return OpenData(rgbMP4) })
	if w, h := p.Size(); w != 64 || h != 48 {
		t.Fatalf("size %d×%d, want 64×48", w, h)
	}
	if d := p.Duration(); d < 1100*time.Millisecond || d > 1300*time.Millisecond {
		t.Errorf("duration %v, want 1.2s", d)
	}
	for _, want := range []color.RGBA{red, green, blue} {
		wait.Until(t, "the picture "+colorName(want), func() bool {
			tt.Frame()
			return near(shows(), want)
		})
	}
	wait.Until(t, "the end", func() bool { tt.Frame(); return p.Ended() })
	if p.Playing() || !near(shows(), blue) {
		t.Fatalf("at the end: playing %v, showing %v", p.Playing(), shows())
	}
}

// TestSeek checks that a paused video shows the picture a seek asks for,
// and reports the position, and that one played again after its end
// starts over.
func TestSeek(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a video.mp4")
	if err := os.WriteFile(path, rgbMP4, 0o644); err != nil {
		t.Fatal(err)
	}
	p, tt, shows := played(t, func() (*ui.VideoPlayer, error) { return Open(path) })
	p.Pause()
	for _, s := range []struct {
		at   time.Duration
		want color.RGBA
	}{{900 * time.Millisecond, blue}, {500 * time.Millisecond, green}, {100 * time.Millisecond, red}} {
		p.Seek(s.at)
		wait.Until(t, "the picture at "+s.at.String(), func() bool {
			tt.Frame()
			return near(shows(), s.want)
		})
		if pos := p.Position(); pos < s.at-100*time.Millisecond || pos > s.at+100*time.Millisecond {
			t.Errorf("sought to %v: position %v", s.at, pos)
		}
		if p.Playing() {
			t.Fatal("a seek played a paused video")
		}
	}
	p.Seek(time.Second)
	p.Play()
	wait.Until(t, "the end", func() bool { tt.Frame(); return p.Ended() })
	p.Play()
	wait.Until(t, "the start, played again", func() bool { tt.Frame(); return near(shows(), red) })
}

// TestStreamEnds checks that a stream plays to its end, and again from its
// start: AVFoundation stops one short of the duration its playlist gives,
// at its last picture, where its time stands still with the player playing.
func TestStreamEnds(t *testing.T) {
	url := servedStream(t)
	p, tt, shows := played(t, func() (*ui.VideoPlayer, error) { return OpenURL(url) })
	for _, want := range []color.RGBA{red, green, blue} {
		wait.Until(t, "the picture "+colorName(want), func() bool {
			tt.Frame()
			checkErr(t, p)
			return near(shows(), want)
		})
	}
	wait.Until(t, "the end", func() bool { tt.Frame(); checkErr(t, p); return p.Ended() })
	if p.Playing() || !near(shows(), blue) {
		t.Fatalf("at the end: playing %v, showing %v", p.Playing(), shows())
	}
	p.Play()
	wait.Until(t, "the start, played again", func() bool { tt.Frame(); return near(shows(), red) })
	wait.Until(t, "the end, again", func() bool { tt.Frame(); checkErr(t, p); return p.Ended() })
}

// TestStreamLoops checks that a looping stream plays again from its start
// as it ends, and never ends.
func TestStreamLoops(t *testing.T) {
	url := servedStream(t)
	p, tt, shows := played(t, func() (*ui.VideoPlayer, error) { return OpenURL(url) })
	p.SetLoop(true)
	for _, want := range []color.RGBA{blue, red, green} {
		wait.Until(t, "the picture "+colorName(want), func() bool {
			tt.Frame()
			checkErr(t, p)
			if p.Ended() {
				t.Fatal("a looping stream ended")
			}
			return near(shows(), want)
		})
	}
}

// TestOpenNonASCIIPath checks that Open plays a file whose folder and name
// are not ASCII, which Media Foundation could not find by its file: URL
// (#1752).
func TestOpenNonASCIIPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "中文")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "測試 é.mp4")
	if err := os.WriteFile(path, rgbMP4, 0o644); err != nil {
		t.Fatal(err)
	}
	p, _, _ := played(t, func() (*ui.VideoPlayer, error) { return Open(path) })
	if w, h := p.Size(); w != 64 || h != 48 {
		t.Fatalf("size %d×%d, want 64×48", w, h)
	}
}

func colorName(c color.RGBA) string {
	switch c {
	case red:
		return "red"
	case green:
		return "green"
	}
	return "blue"
}
