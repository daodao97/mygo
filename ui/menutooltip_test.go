package ui

import (
	"testing"
	"time"
)

// TestMenuItemTooltip gives an item a tooltip: the system's menu takes it
// as its own items' (shown on macOS and Linux), and a drawn menu shows it
// as the pointer rests on the item.
func TestMenuItemTooltip(t *testing.T) {
	const tip = "Runs the query under the caret"
	menu := func(m *Menu) {
		m.Item("Open")
		m.Item("Run").Tooltip(tip)
	}
	t.Run("system", func(t *testing.T) {
		tt := coreNewTester(func(c *context) {
			coreRow(c).Padding(20).Children(func() { coreText(c, "Report.pdf") }).ContextMenu(menu)
		}, 400, 300)
		if err := tt.RightClick("Report.pdf"); err != nil {
			t.Fatal(err)
		}
		if m := tt.h.menu; m == nil || len(m.Items) != 2 || m.Items[1].ToolTip != tip || m.Items[0].ToolTip != "" {
			t.Fatalf("the system's menu %v", m)
		}
	})
	t.Run("drawn", func(t *testing.T) {
		tt, now := clockTester(func(c *context) {
			coreRow(c).Padding(20).Children(func() { coreText(c, "Report.pdf") }).PopoverMenu(menu)
		}, 400, 300)
		tick := ticker(tt, now)
		if err := tt.RightClick("Report.pdf"); err != nil {
			t.Fatal(err)
		}
		r, ok := tt.Find("Run")
		if !ok {
			t.Fatal("no drawn item Run")
		}
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tick(700 * time.Millisecond)
		if !tt.HasText(tip) {
			t.Fatalf("no tip once the pointer rested on the item: texts %q", tt.Texts())
		}
	})
}
