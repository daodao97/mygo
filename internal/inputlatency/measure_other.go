//go:build !(darwin && !ios)

package main

import "errors"

func measure(mode string, events, warmup, x, y, boxes int) error {
	return errors.New("measures macOS only")
}
