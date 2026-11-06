package richtext

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestInputHookConsumesCustomKeyAndKeepsEditing(t *testing.T) {
	doc := NewDocument(NewText(""))
	calls := 0
	tt := ui.NewTester(func(c *ui.Context) {
		EditorBase(c, doc, EditorOptions{Label: "input", OnInput: func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyF2 {
				calls++
				return true
			}
			return false
		}}).Fill()
	}, 320, 120)
	if err := tt.Click("input"); err != nil {
		t.Fatal(err)
	}
	tt.Type("hello")
	tt.Key(0, ui.KeyF2)
	tt.Type(" world")
	tt.Frame()
	if calls != 1 || doc.Text().String() != "hello world" {
		t.Fatalf("hook=%d text=%q", calls, doc.Text().String())
	}
}
