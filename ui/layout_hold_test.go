package ui

import (
	"fmt"
	"strings"
	"testing"
)

// A commit pass takes what a measuring pass kept of the items and lines,
// instead of building them again. What it lays out must be the same, so
// the trees are laid out with what was kept and without it, and every box
// compared.
func TestHoldKeepsLayout(t *testing.T) {
	for _, build := range holdTrees {
		layout := func() [][4]float32 {
			_, root := build()
			forgetLayout(root)
			layoutBox(root, root.w, root.h)
			return boxesOf(root)
		}
		noHold = true
		without := layout()
		noHold = false
		with := layout()
		noHold = false
		if len(with) != len(without) {
			t.Fatalf("%d boxes with what was kept, %d without", len(with), len(without))
		}
		for i := range with {
			if with[i] != without[i] {
				t.Errorf("box %d is %v, and %v without what was kept",
					i, boxString(with[i]), boxString(without[i]))
				break
			}
		}
	}
}

// What the commit passes take is what the measuring passes kept: a tree
// whose rows are measured and then laid out has both.
func TestHoldKeepsWhatMeasured(t *testing.T) {
	for _, build := range holdTrees {
		tt, root := build()
		kept, taken := tt.rt.flex.holdSaved, tt.rt.flex.holdUsed
		refreshed := tt.rt.flex.holdRefreshed
		forgetLayout(root)
		layoutBox(root, root.w, root.h)
		kept, taken = tt.rt.flex.holdSaved-kept, tt.rt.flex.holdUsed-taken
		refreshed = tt.rt.flex.holdRefreshed - refreshed
		t.Logf("kept %8d taken %8d refreshed %8d", kept, taken, refreshed)
		if taken == 0 {
			t.Errorf("no commit pass took what a measuring pass kept")
		}
		if taken > kept {
			t.Errorf("took %d of %d kept", taken, kept)
		}
		if refreshed == 0 {
			t.Errorf("no commit pass crossed the rows it took again")
		}
	}
}

// holdTrees are trees whose rows are measured at an indefinite height and
// then laid out at a definite one, which is what a commit pass takes what
// a measuring pass kept of.
var holdTrees = []func() (*Tester, *node){
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				for range 3 {
					coreRow(c).Width(280).Children(func() {
						coreBox(c).Size(40, 20).Shrink(0)
						coreBox(c).Width(60).Shrink(0)
						coreBox(c).Size(30, 30).Shrink(0)
					})
				}
			})
		}, 300, 400)
		return tt, root
	},
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				coreRow(c).Width(280).Children(func() {
					// A row in a row: the inner one is measured, and
					// laid out at the height it was measured at.
					coreRow(c).Width(200).Children(func() {
						coreBox(c).Size(40, 20).Shrink(0)
						coreBox(c).Width(60).Shrink(0)
					})
					coreBox(c).Size(30, 30).Shrink(0)
				})
			})
		}, 300, 400)
		return tt, root
	},
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				// A row that wraps, with a child in percent: what it
				// resolved is kept, and taken again at its height.
				coreRow(c).Width(280).Wrap().Children(func() {
					for range 8 {
						coreBox(c).Width(60).Height(20).Shrink(0)
					}
					coreBox(c).WidthPercent(50).Height(20).Shrink(0)
				})
			})
		}, 300, 400)
		return tt, root
	},
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				coreRow(c).Width(280).Children(func() {
					// A child that grows into what the row has: its
					// size is resolved in the row's lines.
					coreBox(c).Width(40).Grow(1).Shrink(0)
					coreBox(c).Width(40).Grow(2).Shrink(0)
					coreBox(c).Height(24).Shrink(0)
				})
			})
		}, 300, 400)
		return tt, root
	},
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				coreRow(c).Width(280).Children(func() {
					// A child whose height is absolute, and bounded by a
					// percentage of the row's height: the bound applies
					// at the height the row is committed at, and not at the
					// indefinite height it was measured at, so the size
					// it is given at another height is not the one it was
					// measured at.
					coreBox(c).Width(60).Height(100).Shrink(0)
					coreBox(c).Width(60).Height(10).MinHeightPercent(50).Shrink(0)
				})
			})
		}, 300, 400)
		return tt, root
	},
}

// boxesOf returns the box of every element under root, in the order the
// layout laid them out.
func boxesOf(e *node) [][4]float32 {
	var out [][4]float32
	var walk func(*node)
	walk = func(e *node) {
		for c := e; c != nil; c = c.next {
			out = append(out, [4]float32{c.x, c.y, c.w, c.h})
			walk(c.first)
		}
	}
	walk(e)
	return out
}

func boxString(b [4]float32) string {
	s := make([]string, len(b))
	for i, v := range b {
		s[i] = fmt.Sprintf("%v", v)
	}
	return "{" + strings.Join(s, " ") + "}"
}
