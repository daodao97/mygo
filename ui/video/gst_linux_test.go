//go:build linux && !android && (amd64 || arm64)

package video

import (
	"bytes"
	_ "embed"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// rgb.webm is rgb.mp4 in WebM, VP9 with Opus, a keyframe at each colour,
// made with ffmpeg (libvpx-vp9 -g 4, libopus): its index is at its end,
// as ffmpeg writes a file's.
//
//go:embed testdata/rgb.webm
var rgbWebM []byte

// TestDownloadsOverHTTP checks that a video over HTTP is buffered in a
// file, which GStreamer's demuxers read where they ask, and one from a
// file is not.
func TestDownloadsOverHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "rgb.webm", time.Time{}, bytes.NewReader(rgbWebM))
	}))
	t.Cleanup(srv.Close)
	path := filepath.Join(t.TempDir(), "rgb.webm")
	if err := os.WriteFile(path, rgbWebM, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		name      string
		src       source
		downloads bool
	}{
		{"OpenURL", source{url: srv.URL + "/rgb.webm"}, true},
		{"Open", source{url: fileURL(path), path: path}, false},
	} {
		t.Run(v.name, func(t *testing.T) {
			eng, err := newEngine(v.src)
			if err != nil {
				t.Skip(err)
			}
			defer eng.Close()
			e := eng.(*gstEngine)
			var g gValue
			gValueInit(&g, gTypeFromName("GstPlayFlags"))
			gObjectGetProperty(e.pipe, "flags", &g)
			flags := gValueGetFlags(&g)
			gValueUnset(&g)
			if got := flags&gstPlayFlagDownload != 0; got != v.downloads {
				t.Errorf("download buffering %v (flags %#x), want %v", got, flags, v.downloads)
			}
		})
	}
}

// TestShortWebMSeeksOverHTTP checks that a short WebM over HTTP shows the
// picture a first seek asks for: GStreamer's Matroska demuxer, reading it
// as it came, failed 1 in 5 of those, "Could not demultiplex stream", as
// it sought for the index at the file's end, all of which it had, and
// again for the picture while the first seek still flushed. Only a first
// seek looks for the index, so each try opens the video anew.
func TestShortWebMSeeksOverHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "rgb.webm", time.Time{}, bytes.NewReader(rgbWebM))
	}))
	t.Cleanup(srv.Close)
	for range 20 {
		p, tt, shows := played(t, func() (*ui.VideoPlayer, error) { return OpenURL(srv.URL + "/rgb.webm") })
		p.Pause()
		p.Seek(600 * time.Millisecond)
		wait.Until(t, "the picture at 600ms", func() bool {
			tt.Frame()
			checkErr(t, p)
			return near(shows(), green)
		})
		p.Close()
	}
}
