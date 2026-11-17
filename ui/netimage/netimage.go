// Package netimage shows pictures from the network, in any window of native UI:
//
//	netimage.Image(c, "https://example.com/cat.jpg").Size(80, 80).Radius(8)
//
// It is what a feed, a chat or a shop needs of every picture: while it comes a
// placeholder holds its place; once it came it fades in. Pictures stay in memory
// as long as there is room, and on disk across runs, as their servers' caching
// headers say, so a list scrolled back shows them at once and a phone offline
// shows what it had. What's on screen downloads before what a fling left behind,
// which doesn't download at all if it's gone by its turn.
package netimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Options say how a picture shows.
type Options struct {
	// Placeholder fills its place until it shows; by default a muted color of
	// the theme.
	Placeholder ui.Color
	// Fit says how it fills what it's given; Cover by default.
	Fit ui.Fit
	// MaxSide bounds the longest side of the bitmap kept, in pixels; by
	// default 2048. Give about what it shows at, on a screen of 3 pixels per
	// DIP: a list of 80 DIP thumbnails holds 240, not a phone's 4000 pixel
	// photos.
	MaxSide int
	// NoFade shows it at once.
	NoFade bool
}

// Image returns an element showing the picture at url: a placeholder, then the
// picture. Give it a size, or it takes the picture's as it comes.
func Image(c *ui.Context, url string, o ...Options) ui.Element {
	var opt Options
	if len(o) > 0 {
		opt = o[0]
	}
	src, age, _ := load(c, url, opt.MaxSide)
	fit := opt.Fit
	if fit == 0 {
		fit = ui.Cover
	}
	if src == nil {
		fill := opt.Placeholder
		if fill == (ui.Color{}) {
			fill = c.Theme().Border
		}
		return ui.Box(c).Background(fill).Role(ui.RoleImage)
	}
	e := ui.Image(c, src).Fit(fit).Role(ui.RoleImage)
	if !opt.NoFade && age >= 0 && age < fadeTime {
		e.Opacity(float32(age) / float32(fadeTime))
		c.AnimationFrame()
	}
	return e
}

// Load returns the picture at url, nil while it comes, to draw it in some other
// way, as an ui.Avatar. The view is built again once it comes. err is what went
// wrong the last time it was asked for: it's asked for again a while later, as
// the view goes on asking.
func Load(c *ui.Context, url string, o ...Options) (src ui.ImageSource, err error) {
	maxSide := 0
	if len(o) > 0 {
		maxSide = o[0].MaxSide
	}
	src, _, err = load(c, url, maxSide)
	return src, err
}

// Preload downloads pictures to disk before they show, as the next page of a
// list that's about to come.
func Preload(urls ...string) {
	for _, u := range urls {
		shared.preload(u)
	}
}

// Forget takes a picture out of memory and off the disk: its next showing
// downloads it again.
func Forget(url string) {
	shared.forget(url)
}

// Clear empties the memory and the disk of pictures.
func Clear() {
	shared.clear()
}

// SetClient sets what downloads pictures, as one that signs requests in.
func SetClient(c *http.Client) {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	shared.client = c
}

const (
	fadeTime    = 180 * time.Millisecond
	defaultSide = 2048
	// memoryLimit bounds the pixels held, in bytes.
	memoryLimit = 128 << 20
	// diskLimit bounds the pictures on disk, in bytes.
	diskLimit = 256 << 20
	// workers download at a time.
	workers = 6
	// abandoned is how long after its last showing a picture still waiting
	// is given up: a fling went past it.
	abandoned = 2 * time.Second
	// onScreen is how long after its last showing a picture may still show.
	onScreen = 2 * time.Second
	// maxBytes and maxPixels bound what one picture may ask of the phone.
	maxBytes  = 64 << 20
	maxPixels = 64 << 20
	// tooManyEntries sweeps what failed and what's no longer asked for.
	tooManyEntries = 2048
	// retryAfter waits before asking again for what failed.
	retryAfter = 10 * time.Second
	// defaultFresh is how long a picture is trusted whose server says nothing.
	defaultFresh = 7 * 24 * time.Hour
)

func load(c *ui.Context, url string, maxSide int) (ui.ImageSource, time.Duration, error) {
	if maxSide <= 0 {
		maxSide = defaultSide
	}
	return shared.request(url, maxSide, c)
}

// key names a picture at a size, in memory.
type key struct {
	url  string
	side int
}

