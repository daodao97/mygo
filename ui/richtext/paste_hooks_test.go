package richtext

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestApplicationPasteHookHandlesNativeCommands(t *testing.T) {
	for _, command := range []string{"paste", "pasteAndMatchStyle"} {
		for _, handled := range []bool{false, true} {
			t.Run(command+map[bool]string{false: "/fallback", true: "/handled"}[handled], func(t *testing.T) {
				doc := NewDocument(NewText("abc"))
				calls := 0
				tt := ui.NewTester(func(c *ui.Context) {
					EditorBase(c, doc, EditorOptions{OnPaste: func() bool { calls++; return handled }}).Fill().AutoFocus()
				}, 300, 120)
				tt.Frame()
				doc.SetSelection(1, 2)
				tt.Frame()
				tt.SetClipboard("X")
				version := doc.Version()
				tt.Command(command)
				tt.Frame()
				if calls != 1 {
					t.Fatalf("hook called %d times", calls)
				}
				if handled {
					a, z := doc.Selection()
					if doc.Text().String() != "abc" || doc.Version() != version || a != 1 || z != 2 {
						t.Fatal("consumed paste changed text, history or selection")
					}
				} else if doc.Text().String() != "aXc" {
					t.Fatalf("normal paste changed: %q", doc.Text().String())
				}
			})
		}
	}
}

func TestApplicationPasteHookKeepsDocumentEdits(t *testing.T) {
	doc := NewDocument(NewText("ab"))
	tt := ui.NewTester(func(c *ui.Context) {
		EditorBase(c, doc, EditorOptions{OnPaste: func() bool {
			doc.InsertText("X")
			return true
		}}).Fill().AutoFocus()
	}, 300, 120)
	tt.Frame()
	doc.SetSelection(1, 1)
	tt.Frame()
	tt.Command("paste")
	tt.Frame()
	a, z := doc.Selection()
	if doc.Text().String() != "aXb" || a != 2 || z != 2 {
		t.Fatalf("callback edit lost text or caret: %q %d %d", doc.Text().String(), a, z)
	}
}
