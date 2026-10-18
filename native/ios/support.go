// Package ios contains opt-in native source helpers copied by the iOS packager.
// It is a build-time dependency only; importing mygo does not link these helpers.
package ios

import "embed"

// Sources contains the Swift implementations for ios.native.support.
//
//go:embed *.swift background/*.m background/*.h
var Sources embed.FS