type entry struct {
	key  key
	src  ui.ImageSource
	size int64
	err  error
	// used is its last showing, came when it came to memory, failed when it
	// last failed.
	used, came, failed time.Time
	// loading says a worker has it or it waits for one.
	loading bool
	// fresh says the disk's copy showed at once, not after a wait: no fade.
	fresh bool
	// preload goes to disk alone, and isn't given up; forgotten was let go of as
	// it downloads, and keeps nothing.
	preload, forgotten bool
	// waiters are the views to build again.
	waiters []*ui.Context
}

type cache struct {
	mu      sync.Mutex
	entries map[key]*entry
	queue   []*entry
	running int
	held    int64
	client  *http.Client
	dir     string
	pruned  bool
	once    sync.Once
	now     func() time.Time
	// limit is memoryLimit, smaller in tests.
	limit int64
}

var shared = newCache()

func newCache() *cache {
	return &cache{entries: map[key]*entry{}, now: time.Now, limit: memoryLimit}
}

// request returns the picture, and starts bringing it if it's not there. c may
// be nil for no view to tell. age is how long ago it came, or -1 for long enough
// not to fade.
func (m *cache) request(url string, side int, c *ui.Context) (ui.ImageSource, time.Duration, error) {
	m.once.Do(func() {
		// Short of memory, the system would rather end the app.
		mygo.App.OnMemoryWarning(m.dropMemory)
	})
	k := key{url, side}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[k]
	if e == nil {
		e = &entry{key: k}
		m.entries[k] = e
	}
	e.used = now
	if e.src != nil {
		age := now.Sub(e.came)
		if e.fresh {
			age = -1
		}
		return e.src, age, nil
	}
	if c != nil && !slices.Contains(e.waiters, c) {
		e.waiters = append(e.waiters, c)
	}
	if !e.loading && (e.err == nil || now.Sub(e.failed) >= retryAfter) {
		e.loading = true
		m.queue = append(m.queue, e)
		m.startLocked()
	}
	return nil, -1, e.err
}

// preload downloads a picture to disk, unless it's there or on its way.
func (m *cache) preload(url string) {
	k := key{url: url}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[k] != nil {
		return
	}
	e := &entry{key: k, preload: true, loading: true, used: m.now()}
	m.entries[k] = e
	m.queue = append(m.queue, e)
	m.startLocked()
}

// startLocked gives workers what waits, the last asked for first: what's on
// screen now. What's gone by its turn leaves, and the views that asked build
// again, to ask again if they still show it.
func (m *cache) startLocked() {
	var told []*ui.Context
	for m.running < workers && len(m.queue) > 0 {
		e := m.queue[len(m.queue)-1]
		m.queue = m.queue[:len(m.queue)-1]
		if e.forgotten {
			told = append(told, e.waiters...)
			e.waiters = nil
			continue
		}
		if !e.preload && m.now().Sub(e.used) > abandoned {
			e.loading = false
			told = append(told, e.waiters...)
			e.waiters = nil
			delete(m.entries, e.key)
			continue
		}
		m.running++
		go m.bring(e)
	}
	if len(m.entries) > tooManyEntries {
		now := m.now()
		for k, e := range m.entries {
			if e.src == nil && !e.loading && now.Sub(e.failed) >= retryAfter {
				told = append(told, e.waiters...)
				delete(m.entries, k)
			}
		}
	}
	if len(told) > 0 {
		go func() {
			for _, c := range told {
				c.Invalidate()
			}
		}()
	}
}

func (m *cache) bring(e *entry) {
	r, err := m.fetch(e.key.url)
	var src ui.ImageSource
	size := int64(0)
	if err == nil && !e.preload {
		src, size, err = safeDecode(r.data, e.key.side)
		if err != nil && r.fallback != nil {
			// A 200 of what's no picture, as a captive portal's page: what it
			// had stands.
			src, size, err = safeDecode(r.fallback, e.key.side)
			r.store = nil
		} else if err != nil && r.fromDisk {
			// Spoiled: the next time asks the server.
			if body, meta := m.paths(e.key.url); body != "" {
				os.Remove(body)
				os.Remove(meta)
			}
		}
	} else if err == nil && r.store != nil && !isPicture(r.data) {
		r.store = nil
	}
	m.mu.Lock()
	gone := e.forgotten
	m.mu.Unlock()
	if err == nil && r.store != nil && !gone {
		r.store()
	}
	m.mu.Lock()
	e.loading = false
	now := m.now()
	switch {
	case gone || e.preload:
		if m.entries[e.key] == e {
			delete(m.entries, e.key)
		}
	case err != nil:
		e.err, e.failed = err, now
	default:
		e.src, e.err, e.size = src, nil, size
		e.came, e.fresh = now, r.fromDisk && now.Sub(e.used) < 50*time.Millisecond
		m.held += size
		m.evictLocked(e)
	}
	waiters := e.waiters
	e.waiters = nil
	m.running--
	m.startLocked()
	m.mu.Unlock()
	for _, c := range waiters {
		c.Invalidate()
	}
}

