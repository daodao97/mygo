package tsgen

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

type (
	enumWord  string
	enumLevel int
)

// Constants may have their type from a conversion instead of a declaration:
// the union has their values too, or there is none.
func TestEnumConversions(t *testing.T) {
	word, level := reflect.TypeFor[enumWord](), reflect.TypeFor[enumLevel]()
	for _, c := range []struct {
		name, consts string
		typ          reflect.Type
		want         []string
	}{
		{"beside a declared one", `All enumWord = "all"; Done = enumWord("done")`, word, []string{`"all"`, `"done"`}},
		{"declared and converted", `All enumWord = enumWord("all")`, word, []string{`"all"`}},
		{"several names", `A, B, C = enumWord("a"), "b", enumWord("c")`, word, []string{`"a"`, `"c"`}},
		{"of another type", `All enumWord = "all"; Other = string("other"); Plain = "plain"`, word, []string{`"all"`}},
		{"a string not evaluated", `All enumWord = "all"; Done = enumWord("do" + "ne")`, word, nil},
		{"a string joined to", `All enumWord = "all"; Done = enumWord("do") + "ne"`, word, nil},
		{"repeated with iota", `A = enumLevel(iota); B; C`, level, []string{"0", "1", "2"}},
		{"shifted", `A = enumLevel(1) << iota; B; C`, level, []string{"1", "2", "4"}},
		{"in parentheses", `A = -(enumLevel)(2)`, level, []string{"-2"}},
		{"a shift's count", `A enumLevel = 1; B = 1 << enumLevel(3)`, level, []string{"1"}},
		{"a comparison", `A enumLevel = 1; B = enumLevel(1) == 1`, level, []string{"1"}},
		{"a number not evaluated", `A enumLevel = 1; B = enumLevel(len("ab"))`, level, nil},
	} {
		dir := t.TempDir()
		src := "package p\n\nconst (" + c.consts + ")\n"
		if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		s := newSources()
		s.pkgDir[c.typ.PkgPath()] = dir
		if got := s.enumValues(c.typ); !slices.Equal(got, c.want) {
			t.Errorf("%s: the values of %s are %q, want %q", c.name, c.consts, got, c.want)
		}
	}
}
