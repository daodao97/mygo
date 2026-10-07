package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDemoImportCleanupPreservesOriginal(t *testing.T) {
	file := filepath.Join(t.TempDir(), "original.txt")
	if err := os.WriteFile(file, []byte(pickerSample), 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := copyDemoImports([]string{file})
	if err != nil || len(paths) != 1 {
		t.Fatalf("import: %v %v", paths, err)
	}
	t.Cleanup(func() {
		for _, path := range paths {
			_ = os.Remove(path)
		}
	})
	if paths[0] == file {
		t.Fatal("returned original instead of owned copy")
	}
	data, err := os.ReadFile(paths[0])
	if err != nil || string(data) != pickerSample {
		t.Fatalf("copy contents: %q %v", data, err)
	}
	if err := os.Remove(paths[0]); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(file)
	if err != nil || string(data) != pickerSample {
		t.Fatalf("original changed: %q %v", data, err)
	}
	paths, err = copyDemoImports([]string{file, file + "missing"})
	if err == nil || len(paths) != 0 {
		t.Fatalf("partial failure returned paths: %v %v", paths, err)
	}
}
