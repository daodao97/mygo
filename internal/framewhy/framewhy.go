// Package framewhy tells what keeps a window drawing frames: with
// MYGO_FRAME_WHY set, each request for a frame (an animation, a repaint, a
// timer, an Invalidate or Update from the app) notes where it came from, and
// once a second the log shows how many frames were drawn and the sources
// that asked most. Unset, a note costs one test of On.
package framewhy

import (
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// On tells whether MYGO_FRAME_WHY is set.
var On = os.Getenv("MYGO_FRAME_WHY") != ""

var (
	mu     sync.Mutex
	counts = map[string]int{}
	frames int
	since  time.Time
)

// Note records a request of kind from the caller skip frames above Note's.
func Note(kind string, skip int) {
	if !On {
		return
	}
	pcs := make([]uintptr, 24)
	n := runtime.Callers(skip+2, pcs)
	framesIt := runtime.CallersFrames(pcs[:n])
	var parts []string
	outside := 0
	for len(parts) < 8 {
		f, more := framesIt.Next()
		if f.Function != "" {
			parts = append(parts, fmt.Sprintf("%s:%d", path.Base(f.Function), f.Line))
			if !strings.Contains(f.Function, "egoist/mygo") && !strings.HasPrefix(f.Function, "runtime.") {
				if outside++; outside == 3 {
					break
				}
			}
		}
		if !more {
			break
		}
	}
	key := kind + "  " + strings.Join(parts, " < ")
	mu.Lock()
	counts[key]++
	mu.Unlock()
}

// Frame counts a frame and logs the second's sources once one has passed.
func Frame() {
	if !On {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	if since.IsZero() {
		since = now
	}
	frames++
	if now.Sub(since) < time.Second {
		return
	}
	type kv struct {
		k string
		n int
	}
	list := make([]kv, 0, len(counts))
	for k, n := range counts {
		list = append(list, kv{k, n})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	var b strings.Builder
	fmt.Fprintf(&b, "mygo frame-why: %d frames in %.1fs", frames, now.Sub(since).Seconds())
	for i, e := range list {
		if i == 10 {
			break
		}
		fmt.Fprintf(&b, "\n  %5d  %s", e.n, e.k)
	}
	log.Print(b.String())
	counts, frames, since = map[string]int{}, 0, now
}
