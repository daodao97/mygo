package terminal

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// urlPattern finds the URLs programs print as text, which a Command+click
// opens as it does hyperlinks. Fork: a URL in Chinese or Japanese prose ends
// at proseDelimiters too, as a path does, so 文档（https://x.org/a），继续
// opens https://x.org/a (an upstream PR candidate, see FORK.md).
var urlPattern = regexp.MustCompile(`(?:https?|ftp|file)://[^\s"'<>` + "`" + proseDelimiters + `\x00-\x1f]+|mailto:[^\s"'<>` + "`" + proseDelimiters + `]+`)

// proseDelimiters end a path printed in Chinese or Japanese prose, which
// puts no space around it; a quoted path may hold them.
const proseDelimiters = "，。；：！？、（）【】《》「」『』"

// pathToken finds what may be a file path programs print as text, unquoted:
// a run of anything but spaces, quotes and proseDelimiters.
var pathToken = regexp.MustCompile(`[^\s"'<>` + "`" + proseDelimiters + `]+`)

// pathInToken finds a path inside a token that is not one as a whole, as
// main.go:12 in grep's main.go:12:code, build/app.js in --out=build/app.js
// or /usr/bin in PATH=/usr/bin:/bin.
var pathInToken = regexp.MustCompile(`(?:(?:~|\.{1,2})?/[^\s:]+|[\p{L}\p{N}_.@+-]+(?:/[\p{L}\p{N}_.@+-]+)+/?)(?::\d+(?::\d+)?)?|[\p{L}\p{N}_@+-][\p{L}\p{N}_.@+-]*\.[\p{L}\p{N}_]+(?::\d+(?::\d+)?)?`)

// quotedPlace finds the place after a quoted path, as "main.py":12.
var quotedPlace = regexp.MustCompile(`^:\d+(?::\d+)?`)

// pathPlace finds the place in the file a path ends with: :line or
// :line:col (compilers, grep), (line,col) (TypeScript, MSBuild), or #Lline
// or #LlineCcol (GitHub).
var pathPlace = regexp.MustCompile(`:(\d+)(?::(\d+))?:?$|\((\d+)(?:,(\d+))?\)$|#L(\d+)(?:C(\d+))?$`)

// pathAt returns the file path of text at column col, as urlAt, or "":
// absolute, from ~, ./ or ../, with a slash, or a file name with an
// extension or a line; the place in the file is written path:line:col
// however it was printed, and a quoted path loses its quotes.
func pathAt(text []rune, cols []int, col int) (path string, x0, x1 int) {
	s := string(text)
	span := func(a, b int) (int, int, bool) {
		start := utf8.RuneCountInString(s[:a])
		end := start + utf8.RuneCountInString(s[a:b])
		if end > start && end <= len(cols) && cols[start] <= col && col <= cols[end-1] {
			return cols[start], cols[end-1] + 1, true
		}
		return 0, 0, false
	}
	// A quoted path first, as it may hold spaces and what the tokens below
	// end at.
	for i := 0; i < len(s); i++ {
		a, b := quotedAt(s, i)
		if b < 0 {
			continue
		}
		p, pa, pb := pathOf(s[a:b])
		if p == "" {
			continue
		}
		if x0, x1, ok := span(a+pa, a+pb); ok {
			return p, x0, x1
		}
		i = b - 1
	}
	for _, m := range pathToken.FindAllStringIndex(s, -1) {
		tok := s[m[0]:m[1]]
		if p, a, b := pathOf(tok); p != "" {
			if x0, x1, ok := span(m[0]+a, m[0]+b); ok {
				return p, x0, x1
			}
			continue
		}
		for _, n := range pathInToken.FindAllStringIndex(tok, -1) {
			if p, a, b := pathOf(tok[n[0]:n[1]]); p != "" {
				if x0, x1, ok := span(m[0]+n[0]+a, m[0]+n[0]+b); ok {
					return p, x0, x1
				}
			}
		}
	}
	return "", 0, 0
}

// quotedAt returns the bytes of s, a up to b, of a quoted run that opens at
// byte i and the place after it, or b < 0. A quote between letters or digits,
// as in don't, is an apostrophe and opens or closes nothing, and the inside
// of a quoted path neither starts nor ends with a space.
func quotedAt(s string, i int) (a, b int) {
	q := s[i]
	if q != '"' && q != '\'' || i > 0 && isAlnum(s[i-1]) {
		return 0, -1
	}
	j := strings.IndexByte(s[i+1:], q)
	if j < 0 {
		return 0, -1
	}
	j += i + 1
	inner := s[i+1 : j]
	if inner == "" || strings.ContainsAny(inner, "\r\n") || strings.TrimSpace(inner) != inner {
		return 0, -1
	}
	b = j + 1
	b += len(quotedPlace.FindString(s[b:]))
	if b < len(s) && isAlnum(s[b]) {
		return 0, -1
	}
	return i, b
}

