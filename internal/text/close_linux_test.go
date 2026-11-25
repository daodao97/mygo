//go:build linux && !android && (amd64 || arm64)

package text

import (
	"bytes"
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/egoist/mygo/internal/wait"
)

// Systems New makes, as each Tester's window does, give back their Pango
// font maps once unreachable, and with them the thread fontconfig starts
// for each: a test binary of a thousand Testers ended with a thousand
// threads and gigabytes of memory (#1902).
func TestNewSystemsReleaseFontMaps(t *testing.T) {
	use := func() *System {
		s := New()
		l := s.Layout(Params{Text: "Hello, world", Style: Style{Size: 13}})
		g := l.Lines[0].Glyphs[0]
		s.Glyph(g.Font, g.ID, 2, 0, 0, false)
		if _, ok := s.eng.(*pangoEngine); !ok {
			t.Skipf("the engine is a %T, not Pango", s.eng)
		}
		return s
	}
	// The first loads the libraries, which may start threads of their own.
	use()
	settled := func() int {
		runtime.GC()
		return threads(t)
	}
	before := settled()
	// Enough that the threads the runtime may start meanwhile, as many as
	// it runs goroutines at once, stand out from one for each System.
	systems := max(100, 8*runtime.GOMAXPROCS(0))
	alive := make([]*System, systems)
	for i := range alive {
		alive[i] = use()
	}
	t.Logf("%d threads before, %d with %d Systems", before, threads(t), systems)
	clear(alive)
	wait.Until(t, "the threads of unreachable Systems' font maps ending", func() bool {
		return settled() <= before+systems/10
	})
	t.Logf("%d threads once they are unreachable", threads(t))
}

// threads returns how many threads the process has.
func threads(t *testing.T) int {
	t.Helper()
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for line := range bytes.Lines(status) {
		if v, ok := bytes.CutPrefix(line, []byte("Threads:")); ok {
			n, err := strconv.Atoi(string(bytes.TrimSpace(v)))
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
	}
	t.Fatal("no thread count in /proc/self/status")
	return 0
}
