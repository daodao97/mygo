//go:build !(darwin && !ios)

package glass

// The Metal libraries compiled ahead of time are for macOS alone.
var metalLibrary, blurMetalLibrary []byte

const metalSum, blurMetalSum = "", ""
