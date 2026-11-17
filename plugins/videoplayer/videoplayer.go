// Package videoplayer plays videos and their sound in a window's native UI, on
// phones and tablets, with the system's decoders: AVPlayer on iOS, MediaPlayer
// on Android, from a file or a URL they can read, HLS among them.
//
//	p, err := videoplayer.New(win, "https://example.com/film.m3u8", videoplayer.Options{AutoPlay: true})
//	...
//	ui.WebView(c, p).Fill() // in the window's view; what it draws after shows over the video
//
// The view shows under the content, as a web view does. On desktops and in
// browsers New reports errors.ErrUnsupported.
package videoplayer

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

const pluginID = "videoplayer"

// Options are how a Player starts.
type Options struct {
	// AutoPlay plays it as soon as it can.
	AutoPlay bool
	// Loop plays it again from the start as it ends.
	Loop bool
	// Muted plays it without sound.
	Muted bool
}

// State is what a Player tells of its video.
type State struct {
	// Ready says it can play: Duration and the size are known.
	Ready bool
	// Playing says it plays now, and Ended that it got to the end.
	Playing, Ended bool
	// Position is where it is, and Duration how long the video is.
	Position, Duration time.Duration
	// Width and Height are the video's size in pixels.
	Width, Height int
	// Error says why it cannot play, if so.
	Error string
}

type wireState struct {
	ID         int     `json:"id"`
	Ready      bool    `json:"ready"`
	Playing    bool    `json:"playing"`
	Ended      bool    `json:"ended"`
	PositionMS float64 `json:"positionMs"`
	DurationMS float64 `json:"durationMs"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Error      string  `json:"error"`
}

// Player plays a video. Show it with ui.WebView.
type Player struct {
	view *mygo.PluginView

	mu      sync.Mutex
	state   State
	onState []func(State)
	off     func()
}

// New makes a player of a file's path or a URL, in a window showing native UI.
func New(w *mygo.Window, source string, o Options) (*Player, error) {
	v, err := mygo.NewPluginView(w, "videoplayer", map[string]any{
		"source": source, "autoPlay": o.AutoPlay, "loop": o.Loop, "muted": o.Muted,
	})
	if err != nil {
		return nil, err
	}
	p := &Player{view: v}
	p.off = mygo.Native(pluginID).On("state", func(data []byte) {
		var s wireState
		if json.Unmarshal(data, &s) != nil || s.ID != v.ID {
			return
		}
		st := State{s.Ready, s.Playing, s.Ended, time.Duration(s.PositionMS) * time.Millisecond,
			time.Duration(s.DurationMS) * time.Millisecond, s.Width, s.Height, s.Error}
		p.mu.Lock()
		p.state = st
		list := append([]func(State){}, p.onState...)
		p.mu.Unlock()
		for _, fn := range list {
			fn(st)
		}
	})
	return p, nil
}

// SurfaceWebView gives package ui the view to show.
func (p *Player) SurfaceWebView(conn *surface.Conn) platform.WebView {
	return p.view.SurfaceWebView(conn)
}

// State tells what the player last told of its video.
func (p *Player) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// OnState calls fn on the main thread as the state changes, and a few times a
// second as it plays.
func (p *Player) OnState(fn func(State)) {
	p.mu.Lock()
	p.onState = append(p.onState, fn)
	p.mu.Unlock()
}

func (p *Player) call(method string, args map[string]any) {
	if args == nil {
		args = map[string]any{}
	}
	args["id"] = p.view.ID
	go mygo.Native(pluginID).Call(context.Background(), method, args, nil)
}

// Play plays it from where it is.
func (p *Player) Play() { p.call("play", nil) }

// Pause stops it where it is.
func (p *Player) Pause() { p.call("pause", nil) }

// Seek goes to a position.
func (p *Player) Seek(to time.Duration) { p.call("seek", map[string]any{"ms": to.Milliseconds()}) }

// SetVolume sets how loud it plays, from 0 to 1.
func (p *Player) SetVolume(v float64) { p.call("volume", map[string]any{"volume": v}) }

// SetRate sets how fast it plays: 1 is normal speed.
func (p *Player) SetRate(r float64) { p.call("rate", map[string]any{"rate": r}) }

// Destroy stops it and takes its view away.
func (p *Player) Destroy() {
	p.off()
	p.call("dispose", nil)
	p.view.Destroy()
}
