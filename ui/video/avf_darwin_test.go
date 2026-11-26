package video

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/media"
	"github.com/egoist/mygo/internal/wait"
)

// TestFailedStays checks that a video AVFoundation gives no picture for
// stays failed, its player paused, as the watching goroutine polls it
// on, and as the app plays it again or changes its rate: the poll after
// the one that failed it made it ready again, with its error, and a
// looping one that played to its end without a picture played again.
// It opens each from a file, which AVFoundation reads as its container.
func TestFailedStays(t *testing.T) {
	for _, v := range []struct {
		name string
		data []byte
	}{{"vp9.mp4", vp9MP4}, {"h264.avi", h264AVI}} {
		t.Run(v.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), v.name)
			if err := os.WriteFile(path, v.data, 0o644); err != nil {
				t.Fatal(err)
			}
			eng, err := newEngine(source{url: fileURL(path), path: path})
			if err != nil {
				t.Fatal(err)
			}
			e := eng.(*avEngine)
			t.Cleanup(func() { e.Close() })
			e.SetMuted(true)
			e.SetLoop(true)
			e.Play()
			wait.Until(t, "a picture or an error", func() bool {
				_, _, ok := e.Frame(time.Now())
				st, _ := e.State()
				return ok || st == media.Failed
			})
			if st, _ := e.State(); st != media.Failed {
				t.Skip("the system plays it")
			}
			check := func(after string) {
				t.Helper()
				defer pooled()()
				e.mu.Lock()
				e.poll()
				e.mu.Unlock()
				if st, err := e.State(); st != media.Failed || err == nil {
					t.Errorf("after %s: state %v, err %v; want failed", after, st, err)
				}
				if r := msgFloat32(e.player, sel("rate")); r != 0 {
					t.Errorf("after %s: the player plays at %v", after, r)
				}
			}
			check("another poll")
			e.Play()
			check("Play")
			e.SetRate(2)
			check("SetRate")
		})
	}
}
