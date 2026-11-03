//go:build mygo_ios_input_test

package ios

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/platform"
)

type snapshotClient struct {
	onChange func()
	text     string
	sel      platform.TextSelection
	mark     *platform.TextRange
}

func (c *snapshotClient) TextForRange(r platform.TextRange) (string, platform.TextRange) {
	a, z := platform.UTF16ByteOffset(c.text, r.Start), platform.UTF16ByteOffset(c.text, r.End)
	return c.text[a:z], platform.TextRange{Start: platform.UTF16Len(c.text[:a]), End: platform.UTF16Len(c.text[:z])}
}
func (c *snapshotClient) Selection() platform.TextSelection     { return c.sel }
func (c *snapshotClient) SetSelection(s platform.TextSelection) { c.sel = s }
func (c *snapshotClient) MarkedRange() (platform.TextRange, bool) {
	if c.mark != nil {
		return *c.mark, true
	}
	return platform.TextRange{}, false
}
func (c *snapshotClient) ReplaceText(r *platform.TextRange, text string) {
	if r == nil {
		if c.mark != nil {
			r = c.mark
		} else {
			r = &c.sel.Range
		}
	}
	a, z := platform.UTF16ByteOffset(c.text, r.Start), platform.UTF16ByteOffset(c.text, r.End)
	c.text = c.text[:a] + text + c.text[z:]
	at := r.Start + platform.UTF16Len(text)
	c.sel = platform.TextSelection{Range: platform.TextRange{Start: at, End: at}}
	c.mark = nil
	if c.onChange != nil {
		c.onChange()
	}
}
func (c *snapshotClient) SetMarkedText(r *platform.TextRange, text string, sel platform.TextRange) {
	if r == nil {
		if c.mark != nil {
			r = c.mark
		} else {
			r = &c.sel.Range
		}
	}
	start := r.Start
	c.ReplaceText(r, text)
	c.mark = &platform.TextRange{Start: start, End: start + platform.UTF16Len(text)}
	c.sel = platform.TextSelection{Range: platform.TextRange{Start: start + sel.Start, End: start + sel.End}}
}
func (c *snapshotClient) UnmarkText() { c.mark = nil }
func (c *snapshotClient) BoundsForRange(r platform.TextRange) (platform.RectF, platform.TextRange, bool) {
	return platform.RectF{X: float64(r.Start), W: 1, H: 20}, r, true
}
func (c *snapshotClient) IndexForPoint(x, y float64) (int, bool) { return int(x), true }

func TestClientInputBoundedUnicodeEdits(t *testing.T) {
	prefix := strings.Repeat("前👋", 900)
	c := &snapshotClient{text: prefix + "tail"}
	end := platform.UTF16Len(c.text)
	c.sel.Range = platform.TextRange{Start: end, End: end}
	s := clientInput{client: c}
	state := s.snapshot(platform.TextInputState{})
	if s.base == 0 {
		t.Fatal("did not exercise a bounded proxy")
	}
	s.commit(state.Text+"中", utf8.RuneCountInString(state.Text)+1)
	if c.text != prefix+"tail中" {
		t.Fatal("snapshot replaced text outside its proxy")
	}
	s.snapshot(platform.TextInputState{})
	s.selectRange(0, 2)
	if c.sel.Range.Start != s.base || c.sel.Range.End <= s.base {
		t.Fatal("selection lost its absolute UTF-16 base", c.sel)
	}
}

func TestClientInputCandidateCommitsOnce(t *testing.T) {
	c := &snapshotClient{text: "hi "}
	c.sel.Range = platform.TextRange{Start: 3, End: 3}
	s := clientInput{client: c}
	s.snapshot(platform.TextInputState{})
	s.compose("hi zhong", 3, 8, 8)
	if c.text != "hi zhong" {
		t.Fatal(c.text)
	}
	s.snapshot(platform.TextInputState{})
	s.compose("hi 中wen", 4, 7, 7)
	if c.text != "hi 中wen" {
		t.Fatal("candidate duplicated or old preedit committed", c.text)
	}
	s.commit("hi 中文", 5)
	if c.text != "hi 中文" || c.mark != nil {
		t.Fatal("final candidate was not committed once", c.text, c.mark)
	}
}

func TestClientInputSelectAllKeepsBothEndpoints(t *testing.T) {
	c := &snapshotClient{text: strings.Repeat("long 👋 document\n", 600)}
	c.sel.Range.End = platform.UTF16Len(c.text)
	s := clientInput{client: c}
	state := s.snapshot(platform.TextInputState{})
	if s.base != 0 || state.Text != c.text || state.Start != 0 || state.End != utf8.RuneCountInString(c.text) {
		t.Fatal("full selection clipped to caret context")
	}
}

func TestClientInputCompositionPinsProxy(t *testing.T) {
	c := &snapshotClient{text: strings.Repeat("前", 1800)}
	c.sel.Range = platform.TextRange{Start: 1800, End: 1800}
	s := clientInput{client: c}
	state := s.snapshot(platform.TextInputState{})
	base := s.base
	c.onChange = func() {
		s.snapshot(platform.TextInputState{})
		if s.base != base {
			t.Fatal("reentrant client publication rebased composition")
		}
	}
	at := utf8.RuneCountInString(state.Text)
	s.compose(state.Text+"zhong", at, at+5, at+5)
	s.snapshot(platform.TextInputState{})
	if s.base != base {
		t.Fatal("composition rebased UIKit's live proxy")
	}
	s.commit(state.Text+"中", at+1)
	if c.text != strings.Repeat("前", 1800)+"中" {
		t.Fatal("composition lost context")
	}
}
