package render

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ffmpegSink pipes the frames to ffmpeg, as raw BGRA, and the sound
// track from a file it reads as a second input.
type ffmpegSink struct {
	spec  videoSpec
	cmd   *exec.Cmd
	in    io.WriteCloser
	w     *bufio.Writer
	errs  bytes.Buffer
	sound string // the file of the sound track, "" for none
	buf   []byte
}

func newFFmpegSink(ctx context.Context, spec videoSpec) (_ sink, err error) {
	bin := spec.ffmpeg
	if bin == "" {
		bin = "ffmpeg"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("render: no ffmpeg: %w", err)
	}
	s := &ffmpegSink{spec: spec}
	defer func() {
		if err != nil {
			s.abort()
		}
	}()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "rawvideo", "-pix_fmt", "bgra", "-s", fmt.Sprintf("%dx%d", spec.w, spec.h),
		"-framerate", fmt.Sprintf("%d/%d", spec.num, spec.den), "-i", "pipe:0"}
	if spec.sound != nil {
		if s.sound, err = writeRaw(filepath.Dir(spec.path), spec.sound); err != nil {
			return nil, err
		}
		args = append(args, "-f", "s16le", "-ar", strconv.Itoa(sampleRate), "-ac", "2", "-i", s.sound,
			"-map", "0:v", "-map", "1:a")
		if spec.container == webm {
			args = append(args, "-c:a", "libopus")
		} else {
			args = append(args, "-c:a", "aac")
		}
		args = append(args, "-b:a", strconv.Itoa(spec.audioBitrate))
	}
	args = append(args, s.videoArgs()...)
	args = append(args, "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", spec.path)
	// -nostdin keeps ffmpeg from reading keys; the frames come on its
	// standard input all the same, as pipe:0.
	s.cmd = exec.CommandContext(ctx, path, args...)
	s.cmd.Stderr = &s.errs
	if s.in, err = s.cmd.StdinPipe(); err != nil {
		return nil, err
	}
	if err := s.cmd.Start(); err != nil {
		return nil, fmt.Errorf("render: ffmpeg: %w", err)
	}
	s.w = bufio.NewWriterSize(s.in, 1<<20)
	return s, nil
}

// writeRaw writes the sound track into a file of its own in dir.
func writeRaw(dir string, track []int16) (string, error) {
	f, err := os.CreateTemp(dir, ".render-sound-*.raw")
	if err != nil {
		return "", err
	}
	w := bufio.NewWriterSize(f, 1<<16)
	err = binary.Write(w, binary.LittleEndian, track)
	if err == nil {
		err = w.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// videoArgs returns ffmpeg's arguments for the codec: a bitrate if one
// was asked for, else a constant quality, as x264's CRF 18.
func (s *ffmpegSink) videoArgs() []string {
	sp := s.spec
	rate := func(crf string) []string {
		if sp.bitrate > 0 {
			return []string{"-b:v", strconv.Itoa(sp.bitrate)}
		}
		return []string{"-crf", crf}
	}
	var a []string
	switch sp.codec {
	case H264:
		a = append([]string{"-c:v", "libx264", "-preset", "medium", "-pix_fmt", "yuv420p"}, rate("18")...)
	case HEVC:
		a = append([]string{"-c:v", "libx265", "-preset", "medium", "-pix_fmt", "yuv420p", "-tag:v", "hvc1"}, rate("22")...)
	case ProRes:
		a = []string{"-c:v", "prores_ks", "-profile:v", "3", "-pix_fmt", "yuv422p10le"}
	case ProRes4444:
		pf := "yuv444p10le"
		if sp.alpha {
			pf = "yuva444p10le"
		}
		a = []string{"-c:v", "prores_ks", "-profile:v", "4", "-pix_fmt", pf}
	case VP9:
		pf := "yuv420p"
		if sp.alpha {
			pf = "yuva420p"
		}
		a = []string{"-c:v", "libvpx-vp9", "-pix_fmt", pf, "-row-mt", "1"}
		if sp.bitrate > 0 {
			a = append(a, "-b:v", strconv.Itoa(sp.bitrate))
		} else {
			a = append(a, "-crf", "31", "-b:v", "0")
		}
	}
	if sp.container == mp4 {
		a = append(a, "-movflags", "+faststart")
	}
	return a
}

func (s *ffmpegSink) frame(_ int, pix []byte) error {
	if s.spec.alpha {
		// ffmpeg's BGRA is straight.
		if len(s.buf) < len(pix) {
			s.buf = make([]byte, len(pix))
		}
		unpremultiplyBGRA(s.buf, pix)
		pix = s.buf[:len(pix)]
	}
	if _, err := s.w.Write(pix); err != nil {
		return s.failed(err)
	}
	return nil
}

// unpremultiplyBGRA copies premultiplied BGRA pixels into straight ones.
func unpremultiplyBGRA(dst, src []byte) {
	unpremultiply(dst, src)
	// unpremultiply swapped red and blue, for RGBA: swap them back.
	for i := 0; i+3 < len(dst); i += 4 {
		dst[i], dst[i+2] = dst[i+2], dst[i]
	}
}

// failed returns what ffmpeg said once it stopped, after err.
func (s *ffmpegSink) failed(err error) error {
	s.in.Close()
	s.cmd.Wait()
	if msg := strings.TrimSpace(s.errs.String()); msg != "" {
		return errors.New("render: ffmpeg: " + msg)
	}
	return fmt.Errorf("render: ffmpeg: %w", err)
}

func (s *ffmpegSink) close() error {
	defer s.removeSound()
	if err := s.w.Flush(); err != nil {
		return s.failed(err)
	}
	s.in.Close()
	if err := s.cmd.Wait(); err != nil {
		return s.failed(err)
	}
	return nil
}

func (s *ffmpegSink) removeSound() {
	if s.sound != "" {
		os.Remove(s.sound)
		s.sound = ""
	}
}

func (s *ffmpegSink) abort() {
	if s.in != nil {
		s.in.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	s.removeSound()
}