// isAlnum reports whether c is an ASCII letter or digit.
func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// pathOf returns the path a token of pathToken, or a quoted run, prints,
// and the bytes of the token it shows in, a up to b, or "".
func pathOf(tok string) (path string, a, b int) {
	a, b = 0, len(tok)
	quoted := tok[0] == '"' || tok[0] == '\''
	if !quoted {
		// Brackets and emphasis around a path are not part of it, nor is
		// the call a path is the argument of, as write(notes.md); a bracket
		// the path opens is, as icon_(copy).png.
		a = len(tok) - len(strings.TrimLeft(tok, "([{*"))
		// The name of an option or a variable the path is the value of,
		// as --out=build/app.js or PATH=/usr/bin, is not part of it.
		if i := strings.IndexByte(tok[a:], '='); i >= 0 && !strings.Contains(tok[a:a+i], "/") {
			a += i + 1
		}
		b = a + len(balanced(tok[a:]))
		if strings.HasSuffix(tok[a:b], ")") && !pathPlace.MatchString(tok[a:b]) {
			if i := opening(tok[a:b]); i > 0 {
				a, b = a+i+1, b-1
			}
		}
		for b > a && strings.IndexByte(",;.!?]", tok[b-1]) >= 0 {
			b--
		}
		if !pathPlace.MatchString(tok[a:b]) {
			b = a + len(trimURL(tok[a:b]))
		}
		if b <= a {
			return "", 0, 0
		}
	}
	name, line, col := tok[a:b], 0, 0
	if loc := pathPlace.FindStringSubmatchIndex(name); loc != nil {
		place := name
		name = place[:loc[0]]
		for i := 2; i < len(loc); i += 4 {
			if loc[i] >= 0 {
				line, _ = strconv.Atoi(place[loc[i]:loc[i+1]])
				if loc[i+2] >= 0 {
					col, _ = strconv.Atoi(place[loc[i+2]:loc[i+3]])
				}
				break
			}
		}
		if strings.HasSuffix(place, ":") {
			b-- // the colon after main.go:12:3: ends a compiler's location
		}
	}
	if quoted {
		name = name[1 : len(name)-1]
	}
	if name == "" || strings.ContainsAny(name, ":\x00\r\n") ||
		strings.IndexFunc(name, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
		return "", 0, 0
	}
	// Words are not paths: a name without a slash takes a letter, and an
	// extension or a line, so that version 1.2.3 and 12:30 are neither.
	if !strings.Contains(name, "/") {
		dot := strings.LastIndexByte(name, '.')
		if strings.IndexFunc(name, unicode.IsLetter) < 0 || line == 0 && (dot < 0 || dot == len(name)-1) {
			return "", 0, 0
		}
	}
	path = name
	if line > 0 {
		path += ":" + strconv.Itoa(line)
		if col > 0 {
			path += ":" + strconv.Itoa(col)
		}
	}
	return path, a, b
}

// balanced returns s up to a closing bracket it did not open, as the ) of
// "(image.png)已保存".
func balanced(s string) string {
	var depth [3]int
	for i, r := range s {
		if k := strings.IndexRune("([{", r); k >= 0 {
			depth[k]++
		} else if k := strings.IndexRune(")]}", r); k >= 0 {
			if depth[k] == 0 {
				return s[:i]
			}
			depth[k]--
		}
	}
	return s
}

