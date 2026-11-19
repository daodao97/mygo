//go:build js

package text

// hearFontLoads has the canvas engine tell the System of the faces that
// loaded (fontLoaded). s.mu is held.
func (s *System) hearFontLoads() {
	if e, ok := s.eng.(*canvasEngine); ok {
		e.loaded = s.fontLoaded
	}
}

// fontLoaded forgets what text measured and drew before a face registered
// had loaded, the engine's fonts and words with it, and tells those
// watching (WatchFonts). It runs on a goroutine of its own.
func (s *System) fontLoaded() {
	s.mu.Lock()
	if f, ok := s.eng.(fontForgetter); ok {
		s.forgetAll(f)
	}
	watchers := make([]func(), 0, len(s.watchers))
	for _, fn := range s.watchers {
		watchers = append(watchers, fn)
	}
	s.mu.Unlock()
	for _, fn := range watchers {
		fn()
	}
}

// WatchFonts has fn run, on any goroutine, whenever a face registered has
// loaded, which browsers do in the background (RegisterFont): the layouts
// made before are then to be made anew (Generation), and drawn again.
// stop ends it.
func (s *System) WatchFonts(fn func()) (stop func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watchers == nil {
		s.watchers = map[*func()]func(){}
	}
	key := &fn
	s.watchers[key] = fn
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.watchers, key)
	}
}
