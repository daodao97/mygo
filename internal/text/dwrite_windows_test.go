//go:build windows && (amd64 || arm64)

package text

import (
	"runtime"
	"slices"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

// Each engine's in-memory loader reads a font's data where the app keeps
// it, through the owner register gives it (fontData), rather than copying
// it: a copy an engine, as each Tester's has, grew a test process by a
// large font's size a Tester (#1902). DirectWrite holds the owner of the
// data it does not copy, once more for each engine, and each engine still
// lays out with the font. Go Mono is monospaced, so "iiiiiiii" is as wide
// as "mmmmmmmm" only in it.
func TestDWriteRegisterKeepsFontDataInPlace(t *testing.T) {
	const family = "MyGo Test Owned Mono"
	data := slices.Clone(gomono.TTF) // on the heap, which register pins
	monospaced := func(s *System) bool {
		width := func(text string) float32 {
			return s.Layout(Params{Text: text, Style: Style{Size: 20, Family: family}}).Lines[0].Width
		}
		i, m := width("iiiiiiii"), width("mmmmmmmm")
		return i > m-0.5 && i < m+0.5
	}
	var systems []*System
	for i := range 3 {
		s := newSystem()
		if i == 0 {
			s.mu.Lock()
			added := s.engine().(*dwrite).factory5 != 0
			s.mu.Unlock()
			if !added {
				t.Skip("no IDWriteFactory5 (Windows 10 version 1703)")
			}
		}
		before := fontData.owner.refs.Load()
		if err := s.RegisterFont(data, family); err != nil {
			t.Fatal(err)
		}
		if got := fontData.owner.refs.Load(); got <= before {
			t.Errorf("engine %d: DirectWrite holds the owner of the font's data %d times, as before registering it: it copied the data", i+1, got)
		}
		if !monospaced(s) {
			t.Errorf("engine %d: text does not use the registered font", i+1)
		}
		systems = append(systems, s)
	}
	fontData.mu.Lock()
	pinned := fontData.pinned[&data[0]]
	fontData.mu.Unlock()
	if !pinned {
		t.Error("the font's data is not pinned")
	}
	runtime.KeepAlive(systems)
}
