package netimage

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func picture(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// testCache is a cache of its own, on a clock of the test's.
func testCache(t *testing.T) (*cache, *time.Time) {
	t.Helper()
	m := newCache()
	m.dir = t.TempDir()
	m.once.Do(func() {}) // no app to hear from
	clock := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return clock }
	t.Cleanup(func() { idle(t, m) }) // before its directory goes
	return m, &clock
}

// get waits for the picture.
func get(t *testing.T, m *cache, url string, side int) ui.ImageSource {
	t.Helper()
	var src ui.ImageSource
	wait.Until(t, "the picture to come", func() bool {
		s, _, err := m.request(url, side, nil)
		if err != nil {
			t.Fatal(err)
		}
		src = s
		return s != nil
	})
	return src
}

// A picture downloads once: to memory, then to disk for the next run.
func TestDownloadsOnce(t *testing.T) {
	data := picture(t, 8, 8)
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write(data)
	}))
	defer s.Close()
	m, _ := testCache(t)
	get(t, m, s.URL, 100)
	get(t, m, s.URL, 100)
	if hits.Load() != 1 {
		t.Errorf("downloaded %d times", hits.Load())
	}
	// The next run: memory is gone, the disk stays.
	next, clock := testCache(t)
	next.dir = m.dir
	*clock = m.now()
	get(t, next, s.URL, 100)
	if hits.Load() != 1 {
		t.Errorf("after a restart, downloaded %d times", hits.Load())
	}
}

// Past its freshness it asks, and the server may say it stays the same.
func TestRevalidates(t *testing.T) {
	data := picture(t, 8, 8)
	var hits, same atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			same.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write(data)
	}))
	defer s.Close()
	m, clock := testCache(t)
	get(t, m, s.URL, 100)
	// Memory goes, an hour later: it asks.
	next, c2 := testCache(t)
	next.dir = m.dir
	*c2 = clock.Add(time.Hour)
	get(t, next, s.URL, 100)
	if hits.Load() != 2 || same.Load() != 1 {
		t.Errorf("%d requests, %d answered the same", hits.Load(), same.Load())
	}
	// It said fresh again: the one after doesn't ask.
	third, c3 := testCache(t)
	third.dir = m.dir
	*c3 = c2.Add(time.Second)
	get(t, third, s.URL, 100)
	if hits.Load() != 2 {
		t.Errorf("fresh again, it asked: %d requests", hits.Load())
	}
}

// Offline, what it has shows, however old.
func TestOffline(t *testing.T) {
	data := picture(t, 8, 8)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1")
		w.Write(data)
	}))
	m, clock := testCache(t)
	get(t, m, s.URL, 100)
	url := s.URL
	s.Close()
	next, c2 := testCache(t)
	next.dir = m.dir
	*c2 = clock.Add(24 * time.Hour)
	get(t, next, url, 100)
}

// What fails says so, and is asked for again only a while later.
func TestFailureWaits(t *testing.T) {
	var hits atomic.Int32
	data := picture(t, 8, 8)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	defer s.Close()
	m, clock := testCache(t)
	wait.Until(t, "the failure", func() bool {
		_, _, err := m.request(s.URL, 100, nil)
		return err != nil
	})
	if _, _, err := m.request(s.URL, 100, nil); err == nil {
		t.Error("the failure went")
	}
	if hits.Load() != 1 {
		t.Errorf("asked again at once: %d", hits.Load())
	}
	*clock = clock.Add(retryAfter + time.Second)
	wait.Until(t, "it to come", func() bool {
		src, _, _ := m.request(s.URL, 100, nil)
		return src != nil
	})
}

// A photo stays as big as what shows it.
func TestScales(t *testing.T) {
	data := picture(t, 400, 200)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer s.Close()
	m, _ := testCache(t)
	b := get(t, m, s.URL, 100).(*ui.Bitmap)
	if w, h := b.Size(); w != 100 || h != 50 {
		t.Errorf("scaled to %d×%d", w, h)
	}
}

// Memory holds what was shown last, up to its limit.
func TestEvicts(t *testing.T) {
	data := picture(t, 100, 100)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer s.Close()
	m, clock := testCache(t)
	m.limit = 100 * 100 * 4 * 2 // two
	for i := range 4 {
		get(t, m, fmt.Sprintf("%s/%d", s.URL, i), 100)
		*clock = clock.Add(time.Second)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held > m.limit {
		t.Errorf("holds %d of %d", m.held, m.limit)
	}
	if m.entries[key{s.URL + "/0", 100}] != nil {
		t.Error("the first kept")
	}
	if m.entries[key{s.URL + "/3", 100}] == nil {
		t.Error("the last gone")
	}
}

// The last asked for goes first, and what a fling left behind not at all.
func TestLastFirst(t *testing.T) {
	data := picture(t, 8, 8)
	turn := make(chan struct{}, workers+3)
	arrived := make(chan string, workers+3)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- r.URL.Path
		<-turn
		w.Write(data)
	}))
	defer s.Close()
	m, clock := testCache(t)
	for i := range workers {
		m.request(fmt.Sprintf("%s/b%d", s.URL, i), 100, nil)
	}
	for range workers {
		wait.For(t, "the workers to be busy", arrived)
	}
	m.request(s.URL+"/gone", 100, nil)
	*clock = clock.Add(abandoned + time.Second)
	m.request(s.URL+"/older", 100, nil)
	m.request(s.URL+"/newest", 100, nil)
	// One worker at a time frees up, each taking the next.
	for _, want := range []string{"/newest", "/older"} {
		turn <- struct{}{}
		if got := wait.For(t, "a download", arrived); got != want {
			t.Errorf("downloading %s, want %s", got, want)
		}
	}
	for range workers + 2 {
		turn <- struct{}{}
	}
	idle(t, m)
	if len(arrived) != 0 {
		t.Errorf("what a fling left downloaded: %s", <-arrived)
	}
}

