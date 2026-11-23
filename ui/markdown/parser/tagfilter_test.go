package parser

import "testing"

func TestTagFilter(t *testing.T) {
	if GFM&ExtTagFilter == 0 || Default&ExtTagFilter == 0 {
		t.Fatal("GFM and Default read ExtTagFilter")
	}
	for _, tc := range []struct {
		src   string
		plain string // CommonMark: raw HTML as written
		gfm   string // GFM, its tagfilter included
	}{
		// An HTML block, whose lines are scanned for tags.
		{"<title>x</title>\n", "<title>x</title>\n", "&lt;title>x&lt;/title>\n"},
		{"<plaintext>\nhello\n", "<plaintext>\nhello\n", "&lt;plaintext>\nhello\n"},
		{"<div>\n<script>alert(1)</script>\n</div>\n",
			"<div>\n<script>alert(1)</script>\n</div>\n",
			"<div>\n&lt;script>alert(1)&lt;/script>\n</div>\n"},
		// The tags of the spec's example, in a block quote.
		{"<blockquote>\n  <xmp> is disallowed.  <XMP> is also disallowed.\n</blockquote>\n",
			"<blockquote>\n  <xmp> is disallowed.  <XMP> is also disallowed.\n</blockquote>\n",
			"<blockquote>\n  &lt;xmp> is disallowed.  &lt;XMP> is also disallowed.\n</blockquote>\n"},
		{"<textarea>\nx\n</textarea>\n", "<textarea>\nx\n</textarea>\n", "&lt;textarea>\nx\n&lt;/textarea>\n"},
		{"<iframe src=\"x\"></iframe>\n", "<iframe src=\"x\"></iframe>\n", "&lt;iframe src=\"x\">&lt;/iframe>\n"},
		// Inline tags, escaped as cmark-gfm escapes them.
		{"a <style> b\n", "<p>a <style> b</p>\n", "<p>a &lt;style> b</p>\n"},
		{"<strong> <title> <style> <em>\n", "<p><strong> <title> <style> <em></p>\n", "<p><strong> &lt;title> &lt;style> <em></p>\n"},
		{"<noembed><noframes>\n", "<p><noembed><noframes></p>\n", "<p>&lt;noembed>&lt;noframes></p>\n"},
		// A name, then the end of a tag: a space, a '>' or a "/>".
		{"<title/>\n", "<title/>\n", "&lt;title/>\n"},
		{"<title />\n", "<title />\n", "&lt;title />\n"},
		{"<title>a</title >\n", "<title>a</title >\n", "&lt;title>a&lt;/title >\n"},
		// Tags it does not filter.
		{"<titlex>y</titlex>\n", "<p><titlex>y</titlex></p>\n", "<p><titlex>y</titlex></p>\n"},
		{"<div title=\"script\">x</div>\n", "<div title=\"script\">x</div>\n", "<div title=\"script\">x</div>\n"},
		{"<strong>a</strong>\n", "<p><strong>a</strong></p>\n", "<p><strong>a</strong></p>\n"},
	} {
		if got := render(tc.src, 0); got != tc.plain {
			t.Errorf("CommonMark %q:\ngot  %q\nwant %q", tc.src, got, tc.plain)
		}
		if got := render(tc.src, GFM); got != tc.gfm {
			t.Errorf("GFM %q:\ngot  %q\nwant %q", tc.src, got, tc.gfm)
		}
		if got := render(tc.src, Default); got != tc.gfm {
			t.Errorf("Default %q:\ngot  %q\nwant %q", tc.src, got, tc.gfm)
		}
		// Without the extension, GFM writes raw HTML as it is.
		if got := render(tc.src, GFM&^ExtTagFilter); got != tc.plain {
			t.Errorf("GFM without ExtTagFilter %q:\ngot  %q\nwant %q", tc.src, got, tc.plain)
		}
	}
}
