package mygo

import (
	"os"

	"github.com/egoist/mygo/internal/surface"
)

// production is set to "1" by `mygo build` through
// -ldflags "-X github.com/egoist/mygo.production=1".
var production string

// IsDev reports whether the app runs in development: true unless it was
// built for production with `mygo build`, or MYGO_ENV=production is set.
// Development builds enable the web inspector by default.
func IsDev() bool {
	if production == "1" {
		return false
	}
	return os.Getenv("MYGO_ENV") != "production"
}

// devLaunched tells that `mygo dev` launched the app, which it does with
// MYGO_DEV=1: App.Relaunch then has mygo dev start it again, and the app comes
// back as it was (KeepState).
var devLaunched bool

func init() { takeDevMarker() }

// takeDevMarker takes the variable, which isn't meant for child processes. It
// runs again as the app starts: Android's host sets the environment after this
// package's init.
func takeDevMarker() {
	// Unset only what's there: unsetting reaches the C environment, where
	// Android's host may be setting it right now, as the library loads.
	v, ok := os.LookupEnv("MYGO_DEV")
	if !ok {
		return
	}
	os.Unsetenv("MYGO_DEV")
	if v == "1" && production != "1" {
		devLaunched, surface.DevLaunched = true, true
	}
}

// devRelaunchFile is the file in which `mygo dev` looks for the arguments
// of the build that App.Relaunch has it start again, MYGO_DEV_RELAUNCH,
// which the core takes out of its environment as MYGO_DEV.
var devRelaunchFile = func() string {
	f := os.Getenv("MYGO_DEV_RELAUNCH")
	os.Unsetenv("MYGO_DEV_RELAUNCH")
	return f
}()

// devBackground tells that `mygo dev -background` launched the app, with
// MYGO_DEV_BACKGROUND=1: it starts without coming to the front, and its
// windows show without the focus until it takes it (backgroundShow), so
// that the rebuild of a save leaves the focus in the editor. Production
// builds ignore it.
var devBackground = func() bool {
	set := os.Getenv("MYGO_DEV_BACKGROUND") == "1"
	os.Unsetenv("MYGO_DEV_BACKGROUND")
	return set && production != "1"
}()

// launchedByDev reports whether `mygo dev` launched the app.
func launchedByDev() bool {
	takeDevMarker()
	return devLaunched
}
