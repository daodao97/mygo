package ui

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestHideScrollbarsRetainsContentInteraction(t *testing.T) {
	for _, tc := range []struct {
		name         string
		container    func(*Context) Element
		x, y         int
		wantX, wantY float32
	}{
		{"vertical", Scroll, 94, 10, 0, 30},
		{"horizontal", ScrollHorizontal, 10, 94, 20, 0},
		{"both", ScrollBoth, 94, 10, 20, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hide, clicks := false, 0
			var offset ScrollState
			tt := NewTester(func(c *Context) {
				sc := tc.container(c).Size(100, 100).TrackScroll(&offset)
				if hide {
					sc.HideScrollbars()
				}
				sc.Children(func() {
					if Box(c).Size(400, 300).Background(Hex("#ffffff")).Clicked() {
						clicks++
					}
				})
			}, 100, 100)
			tt.Move(float32(tc.x), float32(tc.y))
			white := color.RGBA{255, 255, 255, 255}
			if tt.Image().RGBAAt(tc.x, tc.y) == white {
				t.Fatal("visible thumb was not painted")
			}
			tt.ClickAt(float32(tc.x), float32(tc.y))
			if clicks != 0 {
				t.Fatal("visible thumb did not intercept its click")
			}
			hide = true
			tt.Frame()
			if got := tt.Image().RGBAAt(tc.x, tc.y); got != white {
				t.Fatalf("hidden thumb still painted: %v", got)
			}
			tt.ClickAt(float32(tc.x), float32(tc.y))
			if clicks != 1 {
				t.Fatal("hidden thumb still intercepted content")
			}
			tt.Scroll(50, 50, 20, 30)
			if offset.X != tc.wantX || offset.Y != tc.wantY {
				t.Fatalf("wheel scrolling lost: %+v", offset)
			}
			if tc.wantY > 0 {
				before := offset.Y
				touch(tt, platform.PointerDown, 1, 50, 70)
				touch(tt, platform.PointerMove, 1, 50, 30)
				touch(tt, platform.PointerUp, 1, 50, 30)
				if offset.Y <= before || clicks != 1 {
					t.Fatalf("touch scrolling lost or clicked content: %+v, %d", offset, clicks)
				}
			}
			hide = false
			offset.X, offset.Y = 0, 0
			tt.Frame()
			tt.Move(float32(tc.x), float32(tc.y))
			if tt.Image().RGBAAt(tc.x, tc.y) == white {
				t.Fatal("omitting HideScrollbars did not restore thumb")
			}
		})
	}
}
