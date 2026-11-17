//go:build (darwin || linux) && !android

package native

import "github.com/ebitengine/purego"

// openLibc returns the C library's malloc and free, with which Go and the
// implementations allocate the blocks they give each other, and the
// function that tells the main thread.
func openLibc() (malloc, free, isMain uintptr, err error) {
	h, err := purego.Dlopen(libcPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, 0, 0, err
	}
	for name, p := range map[string]*uintptr{"malloc": &malloc, "free": &free, isMainSymbol: &isMain} {
		if *p, err = purego.Dlsym(h, name); err != nil {
			return 0, 0, 0, err
		}
	}
	return malloc, free, isMain, nil
}
