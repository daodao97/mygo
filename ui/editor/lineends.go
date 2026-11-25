package editor

import (
	"io"
	"strings"

	"github.com/egoist/mygo/internal/rope"
)

// The codes of line endings that lineEnds keeps, a byte for each.
const (
	endLF   byte = 'n' // \n
	endCRLF byte = 'c' // \r\n
	endCR   byte = 'r' // \r
)

// ending returns the bytes a line ending code stands for.
func ending(code byte) string {
	switch code {
	case endCRLF:
		return "\r\n"
	case endCR:
		return "\r"
	}
	return "\n"
}

// endCode returns the code of a line ending, \n for any but \r\n and \r.
func endCode(ending string) byte {
	switch ending {
	case "\r\n":
		return endCRLF
	case "\r":
		return endCR
	}
	return endLF
}

// lineEnds is how the lines of a text ended as they were read: a code
// for each \n of the text, in order, the text itself keeping only \n.
// It is a rope too, so that an edit changes it in the time it changes
// the text, and a version of it is a pointer that snapshots share.
type lineEnds struct {
	codes    rope.Text
	crlf, cr int // how many codes are endCRLF and endCR; the rest are endLF
}

// readEnds returns s with each line break, \r\n, a lone \r or \n, made
// \n, and the codes of the breaks as s has them; codes is "" when s has
// no \r, all its breaks being \n.
func readEnds(s string) (text, codes string) {
	if strings.IndexByte(s, '\r') < 0 {
		return s, ""
	}
	var t strings.Builder
	var c strings.Builder
	t.Grow(len(s))
	for {
		i := strings.IndexAny(s, "\r\n")
		if i < 0 {
			t.WriteString(s)
			break
		}
		t.WriteString(s[:i])
		t.WriteByte('\n')
		switch {
		case s[i] == '\n':
			c.WriteByte(endLF)
		case i+1 < len(s) && s[i+1] == '\n':
			c.WriteByte(endCRLF)
			i++
		default:
			c.WriteByte(endCR)
		}
		s = s[i+1:]
	}
	return t.String(), c.String()
}

// lf returns s with each line break \n, as the buffer keeps it.
func lf(s string) string {
	t, _ := readEnds(s)
	return t
}

// newLineEnds returns text as the buffer keeps it, and its line endings.
func newLineEnds(text string) (string, lineEnds) {
	t, codes := readEnds(text)
	if codes == "" {
		codes = strings.Repeat(string(endLF), strings.Count(t, "\n"))
	}
	var e lineEnds
	e.codes = rope.New(codes)
	e.count(codes, 1)
	return t, e
}

// count adds codes to the counts, or takes them off with sign -1.
func (e *lineEnds) count(codes string, sign int) {
	e.crlf += sign * strings.Count(codes, string(endCRLF))
	e.cr += sign * strings.Count(codes, string(endCR))
}

// at returns the code of line break i, \n past the codes.
func (e lineEnds) at(i int) byte {
	if i < 0 || i >= e.codes.Len() {
		return endLF
	}
	return e.codes.ByteAt(i)
}

// replace returns e with the codes of breaks a to z replaced by codes.
func (e lineEnds) replace(a, z int, codes string) lineEnds {
	e.count(e.codes.Slice(a, z), -1)
	e.count(codes, 1)
	e.codes = e.codes.Replace(a, z, codes)
	return e
}

