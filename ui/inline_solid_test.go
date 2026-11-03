package ui

import "testing"

// A link's hover underline is solid for the text inside it: the spans of
// a RichText in a Link take it over the link's style.
func TestOverKeepsSolidUnderline(t *testing.T) {
	got := over(Span{Text: "https://example.com/gypsy"}, Span{Underline: true, solid: true})
	if !got.Underline || !got.solid {
		t.Errorf("span over a hovered link: %+v", got)
	}
	if got := over(Span{Text: "x"}, Span{Underline: true}); got.solid {
		t.Errorf("span over an underline: solid")
	}
}
