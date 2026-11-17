package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The generated files are what the spec makes: run go generate in
// internal/mobile/wire after changing it.
func TestUpToDate(t *testing.T) {
	for path, want := range map[string][]byte{goOut: goSource(), swiftOut: swiftSource(), kotlinOut: kotlinSource()} {
		got, err := os.ReadFile(filepath.Join("..", path))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is out of date: run go generate ./internal/mobile/wire", path)
		}
	}
}

func TestLower(t *testing.T) {
	for in, want := range map[string]string{"ID": "id", "URLs": "urls", "SafeTop": "safeTop", "HTMLText": "htmlText", "BaseURL": "baseURL", "PNG": "png"} {
		if got := lower(in); got != want {
			t.Errorf("lower(%q) = %q, want %q", in, got, want)
		}
	}
}
