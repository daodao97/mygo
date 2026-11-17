//go:build !ios && !android

package mobile

import (
	"errors"
	"os"
)

func dupOutput(_, _ *os.File) error { return errors.New("mygo: no host") }

func hostLog(bool, []byte) {}
