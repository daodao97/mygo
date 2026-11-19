//go:build !js

package mygo

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

// crashLogName is the file in the app's logs directory (PathLogs) where the
// runtime writes the panic or fatal error that ends the app.
const crashLogName = "crash.log"

// maxCrashLog is the size past which the crash log starts over, as the app
// starts.
const maxCrashLog = 1 << 20

// logCrashes has the runtime write the panic or fatal error that ends the
// app, with its goroutines' stacks, to crash.log in the app's logs
// directory as well as to its standard error. A packaged app's standard
// error goes nowhere: macOS gives an app opened from the Finder /dev/null,
// Windows gives a GUI app none, and phones show the system's log, to which
// the runtime's report of a crash has no time to get through.
//
// Builds that mygo dev runs, and go run and go test, have their standard
// error shown, and write no crash log. An app that wants its crashes
// elsewhere calls debug.SetCrashOutput itself, once the app is ready.
func logCrashes() {
	if devLaunched {
		return
	}
	if exe, err := os.Executable(); err != nil || goBuildTemp(filepath.Dir(exe)) {
		return
	}
	dir, err := App.Path(PathLogs)
	if err != nil {
		return
	}
	path := filepath.Join(dir, crashLogName)
	// Before opening it to append, which on Windows can't truncate.
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxCrashLog {
		_ = os.Truncate(path, 0)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
}
