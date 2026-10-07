package mygo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestExportDialogValidationAndResult(t *testing.T) {
	file := filepath.Join(t.TempDir(), "export.txt")
	if err := os.WriteFile(file, []byte("export"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, files := range [][]string{nil, {filepath.Dir(file)}, {file + "missing"}} {
		if _, err := Dialog.Export(ExportDialogOptions{Files: files}); err == nil {
			t.Fatalf("accepted invalid files %v", files)
		}
	}
	t.Cleanup(func() {
		onMain(func() { fb.ExportResult = false; fb.ExportError = nil; fb.LastExport = platform.ExportDialogOptions{} })
	})
	onMain(func() { fb.ExportResult = true })
	check := func() {
		completed, err := Dialog.Export(ExportDialogOptions{Title: "Export", Files: []string{file}})
		if !completed || err != nil {
			t.Errorf("export: %v %v", completed, err)
		}
	}
	check()
	onMain(check)
	onMain(func() {
		if fb.LastExport.Title != "Export" || len(fb.LastExport.Files) != 1 || fb.LastExport.Files[0] != file {
			t.Errorf("native files: %+v", fb.LastExport)
		}
		fb.ExportResult = false
	})
	if completed, err := Dialog.Export(ExportDialogOptions{Files: []string{file}}); completed || err != nil {
		t.Fatalf("cancel: %v %v", completed, err)
	}
	onMain(func() { fb.ExportError = platform.ErrUnsupported })
	if _, err := Dialog.Export(ExportDialogOptions{Files: []string{file}}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestPhotoDialogSelectionCancellationAndError(t *testing.T) {
	t.Cleanup(func() {
		onMain(func() { fb.PhotoResult = nil; fb.PhotoError = nil; fb.LastPhoto = platform.PhotoDialogOptions{} })
	})
	onMain(func() { fb.PhotoResult = []string{"one.heic", "two.png"} })
	check := func() {
		paths, err := Dialog.Photos(PhotoDialogOptions{Multiple: true})
		if err != nil || len(paths) != 2 || paths[1] != "two.png" {
			t.Errorf("photos: %v %v", paths, err)
		}
	}
	check()
	onMain(check)
	onMain(func() {
		if !fb.LastPhoto.Multiple {
			t.Error("lost Multiple")
		}
		fb.PhotoResult = nil
	})
	if paths, err := Dialog.Photos(PhotoDialogOptions{}); paths != nil || err != nil {
		t.Fatalf("cancel: %v %v", paths, err)
	}
	onMain(func() { fb.PhotoError = platform.ErrUnsupported })
	if _, err := Dialog.Photos(PhotoDialogOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
