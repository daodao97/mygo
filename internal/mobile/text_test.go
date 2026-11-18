package mobile

import (
	"testing"

	"github.com/egoist/mygo/internal/mobile/wire"
)

// glueHost is the host's glue as start finds it, before the app made its
// backend: it answers calls on the caller's thread.
type glueHost struct{ fonts []string }

func (h *glueHost) call([]byte) []byte { return nil }
func (h *glueHost) post()              {}
func (h *glueHost) callHere(msg []byte) []byte {
	r := wire.Reader{Buf: msg}
	if r.Uvarint() != wire.CallRegisterFont {
		return nil
	}
	r.Bytes()
	family := r.String()
	h.fonts = append(h.fonts, family)
	var w wire.Writer
	w.String(family)
	w.String("")
	return w.Buf
}

// A font registered before App.Run, as an app's main does first, reaches
// the host: the backend comes only with App.Run.
func TestRegisterFontBeforeBackend(t *testing.T) {
	old := current.Swap(nil)
	t.Cleanup(func() { current.Store(old) })
	h := &glueHost{}
	theHost = h
	t.Cleanup(func() { theHost = nil })
	family, err := RegisterFont([]byte("font"), "Brand")
	if err != nil || family != "Brand" {
		t.Fatalf("RegisterFont = %q, %v", family, err)
	}
	if len(h.fonts) != 1 {
		t.Fatalf("the host got %q", h.fonts)
	}
}
