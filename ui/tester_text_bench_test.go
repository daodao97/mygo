package ui

import (
	"fmt"
	"testing"
)

// textHeavyView draws about 250 texts, as an app's real window does: a
// sidebar, a pane of paragraphs and an inspector (#2085).
func textHeavyView(c *context) {
	coreRow(c).Fill().Gap(12).Children(func() {
		coreColumn(c).Width(240).Gap(4).Children(func() {
			for i := range 80 {
				coreText(c, fmt.Sprintf("Lesson %d: shapes and colors", i+1))
			}
		})
		coreColumn(c).Grow(1).Gap(6).Children(func() {
			for i := range 90 {
				coreText(c, fmt.Sprintf("Paragraph %d of the lesson, which wraps across its pane as the window narrows.", i+1))
			}
		})
		coreColumn(c).Width(300).Gap(4).Children(func() {
			for i := range 80 {
				coreText(c, fmt.Sprintf("Property %d = %.2f", i, float64(i)*1.25))
			}
		})
	})
}

// BenchmarkTesterFirstFrame renders a view of about 250 texts into a new
// Tester each iteration, as a test suite of many Testers does (#2085).
func BenchmarkTesterFirstFrame(b *testing.B) {
	for b.Loop() {
		coreRender(textHeavyView, 1500, 940, 1)
	}
}

// BenchmarkNewTesterFirstFrame is BenchmarkTesterFirstFrame through
// NewTester, whose Testers the test drops rather than ends.
func BenchmarkNewTesterFirstFrame(b *testing.B) {
	for b.Loop() {
		tt := coreNewTester(textHeavyView, 1500, 940)
		tt.Image()
	}
}
