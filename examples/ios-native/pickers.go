package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const pickerSample = "MyGo document round trip\nChinese: 你好\nEmoji: 👋\n"

func (s *demo) pickerControls(c *ui.Context) {
	ui.Text(c, "Files & Photos").FontSize(24).Bold()
	ui.Text(c, "Import creates local cache copies. Export sends an existing file to Files. Photos uses the system picker.").FontSize(15).LineHeight(1.4)
	for _, multiple := range []bool{false, true} {
		label := "Import text file"
		if multiple {
			label = "Import multiple files"
		}
		if ui.Button(c, label).Height(44).FillWidth().Clicked() {
			s.pickFiles(multiple)
		}
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "Choose photo").Height(44).Clicked() {
			s.pickPhotos(false)
		}
		if ui.Button(c, "Choose photos").Height(44).Clicked() {
			s.pickPhotos(true)
		}
	})
	if ui.Button(c, "Export sample document").Height(44).FillWidth().Clicked() {
		s.pickerResult = "Exporting sample document…"
		go func() {
			dir, err := mygo.App.Path(mygo.PathCache)
			path := filepath.Join(dir, "mygo-picker-test.txt")
			if err == nil && runtime.GOOS == "ios" {
				// Files hides empty application Documents directories. Keep a
				// second owned fixture there for multiple-selection tests.
				var documents string
				documents, err = mygo.App.Path(mygo.PathDocuments)
				if err == nil {
					err = os.MkdirAll(documents, 0700)
				}
				if err == nil {
					err = os.WriteFile(filepath.Join(documents, "mygo-picker-fixture.txt"), []byte(pickerSample), 0600)
				}
			}
			if err == nil {
				err = os.WriteFile(path, []byte(pickerSample), 0600)
			}
			var completed bool
			if err == nil {
				completed, err = mygo.Dialog.Export(mygo.ExportDialogOptions{Parent: s.window, Files: []string{path}})
			}
			result := "Export cancelled"
			if completed {
				result = "Export completed: mygo-picker-test.txt"
			}
			if err != nil {
				result = err.Error()
			}
			s.window.Update(func() { s.pickerResult = result })
		}()
	}
	if ui.Button(c, "Delete imported copies").Height(44).FillWidth().Clicked() {
		paths := append([]string(nil), s.pickerPaths...)
		s.pickerPaths = nil
		go func() {
			var err error
			for _, path := range paths {
				if e := os.Remove(path); e != nil && !os.IsNotExist(e) {
					err = e
				}
			}
			result := fmt.Sprintf("Deleted %d imported copies", len(paths))
			if err != nil {
				result = err.Error()
			}
			s.window.Update(func() { s.pickerResult = result })
		}()
	}
	if s.pickerResult != "" {
		ui.Text(c, s.pickerResult).FontSize(14).LineHeight(1.4)
	}
}

func (s *demo) pickFiles(multiple bool) {
	s.pickerResult = "Choosing files…"
	go func() {
		opts := mygo.OpenDialogOptions{Parent: s.window, Multiple: multiple}
		if !multiple {
			opts.Filters = []mygo.FileFilter{{Name: "Text", Extensions: []string{"txt"}}}
		}
		paths, err := mygo.Dialog.Open(opts)
		// Desktop Open returns original user files. Own a copy before offering
		// cleanup, so this functional demo can never delete a selected original.
		if err == nil && runtime.GOOS != "ios" {
			paths, err = copyDemoImports(paths)
		}
		s.importResult("Files", paths, err)
	}()
}

func copyDemoImports(paths []string) ([]string, error) {
	var copies []string
	for _, path := range paths {
		source, err := os.Open(path)
		if err != nil {
			for _, p := range copies {
				_ = os.Remove(p)
			}
			return nil, err
		}
		file, err := os.CreateTemp("", "mygo-import-*-"+filepath.Base(path))
		if err == nil {
			_, err = io.Copy(file, source)
			if e := file.Close(); err == nil {
				err = e
			}
		}
		_ = source.Close()
		if err != nil {
			if file != nil {
				_ = os.Remove(file.Name())
			}
			for _, p := range copies {
				_ = os.Remove(p)
			}
			return nil, err
		}
		copies = append(copies, file.Name())
	}
	return copies, nil
}
func (s *demo) pickPhotos(multiple bool) {
	s.pickerResult = "Choosing photos…"
	go func() {
		paths, err := mygo.Dialog.Photos(mygo.PhotoDialogOptions{Parent: s.window, Multiple: multiple})
		s.importResult("Photos", paths, err)
	}()
}
func (s *demo) importResult(kind string, paths []string, err error) {
	result := kind + " cancelled"
	if err != nil {
		result = err.Error()
	} else if len(paths) > 0 {
		result = fmt.Sprintf("%s imported: %d", kind, len(paths))
		for _, path := range paths {
			info, e := os.Stat(path)
			if e != nil {
				result += "\n" + e.Error()
				continue
			}
			result += fmt.Sprintf("\n%s (%d bytes)", filepath.Base(path), info.Size())
			if kind == "Photos" {
				file, e := os.Open(path)
				if e == nil {
					hash := sha256.New()
					_, e = io.Copy(hash, file)
					_ = file.Close()
					if e == nil {
						result += fmt.Sprintf("\nSHA256: %x", hash.Sum(nil)[:8])
					}
				}
				if e != nil {
					result += "\n" + e.Error()
				}
			}
			if strings.EqualFold(filepath.Ext(path), ".txt") && info.Size() <= 4096 {
				data, e := os.ReadFile(path)
				if e == nil {
					result += "\n" + string(data)
				}
			}
		}
	}
	s.window.Update(func() { s.pickerResult = result; s.pickerPaths = append(s.pickerPaths, paths...) })
}
