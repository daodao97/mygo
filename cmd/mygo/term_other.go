//go:build !(darwin && !ios) && !dragonfly && !freebsd && !(linux && !android) && !netbsd && !openbsd && !windows

package main

import "os"

func termWidth(f *os.File) int { return 0 }

func enableVT(f *os.File) bool { return true }
