package platform

import "runtime"

// MacKeys reports whether the keyboard follows the Mac's conventions, as an
// iPad's does: Command for shortcuts (Cmd+C is Command+C), Option and the
// arrows moving by words, Command and the arrows to the ends, and Control
// with the Emacs keys in text. It is what the platform is, not how it
// looks, as macOS alone has a menu bar.
const MacKeys = runtime.GOOS == "darwin" || runtime.GOOS == "ios"