// made returns the codes of the k line breaks an edit inserts in place of
// those of lines line to endLine of the text e is of, which ends line
// endLine; own are the codes of the breaks as the edit's text has them,
// "" for all \n.
//
// The new breaks take, in order, the endings of the breaks they replace,
// so that replacing lines, as Set or moving lines does, leaves the
// endings where they were. Those beyond take the ending of the line the
// edit splits (endLine, which Enter breaks in two), else of the line
// before it on the last line, which has none. The endings the edit's text
// has, as a paste's \r\n, are only kept in a text without line breaks,
// which has no ending to follow: a paste does not mix endings into a
// file.
func (e lineEnds) made(line, endLine, k int, own string) string {
	if k == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(k)
	m := min(endLine-line, k)
	if m > 0 {
		b.WriteString(e.codes.Slice(line, line+m))
	}
	n := e.codes.Len()
	for i := m; i < k; i++ {
		switch {
		case endLine < n:
			b.WriteByte(e.at(endLine))
		case endLine > 0:
			b.WriteByte(e.at(endLine - 1))
		case i < len(own):
			b.WriteByte(own[i])
		default:
			b.WriteByte(endLF)
		}
	}
	return b.String()
}

// fixCR returns ends with no line ending \r, of the breaks from to to of
// text, followed by an empty line ending in \n: written, the two make
// \r\n, which reads back as one line break, losing the empty line. An
// edit can leave them so, as deleting the text of a line after a line
// ending in \r. The empty line's break becomes \r\n instead, a single
// ending changed, which reads back as it is (\r\r\n is two breaks) and
// makes no new such pair, as giving it \r would with another empty line
// after it.
func fixCR(text rope.Text, ends lineEnds, from, to int) lineEnds {
	for j := max(from, 0); j <= to && j+1 < ends.codes.Len(); j++ {
		if ends.at(j) == endCR && ends.at(j+1) == endLF && text.LineStart(j+1) == text.LineEnd(j+1) {
			ends = ends.replace(j+1, j+2, string(endCRLF))
		}
	}
	return ends
}

// dominant returns the ending most lines have, the first line's on a tie
// with it, and \n for a text of one line.
func (e lineEnds) dominant() string {
	n := func(c byte) int {
		switch c {
		case endCRLF:
			return e.crlf
		case endCR:
			return e.cr
		}
		return e.codes.Len() - e.crlf - e.cr
	}
	best := e.at(0)
	for _, c := range []byte{endLF, endCRLF, endCR} {
		if n(c) > n(best) {
			best = c
		}
	}
	return ending(best)
}

// mixed reports whether the line breaks have more than one ending.
func (e lineEnds) mixed() bool {
	lf := e.codes.Len() - e.crlf - e.cr
	kinds := 0
	for _, n := range []int{lf, e.crlf, e.cr} {
		if n > 0 {
			kinds++
		}
	}
	return kinds > 1
}

// convert returns e with every line ending code.
func (e lineEnds) convert(code byte) lineEnds {
	n := e.codes.Len()
	codes := strings.Repeat(string(code), n)
	var out lineEnds
	out.codes = rope.New(codes)
	out.count(codes, 1)
	return out
}

// writeText writes t to w with each line's ending of e.
func writeText(t rope.Text, e lineEnds, w io.Writer) (int64, error) {
	lf := e.codes.Len() - e.crlf - e.cr
	var only string // the one ending all lines have
	switch {
	case e.crlf == 0 && e.cr == 0:
		return rope.WriteTo(t.Root(), w)
	case lf == 0 && e.cr == 0:
		only = "\r\n"
	case lf == 0 && e.crlf == 0:
		only = "\r"
	}
	var n int64
	var err error
	if only != "" {
		t.Chunks(0, t.Len(), func(s string) bool {
			var k int
			k, err = io.WriteString(w, strings.ReplaceAll(s, "\n", only))
			n += int64(k)
			return err == nil
		})
		return n, err
	}
	codes := e.codes.String()
	line := 0
	var buf []byte
	t.Chunks(0, t.Len(), func(s string) bool {
		buf = buf[:0]
		for {
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				buf = append(buf, s...)
				break
			}
			buf = append(buf, s[:i]...)
			c := endLF
			if line < len(codes) {
				c = codes[line]
			}
			buf = append(buf, ending(c)...)
			line++
			s = s[i+1:]
		}
		var k int
		k, err = w.Write(buf)
		n += int64(k)
		return err == nil
	})
	return n, err
}
