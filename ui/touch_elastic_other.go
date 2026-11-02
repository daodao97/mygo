//go:build !ios && !mygo_touch_elastic

package ui

import "time"

// UIKit's rubber band is isolated from desktop and web builds. The build
// tag mygo_touch_elastic exercises the same implementation headlessly.
type elasticScroll struct{}

func (rt *engine) holdElastic()                                  {}
func (rt *engine) releaseElastic()                               {}
func (rt *engine) advanceElastic(time.Time)                      {}
func (rt *engine) elasticOffset(*state) (float32, float32)       { return 0, 0 }
func elasticAccepts(*state, float32, float32) bool               { return false }
func (rt *engine) elasticBy(*state, float32, float32, bool) bool { return false }
