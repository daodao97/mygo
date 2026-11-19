//go:build windows && (amd64 || arm64)

package windows

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// TestThumbButtonLayout asserts the thumbButton the shell reads as C laid
// THUMBBUTTON out (ShObjIdl_core.h): the icon sits 8-aligned between the
// bitmap and the tooltip.
func TestThumbButtonLayout(t *testing.T) {
	var b thumbButton
	if got, want := unsafe.Offsetof(b.icon), uintptr(16); got != want {
		t.Errorf("the icon lies at %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(b.tip), uintptr(24); got != want {
		t.Errorf("the tooltip lies at %d, want %d", got, want)
	}
	if got, want := unsafe.Sizeof(b), uintptr(552); got != want {
		t.Errorf("the button is %d bytes, want %d", got, want)
	}
}

// TestThumbButtonTipAndFlags checks tooltip truncation and flag updates.
func TestThumbButtonTipAndFlags(t *testing.T) {
	var b thumbButton
	b.setTip(strings.Repeat("x", 300))
	if b.tip[258] != 'x' || b.tip[259] != 0 {
		t.Error("a long tooltip is not cut to 259 runes and a zero")
	}
	b.setTip("")
	if b.tip[0] != 0 {
		t.Error("an empty tooltip is not empty")
	}
	b.setFlags(true, true)
	if b.flags != thbfDisabled|thbfHidden {
		t.Errorf("flags = %#x, want THBF_DISABLED and THBF_HIDDEN", b.flags)
	}
	// Reusing the same button clears the old flag bits.
	b.setFlags(false, true)
	if b.flags != thbfHidden {
		t.Errorf("flags = %#x, want THBF_HIDDEN", b.flags)
	}
	b.setFlags(false, false)
	if b.flags != 0 { // THBF_ENABLED
		t.Errorf("flags = %#x, want THBF_ENABLED", b.flags)
	}
}

// TestThumbBarState checks PNG validation with skipTaskbar, nil updates
// keeping icons, replacements, and atomic rollback on a bad PNG.
func TestThumbBarState(t *testing.T) {
	makePNG := func(c color.RGBA) []byte {
		var buf bytes.Buffer
		img := image.NewRGBA(image.Rect(0, 0, 32, 32))
		draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	red := makePNG(color.RGBA{200, 40, 40, 255})
	blue := makePNG(color.RGBA{40, 40, 200, 255})
	green := makePNG(color.RGBA{40, 200, 40, 255})

	w := &window{b: &Backend{taskbarFailed: true}, skipTaskbar: true}

	// Even with skipTaskbar, a bad first PNG must fail and keep nothing.
	if err := w.setThumbBar([]platform.ThumbButton{{Icon: []byte("bad")}}); err == nil {
		t.Fatal("SetThumbBar accepted invalid PNG while skipTaskbar is true")
	}
	if len(w.thumb.buttons) != 0 {
		t.Fatal("buttons kept after failed initial SetThumbBar")
	}

	if err := w.setThumbBar([]platform.ThumbButton{
		{Icon: red, ToolTip: "Play"},
		{Icon: blue, ToolTip: "Next"},
	}); err != nil {
		t.Fatalf("initial SetThumbBar: %v", err)
	}

	// Update with button 0's icon unchanged, replacing only button 1's.
	if err := w.setThumbBar([]platform.ThumbButton{
		{Icon: red, ToolTip: "Play!"},
		{Icon: green, ToolTip: "Skip", Disabled: true},
	}); err != nil {
		t.Fatalf("update SetThumbBar: %v", err)
	}
	if !bytes.Equal(w.thumb.buttons[0].Icon, red) || !bytes.Equal(w.thumb.buttons[1].Icon, green) {
		t.Fatal("the update did not keep the PNGs")
	}
	// Only the icons that differ from the shell's are built.
	shell := []platform.ThumbButton{{Icon: red}, {Icon: blue}}
	icons, err := w.makeThumbIcons(w.thumb.buttons, shell)
	if err != nil {
		t.Fatalf("makeThumbIcons: %v", err)
	}
	if len(icons) != 2 || icons[0] != 0 || icons[1] == 0 {
		t.Fatalf("icons = %v, want only button 1's", icons)
	}
	destroyIcons(icons)

	// A bad replacement PNG fails without mutating the cached buttons, and
	// so does a different count.
	if err := w.setThumbBar([]platform.ThumbButton{
		{Icon: []byte("corrupt"), ToolTip: "Broken"},
		{Icon: green, ToolTip: "Skip"},
	}); err == nil {
		t.Fatal("update SetThumbBar accepted invalid replacement PNG")
	}
	if err := w.setThumbBar([]platform.ThumbButton{{Icon: red}}); err == nil {
		t.Fatal("update SetThumbBar accepted a different count")
	}
	if w.thumb.buttons[0].ToolTip != "Play!" || !bytes.Equal(w.thumb.buttons[0].Icon, red) {
		t.Fatalf("failed update mutated cached button 0: %+v", w.thumb.buttons[0])
	}
}
