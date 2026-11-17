// Package microphone streams raw audio from the device's microphone, for
// speech providers and other code that processes audio itself: mono,
// signed 16-bit little-endian PCM at SampleRate. It is available on iOS
// (AVAudioEngine) and Android (AudioRecord); elsewhere Start reports
// errors.ErrUnsupported.
//
// It is the fork's (FORK.md): #940 has no counterpart.
package microphone

import (
	"context"
	"errors"
	"sync"

	"github.com/egoist/mygo"
)

const pluginID = "microphone"

// SampleRate is the rate of the audio Start delivers.
const SampleRate = 16000

// Chunk is a piece of captured audio. The last chunk has End set; Err tells
// why capture ended early, and is nil after Stop.
type Chunk struct {
	PCM []byte
	End bool
	Err error
}

// ErrDenied is what Start fails with as the user does not allow the
// microphone, and ErrBusy as another capture runs.
var (
	ErrDenied = errors.New("microphone: access is not allowed")
	ErrBusy   = errors.New("microphone: another capture runs")
)

// Capture is a running capture. Its methods are safe from any goroutine,
// idempotent, and do nothing after capture ended: Stop delivers the rest of
// the audio, then the End chunk; Cancel releases the microphone at once.
type Capture struct {
	token             int64
	mu                sync.Mutex
	emit              func(Chunk)
	done              chan struct{}
	finished, stopped bool
}

var (
	captures = struct {
		sync.Mutex
		next int64
		m    map[int64]*Capture
	}{m: map[int64]*Capture{}}
	listen sync.Once

	// stopNative tells the native code to stop capture token, or with
	// cancel to drop it; tests replace it.
	stopNative = func(token int64, cancel bool) {
		go mygo.Native(pluginID).Call(context.Background(), "stop", map[string]any{"token": token, "cancel": cancel}, nil)
	}
)

// Start asks for the microphone's permission and begins capture. Cancel ctx
// to abandon it, which then ends with ctx's error.
//
// emit runs on the main thread, one call at a time, in order. It must not
// block, and must not call Stop or Cancel. Only one capture runs at a time.
func Start(ctx context.Context, emit func(Chunk)) (*Capture, error) {
	if ctx == nil || emit == nil {
		return nil, errors.New("microphone: Start needs a context and a callback")
	}
	p := mygo.Native(pluginID)
	if !p.Available() {
		return nil, mygo.ErrUnsupported
	}
	listen.Do(func() { p.On("audio", onAudio) })
	c := track(emit)
	if err := p.Call(ctx, "start", map[string]any{"token": c.token}, nil); err != nil {
		forget(c)
		stopNative(c.token, true) // as ctx may have ended once it started
		return nil, classify(err)
	}
	go func() {
		select {
		case <-ctx.Done():
			c.end(ctx.Err())
		case <-c.done:
		}
	}()
	return c, nil
}

func track(emit func(Chunk)) *Capture {
	captures.Lock()
	defer captures.Unlock()
	captures.next++
	c := &Capture{token: captures.next, emit: emit, done: make(chan struct{})}
	captures.m[c.token] = c
	return c
}

func forget(c *Capture) {
	captures.Lock()
	delete(captures.m, c.token)
	captures.Unlock()
}

func classify(err error) error {
	var n *mygo.NativeError
	if errors.As(err, &n) {
		switch n.Code {
		case "Denied":
			return ErrDenied
		case "Busy":
			return ErrBusy
		}
	}
	return err
}

// onAudio is the native code's audio of a capture, or its end.
func onAudio(raw []byte) {
	var m struct {
		Token int64  `json:"token"`
		PCM   []byte `json:"pcm"`
		End   bool   `json:"end"`
		Error string `json:"error"`
	}
	if jsonUnmarshal(raw, &m) != nil {
		return
	}
	captures.Lock()
	c := captures.m[m.Token]
	captures.Unlock()
	if c == nil {
		return
	}
	var err error
	if m.Error != "" {
		err = errors.New("microphone: " + m.Error)
	}
	c.deliver(m.PCM, err, m.End)
}

// Stop ends capture; the remaining audio arrives, then the End chunk.
func (c *Capture) Stop() {
	c.mu.Lock()
	if c.finished || c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	c.mu.Unlock()
	stopNative(c.token, false)
}

// Cancel releases the microphone at once; emit is not called again.
func (c *Capture) Cancel() {
	c.mu.Lock()
	if c.finished {
		c.mu.Unlock()
		return
	}
	c.finished = true
	close(c.done)
	c.mu.Unlock()
	forget(c)
	stopNative(c.token, true)
}

// end finishes capture with err, delivering it as the End chunk.
func (c *Capture) end(err error) {
	c.mu.Lock()
	if c.finished {
		c.mu.Unlock()
		return
	}
	c.finished = true
	close(c.done)
	c.emit(Chunk{End: true, Err: err})
	c.mu.Unlock()
	forget(c)
	stopNative(c.token, true)
}

// deliver hands the native code's audio on. The lock keeps chunks in order
// and keeps any from arriving after End or Cancel.
func (c *Capture) deliver(pcm []byte, err error, end bool) {
	c.mu.Lock()
	if c.finished {
		c.mu.Unlock()
		return
	}
	end = end || err != nil
	if end {
		c.finished = true
		close(c.done)
	}
	c.emit(Chunk{PCM: pcm, End: end, Err: err})
	c.mu.Unlock()
	if end {
		forget(c)
	}
}
