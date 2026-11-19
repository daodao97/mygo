//go:build !js

package text

// hearFontLoads does nothing: the engines of desktops and phones have
// their fonts once registered.
func (s *System) hearFontLoads() {}
