package ui

import "github.com/egoist/mygo/internal/framewhy"

// animate asks for the next frame built anew, as what moves needs.
func (rt *engine) animate() {
	rt.animating = true
	if framewhy.On {
		framewhy.Note("animate", 1)
	}
}

// repaintFrames asks for frames painting the elements again; full at the
// display's rate rather than at repaintRate.
func (rt *engine) repaintFrames(full bool) {
	rt.repainting = true
	if full {
		rt.repaintFull = true
	}
	if framewhy.On {
		framewhy.Note("repaint", 1)
	}
}

func whyNote(kind string) {
	if framewhy.On {
		framewhy.Note(kind, 2)
	}
}

func whyFrame() { framewhy.Frame() }
