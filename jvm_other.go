//go:build !android

package mygo

func javaVM() uintptr { return 0 }

func androidContext() uintptr { return 0 }
