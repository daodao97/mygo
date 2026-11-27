//go:build !js

package mygo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// SaveAs writes the document where the dialog says, and the document
// becomes that file; an empty answer cancels and saves nothing.
func TestDocumentSaveAs(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	path := writeTemp(t, "a.txt", "one")
	d, _ := ds.Open(path)
	d.Set("two")

	// An explicit path writes there without asking, and leaves the old
	// file as it is.
	other := filepath.Join(t.TempDir(), "b.txt")
	if err := d.SaveAs(other); err != nil {
		t.Fatal(err)
	}
	if readString(t, other) != "two" || readString(t, path) != "one" {
		t.Fatalf("wrote %q, left %q", readString(t, other), readString(t, path))
	}
	if d.Path() != other || d.IsEdited() {
		t.Fatalf("the document is %s, edited %v", d.Path(), d.IsEdited())
	}
	fw := fakeWindow(d.Window())
	waitFor(t, "the represented file to follow", func() bool { p, _ := fw.Document(); return p == other })

	// The empty path asks. A cancel saves nothing and stays edited.
	asked := 0
	onMain(func() {
		fb.OnSaveDialog = func(*platform.SaveDialogOptions) string { asked++; return "" }
	})
	d.Set("three")
	if err := d.SaveAs(""); !errors.Is(err, ErrCanceled) {
		t.Fatalf("a canceled save-as: %v", err)
	}
	if asked != 1 || !d.IsEdited() || d.Path() != other {
		t.Fatalf("asked %d, edited %v, path %s", asked, d.IsEdited(), d.Path())
	}

	// An answer writes where it says.
	dir := t.TempDir()
	target := filepath.Join(dir, "c.txt")
	onMain(func() {
		fb.OnSaveDialog = func(*platform.SaveDialogOptions) string { return target }
	})
	if err := d.SaveAs(""); err != nil {
		t.Fatal(err)
	}
	if readString(t, target) != "three" || d.Path() != target || d.IsEdited() {
		t.Fatalf("wrote %q to %s, edited %v", readString(t, target), d.Path(), d.IsEdited())
	}
}

// SaveAnyway writes over a file another app changed without asking, and
// an untitled document saves where the dialog says.
func TestDocumentSaveAnyway(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	path := writeTemp(t, "a.txt", "one")
	d, _ := ds.Open(path)
	d.Set("mine")
	future := time.Now().Add(time.Hour)
	os.WriteFile(path, []byte("theirs"), 0o644)
	os.Chtimes(path, future, future)
	d.checkDisk()
	if !d.HasConflict() {
		t.Fatal("no conflict")
	}
	if err := d.SaveAnyway(); err != nil {
		t.Fatal(err)
	}
	if readString(t, path) != "mine" || d.HasConflict() || d.IsEdited() {
		t.Fatalf("file %q, conflict %v, edited %v", readString(t, path), d.HasConflict(), d.IsEdited())
	}

	// An untitled document saves where the dialog says.
	untitled := ds.New()
	untitled.Set("fresh")
	dir := t.TempDir()
	target := filepath.Join(dir, "new.txt")
	onMain(func() {
		fb.OnSaveDialog = func(*platform.SaveDialogOptions) string { return target }
	})
	if err := untitled.SaveAnyway(); err != nil {
		t.Fatal(err)
	}
	if readString(t, target) != "fresh" || untitled.Path() != target || untitled.IsEdited() {
		t.Fatalf("wrote %q to %s, edited %v", readString(t, target), untitled.Path(), untitled.IsEdited())
	}
}

// Revert reads the file again; an untitled document has nothing to
// revert to.
func TestDocumentRevert(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	if err := ds.New().Revert(); err == nil {
		t.Fatal("an untitled document reverted to nothing")
	}
	path := writeTemp(t, "a.txt", "one")
	d, _ := ds.Open(path)
	reloaded := make(chan struct{}, 1)
	d.OnReload(func() { reloaded <- struct{}{} })
	d.Set("two")
	if err := d.Revert(); err != nil {
		t.Fatal(err)
	}
	<-reloaded
	if d.Content() != "one" || d.IsEdited() {
		t.Fatalf("content %q edited %v", d.Content(), d.IsEdited())
	}
}

