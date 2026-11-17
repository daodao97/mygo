//go:build !js

package mygo

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// shownNowPlaying waits for the backend to have been told n times and
// returns what it shows.
func shownNowPlaying(t *testing.T, n int) *platform.NowPlayingInfo {
	t.Helper()
	wait.Until(t, "the backend shows what plays", func() bool { return onMainValue(func() bool { return fb.NowPlayingSets >= n }) })
	return onMainValue(func() *platform.NowPlayingInfo { return fb.Playing })
}

func TestNowPlaying(t *testing.T) {
	defer NowPlaying.Clear()
	sets := onMainValue(func() int { return fb.NowPlayingSets })
	NowPlaying.Set(NowPlayingInfo{
		Title: "Spring", Artist: "Vivaldi", Album: "The Four Seasons", Artwork: []byte("\x89PNG"),
		Duration: 3 * time.Minute, Position: time.Minute, State: PlaybackPaused,
	})
	got := shownNowPlaying(t, sets+1)
	if got == nil || got.Title != "Spring" || got.Artist != "Vivaldi" || got.Album != "The Four Seasons" || string(got.Artwork) != "\x89PNG" {
		t.Fatalf("shown = %+v", got)
	}
	if got.State != "paused" || got.Position != time.Minute || got.Duration != 3*time.Minute || got.Rate != 1 {
		t.Errorf("state %q, position %v, duration %v, rate %v", got.State, got.Position, got.Duration, got.Rate)
	}
	if want := platform.MediaPlay | platform.MediaPause | platform.MediaStop; got.Commands != want {
		t.Errorf("default commands = %b, want %b", got.Commands, want)
	}

	type command struct {
		cmd MediaCommand
		pos time.Duration
	}
	var cmds []command
	off := NowPlaying.OnCommand(func(cmd MediaCommand, pos time.Duration) { cmds = append(cmds, command{cmd, pos}) })
	defer off()
	onMain(func() {
		fb.EmitMediaCommand(platform.MediaTogglePlayPause, 0) // paused: play
		fb.EmitMediaCommand(platform.MediaNext, 0)            // not offered
		fb.EmitMediaCommand(platform.MediaStop, 0)
	})
	NowPlaying.Set(NowPlayingInfo{Title: "Summer", Duration: time.Minute, Commands: MediaPause | MediaNext | MediaSeek})
	got = shownNowPlaying(t, sets+2)
	if got.State != "playing" || got.Commands != platform.MediaPause|platform.MediaNext|platform.MediaSeek {
		t.Errorf("state %q, commands %b", got.State, got.Commands)
	}
	onMain(func() {
		fb.EmitMediaCommand(platform.MediaTogglePlayPause, 0) // playing: pause
		fb.EmitMediaCommand(platform.MediaNext, 0)
		fb.EmitMediaCommand(platform.MediaSeek, 2*time.Minute) // past the end
		fb.EmitMediaCommand(platform.MediaPlay, 0)             // not offered
	})
	// Commands other than seeking get where playback is: a minute into
	// Spring, paused, and somewhere in Summer, which plays.
	const playing = -1
	want := []command{{MediaPlay, time.Minute}, {MediaStop, time.Minute}, {MediaPause, playing}, {MediaNext, playing}, {MediaSeek, time.Minute}}
	if len(cmds) != len(want) {
		t.Fatalf("commands = %v, want %v", cmds, want)
	}
	for i, w := range want {
		if c := cmds[i]; c.cmd != w.cmd || w.pos != playing && c.pos != w.pos || w.pos == playing && (c.pos < 0 || c.pos > time.Minute) {
			t.Errorf("command %d = %v, want %v", i, c, w)
		}
	}

	NowPlaying.SetPlayback(PlaybackStopped, 0)
	if got = shownNowPlaying(t, sets+3); got.State != "stopped" {
		t.Errorf("state after SetPlayback = %q", got.State)
	}
	NowPlaying.Clear()
	if got = shownNowPlaying(t, sets+4); got != nil {
		t.Errorf("shown after Clear = %+v", got)
	}
	if _, ok := NowPlaying.Info(); ok {
		t.Error("Info after Clear reports something shown")
	}
	n := len(cmds)
	onMain(func() { fb.EmitMediaCommand(platform.MediaPause, 0) })
	if len(cmds) != n {
		t.Error("a command reached OnCommand with nothing shown")
	}
}

