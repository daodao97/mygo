//go:build darwin

package metal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CompileLibrary compiles Metal Shading Language into a Metal library with
// Xcode's metal tools, for code compiled ahead of time: go generate runs
// it, for the renderer's shader (shaderlib.go) and effects' (EffectSource).
func CompileLibrary(src string) ([]byte, error) {
	return CompileLibraryFor(src, "macosx", "-mmacosx-version-min=12.0")
}

// CompileLibraryFor compiles a Metal library for an SDK of Xcode's
// (macosx, iphoneos, iphonesimulator), whose oldest version the flag gives,
// as "-mios-version-min=17.0": a library loads only on its own platform.
func CompileLibraryFor(src, sdk, minVersion string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "metal")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in, air, lib := filepath.Join(dir, "shader.metal"), filepath.Join(dir, "shader.air"), filepath.Join(dir, "shader.metallib")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		return nil, err
	}
	for _, args := range [][]string{
		{"-sdk", sdk, "metal", "-c", in, "-o", air, minVersion},
		{"-sdk", sdk, "metallib", air, "-o", lib},
	} {
		if out, err := exec.Command("xcrun", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("metal: %s: %v\n%s", args[2], err, strings.TrimSpace(string(out)))
		}
	}
	return os.ReadFile(lib)
}