// idle waits for the workers to finish.
func idle(t *testing.T, m *cache) {
	t.Helper()
	wait.Until(t, "the workers to finish", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.running == 0
	})
}

// The disk keeps what was written last, without what a run left half written.
func TestPrune(t *testing.T) {
	m, _ := testCache(t)
	old := filepath.Join(m.dir, "old")
	young := filepath.Join(m.dir, "young")
	tmp := filepath.Join(m.dir, "tmp-1")
	for _, f := range []string{old, young, tmp} {
		if err := os.WriteFile(f, make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	long := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, long, long); err != nil {
		t.Fatal(err)
	}
	m.prune(m.dir, 100)
	if fileThere(old) || !fileThere(young) || fileThere(tmp) {
		t.Errorf("after pruning: old %v, young %v, tmp %v", fileThere(old), fileThere(young), fileThere(tmp))
	}
}

func fileThere(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// A view shows the placeholder, and the picture once it comes, on its own.
func TestImageInView(t *testing.T) {
	data := picture(t, 8, 8)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer s.Close()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
	var src ui.ImageSource
	tt := ui.NewTester(func(c *ui.Context) {
		Image(c, s.URL).Size(40, 40)
		src, _ = Load(c, s.URL)
	}, 100, 100)
	tt.Frame()
	wait.Until(t, "the picture to show", func() bool {
		tt.Frame()
		return src != nil
	})
}

// A page that answers for the picture, as a captive portal's, leaves what the
// disk has.
func TestCaptivePortal(t *testing.T) {
	data := picture(t, 8, 8)
	var portal atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1")
		if portal.Load() {
			w.Write([]byte("<html>Sign in to the Wi-Fi</html>"))
			return
		}
		w.Write(data)
	}))
	defer s.Close()
	m, clock := testCache(t)
	get(t, m, s.URL, 100)
	portal.Store(true)
	next, c2 := testCache(t)
	next.dir = m.dir
	*c2 = clock.Add(time.Hour)
	get(t, next, s.URL, 100)
	idle(t, next)
	// And it's still there for the run after, the portal gone or not.
	body, _ := m.paths(s.URL)
	kept, err := os.ReadFile(body)
	if err != nil || !bytes.Equal(kept, data) {
		t.Errorf("the disk holds %d bytes, %v", len(kept), err)
	}
}

// A small file that says it's enormous fails, rather than ending the app.
func TestEnormousPicture(t *testing.T) {
	// A PNG's header says its size; what follows doesn't matter to the check.
	data := picture(t, 8, 8)
	data[16], data[17], data[18], data[19] = 0, 0, 0x75, 0x30 // 30000
	data[20], data[21], data[22], data[23] = 0, 0, 0x75, 0x30
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer s.Close()
	m, _ := testCache(t)
	wait.Until(t, "it to fail", func() bool {
		_, _, err := m.request(s.URL, 100, nil)
		return err != nil
	})
}

// Pictures on screen that don't fit the limit stay, rather than each pushing out
// the next for good.
func TestOnScreenStays(t *testing.T) {
	data := picture(t, 100, 100)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer s.Close()
	m, _ := testCache(t)
	m.limit = 100 * 100 * 4 // one
	for i := range 3 {
		get(t, m, fmt.Sprintf("%s/%d", s.URL, i), 100)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entries) != 3 {
		t.Errorf("%d of 3 on screen stay", len(m.entries))
	}
}

// What's let go of as it downloads, as on logging out, stays off the disk.
func TestForgetWhileDownloading(t *testing.T) {
	data := picture(t, 8, 8)
	arrived, finish := make(chan struct{}), make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-finish
		w.Write(data)
	}))
	defer s.Close()
	m, _ := testCache(t)
	m.request(s.URL, 100, nil)
	wait.For(t, "the download", arrived)
	m.clear()
	close(finish)
	idle(t, m)
	body, _ := m.paths(s.URL)
	if fileThere(body) {
		t.Error("it went to disk")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entries) != 0 || m.held != 0 {
		t.Errorf("%d entries, %d bytes", len(m.entries), m.held)
	}
}

// Preloading fills the disk, not memory, and gives up on nothing however long it
// waits for a worker.
func TestPreload(t *testing.T) {
	data := picture(t, 8, 8)
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write(data)
	}))
	defer s.Close()
	m, _ := testCache(t)
	m.preload(s.URL)
	idle(t, m)
	m.mu.Lock()
	held, n := m.held, len(m.entries)
	m.mu.Unlock()
	if held != 0 || n != 0 {
		t.Errorf("preloading holds %d bytes, %d entries", held, n)
	}
	get(t, m, s.URL, 100)
	if hits.Load() != 1 {
		t.Errorf("downloaded %d times", hits.Load())
	}
}