// Changed tells of content that changed without being replaced: the
// document is edited, and a save writes what it has.
func TestDocumentChanged(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	path := writeTemp(t, "a.txt", "one")
	d, _ := ds.Open(path)
	d.Changed()
	if !d.IsEdited() {
		t.Fatal("Changed did not edit the document")
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if !fileIs(path, "one") || d.IsEdited() {
		t.Fatalf("file %q, edited %v", readString(t, path), d.IsEdited())
	}
}

// OnStateChange follows the document until it is turned off.
func TestDocumentOnStateChange(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	path := writeTemp(t, "a.txt", "one")
	d, _ := ds.Open(path)
	states := make(chan DocumentState, 8)
	off := d.OnStateChange(func(s DocumentState) { states <- s })

	d.Set("x")
	s := <-states
	if !s.Edited || s.Name != "a.txt" {
		t.Fatalf("state %v", s)
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	s = <-states
	if s.Edited {
		t.Fatalf("state after the save: %v", s)
	}

	off()
	d.Set("y")
	onMainValue(func() bool { return true })
	if n := len(states); n != 0 {
		t.Fatalf("%d states after off", n)
	}
}

// OpenDialog opens the documents the dialog names, and none when it is
// canceled.
func TestDocumentOpenDialog(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	onMain(func() {
		fb.OnOpenDialog = func(*platform.OpenDialogOptions) []string { return nil }
	})
	docs, err := ds.OpenDialog()
	if err != nil || len(docs) != 0 {
		t.Fatalf("a canceled open: %v, %d documents", err, len(docs))
	}
	a := writeTemp(t, "a.txt", "one")
	b := writeTemp(t, "b.txt", "two")
	onMain(func() {
		fb.OnOpenDialog = func(*platform.OpenDialogOptions) []string { return []string{a, b} }
	})
	docs, err = ds.OpenDialog()
	if err != nil || len(docs) != 2 {
		t.Fatalf("opened %d documents: %v", len(docs), err)
	}
	if docs[0].Path() != a || docs[0].Content() != "one" || docs[1].Path() != b {
		t.Fatalf("opened %s %q and %s", docs[0].Path(), docs[0].Content(), docs[1].Path())
	}
}

// A file the app can't write opens read-only, and saving it asks to save
// as instead; a cancel leaves it as it is.
func TestDocumentReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the read-only attribute needs another cleanup")
	}
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	path := writeTemp(t, "a.txt", "one")
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	d, _ := ds.Open(path)
	if !d.IsReadOnly() {
		t.Fatal("the document is not read-only")
	}
	asked := answer("Cancel")
	d.Set("two")
	if err := d.Save(); !errors.Is(err, ErrCanceled) {
		t.Fatalf("saving a read-only document: %v", err)
	}
	if len(asked.all()) != 1 || !d.IsEdited() {
		t.Fatalf("asked %q, edited %v", asked.all(), d.IsEdited())
	}
}

// The service's methods act on the document of the window that calls:
// they save, save as, duplicate, rename, move and revert it, and fail
// for a window that shows none of its documents.
func TestDocumentServiceFlows(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	serviceRuns++
	note := fmt.Sprintf("Note%d", serviceRuns)
	if err := bind(note, ds.Service(), false); err != nil {
		t.Fatal(err)
	}
	path := writeTemp(t, "page.txt", "from Go")
	d, _ := ds.Open(path)
	fw := fakeWindow(d.Window())
	onMain(func() { fw.H.NavigationCommitted("about:blank") })
	page(fw, `{"t":"dom-ready"}`)

	// Save writes the file.
	call(t, fw, 1, note+".SetContent", "edited")
	if r := call(t, fw, 2, note+".Save"); r["ok"] == false {
		t.Fatalf("save: %v", r["e"])
	}
	if readString(t, path) != "edited" || d.IsEdited() {
		t.Fatalf("file %q, edited %v", readString(t, path), d.IsEdited())
	}

	// SaveAs asks, and a cancel fails the call and keeps the edits.
	asked := 0
	onMain(func() {
		fb.OnSaveDialog = func(*platform.SaveDialogOptions) string { asked++; return "" }
	})
	call(t, fw, 3, note+".SetContent", "more")
	if r := call(t, fw, 4, note+".SaveAs"); r["ok"] != false {
		t.Fatalf("a canceled save-as succeeded: %v", r)
	}
	if asked != 1 || !d.IsEdited() || d.Path() != path {
		t.Fatalf("asked %d, edited %v, path %s", asked, d.IsEdited(), d.Path())
	}

	// Duplicate opens a copy in a window of its own.
	var dup *Document[string]
	if r := call(t, fw, 5, note+".Duplicate"); r["ok"] == false {
		t.Fatalf("duplicate: %v", r["e"])
	}
	for _, e := range ds.Documents() {
		if e.Name() == "page copy" {
			dup = e
		}
	}
	if dup == nil || dup.Content() != "more" || dup.Path() != "" || !dup.IsEdited() {
		t.Fatalf("the copy is %v", dup)
	}
	dup.SetEdited(false)

	// Rename asks for a name and takes the file with it.
	onMain(func() {
		fb.OnSaveDialog = func(*platform.SaveDialogOptions) string {
			return filepath.Join(filepath.Dir(path), "renamed.txt")
		}
	})
	if r := call(t, fw, 6, note+".Rename"); r["ok"] == false {
		t.Fatalf("rename: %v", r["e"])
	}
	renamed := filepath.Join(filepath.Dir(path), "renamed.txt")
	if d.Path() != renamed || readString(t, renamed) != "edited" {
		t.Fatalf("renamed to %s, file %q", d.Path(), readString(t, renamed))
	}

	// MoveTo asks for a folder and moves the file there.
	dir := t.TempDir()
	onMain(func() {
		fb.OnOpenDialog = func(*platform.OpenDialogOptions) []string { return []string{dir} }
	})
	if r := call(t, fw, 7, note+".MoveTo"); r["ok"] == false {
		t.Fatalf("move: %v", r["e"])
	}
	if d.Path() != filepath.Join(dir, "renamed.txt") {
		t.Fatalf("moved to %s", d.Path())
	}

	// Revert asks, and a cancel keeps the edits; the answer reads the
	// file again.
	a := answer("Cancel")
	call(t, fw, 8, note+".SetContent", "changed")
	if r := call(t, fw, 9, note+".Revert"); r["ok"] != false {
		t.Fatalf("a canceled revert succeeded: %v", r)
	}
	if d.Content() != "changed" || len(a.all()) != 1 {
		t.Fatalf("content %q, asked %q", d.Content(), a.all())
	}
	a2 := answer("Revert")
	if r := call(t, fw, 10, note+".Revert"); r["ok"] == false {
		t.Fatalf("revert: %v", r["e"])
	}
	if d.Content() != "edited" || d.IsEdited() || len(a2.all()) != 1 {
		t.Fatalf("content %q, edited %v, asked %q", d.Content(), d.IsEdited(), a2.all())
	}

	// A window that shows none of the service's documents fails.
	_, other := readyWindow(t, WindowOptions{})
	if r := call(t, other, 11, note+".Content"); r["ok"] != false {
		t.Fatalf("a window without a document: %v", r)
	}
}

