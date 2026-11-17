package mygo

import (
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/internal/keepstate"
	"github.com/egoist/mygo/internal/platform"
)

// What OnSaveState gives goes in a file of the app's, which the next launch
// reads and removes unless the host says it was not ended by the system.

func restorationFile() (string, error) {
	dir, err := App.Path(PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "restoration.state"), nil
}

// inDevelopment reports whether mygo dev runs the app, which it restarts as the
// code changes: the app comes back as it was, as hot restart keeps it.
func inDevelopment() bool { return launchedByDev() }

// keepStateInDevelopment keeps what the app gives every second, as mygo dev ends
// the app without a word.
func (a *Application) keepStateInDevelopment() {
	if !inDevelopment() || a.keepingState {
		return
	}
	a.keepingState = true
	a.WhenReady(func() {
		go func() {
			for range time.Tick(time.Second) {
				if !postMain(a.writeState) {
					return
				}
			}
		}()
	})
}

// writeState keeps what the app gives, on phones and under mygo dev.
func (a *Application) writeState() {
	if a.saveState == nil {
		return
	}
	if _, ok := backend().(platform.DeviceBackend); !ok && !inDevelopment() {
		return
	}
	data := a.saveState()
	path, err := restorationFile()
	if err != nil {
		return
	}
	if len(data) == 0 {
		os.Remove(path)
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		os.Rename(tmp, path)
	}
}

// startedHidden reports whether the system started the app without showing it.
func startedHidden() bool {
	h, ok := backend().(interface{ StartedHidden() bool })
	return ok && h.StartedHidden()
}

// forgetKeptState removes what was kept, as the app comes back to the screen: an
// end while it's there is a crash or the user's, which what it kept in the
// background has no business surviving.
func forgetKeptState() {
	if inDevelopment() {
		return
	}
	if _, ok := backend().(platform.DeviceBackend); !ok {
		return
	}
	if path, err := restorationFile(); err == nil {
		os.Remove(path)
	}
}

// restoredState reads what was kept, once.
func restoredState() []byte {
	needsApp("App.RestoredState")
	path, err := restorationFile()
	if err != nil {
		return nil
	}
	if d, ok := backend().(interface{ WasEndedBySystem() bool }); ok && !d.WasEndedBySystem() && !inDevelopment() && !startedHidden() {
		os.Remove(path)
		return nil
	}
	if _, ok := backend().(platform.DeviceBackend); !ok && !inDevelopment() {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Kept for a restart right after this one, under mygo dev, which comes
	// before the next write, and for a launch out of sight, as for a background
	// task, after which the system may end the app again with nothing newer
	// written.
	if !inDevelopment() && !startedHidden() {
		os.Remove(path)
	}
	return data
}

// KeepState has the app come back with v as it was, across the restarts of mygo
// dev and as the system ends the app in the background: v, a pointer, is written
// down whole as it goes, unexported fields and all, and read back as the app is
// ready. Call it before App.WhenReady opens the windows, once:
//
//	a := &app{tab: 0}
//	mygo.KeepState(a)
//
// It keeps data. Functions, channels, interfaces, the standard library's structs
// but time.Time, and pointers to other packages' types stay as the program makes
// them, and so does a slice or a map of elements that hold any of those. What the
// change of the code gave another type stays as made. A pointer met a second time,
// as a list's link back, comes back nil: it keeps data, not a graph.
//
// It reads v on the main thread: change v there too, as views do, through
// Window.Update from other goroutines. A map changed on another goroutine as it
// reads would end the program, as Go does with any such map. For more control, use
// OnSaveState and RestoredState.
//
// A function, not a method of App, so that apps that don't use it don't have it.
func KeepState(v any) {
	App.OnSaveState(func() []byte {
		data, err := keepstate.Encode(v)
		if err != nil {
			return nil
		}
		return data
	})
	App.WhenReady(func() {
		if data := App.RestoredState(); data != nil {
			keepstate.Decode(data, v)
		}
	})
}
