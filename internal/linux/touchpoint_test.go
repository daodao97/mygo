//go:build linux && (amd64 || arm64)

package linux

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestTouchPoints checks that GDK's touch sequences become IDs that
// stay with a finger from its begin to its end or cancel, and that a
// sequence GDK frees and gives the next finger gets another (#1879).
func TestTouchPoints(t *testing.T) {
	type touch struct {
		typ int32 // 37 begin, 38 update, 39 end, 40 cancel, 0 reset
		seq uintptr
	}
	type finger struct {
		kind platform.SurfaceEventKind
		id   uint64
	}
	const (
		down   = platform.TouchDown
		move   = platform.TouchMove
		up     = platform.TouchUp
		cancel = platform.TouchCancel
	)
	for _, tc := range []struct {
		name    string
		touches []touch
		want    []finger
	}{
		{"one finger", []touch{{37, 0xa0}, {38, 0xa0}, {39, 0xa0}}, []finger{{down, 1}, {move, 1}, {up, 1}}},
		{"four fingers swiping", []touch{
			{37, 0xa0}, {37, 0xb0}, {37, 0xc0}, {37, 0xd0},
			{38, 0xc0}, {38, 0xa0}, {38, 0xd0}, {38, 0xb0},
			{39, 0xb0}, {39, 0xa0}, {39, 0xd0}, {39, 0xc0},
		}, []finger{
			{down, 1}, {down, 2}, {down, 3}, {down, 4},
			{move, 3}, {move, 1}, {move, 4}, {move, 2},
			{up, 2}, {up, 1}, {up, 4}, {up, 3},
		}},
		{"canceled", []touch{{37, 0xa0}, {37, 0xb0}, {40, 0xa0}, {38, 0xb0}, {38, 0xa0}},
			[]finger{{down, 1}, {down, 2}, {cancel, 1}, {move, 2}}},
		// GDK frees a sequence as its finger lifts, and may give the next
		// finger the same.
		{"sequence used again", []touch{{37, 0xa0}, {39, 0xa0}, {37, 0xa0}, {39, 0xa0}},
			[]finger{{down, 1}, {up, 1}, {down, 2}, {up, 2}}},
		// Fingers whose ends went unseen, as under a grab, are canceled
		// as new touches begin.
		{"reset", []touch{{37, 0xa0}, {37, 0xb0}, {0, 0}, {37, 0xa0}, {38, 0xb0}},
			[]finger{{down, 1}, {down, 2}, {cancel, 1}, {cancel, 2}, {down, 3}}},
		{"unknown sequences", []touch{{38, 0xe0}, {39, 0xe0}, {40, 0xe0}}, nil},
		{"begun twice", []touch{{37, 0xa0}, {37, 0xa0}}, []finger{{down, 1}}},
	} {
		var p touchPoints
		var got []finger
		for _, tt := range tc.touches {
			if tt.typ == 0 {
				for _, id := range p.reset() {
					got = append(got, finger{cancel, id})
				}
				continue
			}
			if kind, id, ok := p.event(tt.typ, tt.seq); ok {
				got = append(got, finger{kind, id})
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got %v\nwant %v", tc.name, got, tc.want)
		}
	}
}
