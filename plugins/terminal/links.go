package terminal

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// urlPattern finds the URLs programs print as text, which a Command+click
// opens as it does hyperlinks.
var urlPattern = regexp.MustCompile(`(?:https?|ftp|file)://[^\s"'<>` + "`" + `\x00-\x1f]+|mailto:[^\s"'<>` + "`" + `]+`)

// proseDelimiters end a path printed in Chinese or Japanese prose, which
// puts no space around it; a quoted path may hold them.
const proseDelimiters = "，。；：！？、（）【】《》「」『』"

// pathToken finds what may be a file path programs print as text: a quoted
// path, which may hold spaces, or a run of anything but spaces, quotes and
// proseDelimiters.
var pathToken = regexp.MustCompile(`"[^"\r\n]+"(?::\d+(?::\d+)?)?|'[^'\r\n]+'(?::\d+(?::\d+)?)?|[^\s"'<>` + "`" + proseDelimiters + `]+`)

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
	for _, m := range pathToken.FindAllStringIndex(s, -1) {
		p, a, b := pathOf(s[m[0]:m[1]])
		if p == "" {
			continue
		}
		start := utf8.RuneCountInString(s[:m[0]+a])
		end := start + utf8.RuneCountInString(s[m[0]+a:m[0]+b])
		if end <= len(cols) && cols[start] <= col && col <= cols[end-1] {
			return p, cols[start], cols[end-1] + 1
		}
	}
	return "", 0, 0
}

// pathOf returns the path a token of pathToken prints, and the bytes of the
// token it shows in, a up to b, or "".
func pathOf(tok string) (path string, a, b int) {
	a, b = 0, len(tok)
	quoted := tok[0] == '"' || tok[0] == '\''
	if !quoted {
		// Brackets and emphasis around a path are not part of it, nor is
		// the call a path is the argument of, as write(notes.md); a bracket
		// the path opens is, as icon_(copy).png.
		a = len(tok) - len(strings.TrimLeft(tok, "([{*"))
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
		u := trimURL(s[m[0]:m[1]])
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
