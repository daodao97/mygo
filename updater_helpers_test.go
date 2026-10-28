//go:build !js

package mygo

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStagedBundleRetainsResourcesForBackgroundHelpers(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only macOS installs bundles")
	}
	Updater.SetKeepPreviousBundles(true)
	t.Cleanup(func() { Updater.SetKeepPreviousBundles(false) })
	dir := t.TempDir()
	target := filepath.Join(dir, "Fixture.app")
	work := filepath.Join(dir, "work")
	archive := filepath.Join(work, "unpacked")
	for path, content := range map[string]string{filepath.Join(target, "Contents", "resource"): "old helper resource", filepath.Join(archive, "Fixture.app", "Contents", "resource"): "new resource"} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	quit := runningApp(t)
	old := filepath.Join(dir, fmt.Sprintf(".Fixture.app.old-%d", appPid), "Contents", "resource")
	if err := stageBundle(work, archive, target); err != nil {
		t.Fatal(err)
	}
	quit()
	data, err := os.ReadFile(old)
	if err != nil || string(data) != "old helper resource" {
		t.Fatal("background helper bundle removed", string(data), err)
	}
	data, err = os.ReadFile(filepath.Join(target, "Contents", "resource"))
	if err != nil || string(data) != "new resource" {
		t.Fatal("new bundle not activated", string(data), err)
	}
}
