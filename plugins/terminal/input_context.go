package terminal

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/plugins/terminal/internal/vt"
	"github.com/egoist/mygo/ui"
	"github.com/go-text/typesetting/segmenter"
)

// inputContext mirrors only locally typed, unsubmitted text. The remote
// program remains the editor; native caret changes become terminal keys.
// Output/prompt decorations are never copied into the keyboard's document.
type inputContext struct {
	mu    sync.Mutex
	text  []rune
	caret int
}

func (t *Terminal) textContext() (string, int) {
	t.context.mu.Lock()
	defer t.context.mu.Unlock()
	return string(t.context.text), t.context.caret
}
func (t *Terminal) clearInputContext() {
	t.context.mu.Lock()
	defer t.context.mu.Unlock()
	t.context.text, t.context.caret = nil, 0
}
func (t *Terminal) contextText(text string) {
	if !t.opts.InputContext {
		return
	}
	// Use the same lock order as pausing input, so a discarded keystroke
	// cannot leave editable native context behind during recovery.
	t.in.mu.Lock()
	defer t.in.mu.Unlock()
	if t.in.paused {
		return
	}
	t.context.mu.Lock()
	defer t.context.mu.Unlock()
	d := &t.context
	if strings.ContainsAny(text, "\x00\x1b\r") {
		d.text, d.caret = nil, 0
		return
	}
	insert := []rune(text)
	d.text = append(append(append([]rune(nil), d.text[:d.caret]...), insert...), d.text[d.caret:]...)
	d.caret += len(insert)
	// This is keyboard context, not a second persistent editor.
	if len(d.text) > 4096 {
		d.text, d.caret = nil, 0
	}
}
func contextBoundaries(text []rune) []int {
	var seg segmenter.Segmenter
	seg.Init(text)
	bounds := []int{0}
	it := seg.GraphemeIterator()
	for it.Next() {
		g := it.Grapheme()
		bounds = append(bounds, g.Offset+len(g.Text))
	}
	return bounds
}
func contextIndex(bounds []int, caret int) int {
	i := 0
	for i+1 < len(bounds) && bounds[i+1] <= caret {
		i++
	}
	return i
}
func (t *Terminal) contextKey(key vt.Key, mods ui.Modifiers, text string) {
	if !t.opts.InputContext {
		return
	}
	if key == vt.KeyEnter && mods == ui.Alt {
		t.contextText("\n")
		return
	}
	if mods&(ui.Ctrl|ui.Alt|ui.Super) != 0 {
		t.clearInputContext()
		return
	}
	if text != "" {
		t.contextText(text)
		return
	}
	t.context.mu.Lock()
	defer t.context.mu.Unlock()
	d := &t.context
	bounds := contextBoundaries(d.text)
	i := contextIndex(bounds, d.caret)
	switch key {
	case vt.KeyArrowLeft:
		d.caret = bounds[max(0, i-1)]
	case vt.KeyArrowRight:
		d.caret = bounds[min(len(bounds)-1, i+1)]
	case vt.KeyBackspace:
		if i > 0 {
			a, b := bounds[i-1], bounds[i]
			d.text = append(d.text[:a], d.text[b:]...)
			d.caret = a
		}
	case vt.KeyDelete:
		if i+1 < len(bounds) {
			a, b := bounds[i], bounds[i+1]
			d.text = append(d.text[:a], d.text[b:]...)
		}
	default:
		// History, completion, submission and navigation with unknown geometry
		// invalidate the mirror. Ordinary arrow keys still edit the live PTY.
		d.text, d.caret = nil, 0
	}
}

// InsertNewline inserts a line break without sending an unmodified Return.
// It uses bracketed paste when the program enables it;
// Alt+Return is the fallback when the program has not enabled paste mode.
//
// 🔴 两条路径都要把换行镜像进 inputContext。非 paste 那条走 SendKey →
// contextKey，`key == KeyEnter && mods == Alt` 已经处理；paste 这条走
// Paste，而 Paste 是通用剪贴板粘贴 —— 它不该碰 IME 的本地缓冲（粘贴进来的
// 东西不是"本地敲的"），所以这一条得自己写。
//
// 漏了它的症状：输入框里换行不显示，但远端收到了 —— 本地看到的和发出去的
// 不是同一份文本。TestTerminalNativeContextEditsLiveInput 守的就是这条。
func (t *Terminal) InsertNewline() {
	t.mu.Lock()
	paste := t.term != nil && t.term.Mode(vt.ModeBracketedPaste)
	t.mu.Unlock()
	if paste {
		// pasteData mirrors what it pastes into the input context (paste.go),
		// so writing it here too records the newline twice: the editable
		// text grows a blank line the program never received.
		t.Paste("\n")
	} else {
		t.SendKey(ui.KeyEnter, ui.Alt)
	}
}
