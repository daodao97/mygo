//go:build !js

package mygo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCrashLog runs a copy of the test binary outside go test's build
// directory, as a packaged app runs, which panics: the panic and its stack
// are in crash.log, after what an earlier crash left there, and a log past
// maxCrashLog starts over.
func TestCrashLog(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(t.TempDir(), "crashapp")
	if runtime.GOOS == "windows" {
		app += ".exe"
	}
	if err := os.WriteFile(app, data, 0o755); err != nil {
		t.Fatal(err)
	}
	logs := t.TempDir()
	crash := func() string {
		t.Helper()
		cmd := exec.Command(app, "-test.run=^$")
		cmd.Env = append(os.Environ(), "MYGO_TEST_CRASH_LOG="+logs)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			t.Fatal("the app did not crash")
		}
		if !strings.Contains(stderr.String(), "panic: mygo test crash") {
			t.Fatalf("standard error lacks the panic:\n%s", stderr.String())
		}
		b, err := os.ReadFile(filepath.Join(logs, crashLogName))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	first := crash()
	if !strings.HasPrefix(first, "panic: mygo test crash") || !strings.Contains(first, "goroutine 1 [running") || !strings.Contains(first, "TestMain") {
		t.Fatalf("crash.log is not the panic with its stack:\n%s", first)
	}
	if second := crash(); !strings.HasPrefix(second, first) || strings.Count(second, "panic: mygo test crash") != 2 {
		t.Fatalf("crash.log does not keep the earlier crash:\n%s", second)
	}

	big := bytes.Repeat([]byte("x"), maxCrashLog+1)
	if err := os.WriteFile(filepath.Join(logs, crashLogName), big, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := crash(); !strings.HasPrefix(got, "panic: mygo test crash") {
		t.Fatalf("a crash log past maxCrashLog did not start over: %d bytes", len(got))
	}
}

// TestCrashLogSkipped: go test's builds (and go run's) write no crash log,
// as their standard error is shown.
func TestCrashLogSkipped(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !goBuildTemp(filepath.Dir(exe)) {
		t.Skipf("%s is not in a go-build directory", exe)
	}
	dir := t.TempDir()
	App.SetPath(PathLogs, dir)
	defer App.SetPath(PathLogs, "")
	logCrashes()
	if _, err := os.Stat(filepath.Join(dir, crashLogName)); !os.IsNotExist(err) {
		t.Fatalf("a test build wrote a crash log: %v", err)
	}
}
