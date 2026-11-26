package parser

import "testing"

// A run of backticks no run of its length closes is text, and the code
// spans after it, of other lengths, are code: the second of them too,
// which an opener scanning for its closer must not hide from the next.
func TestCodeSpansAfterUnclosedRun(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"Type `` to open, then `a` and `b`.", "<p>Type `` to open, then <code>a</code> and <code>b</code>.</p>\n"},
		{"```x `a` `b` `c`", "<p>```x <code>a</code> <code>b</code> <code>c</code></p>\n"},
		{"``~`_`\\```1` `", "<p>``~<code>_</code>```1<code> </code></p>\n"},
	} {
		if got := render(c.src, 0); got != c.want {
			t.Errorf("%q:\ngot  %q\nwant %q", c.src, got, c.want)
		}
	}
}
