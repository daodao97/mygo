// Package ios holds the sources of the iOS host, Swift and UIKit, which
// mygo build compiles and links with the app's Go code. See
// internal/mobile for how the two work together.
package ios

import "embed"

// Files are the host's sources: MyGo.h, the C interface of the Go code,
// and Sources/MyGoHost, the Swift.
//
//go:embed MyGo.h Sources
var Files embed.FS