// Control drives audio's players, and hears of their ends.
var (
	_ MediaPlayer                       = (*audio.Player)(nil)
	_ interface{ OnEnd(func()) func() } = (*audio.Player)(nil)
)

// player is a MediaPlayer of a minute.
type player struct {
	playing bool
	pos     time.Duration
	end     func()
}

func (p *player) Play()                   { p.playing = true }
func (p *player) Pause()                  { p.playing = false }
func (p *player) Stop()                   { p.playing, p.pos = false, 0 }
func (p *player) Playing() bool           { return p.playing }
func (p *player) Seek(t time.Duration)    { p.pos = t }
func (p *player) Position() time.Duration { return p.pos }
func (p *player) Duration() time.Duration { return time.Minute }
func (p *player) OnEnd(fn func()) func()  { p.end = fn; return func() { p.end = nil } }

func TestNowPlayingControl(t *testing.T) {
	defer NowPlaying.Clear()
	p := &player{pos: 10 * time.Second}
	release := NowPlaying.Control(p)
	defer release()
	sets := onMainValue(func() int { return fb.NowPlayingSets })
	NowPlaying.Set(NowPlayingInfo{Title: "Autumn", Commands: MediaNext})
	got := shownNowPlaying(t, sets+1)
	if got.State != "paused" || got.Position != 10*time.Second || got.Duration != time.Minute {
		t.Errorf("state %q, position %v, duration %v", got.State, got.Position, got.Duration)
	}
	if want := platform.MediaPlay | platform.MediaPause | platform.MediaStop | platform.MediaNext | platform.MediaSeek; got.Commands != want {
		t.Errorf("commands = %b, want %b", got.Commands, want)
	}

	var next int
	off := NowPlaying.OnCommand(func(cmd MediaCommand, _ time.Duration) {
		if cmd == MediaNext {
			next++
		}
	})
	defer off()
	onMain(func() {
		fb.EmitMediaCommand(platform.MediaTogglePlayPause, 0)
		fb.EmitMediaCommand(platform.MediaSeek, 30*time.Second)
		fb.EmitMediaCommand(platform.MediaNext, 0)
	})
	if !p.playing || p.pos != 30*time.Second || next != 1 {
		t.Errorf("playing %v, position %v, next %d", p.playing, p.pos, next)
	}
	got = shownNowPlaying(t, sets+3)
	if got.State != "playing" {
		t.Errorf("state after play = %q", got.State)
	}

	// The player plays to its end.
	p.playing, p.pos = false, time.Minute
	p.end()
	if got = shownNowPlaying(t, sets+4); got.State != "stopped" {
		t.Errorf("state at the end = %q", got.State)
	}

	release()
	if p.end != nil {
		t.Error("release left the end listener")
	}
	onMain(func() { fb.EmitMediaCommand(platform.MediaPlay, 0) })
	if p.playing {
		t.Error("a command reached the released player")
	}
}