// The File menu's Open asks which documents to open and opens them; one
// it cannot open says so.
func TestDocumentMenuOpen(t *testing.T) {
	testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	a := writeTemp(t, "a.txt", "one")
	b := writeTemp(t, "b.txt", "two")
	onMain(func() {
		fb.OnOpenDialog = func(*platform.OpenDialogOptions) []string { return []string{a, b} }
	})
	before := len(allDocuments())
	documentRole(RoleOpen, nil)
	waitFor(t, "the dialog's documents to open", func() bool { return len(allDocuments()) == before+2 })

	asked := answer("OK")
	onMain(func() {
		fb.OnOpenDialog = func(*platform.OpenDialogOptions) []string {
			return []string{filepath.Join(filepath.Dir(a), "gone.txt")}
		}
	})
	documentRole(RoleOpen, nil)
	waitFor(t, "the failed open to say so", func() bool { return len(asked.all()) > 0 })
}

// The File menu's Revert asks, reads the file again, and says what went
// wrong when the file is gone.
func TestDocumentMenuRevert(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	path := writeTemp(t, "a.txt", "one")
	d, _ := ds.Open(path)
	d.Set("two")

	// A cancel keeps the edits.
	a0 := answer("Cancel")
	if err := d.revert(); !errors.Is(err, ErrCanceled) {
		t.Fatalf("a canceled revert: %v", err)
	}
	if d.Content() != "two" || len(a0.all()) != 1 {
		t.Fatalf("content %q, asked %q", d.Content(), a0.all())
	}

	a := answer("Revert")
	documentRole(RoleRevert, d.Window())
	waitFor(t, "the revert to read the file again", func() bool { return d.Content() == "one" && !d.IsEdited() })
	if len(a.all()) != 1 {
		t.Fatalf("asked %q", a.all())
	}

	// A file that is gone says so.
	d.Set("x")
	os.Remove(path)
	a2 := answer("Revert")
	documentRole(RoleRevert, d.Window())
	waitFor(t, "the failed revert to say so", func() bool { return len(a2.all()) == 2 })
}

// The File menu's commands follow the focused window's document: Save
// and its friends wait for one, and Revert also for edits to revert.
func TestDocumentMenuRefreshes(t *testing.T) {
	ds := testDocs(t, DocumentsOptions[string]{Autosave: AutosaveOff})
	m := NewMenu([]*MenuItem{{Role: RoleFileMenu}})
	file := m.Items()[0]
	var save, revert *MenuItem
	for _, it := range file.Submenu {
		switch it.Role {
		case RoleSave:
			save = it
		case RoleRevert:
			revert = it
		}
	}
	if save == nil || revert == nil {
		t.Fatal("the File menu lacks Save or Revert")
	}

	path := writeTemp(t, "a.txt", "one")
	d, _ := ds.Open(path)
	d.Window().SetMenu(m)
	d.Window().Focus()
	refresh := func() { onMainValue(func() bool { refreshDocumentMenus(); return false }) }

	refresh()
	if !save.IsEnabled() {
		t.Fatal("Save is off with a document open")
	}
	if revert.IsEnabled() {
		t.Fatal("Revert is on before an edit")
	}
	d.Set("x")
	refresh()
	if !revert.IsEnabled() {
		t.Fatal("Revert is off for an edited document")
	}
	d.SetEdited(false)
	refresh()
	if revert.IsEnabled() {
		t.Fatal("Revert is on for a document with nothing to revert")
	}
}
