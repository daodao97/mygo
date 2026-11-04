package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/platform"
)

func edgeSelectionFixture() *Tester {
	tt := snapshotTester(func(c *Context) {
		Scroll(c).Width(320).Height(300).Children(func() {
			Column(c).Selectable().Children(func() {
				for i := 0; i < 30; i++ {
					Text(c, strings.Repeat("中文字", 20)).Width(300)
				}
			})
		})
	}, 320, 300)
	tt.rt.host = &selectionTestHost{tt.h}
	tt.Frame()
	return tt
}

func copiedRunes(tt *Tester) int {
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceCommand, Text: "copy"})
	return utf8.RuneCountInString(tt.Clipboard())
}

// A long press on the last lines above a scroll container's edge used to
// auto-scroll at once, so a slight drag selected hundreds of runes.
func TestTouchSelectionEdgeNeedsDragTowardEdge(t *testing.T) {
	tt := edgeSelectionFixture()
	touch(tt, platform.PointerDown, 1, 50, 285)
	tt.Advance(500 * time.Millisecond)
	for i := 1; i <= 3; i++ {
		touch(tt, platform.PointerMove, 1, 50+float32(2*i), 285+float32(i))
		tt.Advance(16 * time.Millisecond)
	}
	tt.Advance(500 * time.Millisecond)
	if n := copiedRunes(tt); n != 1 {
		t.Fatalf("a slight drag in the edge band selected %d runes", n)
	}
	touch(tt, platform.PointerCancel, 1, 56, 288)

	// A drag that moves into the edge still scrolls the selection on.
	tt = edgeSelectionFixture()
	touch(tt, platform.PointerDown, 2, 50, 200)
	tt.Advance(500 * time.Millisecond)
	touch(tt, platform.PointerMove, 2, 50, 290)
	tt.Advance(16 * time.Millisecond)
	before := copiedRunes(tt)
	tt.Advance(300 * time.Millisecond)
	if after := copiedRunes(tt); after <= before {
		t.Fatalf("dragging into the edge did not scroll the selection: %d then %d", before, after)
	}
	touch(tt, platform.PointerUp, 2, 50, 290)
}
