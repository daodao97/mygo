package mygo

import "github.com/egoist/mygo/internal/mobile"

func javaVM() uintptr { return mobile.JavaVM() }

func androidContext() uintptr { return mobile.AppContext() }
