package mygo

import (
	"go/build"
	"testing"
)

// Mobile targets inherit the darwin or linux build tag, but use their own
// backends. They still need the no-op menu icon hook when cgo is enabled.
func TestMenuIconsBuildSelection(t *testing.T) {
	for _, target := range []struct {
		os, arch string
		cgo      bool
	}{
		{"darwin", "arm64", false},
		{"darwin", "amd64", false},
		{"linux", "arm64", false},
		{"linux", "amd64", false},
		{"linux", "386", false},
		{"windows", "arm64", false},
		{"windows", "amd64", false},
		{"windows", "386", false},
		{"ios", "arm64", true},
		{"ios", "arm64", false},
		{"android", "arm64", true},
		{"android", "arm64", false},
		{"js", "wasm", false},
		{"freebsd", "amd64", false},
	} {
		name := target.os + "/" + target.arch
		if target.cgo {
			name += "/cgo"
		}
		t.Run(name, func(t *testing.T) {
			ctx := build.Default
			ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = target.os, target.arch, target.cgo
			var hooks []string
			for _, file := range []string{"backend_darwin.go", "backend_linux.go", "backend_windows.go", "menuicon_other.go"} {
				match, err := ctx.MatchFile(".", file)
				if err != nil {
					t.Fatal(err)
				}
				if match {
					hooks = append(hooks, file)
				}
			}
			if len(hooks) != 1 {
				t.Fatalf("want one menu icon hook, selected %v", hooks)
			}
		})
	}
}
