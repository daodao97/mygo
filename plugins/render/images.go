package render

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/egoist/mygo/internal/raster"
	"github.com/egoist/mygo/internal/scene"
)

// Images renders the composition's frames into image files named by
// pattern, a path with a verb of fmt for the frame's number, as
// "frames/%04d.png": PNGs, with the alpha of a Transparent composition,
// or JPEGs (.jpg, .jpeg) of Options.Quality. The directory is made if
// need be. Files are encoded on as many cores as there are, while the
// next frames draw.
func Images(ctx context.Context, comp Composition, pattern string, o Options) error {
	if err := comp.validate(); err != nil {
		return err
	}
	if n := strings.Count(strings.ReplaceAll(pattern, "%%", ""), "%"); n != 1 {
		return errors.New("render: name the frames' files with one verb for their number, as frame-%04d.png")
	}
	jpg := false
	switch strings.ToLower(filepath.Ext(pattern)) {
	case ".png":
	case ".jpg", ".jpeg":
		jpg = true
	default:
		return fmt.Errorf("render: no image format for %q: name .png or .jpg files", filepath.Ext(pattern))
	}
	quality := o.Quality
	if quality == 0 {
		quality = 90
	}
	if quality < 1 || quality > 100 {
		return errors.New("render: Quality is between 1 and 100")
	}
	if dir := filepath.Dir(fmt.Sprintf(pattern, 0)); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return run(ctx, comp, o, func(*session, drawer) (sink, error) {
		return newImageSink(pattern, comp.Width, comp.Height, jpg, quality, comp.Transparent && !jpg), nil
	})
}

// imageSink encodes frames into files on workers of its own.
type imageSink struct {
	pattern string
	w, h    int
	jpg     bool
	quality int
	alpha   bool

	jobs chan imageJob
	stop sync.Once
	wg   sync.WaitGroup
	free sync.Pool
	mu   sync.Mutex
	err  error
}

type imageJob struct {
	i   int
	pix *[]byte
}

func newImageSink(pattern string, w, h int, jpg bool, quality int, alpha bool) *imageSink {
	s := &imageSink{pattern: pattern, w: w, h: h, jpg: jpg, quality: quality, alpha: alpha}
	workers := max(runtime.GOMAXPROCS(0), 1)
	s.jobs = make(chan imageJob, workers)
	for range workers {
		s.wg.Add(1)
		go s.work()
	}
	return s
}

func (s *imageSink) frame(i int, pix []byte) error {
	if err := s.failed(); err != nil {
		return err
	}
	buf, _ := s.free.Get().(*[]byte)
	if buf == nil {
		b := make([]byte, len(pix))
		buf = &b
	}
	copy(*buf, pix)
	s.jobs <- imageJob{i, buf}
	return nil
}

func (s *imageSink) failed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *imageSink) work() {
	defer s.wg.Done()
	for j := range s.jobs {
		if s.failed() == nil {
			if err := s.write(j.i, *j.pix); err != nil {
				s.mu.Lock()
				if s.err == nil {
					s.err = err
				}
				s.mu.Unlock()
			}
		}
		s.free.Put(j.pix)
	}
}

// write encodes frame i's pixels, BGRA, into its file.
func (s *imageSink) write(i int, pix []byte) error {
	r := image.Rect(0, 0, s.w, s.h)
	var img image.Image
	if s.alpha {
		m := image.NewNRGBA(r)
		unpremultiply(m.Pix, pix)
		img = m
	} else {
		m := image.NewRGBA(r)
		bgraToRGBA(m.Pix, pix)
		for k := 3; k < len(m.Pix); k += 4 {
			// Opaque frames: what the view left transparent is black, as
			// in a video.
			m.Pix[k] = 255
		}
		img = m
	}
	f, err := os.Create(fmt.Sprintf(s.pattern, i))
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<16)
	if s.jpg {
		err = jpeg.Encode(w, img, &jpeg.Options{Quality: s.quality})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(w, img)
	}
	if err == nil {
		err = w.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// unpremultiply copies premultiplied BGRA pixels into straight RGBA ones.
func unpremultiply(dst, src []byte) {
	for i := 0; i+3 < len(src); i += 4 {
		a := src[i+3]
		switch a {
		case 0:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = 0, 0, 0, 0
		case 255:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], 255
		default:
			un := func(c byte) byte { return byte(min((int(c)*255+int(a)/2)/int(a), 255)) }
			dst[i], dst[i+1], dst[i+2], dst[i+3] = un(src[i+2]), un(src[i+1]), un(src[i]), a
		}
	}
}

func (s *imageSink) close() error {
	s.stop.Do(func() { close(s.jobs) })
	s.wg.Wait()
	return s.failed()
}

func (s *imageSink) abort() {
	s.mu.Lock()
	if s.err == nil {
		s.err = context.Canceled
	}
	s.mu.Unlock()
	s.stop.Do(func() { close(s.jobs) })
	s.wg.Wait()
}

// cpuDrawer draws frames with the CPU renderer, which redraws only what
// changed from frame to frame.
type cpuDrawer struct{ r raster.Renderer }

func (d *cpuDrawer) submit(s *scene.Scene) error { d.r.Render(s); return nil }
func (d *cpuDrawer) pixels() ([]byte, error)     { return d.r.Image.Pix, nil }
func (d *cpuDrawer) name() string                { return "CPU" }
func (d *cpuDrawer) release()                    { d.r.Release() }
