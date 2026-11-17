//go:build ios || android || js

package ui

// Phones and tablets have touch screens, and browsers run on them.
func init() { enableTouch() }