// Artists reach the backend as a list, joined for the systems that show
// one artist, and win over Artist; Artist alone is a list of one.
func TestNowPlayingArtists(t *testing.T) {
	defer NowPlaying.Clear()
	sets := onMainValue(func() int { return fb.NowPlayingSets })
	artists := []string{"Daft Punk", "", "Pharrell Williams"}
	NowPlaying.Set(NowPlayingInfo{Title: "Get Lucky", Artist: "Someone else", Artists: artists})
	artists[0] = "changed after Set"
	got := shownNowPlaying(t, sets+1)
	if want := []string{"Daft Punk", "Pharrell Williams"}; !slices.Equal(got.Artists, want) || got.Artist != "Daft Punk, Pharrell Williams" {
		t.Errorf("artists %q, artist %q; want %q joined", got.Artists, got.Artist, want)
	}
	if info, _ := NowPlaying.Info(); !slices.Equal(info.Artists, []string{"Daft Punk", "", "Pharrell Williams"}) || info.Artist != "Someone else" {
		t.Errorf("Info: artists %q, artist %q, want them as set", info.Artists, info.Artist)
	}

	for i, c := range []struct {
		info    NowPlayingInfo
		artists []string
		artist  string
	}{
		{NowPlayingInfo{Artist: "Vivaldi"}, []string{"Vivaldi"}, "Vivaldi"},
		{NowPlayingInfo{Artist: "Vivaldi", Artists: []string{""}}, []string{"Vivaldi"}, "Vivaldi"},
		{NowPlayingInfo{Artist: "Simon, Garfunkel"}, []string{"Simon, Garfunkel"}, "Simon, Garfunkel"},
		{NowPlayingInfo{}, nil, ""},
	} {
		NowPlaying.Set(c.info)
		got := shownNowPlaying(t, sets+2+i)
		if !slices.Equal(got.Artists, c.artists) || got.Artist != c.artist {
			t.Errorf("%+v: artists %q, artist %q; want %q, %q", c.info, got.Artists, got.Artist, c.artists, c.artist)
		}
	}
}

// Raising the player, as MPRIS clients do, shows and focuses the window
// that had the keyboard last, and is no command of the app's.
func TestNowPlayingRaise(t *testing.T) {
	defer NowPlaying.Clear()
	var cmds []MediaCommand
	off := NowPlaying.OnCommand(func(cmd MediaCommand, _ time.Duration) { cmds = append(cmds, cmd) })
	defer off()
	sets := onMainValue(func() int { return fb.NowPlayingSets })
	NowPlaying.Set(NowPlayingInfo{Title: "Spring", Commands: MediaPlay | MediaCommand(platform.MediaRaise)})
	if got := shownNowPlaying(t, sets+1); got.Commands != platform.MediaPlay {
		t.Errorf("commands = %b, want play alone", got.Commands)
	}

	a := NewWindow(WindowOptions{Title: "A", Hidden: true})
	defer a.Destroy()
	b := NewWindow(WindowOptions{Title: "B", Hidden: true})
	defer b.Destroy()
	fl, _ := testFlyout(t, FlyoutOptions{Parent: a, Focusable: true, Content: ui.View(func(c *ui.Context) {})})
	shows := map[*Window]int{}
	for _, w := range []*Window{a, b} {
		w.OnShow(func() { shows[w]++ })
	}
	raise := func() { onMain(func() { fb.EmitMediaCommand(platform.MediaRaise, 0) }) }

	// a, then b, then a flyout had the keyboard: b shows, not the flyout.
	a.Focus()
	b.Focus()
	fl.Focus()
	fl.Hide()
	a.Hide()
	b.Hide()
	raise()
	if !b.IsVisible() || a.IsVisible() || fl.IsVisible() || shows[b] != 1 || shows[a] != 0 {
		t.Errorf("after b had the keyboard: a visible %v, b visible %v, flyout visible %v, shows %v", a.IsVisible(), b.IsVisible(), fl.IsVisible(), shows)
	}
	// Then a had it: a shows.
	a.Focus()
	b.Hide()
	raise()
	if !a.IsVisible() || b.IsVisible() || shows[a] != 1 {
		t.Errorf("after a had the keyboard: a visible %v, b visible %v, shows %v", a.IsVisible(), b.IsVisible(), shows)
	}
	if len(cmds) != 0 {
		t.Errorf("raising reached OnCommand: %v", cmds)
	}
}
