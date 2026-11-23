package ui

import (
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/egoist/mygo/transfer"
)

func TestOnPaste(t *testing.T) {
	text := "hello world"
	var seen []string
	attached := 0
	tt := NewTester(func(c *Context) {
		Text(c, strconv.Itoa(attached)+" attached")
		TextArea(c, &text).Width(200).Label("draft").OnPaste(func(s string) bool {
			seen = append(seen, s)
			if len(s) > 5 {
				attached++
				return true
			}
			return false
		})
	}, 300, 100)
	tt.Click("draft")
	tt.Key(Cmd, KeyEnd)

	// Left: the paste inserts as usual.
	tt.SetClipboard("!")
	tt.Key(Cmd, KeyV)
	if text != "hello world!" {
		t.Fatalf("a paste left became %q", text)
	}

	// Taken, by Cmd+V and by the menu: nothing changes, the selection
	// stays, as when a web page prevents a paste's default.
	tt.SetClipboard("a large block")
	tt.Key(Cmd, KeyV)
	if !tt.HasText("1 attached") {
		t.Fatalf("a paste taken showed %q", tt.Texts())
	}
	tt.Key(Cmd, KeyA)
	tt.Command("paste")
	if text != "hello world!" {
		t.Fatalf("a paste taken became %q", text)
	}
	if !tt.HasText("2 attached") {
		t.Fatalf("a paste taken showed %q", tt.Texts())
	}
	tt.Command("delete")
	if text != "" {
		t.Fatalf("the selection a paste taken left became %q", text)
	}
	// A paste taken leaves nothing to undo.
	tt.Key(Cmd, KeyZ)
	if text != "hello world!" {
		t.Fatalf("undone, it became %q", text)
	}
	tt.Key(Cmd, KeyZ)
	if text != "hello world" {
		t.Fatalf("undone twice, it became %q", text)
	}

	// An empty clipboard, as with an image, is seen too.
	tt.SetClipboard("")
	tt.Command("paste")
	want := []string{"!", "a large block", "a large block", ""}
	if len(seen) != len(want) {
		t.Fatalf("saw %q", seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("saw %q", seen)
		}
	}
}

// TestOnPasteReplaceText checks OnPaste's pattern for text of the app's
// own in place of a paste: fn reads the image or the files from the
// clipboard, and the view inserts a link with ReplaceText in the frame
// the paste builds, at the caret or over the selection, as one step of
// undo. Setting the bound value from fn is lost.
func TestOnPasteReplaceText(t *testing.T) {
	text := "before after"
	setValue := false
	var insert string
	var pasted transfer.Data
	tt := NewTester(func(c *Context) {
		svc := c.Services()
		in := TextArea(c, &text).Width(300).Label("draft").OnPaste(func(string) bool {
			d := svc.ReadClipboardData(transfer.PNG, transfer.FileList)
			if len(d.Items()) == 0 {
				return false
			}
			pasted = d
			if setValue {
				text = "lost"
			} else {
				insert = "![image](u)"
			}
			return true
		})
		if insert != "" {
			start, end := in.TextSelection()
			in.ReplaceText(start, end, insert)
			insert = ""
		}
	}, 400, 200)
	tt.Click("draft")
	tt.Key(Cmd, KeyHome)
	for range len("before ") {
		tt.Key(0, KeyRight)
	}

	png := transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, []byte("\x89PNG"))))
	tt.SetClipboardData(png)
	setValue = true
	tt.Key(Cmd, KeyV)
	tt.Frame()
	if text != "before after" {
		t.Fatalf("the value fn set became %q", text)
	}

	// No frame between: the paste builds the one that inserts.
	setValue = false
	tt.Key(Cmd, KeyV)
	if b, err := pasted.Read(transfer.PNG); err != nil || string(b) != "\x89PNG" {
		t.Fatalf("fn read the image as %q, %v", b, err)
	}
	if text != "before ![image](u)after" {
		t.Fatalf("inserted at the caret, it became %q", text)
	}
	tt.Type("!")
	if text != "before ![image](u)!after" {
		t.Fatalf("typed after the insert, it became %q", text)
	}
	tt.Key(Cmd, KeyZ)
	tt.Key(Cmd, KeyZ)
	if text != "before after" {
		t.Fatalf("undone, it became %q", text)
	}

	// Files over a selection.
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	files, err := transfer.FileData(a, b)
	if err != nil {
		t.Fatal(err)
	}
	tt.SetClipboardData(files)
	tt.Key(Cmd, KeyA)
	tt.Key(Cmd, KeyV)
	if got, err := pasted.Files(); err != nil || len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("fn read the files as %q, %v", got, err)
	}
	if text != "![image](u)" {
		t.Fatalf("inserted over the selection, it became %q", text)
	}
	tt.Key(Cmd, KeyZ)
	if text != "before after" {
		t.Fatalf("undone, it became %q", text)
	}
}

// TestPasteCommandShortcut checks that the Edit menu's Paste, which takes
// Command+V on macOS before the window sees it, runs the window's Cmd+V
// when no text input has the focus, and pastes into one that does.
func TestPasteCommandShortcut(t *testing.T) {
	text := ""
	pastes := 0
	tt := NewTester(func(c *Context) {
		TextInput(c, &text).Width(200).Label("field")
		c.OnShortcut(Cmd, KeyV, func() { pastes++ })
	}, 300, 100)
	want := 0
	if runtime.GOOS == "darwin" {
		want = 1
	}
	tt.Command("paste")
	tt.Frame()
	if pastes != want {
		t.Fatalf("the menu's Paste ran Cmd+V %d times, want %d", pastes, want)
	}
	tt.Click("field")
	tt.SetClipboard("hi")
	tt.Command("paste")
	tt.Frame()
	if text != "hi" || pastes != want {
		t.Fatalf("a paste into the field made %q and ran Cmd+V %d times", text, pastes)
	}
}

// TestPasteLoneCR pastes text whose lines end with a lone \r, as old Mac
// text does: a TextArea takes each as a newline, as it does \r\n, and an
// Input as a space (#1915).
func TestPasteLoneCR(t *testing.T) {
	area, input := "", ""
	tt := NewTester(func(c *Context) {
		TextArea(c, &area).Width(200).Label("area")
		TextInput(c, &input).Width(200).Label("input")
	}, 300, 200)
	tt.Click("area")
	tt.SetClipboard("1. A\r2. B\r\n3. C")
	tt.Key(Cmd, KeyV)
	if area != "1. A\n2. B\n3. C" {
		t.Errorf("a TextArea pasted %q, want each line break a newline", area)
	}
	tt.Click("input")
	tt.Key(Cmd, KeyV)
	if input != "1. A 2. B 3. C" {
		t.Errorf("a TextInput pasted %q, want each line break a space", input)
	}
}

// A paste is a step of undo of its own, even of one character right after
// typing, as NSTextView's: typing before it and after it stay apart.
func TestPasteIsItsOwnUndoStep(t *testing.T) {
	text := ""
	tt := NewTester(func(c *Context) { TextArea(c, &text).Width(200).Label("draft") }, 300, 100)
	tt.Click("draft")
	tt.Type("a")
	tt.Type("b")
	tt.SetClipboard("x")
	tt.Key(Cmd, KeyV)
	tt.Type("c")
	for _, want := range []string{"abx", "ab", ""} {
		tt.Key(Cmd, KeyZ)
		if text != want {
			t.Fatalf("undo made %q, want %q", text, want)
		}
	}
}
