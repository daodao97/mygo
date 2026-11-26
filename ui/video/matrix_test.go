package video

import (
	"embed"
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// testdata/matrix holds the samples of TestFormatMatrix, which make.sh
// makes: 1.2 s of 64×48 (AV1: 64×64), 0.4 s red, then green, then blue,
// a keyframe at each, with silent sound.
//
//go:embed testdata/matrix
var matrixFiles embed.FS

// A matrixWant is what a system does with a sample.
type matrixWant uint8

const (
	// plays: it shows its first picture, those after seeks to 0.6 s, 1 s
	// and 0.2 s, and ends.
	plays matrixWant = 1 << iota
	// fails: an error, and never a picture.
	fails
	// noSeek, with plays: a seek shows another picture than the one it
	// asks for, or none, or fails it, a bug its row names: the test does
	// not seek it, and plays it to its end from its first picture.
	noSeek

	// varies: either, as the system's codecs vary by machine: AV1 by the
	// chip, HEVC and VP9 by the codecs Windows has.
	varies = plays | fails
)

// matrixRow is a sample, and what each system does with it, by GOOS
// ("ios" is the simulator); by says what one does opened one way, where
// it differs, as "linux/OpenURL". On Linux, a sample plays where
// GStreamer has the elements gst names, and may fail where it has not
// one of them.
type matrixRow struct {
	file                        string
	darwin, ios, windows, linux matrixWant
	by                          map[string]matrixWant
	gst                         []string
}

// matrixRows is the format table of docs/ui/video.md, which says the
// same: change both.
var matrixRows = []matrixRow{
	{file: "mp4_h264_aac.mp4", darwin: plays, ios: plays, windows: plays, linux: plays, gst: []string{"qtdemux", "avdec_h264"}},
	{file: "m4v_h264_aac.m4v", darwin: plays, ios: plays, windows: plays, linux: plays, gst: []string{"qtdemux", "avdec_h264"}},
	{file: "3gp_h264_aac.3gp", darwin: plays, ios: plays, windows: plays, linux: plays, gst: []string{"qtdemux", "avdec_h264"}},
	{file: "fmp4_h264_aac.mp4", darwin: plays, ios: plays, windows: plays, linux: plays, gst: []string{"qtdemux", "avdec_h264"}},
	{file: "mov_h264_aac.mov", darwin: plays, ios: plays, windows: plays, linux: plays, gst: []string{"qtdemux", "avdec_h264"}},
	{file: "mkv_h264_aac.mkv", darwin: fails, ios: fails, windows: plays, linux: plays, gst: []string{"matroskademux", "avdec_h264"}},
	// Windows's VP9 decoder, where the Store's VP9 extension is installed
	// (Windows 11 on Arm), shows another picture after a seek, in WebM and
	// MP4 alike.
	{file: "webm_vp9_opus.webm", darwin: fails, ios: fails, windows: varies | noSeek, linux: plays, gst: []string{"matroskademux", "vp9dec"}},
	{file: "webm_av1_opus.webm", darwin: fails, ios: fails, windows: varies, linux: plays, gst: []string{"matroskademux", "av1dec"}},
	// Media Foundation's MPEG-TS source, and GStreamer's MPEG-TS and
	// MPEG-PS demuxers, land a seek elsewhere than asked, from a file too
	// (#2371).
	{file: "ts_h264_aac.ts", darwin: plays, ios: fails, windows: plays | noSeek, linux: plays | noSeek, gst: []string{"tsdemux", "h264parse", "avdec_h264"}},
	{file: "m2ts_h264_aac.m2ts", darwin: plays, ios: fails, windows: plays | noSeek, linux: plays | noSeek, gst: []string{"tsdemux", "h264parse", "avdec_h264"}},
	{file: "mpg_mpeg2_mp2.mpg", darwin: plays, ios: fails, windows: varies | noSeek, linux: plays | noSeek, gst: []string{"mpegpsdemux", "avdec_mpeg2video"}},
	// macOS 26 plays it to its end without a picture, and fails it then.
	{file: "avi_mjpeg_pcm.avi", darwin: varies, ios: plays, windows: plays, linux: plays, gst: []string{"avidemux", "jpegdec"}},
	{file: "mp4_hevc_aac.mp4", darwin: plays, ios: plays, windows: varies, linux: plays, gst: []string{"qtdemux", "avdec_h265"}},
	{file: "mkv_hevc_aac.mkv", darwin: fails, ios: fails, windows: varies, linux: plays, gst: []string{"matroskademux", "avdec_h265"}},
	// AVFoundation's MPEG-TS demuxer leaves out HEVC (#2377).
	{file: "ts_hevc_aac.ts", darwin: fails, ios: fails, windows: varies | noSeek, linux: plays | noSeek, gst: []string{"tsdemux", "h265parse", "avdec_h265"}},
	{file: "mp4_vp9_opus.mp4", darwin: fails, ios: fails, windows: varies | noSeek, linux: plays, gst: []string{"qtdemux", "vp9dec"}},
	// The iOS simulator decodes no AV1, on a Mac whose chip does (M4, iOS
	// 18.6 and 27).
	{file: "fmp4_av1_aac.mp4", darwin: varies, ios: fails, windows: varies, linux: plays, gst: []string{"qtdemux", "av1dec"}},
	{file: "hls_fmp4/index.m3u8", darwin: plays, ios: plays, windows: plays, linux: plays, gst: []string{"hlsdemux", "qtdemux", "avdec_h264"}},
	{file: "hls_ts/index.m3u8", darwin: plays, ios: plays, windows: plays, linux: plays, gst: []string{"hlsdemux", "tsdemux", "h264parse", "avdec_h264"}},
}

// wants is what the system does with the sample opened one way.
func (r matrixRow) wants(open string) matrixWant {
	if w, ok := r.by[runtime.GOOS+"/"+open]; ok {
		return w
	}
	switch runtime.GOOS {
	case "darwin":
		return r.darwin
	case "ios":
		return r.ios
	case "windows":
		return r.windows
	case "linux":
		return r.linux
	}
	return 0
}

// TestFormatMatrix opens each sample with Open, OpenData and OpenURL (over
// loopback HTTP; an HLS stream with OpenURL alone), and checks it plays,
// or fails, as matrixRows says the system does: a sample that plays shows
// the pictures it should, after seeks too, and ends; one that fails says
// so, and none hangs. MYGO_VIDEO_MATRIX_REPORT=1 checks every sample as
// one that plays, and reports what each does, for a new system's column.
func TestFormatMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("the samples take a minute")
	}
	report := os.Getenv("MYGO_VIDEO_MATRIX_REPORT") != ""
	files, err := fs.Sub(matrixFiles, "testdata/matrix")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServerFS(files))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	var table []string
	for _, row := range matrixRows {
		t.Run(row.file, func(t *testing.T) {
			data, err := fs.ReadFile(files, row.file)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, strings.ReplaceAll(row.file, "/", "_"))
			if err := os.WriteFile(file, data, 0o644); err != nil {
				t.Fatal(err)
			}
			opens := []struct {
				name string
				open func() (*ui.VideoPlayer, error)
			}{
				{"Open", func() (*ui.VideoPlayer, error) { return Open(file) }},
				{"OpenData", func() (*ui.VideoPlayer, error) { return OpenData(data) }},
				{"OpenURL", func() (*ui.VideoPlayer, error) { return OpenURL(srv.URL + "/" + row.file) }},
			}
			if path.Ext(row.file) == ".m3u8" {
				opens = opens[2:]
			}
			for _, o := range opens {
				t.Run(o.name, func(t *testing.T) {
					want := row.wants(o.name)
					if want == 0 {
						t.Skipf("matrixRows says nothing of %s", runtime.GOOS)
					}
					if want&fails == 0 {
						if missing := matrixMissing(row.gst); len(missing) > 0 {
							want |= fails
							t.Logf("GStreamer has no %s: it may fail", strings.Join(missing, ", "))
						}
					}
					if report {
						want = plays
					}
					err := matrixPlay(t, o.open, want)
					outcome := "plays"
					if err != nil {
						outcome = "fails: " + err.Error()
					}
					table = append(table, fmt.Sprintf("| %s | %s | %s |", row.file, o.name, outcome))
					switch {
					case report:
						t.Log(outcome)
					case err != nil && want&fails == 0:
						t.Errorf("%s, and matrixRows says it plays", outcome)
					case err == nil && want&plays == 0:
						t.Errorf("it plays, and matrixRows says it fails")
					default:
						t.Log(outcome)
					}
				})
			}
		})
	}
	if report {
		fmt.Printf("### %s/%s\n\n| sample | opened with | what it does |\n|---|---|---|\n%s\n", runtime.GOOS, runtime.GOARCH, strings.Join(table, "\n"))
	}
}