// isPicture reports whether data can be a picture, without decoding it all.
func isPicture(data []byte) bool {
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		return true
	}
	return bytes.Contains(data[:min(len(data), 4096)], []byte("<svg"))
}

// safeDecode decodes what came from anywhere, which a decoder may panic on, and
// which may say it's enormous: a small file can ask for gigabytes, which would
// end the app.
func safeDecode(data []byte, side int) (src ui.ImageSource, size int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("netimage: decoding: %v", r)
		}
	}()
	if c, _, cerr := image.DecodeConfig(bytes.NewReader(data)); cerr == nil && int64(c.Width)*int64(c.Height) > maxPixels {
		return nil, 0, fmt.Errorf("netimage: %d×%d pixels is too many", c.Width, c.Height)
	}
	return decode(data, side)
}

// decode makes the bitmap: on this goroutine, off the main thread.
func decode(data []byte, side int) (ui.ImageSource, int64, error) {
	b, err := ui.DecodeBitmapScaled(data, side)
	if err != nil {
		// An SVG, which has no pixels to scale.
		s, serr := ui.DecodeImage(data)
		if serr != nil {
			return nil, 0, err
		}
		return s, int64(len(data)), nil
	}
	w, h := b.Size()
	return b, int64(w) * int64(h) * 4, nil
}

// evictLocked lets go of the pictures shown longest ago, down to the limit, but
// keep, which just came, and what showed a moment ago: pictures on screen that
// don't fit the limit go over it, rather than each pushing out the next for good.
func (m *cache) evictLocked(keep *entry) {
	if m.held <= m.limit {
		return
	}
	now := m.now()
	for _, e := range m.heldOldestFirstLocked() {
		if m.held <= m.limit || now.Sub(e.used) < onScreen {
			return
		}
		if e != keep {
			m.held -= e.size
			delete(m.entries, e.key)
		}
	}
}

func (m *cache) heldOldestFirstLocked() []*entry {
	var held []*entry
	for _, e := range m.entries {
		if e.src != nil {
			held = append(held, e)
		}
	}
	slices.SortFunc(held, func(a, b *entry) int { return a.used.Compare(b.used) })
	return held
}

// dropMemory lets go of the oldest three quarters. What's on screen is the last
// to go: a window that hasn't built lately still shows it, and would flicker.
func (m *cache) dropMemory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	target := m.held / 4
	for _, e := range m.heldOldestFirstLocked() {
		if m.held <= target {
			return
		}
		m.held -= e.size
		delete(m.entries, e.key)
	}
}

func (m *cache) forget(url string) {
	m.dropLocked(func(k key) bool { return k.url == url })
	if body, meta := m.paths(url); body != "" {
		os.Remove(body)
		os.Remove(meta)
	}
}

func (m *cache) clear() {
	m.dropLocked(func(key) bool { return true })
	if dir := m.directory(); dir != "" {
		os.RemoveAll(dir)
	}
}

// dropLocked lets go of the entries match says: those downloading finish for
// nothing, keeping nothing, as after logging out.
func (m *cache) dropLocked(match func(key) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.entries {
		if !match(k) {
			continue
		}
		e.forgotten = true
		m.held -= e.size
		e.size = 0
		delete(m.entries, k)
	}
}

func (m *cache) directory() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return ""
		}
		m.dir = filepath.Join(cache, "mygo-netimage")
	}
	return m.dir
}

func (m *cache) paths(url string) (body, meta string) {
	dir := m.directory()
	if dir == "" {
		return "", ""
	}
	sum := sha256.Sum256([]byte(url))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(dir, name), filepath.Join(dir, name+".json")
}

// metadata is what a picture's response said of its caching.
type metadata struct {
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	FreshUntil   time.Time `json:"freshUntil"`
}

// fetched is what fetch found.
type fetched struct {
	data []byte
	// fromDisk says it took no download; store writes it to disk, once it's
	// known to be a picture; fallback is the disk's copy, older, while data is
	// new.
	fromDisk bool
	store    func()
	fallback []byte
}

