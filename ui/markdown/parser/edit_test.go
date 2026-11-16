package parser

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// TestEditKeepsTailInlines checks that incremental edits that resync a
// later block keep the inlines and offsets a parse of the whole source
// gives, including when a reference definition comes or goes.
func TestEditKeepsTailInlines(t *testing.T) {
	src := "first [docs] **bold**\n\n" +
		"[docs]: /one\n\n" +
		"# middle\n\n" +
		"tail [docs] *emphasis*\n\n" +
		"last line\n"
	const tail = "tail [docs] *emphasis*"
	for _, c := range []struct {
		name, from, text string
		length           int
		preserve         bool
	}{
		{"insert", "middle", "X", 0, true},
		{"replace", "middle", "middles", len("middle"), true},
		{"delete", "m", "", 1, true},
		{"same length", "middle", "Middle", len("middle"), true},
		{"add definition", "last line", "[docs]: /two\n\nlast line", len("last line"), false},
		{"change definition", "/one", "/changed", len("/one"), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			from := strings.Index(src, c.from)
			if from < 0 {
				t.Fatalf("%q is not in the source", c.from)
			}
			to := from + c.length
			next := src[:from] + c.text + src[to:]
			d := New(Default)
			d.Set(src)
			inlinesAll(d)
			rev := uint32(0)
			if c.preserve {
				rev = d.Block(blockWithText(d, tail)).Revision()
			}
			d.Edit(from, to, c.text)
			if d.Source() != next {
				t.Fatalf("source = %q, want %q", d.Source(), next)
			}
			fresh := Parse(next)
			if c.preserve {
				if got := d.Block(blockWithText(d, tail)).Revision(); got != rev {
					t.Errorf("resynced tail was parsed again: revision %d, want %d", got, rev)
				}
			}
			got, want := dumpEditedDocument(d), dumpEditedDocument(fresh)
			if got != want {
				t.Errorf("edited document:\n%s\nwhole parse:\n%s", got, want)
			}
		})
	}
}

func blockWithText(d *Document, text string) int {
	for i := range d.Children(0) {
		if d.LineCount(i) > 0 {
			line, _ := d.Line(i, 0)
			if line == text {
				return i
			}
		}
	}
	return -1
}

// TestEditWorkScalesWithTail checks that editing near the end does not
// copy all the segments and inlines of the document before the edit.
func TestEditWorkScalesWithTail(t *testing.T) {
	editBytes := func(src string) uint64 {
		var before, after runtime.MemStats
		d := New(Default)
		d.Set(src)
		inlinesAll(d)
		at := strings.Index(src, "two")
		runtime.ReadMemStats(&before)
		d.Edit(at, at, "x")
		inlinesAll(d)
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	many, few := editSources(2000)
	if len(many) != len(few) {
		t.Fatalf("sources differ in length: %d and %d", len(many), len(few))
	}
	withManySegs, withFewSegs := editBytes(many), editBytes(few)
	if withManySegs > withFewSegs*2 {
		t.Errorf("prefix of many segments: %d B, of few segments: %d B", withManySegs, withFewSegs)
	}
}

// editSources gives two sources of the same length: the first has many
// segments before the edit, the second has one long line instead.
func editSources(n int) (string, string) {
	tail := "one\n\ntwo\n\ntail [docs]\n\n[docs]: /u\n"
	return strings.Repeat("a\n", n) + "\n" + tail,
		strings.Repeat("a", 2*n-1) + "\n\n" + tail
}

func dumpEditedDocument(d *Document) string {
	var out strings.Builder
	for i := 1; i < d.Len(); i++ {
		b := d.Block(i)
		fmt.Fprintf(&out, "%d %v [%d,%d) parent %d next %d\n", i, b.Kind, b.Start, b.End, b.Parent, b.Next)
		for k := 0; k < d.LineCount(i); k++ {
			line, at := d.Line(i, k)
			if got := d.Source()[at : at+len(line)]; got != line {
				fmt.Fprintf(&out, "  line %d %q at %d is %q\n", k, line, at, got)
			} else {
				fmt.Fprintf(&out, "  line %d %q at %d\n", k, line, at)
			}
		}
		for _, in := range d.Inlines(i) {
			fmt.Fprintf(&out, "  %v %q %q [%d,%d) next %d\n", in.Kind, in.Text, in.Dest, in.Start, in.End, in.Next)
		}
	}
	return out.String()
}