// matrixPlay opens a sample, and returns the error it fails with, or nil
// once it played as want says one that plays does: its pictures, after
// seeks unless want has noSeek, and its end. One that shows a wrong
// picture, or none for good, fails the test; one want says fails is not
// waited for past its first picture.
func matrixPlay(t *testing.T, open func() (*ui.VideoPlayer, error), want matrixWant) error {
	t.Helper()
	p, err := open()
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		return err
	}
	defer p.Close()
	p.SetMuted(true)
	tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(64, 48) }, 64, 48)
	shows := func() color.RGBA { return tt.Image().RGBAAt(32, 24) }
	wait.Until(t, "a picture or an error", func() bool {
		tt.Frame()
		return p.Ready() || p.Err() != nil
	})
	if err := p.Err(); err != nil || want&plays == 0 {
		return err
	}
	if want&noSeek == 0 {
		p.Pause()
		for _, s := range []struct {
			at   time.Duration
			want color.RGBA
		}{{600 * time.Millisecond, green}, {time.Second, blue}, {200 * time.Millisecond, red}} {
			p.Seek(s.at)
			wait.Until(t, fmt.Sprintf("the picture at %v, %s", s.at, colorName(s.want)), func() bool {
				tt.Frame()
				if err := p.Err(); err != nil {
					t.Fatalf("sought to %v: %v", s.at, err)
				}
				return near(shows(), s.want)
			})
		}
		p.Seek(time.Second)
		p.Play()
	}
	wait.Until(t, "the end", func() bool {
		tt.Frame()
		return p.Ended() || p.Err() != nil
	})
	if err := p.Err(); err != nil {
		t.Fatalf("played to its end: %v", err)
	}
	return nil
}
