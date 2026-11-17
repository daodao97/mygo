//go:build (darwin || linux || windows) && !android

package native

import "testing"

func TestInModuleCache(t *testing.T) {
	for dir, want := range map[string]bool{
		"/home/u/go/pkg/mod/github.com/egoist/mygo@v0.9.0/plugins/sqlite/internal/sqlitecore":  true,
		`C:\Users\u\go\pkg\mod\github.com\egoist\mygo@v0.9.0-0.20261108-abcdef\plugins\sqlite`: true,
		"/home/u/src/mygo/plugins/sqlite/internal/sqlitecore":                                  false,
		"/home/u/src/me@work/app/modules/language":                                             false,
		"/home/u/go/pkg/mod/example.com/m@vx/language":                                         false,
	} {
		if got := inModuleCache(dir); got != want {
			t.Errorf("inModuleCache(%q) = %v, want %v", dir, got, want)
		}
	}
}