// opening returns the index of the ( that opens the ) s ends with, or -1.
func opening(s string) int {
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

// urlAt returns the URL of text, a row whose runes are in columns cols, at
// column col, and the columns it spans, x0 up to x1, or "".
func urlAt(text []rune, cols []int, col int) (url string, x0, x1 int) {
	return matchAt(urlPattern, text, cols, col)
}

func matchAt(re *regexp.Regexp, text []rune, cols []int, col int) (string, int, int) {
	s := string(text)
	for _, m := range re.FindAllStringIndex(s, -1) {
		// Fork: a bracket the URL did not open ends it even before prose, as
		// the ) of (https://x.org/a)已保存 (with urlPattern's proseDelimiters).
		u := trimURL(balanced(s[m[0]:m[1]]))
		if u == "" {
			continue
		}
		start := len([]rune(s[:m[0]]))
		end := start + len([]rune(u))
		if start < len(cols) && cols[start] <= col && col <= cols[end-1] {
			return u, cols[start], cols[end-1] + 1
		}
	}
	return "", 0, 0
}

// trimURL trims what likely ends the sentence around a URL rather than the
// URL: punctuation, and closing brackets that the URL did not open.
func trimURL(u string) string {
	for len(u) > 0 {
		switch last := u[len(u)-1]; last {
		case '.', ',', ';', ':', '!', '?':
			u = u[:len(u)-1]
			continue
		case ')', ']', '}':
			open := map[byte]string{')': "(", ']': "[", '}': "{"}[last]
			if strings.Count(u, open) < strings.Count(u, string(last)) {
				u = u[:len(u)-1]
				continue
			}
		}
		return u
	}
	return u
}

// The fork's view (pointer hit testing, tap-to-open, HoveredLink and the
// hard-wrapped continuations below) works on linkMatch. Recognition itself is
// upstream's urlAt and pathAt; matchLink only adapts their result.

// linkTarget is a printed URL or a file reference. Line and Column are 1-based;
// zero means the output did not specify a location.
type linkTarget struct {
	URL, Path    string
	Line, Column int
}

type linkMatch struct {
	linkTarget
	raw               string // what OpenLink receives: a URL, or path:line:col
	start, end        int    // rune indices, excluding surrounding punctuation
	cellA, cellB, row int
}

// pathLine finds the place upstream's pathAt appends to a path; pathOf never
// keeps a colon in the name, so a trailing :line(:col) is always the place.
var pathLine = regexp.MustCompile(`:(\d+)(?::(\d+))?$`)

// matchLink returns the URL, else the file path, of text at column col.
func matchLink(text []rune, cols []int, col int) linkMatch {
	if u, x0, x1 := urlAt(text, cols, col); u != "" {
		return spanMatch(linkMatch{linkTarget: linkTarget{URL: u}, raw: u}, cols, x0, x1)
	}
	p, x0, x1 := pathAt(text, cols, col)
	if p == "" {
		return linkMatch{}
	}
	m := linkMatch{linkTarget: linkTarget{Path: p}, raw: p}
	if loc := pathLine.FindStringSubmatchIndex(p); loc != nil {
		m.Path = p[:loc[0]]
		m.Line, _ = strconv.Atoi(p[loc[2]:loc[3]])
		if loc[4] >= 0 {
			m.Column, _ = strconv.Atoi(p[loc[4]:loc[5]])
		}
	}
	return spanMatch(m, cols, x0, x1)
}

// spanMatch sets m's rune indices from the columns x0 up to x1 it spans.
func spanMatch(m linkMatch, cols []int, x0, x1 int) linkMatch {
	m.start, m.end = -1, 0
	for i, c := range cols {
		if c >= x0 && m.start < 0 {
			m.start = i
		}
		if c < x1 {
			m.end = i + 1
		}
	}
	if m.start < 0 || m.end <= m.start {
		return linkMatch{}
	}
	return m
}

func (m linkMatch) valid() bool { return m.URL != "" || m.Path != "" }

// linkAt reads just the pointed logical line, never the full scrollback.
// The emulator lock is held by its caller.
func (v *view) linkAt(x, y float32) linkMatch {
	if v.t.term == nil || v.cellW == 0 || v.overScrollbar(x, y) || x*v.scale < float32(v.ox) || y*v.scale < float32(v.oy) ||
		x*v.scale >= float32(v.ox+v.cols*v.cellW) || y*v.scale >= float32(v.oy+v.rows*v.cellH) {
		return linkMatch{}
	}
	col, row := v.cellAt(x, y)
	if ref, ok := v.t.term.CellAt(col, row); ok {
		if url := v.t.term.Hyperlink(ref); url != "" {
			a, b := col, col+1
			for a > 0 {
				ref, ok := v.t.term.CellAt(a-1, row)
				if !ok || v.t.term.Hyperlink(ref) != url {
					break
				}
				a--
			}
			for b < v.cols {
				ref, ok := v.t.term.CellAt(b, row)
				if !ok || v.t.term.Hyperlink(ref) != url {
					break
				}
				b++
			}
			return linkMatch{linkTarget: linkTarget{URL: url}, raw: url, cellA: a, cellB: b, row: row}
		}
	}
	if m := v.hardWrappedFileAt(col, row); m.valid() && v.t.opts.OpenLink != nil {
		return m
	}
	text, cols, start := v.t.term.LogicalRowText(row)
	m := matchLink(text, cols, col+(row-start)*v.cols)
	if m.Path != "" && v.t.opts.OpenLink == nil {
		return linkMatch{}
	}
	if m.valid() {
		m.setCells(cols, start)
	}
	return m
}

func (m *linkMatch) setCells(cols []int, start int) {
	m.cellA, m.cellB, m.row = cols[m.start], cols[m.end-1]+1, start
	if m.end < len(cols) {
		m.cellB = max(m.cellB, cols[m.end])
	}
}

// An indented continuation has gaps between its display rows. Only underline
// and hit-test the actual path cells in the pointed row, never table padding.
func (m *linkMatch) setCellsInRow(cols []int, start, row, width int) {
	offset := (row - start) * width
	m.cellA, m.cellB, m.row = width, 0, row
	for i := m.start; i < m.end; i++ {
		x := cols[i] - offset
		if x < 0 || x >= width {
			continue
		}
		right := x + 1
		if i+1 < len(cols) && cols[i+1]-offset < width {
			right = max(right, cols[i+1]-offset)
		}
		m.cellA, m.cellB = min(m.cellA, x), max(m.cellB, right)
	}
}

// Some TUIs draw each display row themselves instead of letting the emulator
// wrap it. Recover file references at those edges, without changing the VT's
// actual line boundaries (selection, search and ordinary URLs still use them).
func (v *view) hardWrappedFileAt(col, row int) linkMatch {
	first, last := max(0, row-8), min(v.rows-1, row+7)
	texts, columns := make([][]rune, last-first+1), make([][]int, last-first+1)
	for y := first; y <= last; y++ {
		r, c := v.t.term.RowText(y)
		end := len(r)
		for end > 0 && unicode.IsSpace(r[end-1]) {
			end--
		}
		texts[y-first], columns[y-first] = r[:end], c[:end]
	}
	for start := first; start <= row; start++ {
		text := append([]rune(nil), texts[start-first]...)
		cols := append([]int(nil), columns[start-first]...)
		for end := start; end < last; end++ {
			r, c := texts[end-first], columns[end-first]
			if len(r) == 0 || v.t.term.RowsJoined(end) ||
				v.cols-c[len(c)-1]-1 > 8 || !unfinishedFileAtEdge(text, cols) {
				break
			}
			next, nc := texts[end+1-first], columns[end+1-first]
			a := 0
			for a < len(next) && unicode.IsSpace(next[a]) {
				a++
			}
			b := a
			for b < len(next) && !unicode.IsSpace(next[b]) && !strings.ContainsRune(proseDelimiters, next[b]) {
				b++
			}
			edge := matchLink(text, cols, cols[len(cols)-1])
			// A table continuation starts at its cell's left edge, which can
			// be far from column zero but must not pass the original path.
			indent := max(4, cols[edge.start]%v.cols)
			if a == b || nc[a] > indent || !fileContinuation(string(next[a:b])) {
				break
			}
			text = append(text, next[a:b]...)
			for _, x := range nc[a:b] {
				cols = append(cols, x+(end+1-start)*v.cols)
			}
			if end+1 >= row {
				m := matchLink(text, cols, col+(row-start)*v.cols)
				if m.valid() && (m.Path != "" || strings.HasPrefix(m.URL, "file://")) {
					m.setCellsInRow(cols, start, row, v.cols)
					if col < m.cellA || col >= m.cellB {
						break
					}
					// Keep reading if another row may finish the filename.
					if b < len(next) || end+1 == last || !unfinishedFileAtEdge(text, cols) ||
						v.cols-nc[b-1]-1 > 8 {
						return m
					}
				}
			}
			if b < len(next) {
				break
			}
		}
	}
	return linkMatch{}
}

func unfinishedFileAtEdge(text []rune, cols []int) bool {
	if len(cols) == 0 {
		return false
	}
	m := matchLink(text, cols, cols[len(cols)-1])
	if !m.valid() || m.end != len(text) || m.Line != 0 {
		return false
	}
	path := m.Path
	if m.URL != "" {
		if !strings.HasPrefix(m.URL, "file://") {
			return false
		}
		path = strings.TrimPrefix(m.URL, "file://")
	}
	if !strings.Contains(path, "/") {
		return false
	}
	base := path[strings.LastIndex(path, "/")+1:]
	// A completed filename/location is a separate reference, even near an edge.
	return !strings.Contains(base, ".") || strings.HasSuffix(base, "-")
}

func fileContinuation(s string) bool {
	s = trimURL(balanced(s))
	if loc := pathPlace.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	}
	if s == "" {
		return false
	}
	if strings.Contains(s, "://") || strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "~/") {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-.%/:#", r) {
			return false
		}
	}
	return true
}