// fetch returns the picture's bytes, from disk while they're fresh, asking the
// server whether they changed once they aren't. A phone offline shows what it
// has, however old.
func (m *cache) fetch(url string) (fetched, error) {
	body, metaPath := m.paths(url)
	var meta metadata
	var cached []byte
	if body != "" {
		if d, err := os.ReadFile(body); err == nil {
			cached = d
			if raw, err := os.ReadFile(metaPath); err == nil {
				json.Unmarshal(raw, &meta)
			}
			if m.now().Before(meta.FreshUntil) {
				return fetched{data: cached, fromDisk: true}, nil
			}
		}
	}
	stale := fetched{data: cached, fromDisk: true}
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fetched{}, err
	}
	if cached != nil {
		if meta.ETag != "" {
			req.Header.Set("If-None-Match", meta.ETag)
		}
		if meta.LastModified != "" {
			req.Header.Set("If-Modified-Since", meta.LastModified)
		}
	}
	res, err := client.Do(req)
	if err != nil {
		if cached != nil {
			return stale, nil
		}
		return fetched{}, err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotModified && cached != nil:
		next := m.metadataOf(res)
		if next.ETag == "" {
			next.ETag = meta.ETag
		}
		if next.LastModified == "" {
			next.LastModified = meta.LastModified
		}
		m.writeMeta(metaPath, next)
		return stale, nil
	case res.StatusCode != http.StatusOK:
		if cached != nil && res.StatusCode >= 500 {
			return stale, nil
		}
		return fetched{}, fmt.Errorf("netimage: %s: %s", url, res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err == nil && len(data) > maxBytes {
		err = fmt.Errorf("netimage: %s is larger than %d bytes", url, maxBytes)
	}
	if err != nil {
		if cached != nil {
			return stale, nil
		}
		return fetched{}, err
	}
	r := fetched{data: data, fallback: cached}
	if body != "" && !noStore(res) {
		meta := m.metadataOf(res)
		r.store = func() { m.store(body, metaPath, data, meta) }
	}
	return r, nil
}

func noStore(res *http.Response) bool {
	return strings.Contains(strings.ToLower(res.Header.Get("Cache-Control")), "no-store")
}

// metadataOf reads Cache-Control's max-age and no-cache, then Expires.
func (m *cache) metadataOf(res *http.Response) metadata {
	meta := metadata{ETag: res.Header.Get("ETag"), LastModified: res.Header.Get("Last-Modified")}
	now := m.now()
	fresh := defaultFresh
	found := false
	for part := range strings.SplitSeq(strings.ToLower(res.Header.Get("Cache-Control")), ",") {
		part = strings.TrimSpace(part)
		if part == "no-cache" {
			fresh, found = 0, true
			break
		}
		if v, ok := strings.CutPrefix(part, "max-age="); ok {
			if n, err := strconv.Atoi(v); err == nil {
				fresh, found = time.Duration(n)*time.Second, true
			}
		}
	}
	if !found {
		if t, err := http.ParseTime(res.Header.Get("Expires")); err == nil {
			fresh = t.Sub(now)
		}
	}
	meta.FreshUntil = now.Add(fresh)
	return meta
}

// store writes the picture, whole or not at all, as other goroutines read it.
func (m *cache) store(body, metaPath string, data []byte, meta metadata) {
	dir := filepath.Dir(body)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "tmp-")
	if err != nil {
		return
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), body)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return
	}
	m.writeMeta(metaPath, meta)
	m.mu.Lock()
	first := !m.pruned
	m.pruned = true
	m.mu.Unlock()
	if first {
		go m.prune(dir, diskLimit)
	}
}

func (m *cache) writeMeta(path string, meta metadata) {
	if raw, err := json.Marshal(meta); err == nil {
		os.WriteFile(path, raw, 0o644)
	}
}

// prune removes what was written longest ago, once a run: a cache may not grow
// without end on a phone.
func (m *cache) prune(dir string, limit int64) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		at   time.Time
	}
	var all []file
	var total int64
	for _, f := range files {
		info, err := f.Info()
		if err != nil || strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		if strings.HasPrefix(f.Name(), "tmp-") {
			os.Remove(filepath.Join(dir, f.Name())) // of a run that ended early
			continue
		}
		all = append(all, file{filepath.Join(dir, f.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	slices.SortFunc(all, func(a, b file) int { return a.at.Compare(b.at) })
	for _, f := range all {
		if total <= limit {
			return
		}
		os.Remove(f.path)
		os.Remove(f.path + ".json")
		total -= f.size
	}
}
