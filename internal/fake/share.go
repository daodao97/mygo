package fake

import "github.com/egoist/mygo/internal/platform"

type sharing struct{ b *Backend }

func (b *Backend) Sharing() platform.Sharing { return sharing{b} }

func (s sharing) Show(_ platform.Window, o platform.ShareOptions, done func(platform.ShareResult, error)) {
	s.b.mu.Lock()
	s.b.LastShare = o
	s.b.LastShare.Files = append([]string(nil), o.Files...)
	r, err := s.b.ShareResult, s.b.ShareError
	s.b.mu.Unlock()
	done(r, err)
}
