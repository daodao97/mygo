package preferences

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

// launch begins a run of the app whose preferences are the file it returns,
// in dir.
func launch(t *testing.T, dir string) string {
	t.Helper()
	mygo.App.SetPath(mygo.PathUserData, dir)
	reset := func() {
		mu.Lock()
		defer mu.Unlock()
		loaded, values = false, nil
	}
	reset()
	t.Cleanup(func() {
		reset()
		mygo.App.SetPath(mygo.PathUserData, "")
	})
	return filepath.Join(dir, "preferences.json")
}

// A file that could not be read is not one to write over: the next time it
// may be read.
func TestFailedReadKeepsFile(t *testing.T) {
	dir := t.TempDir()
	file := launch(t, dir)
	// A directory in the file's place fails to be read on every system.
	if err := os.Mkdir(file, 0o755); err != nil {
		t.Fatal(err)
	}
	var theme string
	if Get("theme", &theme) {
		t.Error("a value from a file that can't be read")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Set("window", 1); err != nil {
		t.Fatal(err)
	}
	launch(t, dir)
	if !Get("theme", &theme) || theme != "dark" {
		t.Errorf("theme is %q after a read that failed, want dark", theme)
	}
}

// While the file can't be read, Set and Delete say so.
func TestUnreadableFile(t *testing.T) {
	file := launch(t, t.TempDir())
	if err := os.Mkdir(file, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Set("window", 1); err == nil {
		t.Error("Set wrote over a file it can't read")
	}
	if err := Delete("theme"); err == nil {
		t.Error("Delete wrote over a file it can't read")
	}
	if keys := Keys(); len(keys) != 0 {
		t.Errorf("keys %q of a file that can't be read", keys)
	}
}

// A file with no preferences in it, null too, starts over.
func TestStartsOver(t *testing.T) {
	for _, content := range []string{"", "null", "[]", `{"theme":"da`} {
		dir := t.TempDir()
		file := launch(t, dir)
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Set("theme", "dark"); err != nil {
			t.Errorf("%q: %v", content, err)
		}
		launch(t, dir)
		var theme string
		if !Get("theme", &theme) || theme != "dark" {
			t.Errorf("%q: theme is %q, want dark", content, theme)
		}
	}
}
