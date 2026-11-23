package platform

import "runtime"

// MacKeys reports whether the keyboard follows the Mac's conventions, as an
// iPad's does: Command for shortcuts (Cmd+C is Command+C), Option and the
// arrows moving by words, Command and the arrows to the ends, and Control
// with the Emacs keys in text. It is what the platform is, not how it
// looks, as macOS alone has a menu bar.
const MacKeys = runtime.GOOS == "darwin" || runtime.GOOS == "ios"

// MacBrowser reports whether the app runs in a browser on a Mac, which the
// browsers' backend (internal/web) sets as it starts. Its keys follow
// another system's conventions, with Command sent as Control and Control
// as Super, so shortcuts are written with the Mac's symbols for the keys
// pressed: Control as ⌘ and Super as ⌃.
var MacBrowser bool
