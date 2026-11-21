package editor

import (
	"strings"
)

// Lexers for languages the general lexer does not fit: JSON's keys,
// TOML's tables, HCL's heredocs, CSS, markup with the scripts and styles
// inside it and Svelte's expressions, ERB's Ruby, Markdown with the code
// of its fenced blocks, and diffs.

// relabel gives the tokens of line covering start to end kind k,
// splitting those that reach past them.
func relabel(toks []Token, start, end int, k TokenKind) []Token {
	var out []Token
	from := 0
	for _, t := range toks {
		a, b := from, t.End
		from = t.End
		if b <= start || a >= end {
			out = appendTok(out, b, t.Kind)
			continue
		}
		if a < start {
			out = appendTok(out, start, t.Kind)
		}
		out = appendTok(out, min(b, end), k)
		if b > end {
			out = appendTok(out, b, t.Kind)
		}
	}
	return out
}

func appendTok(toks []Token, end int, k TokenKind) []Token {
	if n := len(toks); n > 0 {
		if toks[n-1].End >= end {
			return toks
		}
		if toks[n-1].Kind == k {
			toks[n-1].End = end
			return toks
		}
	}
	return append(toks, Token{End: end, Kind: k})
}

// jsonLexer makes the strings before colons keys.
type jsonLexer struct{ lexer }

// Tokenize relabels each key's tokens where they are, in one pass over
// the line: relabeling a key at a time would copy every token after it,
// and a minified file's one line, of thousands of keys, would take
// minutes.
func (j *jsonLexer) Tokenize(line string, state uint64, tokens []Token) ([]Token, uint64) {
	n0 := len(tokens)
	tokens, next := j.lexer.Tokenize(line, state, tokens)
	toks := tokens[n0:]
	out := toks[:0] // written behind where it is read
	for i := 0; i < len(toks); i++ {
		if toks[i].Kind != TokenString {
			out = appendTok(out, toks[i].End, toks[i].Kind)
			continue
		}
		// A string, with its escapes.
		k := i
		for k+1 < len(toks) && (toks[k+1].Kind == TokenString || toks[k+1].Kind == TokenEscape) {
			k++
		}
		end := toks[k].End
		p := end
		for p < len(line) && (line[p] == ' ' || line[p] == '\t') {
			p++
		}
		if p < len(line) && line[p] == ':' {
			out = appendTok(out, end, TokenProperty)
		} else {
			for ; i <= k; i++ {
				out = appendTok(out, toks[i].End, toks[i].Kind)
			}
		}
		i = k
	}
	return tokens[:n0+len(out)], next
}

// tomlLexer makes tables types and keys properties.
type tomlLexer struct{ lexer }

func (t *tomlLexer) Tokenize(line string, state uint64, tokens []Token) ([]Token, uint64) {
	n0 := len(tokens)
	tokens, next := t.lexer.Tokenize(line, state, tokens)
	if state != 0 {
		return tokens, next
	}
	toks := tokens[n0:]
	trimmed := strings.TrimLeft(line, " \t")
	start := len(line) - len(trimmed)
	switch {
	case strings.HasPrefix(trimmed, "["):
		end := strings.LastIndexByte(line, ']')
		if end > start {
			toks = relabel(toks, start, end+1, TokenType)
		}
	default:
		if eq := strings.IndexByte(line, '='); eq > start && !strings.ContainsAny(line[start:eq], "\"'#") {
			key := strings.TrimRight(line[start:eq], " \t")
			toks = relabel(toks, start, start+len(key), TokenProperty)
		}
	}
	return append(tokens[:n0], toks...), next
}

// hclLexer adds HCL's heredocs, <<EOT or <<-EOT ending a line, to the
// general lexer. While in one, its state is the mode hclHeredoc (bits
// 0-7), which the general lexer never uses, and a hash of the word that
// ends it (bits 8-39).
type hclLexer struct{ lexer }

const hclHeredoc = 0xfe

func (h *hclLexer) Tokenize(line string, state uint64, tokens []Token) ([]Token, uint64) {
	if state&0xff == hclHeredoc {
		out := lexOut{toks: tokens}
		out.add(len(line), TokenString)
		if hclHash(strings.TrimSpace(line)) == state>>8 {
			return out.toks, 0
		}
		return out.toks, state
	}
	n0 := len(tokens)
	tokens, next := h.lexer.Tokenize(line, state, tokens)
	// A block word before a dot, as in data.x.id or module.m.out, names
	// a value the way var.x and local.x do.
	for i := n0; i < len(tokens); i++ {
		if t := &tokens[i]; t.Kind == TokenKeyword && t.End < len(line) && line[t.End] == '.' {
			t.Kind = TokenVariable
		}
	}
	if next != 0 {
		return tokens, next
	}
	body := strings.TrimRight(line, " \t\r")
	j := len(body)
	for j > 0 && isIdentByte(body[j-1]) {
		j--
	}
	word := body[j:]
	k := j
	if k > 0 && body[k-1] == '-' {
		k--
	}
	if word == "" || !identStart(word, "") || k < 2 || body[k-2:k] != "<<" || kindAt(tokens[n0:], k-2) != TokenOperator {
		return tokens, next
	}
	toks := relabel(tokens[n0:], k-2, len(body), TokenString)
	return append(tokens[:n0], toks...), hclHeredoc | hclHash(word)<<8
}

// hclHash hashes a heredoc's word into 32 bits, FNV-1a.
func hclHash(s string) uint64 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h = (h ^ uint32(s[i])) * 16777619
	}
	return uint64(h)
}

// templateLexer lexes a template: text in a host language with code in
// tags, as ERB's <% %> or Jinja's {{ }} and {% %}, and comments in others.
// Outside a tag its state is the host's. In a tag that goes on to the next
// line, it is the code's state (bits 0-39), the host's without the state of
// a script or style in it (bits 40-55), and one more than the tag's index
// (bits 62-63). A host whose state needs more bits, as Markdown's in a
// fence, resumes after such a tag from those it kept.
type templateLexer struct {
	// host lexes the text outside tags; without one, it is plain text.
	host, code Tokenizer
	// tags are tried in order where one opens, so a longer opener goes
	// before one it starts with, as <%# before <%. All their openers
	// start with the same byte, and there are at most three.
	tags []templateTag
	// marks are characters after an opener that belong to it, as = and -
	// in <%= and <%-; trims before a closer, as - in -%>.
	marks, trims string
	// escape is an opener written out as text, as <%% in ERB.
	escape string
}

type templateTag struct {
	open, close string
	comment     bool
}

func (t *templateLexer) Tokenize(line string, state uint64, tokens []Token) ([]Token, uint64) {
	out := lexOut{toks: tokens}
	n := len(line)
	tag := int(state >> 62) // the open tag's index plus 1; 0 outside one
	hostState, codeState := state, uint64(0)
	if tag != 0 {
		codeState, hostState = state&(1<<40-1), state>>40&0xffff
	}
	// sub lexes line[from:to] with tk, from state.
	sub := func(tk Tokenizer, from, to int, state uint64) uint64 {
		if from >= to {
			return state
		}
		if tk == nil {
			out.add(to, TokenText)
			return state
		}
		// Lexed into the room after the line's tokens, and each written
		// back behind where it is read.
		toks, next := tk.Tokenize(line[from:to], state, out.toks[len(out.toks):])
		for _, tk := range toks {
			out.add(from+tk.End, tk.Kind)
		}
		return next
	}
	for i := 0; i < n; {
		if tag == 0 {
			j, k := t.opener(line, i)
			hostState = sub(t.host, i, j, hostState)
			if j == n {
				break
			}
			e := j + len(t.tags[k].open)
			if t.tags[k].comment {
				out.add(e, TokenComment)
			} else {
				if e < n && strings.IndexByte(t.marks, line[e]) >= 0 {
					e++
				}
				out.add(e, TokenMeta)
			}
			i, tag, codeState = e, k+1, 0
			continue
		}
		tg := t.tags[tag-1]
		end := strings.Index(line[i:], tg.close)
		stop := n
		if end >= 0 {
			stop = i + end
		}
		if tg.comment {
			out.add(min(stop+len(tg.close), n), TokenComment)
		} else {
			code := stop
			if end >= 0 && code > i && strings.IndexByte(t.trims, line[code-1]) >= 0 {
				code--
			}
			codeState = sub(t.code, i, code, codeState)
			if end >= 0 {
				out.add(stop+len(tg.close), TokenMeta)
			}
		}
		if end < 0 {
			break
		}
		i, tag, codeState = stop+len(tg.close), 0, 0
	}
	if tag != 0 {
		return out.toks, uint64(tag)<<62 | (hostState&0xffff)<<40 | codeState&(1<<40-1)
	}
	return out.toks, hostState
}

// opener returns where the next tag opens in line from i, and which tag,
// skipping escapes; len(line) when none does.
func (t *templateLexer) opener(line string, i int) (int, int) {
	c := t.tags[0].open[0]
	for j := i; ; {
		k := strings.IndexByte(line[j:], c)
		if k < 0 {
			return len(line), 0
		}
		j += k
		if t.escape != "" && strings.HasPrefix(line[j:], t.escape) {
			j += len(t.escape)
			continue
		}
		for k, tg := range t.tags {
			if strings.HasPrefix(line[j:], tg.open) {
				return j, k
			}
		}
		j++
	}
}

// cssLexer lexes CSS, SCSS and Less: selectors, at-rules, properties and
// their values. Its state packs the mode (bits 0-3: 1 a comment) and how
// many blocks the line starts in (bits 8-15).
type cssLexer struct{}

func (cssLexer) Tokenize(line string, state uint64, tokens []Token) ([]Token, uint64) {
	out := lexOut{toks: tokens}
	mode := int(state & 0xf)
	depth := int(state >> 8 & 0xff)
	n := len(line)
	// value: after a property's colon, until ; or }.
	value := false
	for i := 0; i < n; {
		c := line[i]
		if mode == 1 {
			end := strings.Index(line[i:], "*/")
			if end < 0 {
				out.add(n, TokenComment)
				i = n
				continue
			}
			out.add(i+end+2, TokenComment)
			i += end + 2
			mode = 0
			continue
		}
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			out.add(i+1, TokenText)
			i++
		case strings.HasPrefix(line[i:], "/*"):
			mode = 1
			out.add(i+2, TokenComment)
			i += 2
		case strings.HasPrefix(line[i:], "//") && (i == 0 || line[i-1] != ':'):
			out.add(n, TokenComment)
			i = n
		case c == '"' || c == '\'':
			j := i + 1
			for j < n && line[j] != c {
				if line[j] == '\\' {
					j++
				}
				j++
			}
			out.add(min(n, j+1), TokenString)
			i = min(n, j+1)
		case c == '{':
			depth = min(depth+1, 255)
			value = false
			out.add(i+1, TokenPunctuation)
			i++
		case c == '}':
			depth = max(depth-1, 0)
			value = false
			out.add(i+1, TokenPunctuation)
			i++
		case c == ';':
			value = false
			out.add(i+1, TokenPunctuation)
			i++
		case c == '@':
			j := i + 1
			for j < n && (isIdentByte(line[j]) || line[j] == '-') {
				j++
			}
			out.add(j, TokenKeyword)
			i = j
		case c == '$' && i+1 < n && (isIdentByte(line[i+1]) || line[i+1] == '-'):
			j := i + 1
			for j < n && (isIdentByte(line[j]) || line[j] == '-') {
				j++
			}
			out.add(j, TokenVariable)
			i = j
		case c == '#' && value:
			j := i + 1
			for j < n && isIdentByte(line[j]) {
				j++
			}
			out.add(j, TokenNumber)
			i = j
		case c >= '0' && c <= '9' || (c == '.' || c == '-' || c == '+') && i+1 < n && line[i+1] >= '0' && line[i+1] <= '9' && value:
			j := i + 1
			for j < n && (line[j] >= '0' && line[j] <= '9' || line[j] == '.') {
				j++
			}
			// The unit.
			for j < n && (line[j] >= 'a' && line[j] <= 'z' || line[j] == '%') {
				j++
			}
			out.add(j, TokenNumber)
			i = j
		case isIdentByte(c) || c == '-' || c >= 0x80:
			j := i
			for j < n && (isIdentByte(line[j]) || line[j] == '-' || line[j] >= 0x80) {
				j++
			}
			k := j
			for k < n && (line[k] == ' ' || line[k] == '\t') {
				k++
			}
			kind := TokenTag
			switch {
			case value && k < n && line[k] == '(':
				kind = TokenFunction
			case value:
				kind = TokenConstant
				if line[i] == '-' && strings.HasPrefix(line[i:], "--") {
					kind = TokenVariable
				}
			case depth > 0 && k < n && line[k] == ':' && !cssSelectorAhead(line[k:]):
				kind = TokenProperty
			case strings.HasPrefix(line[i:], "--"):
				kind = TokenVariable
			}
			out.add(j, kind)
			i = j
		case c == ':' && !value && depth > 0 && !cssSelectorAhead(line[i:]):
			value = true
			out.add(i+1, TokenPunctuation)
			i++
		case c == ':':
			// A pseudo-class or element.
			j := i + 1
			for j < n && (line[j] == ':' || isIdentByte(line[j]) || line[j] == '-') {
				j++
			}
			out.add(j, TokenKeyword)
			i = j
		case (c == '.' || c == '#') && !value && i+1 < n && (isIdentByte(line[i+1]) || line[i+1] == '-'):
			j := i + 1
			for j < n && (isIdentByte(line[j]) || line[j] == '-') {
				j++
			}
			out.add(j, TokenType)
			i = j
		case c == '!' && strings.HasPrefix(line[i:], "!important"):
			out.add(i+10, TokenKeyword)
			i += 10
		default:
			kind := TokenOperator
			if strings.IndexByte("(),[]", c) >= 0 {
				kind = TokenPunctuation
			}
			out.add(i+1, kind)
			i++
		}
	}
	return out.toks, uint64(mode) | uint64(depth)<<8
}

// cssSelectorAhead reports whether what follows a colon is a selector
// going on to a block, as a:hover {, rather than a value.
func cssSelectorAhead(s string) bool {
	brace := strings.IndexByte(s, '{')
	semi := strings.IndexByte(s, ';')
	return brace >= 0 && (semi < 0 || brace < semi)
}

// markupLexer lexes HTML and XML: tags, attributes, comments, entities,
// and, in HTML, the scripts and styles inside it with their own
// tokenizers. With braces, as Svelte's, it lexes {expressions} in text,
// in tags and in attribute values with the script's tokenizer, and
// {#blocks}. Its state packs the mode (bits 0-3), what the tag being
// lexed opens (bits 4-5: 1 a script, 2 a style), whether the script is
// TypeScript (bit 6), the braces open in an expression (bits 8-13) and
// the mode it goes back to (bits 14-15: text, a tag, or a double- or
// single-quoted value), and the state of the script, style or
// expression (bits 16-55).
type markupLexer struct {
	html          bool
	script, style Tokenizer
	// braces lexes {…} as expressions, as Svelte's.
	braces bool
	// ts lexes a <script lang="ts">, nil for none.
	ts Tokenizer
}

const (
	mlText = iota
	mlTag
	mlComment
	mlDouble
	mlSingle
	mlCData
	mlScript
	mlStyle
	mlExpr
)

// The modes an expression goes back to when it ends.
var mlExprBack = [...]int{mlText, mlTag, mlDouble, mlSingle}

func (m *markupLexer) Tokenize(line string, state uint64, tokens []Token) ([]Token, uint64) {
	out := lexOut{toks: tokens}
	mode := int(state & 0xf)
	embed := int(state >> 4 & 3)
	ts := state >> 6 & 1
	depth := int(state >> 8 & 0x3f)
	back := int(state >> 14 & 3)
	inner := state >> 16
	n := len(line)
	// expr starts an expression at the { at i, going back to the mode
	// after it ends.
	expr := func(i, after int) int {
		out.add(i+1, TokenPunctuation)
		mode, back, depth, inner = mlExpr, after, 0, 0
		// A block's tag: {#if}, {:else}, {/each}, {@html}.
		j := i + 1
		if j+1 < n && strings.IndexByte("#:/@", line[j]) >= 0 && identStart(line[j+1:], "") {
			j++
			for j < n && isIdentByte(line[j]) {
				j++
			}
			out.add(j, TokenKeyword)
		}
		return j
	}
	for i := 0; i < n; {
		switch mode {
		case mlExpr:
			// Lexed into the room after the line's tokens, and each
			// written back behind where it is read.
			toks, next := m.script.Tokenize(line[i:], inner, out.toks[len(out.toks):])
			end := -1
			from := 0
			for _, tk := range toks {
				if tk.Kind == TokenPunctuation {
					for k := from; k < tk.End && end < 0; k++ {
						switch line[i+k] {
						case '{':
							depth = min(depth+1, 0x3f)
						case '}':
							if depth == 0 {
								end = k
							}
							depth--
						}
					}
				}
				if end >= 0 {
					// The token up to the expression's }.
					out.add(i+end, tk.Kind)
					break
				}
				out.add(i+tk.End, tk.Kind)
				from = tk.End
			}
			if end < 0 {
				// The expression goes on to the next line.
				inner = next
				i = n
				continue
			}
			out.add(i+end+1, TokenPunctuation)
			i += end + 1
			mode, depth, back, inner = mlExprBack[back], 0, 0, 0
		case mlScript, mlStyle:
			tag := "</script"
			t := m.script
			if ts == 1 && m.ts != nil {
				t = m.ts
			}
			if mode == mlStyle {
				tag, t = "</style", m.style
			}
			end := indexFold(line[i:], tag)
			stop := n
			if end >= 0 {
				stop = i + end
			}
			if t != nil && stop > i {
				var toks []Token
				toks, inner = t.Tokenize(line[i:stop], inner, nil)
				for _, tk := range toks {
					out.add(i+tk.End, tk.Kind)
				}
			}
			if stop > i {
				out.add(stop, TokenText)
			}
			i = stop
			if end >= 0 {
				mode, inner, ts = mlText, 0, 0
			}
		case mlComment:
			end := strings.Index(line[i:], "-->")
			if end < 0 {
				out.add(n, TokenComment)
				i = n
				continue
			}
			out.add(i+end+3, TokenComment)
			i += end + 3
			mode = mlText
		case mlCData:
			end := strings.Index(line[i:], "]]>")
			if end < 0 {
				out.add(n, TokenString)
				i = n
				continue
			}
			out.add(i+end+3, TokenString)
			i += end + 3
			mode = mlText
		case mlDouble, mlSingle:
			q, after := "\"{", 2
			if mode == mlSingle {
				q, after = "'{", 3
			}
			if !m.braces {
				q = q[:1]
			}
			end := strings.IndexAny(line[i:], q)
			if end < 0 {
				out.add(n, TokenString)
				i = n
				continue
			}
			if line[i+end] == '{' {
				out.add(i+end, TokenString)
				i = expr(i+end, after)
				continue
			}
			out.add(i+end+1, TokenString)
			i += end + 1
			mode = mlTag
		case mlTag:
			c := line[i]
			switch {
			case c == '>':
				out.add(i+1, TokenPunctuation)
				i++
				mode = mlText
				switch embed {
				case 1:
					mode = mlScript
				case 2:
					mode = mlStyle
				}
				embed = 0
			case c == '/' && i+1 < n && line[i+1] == '>':
				out.add(i+2, TokenPunctuation)
				i += 2
				mode, embed = mlText, 0
			case c == '"':
				out.add(i+1, TokenString)
				i++
				mode = mlDouble
			case c == '\'':
				out.add(i+1, TokenString)
				i++
				mode = mlSingle
			case c == '=':
				out.add(i+1, TokenOperator)
				i++
			case c == ' ' || c == '\t' || c == '\r':
				out.add(i+1, TokenText)
				i++
			case c == '{' && m.braces:
				i = expr(i, 1)
			default:
				j := i
				for j < n && strings.IndexByte(" \t\r=>/\"'{", line[j]) < 0 {
					j++
				}
				if j == i {
					j++
				}
				if embed == 1 && m.ts != nil && strings.EqualFold(line[i:j], "lang") {
					ts = 0
					if v := strings.TrimLeft(line[j:], " \t="); len(v) > 0 && (v[0] == '"' || v[0] == '\'') {
						if e := strings.IndexByte(v[1:], v[0]); e >= 0 && (v[1:1+e] == "ts" || v[1:1+e] == "typescript") {
							ts = 1
						}
					}
				}
				out.add(j, TokenAttribute)
				i = j
			}
		default:
			c := line[i]
			switch {
			case strings.HasPrefix(line[i:], "<!--"):
				out.add(i+4, TokenComment)
				i += 4
				mode = mlComment
			case strings.HasPrefix(line[i:], "<![CDATA["):
				out.add(i+9, TokenString)
				i += 9
				mode = mlCData
			case c == '<' && i+1 < n && (line[i+1] == '!' || line[i+1] == '?'):
				j := i + 2
				for j < n && line[j] != '>' && line[j] != ' ' {
					j++
				}
				out.add(j, TokenMeta)
				i = j
				mode = mlTag
			case c == '<' && i+1 < n && (line[i+1] == '/' || identStart(line[i+1:], "")):
				out.add(i+1, TokenPunctuation)
				j := i + 1
				if line[j] == '/' {
					out.add(j+1, TokenPunctuation)
					j++
				}
				k := j
				for k < n && (isIdentByte(line[k]) || line[k] == '-' || line[k] == ':' || line[k] == '.') {
					k++
				}
				out.add(k, TokenTag)
				name := strings.ToLower(line[j:k])
				embed, ts = 0, 0
				if m.html && line[i+1] != '/' {
					switch name {
					case "script":
						embed = 1
					case "style":
						embed = 2
					}
				}
				i = k
				mode = mlTag
			case c == '&':
				j := i + 1
				for j < n && j-i < 32 && (isIdentByte(line[j]) || line[j] == '#') {
					j++
				}
				if j < n && line[j] == ';' {
					out.add(j+1, TokenEscape)
					i = j + 1
				} else {
					out.add(i+1, TokenText)
					i++
				}
			case c == '{' && m.braces:
				i = expr(i, 0)
			default:
				j := i + 1
				for j < n && line[j] != '<' && line[j] != '&' && (line[j] != '{' || !m.braces) {
					j++
				}
				out.add(j, TokenText)
				i = j
			}
		}
	}
	return out.toks, uint64(mode) | uint64(embed)<<4 | ts<<6 | uint64(depth)<<8 | uint64(back)<<14 | inner<<16
}

// indexFold returns where sub, in lower case, first is in s, ignoring
// the case of s's ASCII letters, or -1.
func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// astroLexer lexes Astro: the frontmatter between --- lines at the top
// with its own tokenizer, and the template after it with body's. Its
// state is 0 until the first line with text, the frontmatter's with
// bit 63 inside it, and the template's with bit 62 after.
type astroLexer struct {
	frontmatter, body Tokenizer
}

const (
	astroFrontmatter = 1 << 63
	astroBody        = 1 << 62
)

func (a *astroLexer) Tokenize(line string, state uint64, tokens []Token) ([]Token, uint64) {
	fence := strings.TrimRight(line, " \t\r") == "---"
	switch {
	case state&astroFrontmatter != 0 && fence:
		return appendTok(tokens, len(line), TokenPunctuation), astroBody
	case state&astroFrontmatter != 0:
		toks, inner := a.frontmatter.Tokenize(line, state&^astroFrontmatter, tokens)
		return toks, inner | astroFrontmatter
	case state == 0 && fence:
		return appendTok(tokens, len(line), TokenPunctuation), astroFrontmatter
	case state == 0 && strings.TrimSpace(line) == "":
		toks, _ := a.body.Tokenize(line, 0, tokens)
		return toks, 0
	}
	toks, inner := a.body.Tokenize(line, state&^astroBody, tokens)
	return toks, inner | astroBody
}

// markdownLexer lexes Markdown: headings, emphasis, code spans and
// blocks, links, lists and quotes, and the code of fenced blocks with
// their language. Its state packs the state of a fenced block's language
// (bits 0-39), whether a fenced block is open (bit 40) and its fence
// (bit 41 tildes; bits 42-45 its length), the block's language (bits
// 46-53, its index in Languages plus one), and whether an HTML comment is
// open (bit 54).
type markdownLexer struct{}

const (
	mdFence   = 1 << 40
	mdTilde   = 1 << 41
	mdComment = 1 << 54
	mdInner   = 1<<40 - 1
)

func (markdownLexer) Tokenize(line string, state uint64, tokens []Token) ([]Token, uint64) {
	out := lexOut{toks: tokens}
	n := len(line)
	trimmed := strings.TrimLeft(line, " ")
	indent := n - len(trimmed)
	if state&mdComment != 0 {
		end := strings.Index(line, "-->")
		if end < 0 {
			out.add(n, TokenComment)
			return out.toks, state
		}
		out.add(end+3, TokenComment)
		state &^= mdComment
		mdInline(line, end+3, &out)
		return out.toks, state
	}
	if state&mdFence != 0 {
		ch := byte('`')
		if state&mdTilde != 0 {
			ch = '~'
		}
		length := int(state >> 42 & 0xf)
		run := 0
		for run < len(trimmed) && trimmed[run] == ch {
			run++
		}
		if indent < 4 && run >= length && strings.TrimSpace(trimmed[run:]) == "" {
			out.add(n, TokenPunctuation)
			return out.toks, 0
		}
		li := int(state >> 46 & 0xff)
		langs := Languages()
		if li > 0 && li <= len(langs) && langs[li-1].Tokenizer != nil {
			var inner uint64
			out.toks, inner = langs[li-1].Tokenizer.Tokenize(line, state&mdInner, out.toks)
			if inner > mdInner {
				inner = 0
			}
			out.add(n, TokenText)
			return out.toks, state&^mdInner | inner
		}
		out.add(n, TokenCode)
		return out.toks, state
	}
	if indent < 4 && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
		ch := trimmed[0]
		run := 0
		for run < len(trimmed) && trimmed[run] == ch {
			run++
		}
		info := strings.TrimSpace(trimmed[run:])
		if ch == '`' && strings.Contains(info, "`") {
			mdInline(line, 0, &out)
			return out.toks, state
		}
		out.add(indent+run, TokenPunctuation)
		out.add(n, TokenType)
		next := uint64(mdFence) | uint64(min(run, 15))<<42
		if ch == '~' {
			next |= mdTilde
		}
		name := info
		if sp := strings.IndexAny(name, " \t{"); sp >= 0 {
			name = name[:sp]
		}
		if l := LanguageNamed(name); l != nil {
			for k, x := range Languages() {
				if x == l {
					next |= uint64(k+1) << 46
				}
			}
		}
		return out.toks, next
	}
	switch {
	case indent < 4 && strings.HasPrefix(trimmed, "#"):
		h := 0
		for h < len(trimmed) && trimmed[h] == '#' {
			h++
		}
		if h <= 6 && (h == len(trimmed) || trimmed[h] == ' ' || trimmed[h] == '\t') {
			out.add(n, TokenHeading)
			return out.toks, state
		}
	case indent < 4 && len(trimmed) > 0 && strings.Trim(trimmed, "=") == "" || indent < 4 && len(trimmed) > 1 && strings.Trim(trimmed, "- ") == "" && strings.Count(trimmed, "-") >= 2:
		out.add(n, TokenHeading)
		return out.toks, state
	case indent < 4 && len(trimmed) >= 3 && (strings.Trim(trimmed, "* ") == "" || strings.Trim(trimmed, "_ ") == ""):
		out.add(n, TokenPunctuation)
		return out.toks, state
	}
	i := indent
	out.add(i, TokenText)
	// Quotes and list markers.
	for i < n && line[i] == '>' {
		out.add(i+1, TokenPunctuation)
		i++
		for i < n && line[i] == ' ' {
			i++
		}
		out.add(i, TokenText)
	}
	if i+1 < n && (line[i] == '-' || line[i] == '*' || line[i] == '+') && line[i+1] == ' ' {
		out.add(i+1, TokenPunctuation)
		i++
	} else {
		j := i
		for j < n && j-i < 9 && line[j] >= '0' && line[j] <= '9' {
			j++
		}
		if j > i && j+1 < n && (line[j] == '.' || line[j] == ')') && line[j+1] == ' ' {
			out.add(j+1, TokenPunctuation)
			i = j + 1
		}
	}
	rest := line[i:]
	if strings.HasPrefix(strings.TrimLeft(rest, " "), "[ ] ") || strings.HasPrefix(strings.TrimLeft(rest, " "), "[x] ") || strings.HasPrefix(strings.TrimLeft(rest, " "), "[X] ") {
		k := i + strings.IndexByte(rest, '[')
		out.add(k, TokenText)
		out.add(k+3, TokenKeyword)
		i = k + 3
	}
	if open := mdInline(line, i, &out); open {
		state |= mdComment
	}
	return out.toks, state
}

// mdInline lexes Markdown's inline syntax from i, and reports whether an
// HTML comment stays open at the end of the line.
func mdInline(line string, i int, out *lexOut) bool {
	n := len(line)
	for i < n {
		c := line[i]
		switch {
		case c == '\\' && i+1 < n && strings.IndexByte("\\`*_{}[]()#+-.!|<>~", line[i+1]) >= 0:
			out.add(i+2, TokenEscape)
			i += 2
		case c == '`':
			run := 1
			for i+run < n && line[i+run] == '`' {
				run++
			}
			end := strings.Index(line[i+run:], strings.Repeat("`", run))
			if end < 0 {
				out.add(i+run, TokenText)
				i += run
				continue
			}
			out.add(i+run+end+run, TokenCode)
			i += run + end + run
		case strings.HasPrefix(line[i:], "<!--"):
			end := strings.Index(line[i+4:], "-->")
			if end < 0 {
				out.add(n, TokenComment)
				return true
			}
			out.add(i+4+end+3, TokenComment)
			i += 4 + end + 3
		case c == '<' && i+1 < n && (identStart(line[i+1:], "") || line[i+1] == '/'):
			end := strings.IndexByte(line[i:], '>')
			if end < 0 {
				out.add(i+1, TokenText)
				i++
				continue
			}
			kind := TokenTag
			if strings.Contains(line[i:i+end], "://") || strings.Contains(line[i:i+end], "@") {
				kind = TokenLink
			}
			out.add(i+end+1, kind)
			i += end + 1
		case c == '[' || c == '!' && i+1 < n && line[i+1] == '[':
			start := i
			if c == '!' {
				i++
			}
			close := strings.IndexByte(line[i:], ']')
			if close < 0 {
				out.add(i+1, TokenText)
				i++
				continue
			}
			close += i
			out.add(start, TokenText)
			out.add(close+1, TokenLink)
			i = close + 1
			if i < n && (line[i] == '(' || line[i] == '[') {
				q := byte(')')
				if line[i] == '[' {
					q = ']'
				}
				if end := strings.IndexByte(line[i:], q); end >= 0 {
					out.add(i+end+1, TokenString)
					i += end + 1
				}
			}
		case c == '*' || c == '_' || c == '~':
			run := 1
			for i+run < n && line[i+run] == c {
				run++
			}
			if c == '~' && run < 2 || i+run >= n || line[i+run] == ' ' {
				out.add(i+run, TokenText)
				i += run
				continue
			}
			if c == '_' && i > 0 && isIdentByte(line[i-1]) {
				out.add(i+run, TokenText)
				i += run
				continue
			}
			end := strings.Index(line[i+run:], line[i:i+run])
			if end <= 0 || line[i+run+end-1] == ' ' {
				out.add(i+run, TokenText)
				i += run
				continue
			}
			kind := TokenEmphasis
			switch {
			case c == '~':
				kind = TokenDeleted
			case run >= 2:
				kind = TokenStrong
			}
			out.add(i+run+end+run, kind)
			i += run + end + run
		case strings.HasPrefix(line[i:], "http://") || strings.HasPrefix(line[i:], "https://"):
			j := i
			for j < n && line[j] != ' ' && line[j] != ')' && line[j] != '>' {
				j++
			}
			out.add(j, TokenLink)
			i = j
		case c == '|':
			out.add(i+1, TokenPunctuation)
			i++
		default:
			j := i + 1
			for j < n && strings.IndexByte("\\`<[!*_~h|", line[j]) < 0 {
				j++
			}
			out.add(j, TokenText)
			i = j
		}
	}
	return false
}

// tokenizeDiff lexes unified diffs by the first characters of their
// lines.
func tokenizeDiff(line string, state uint64, tokens []Token) ([]Token, uint64) {
	kind := TokenText
	switch {
	case line == "":
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "diff "),
		strings.HasPrefix(line, "index "), strings.HasPrefix(line, "new file"), strings.HasPrefix(line, "deleted file"),
		strings.HasPrefix(line, "similarity"), strings.HasPrefix(line, "rename "), strings.HasPrefix(line, "old mode"), strings.HasPrefix(line, "new mode"):
		kind = TokenMeta
	case strings.HasPrefix(line, "@@"):
		kind = TokenKeyword
		if end := strings.Index(line[2:], "@@"); end >= 0 {
			tokens = append(tokens, Token{End: end + 4, Kind: TokenKeyword})
			if end+4 < len(line) {
				tokens = append(tokens, Token{End: len(line), Kind: TokenFunction})
			}
			return tokens, 0
		}
	case line[0] == '+', line[0] == '>':
		kind = TokenInserted
	case line[0] == '-', line[0] == '<':
		kind = TokenDeleted
	case line[0] == '\\':
		kind = TokenComment
	}
	if line != "" {
		tokens = append(tokens, Token{End: len(line), Kind: kind})
	}
	return tokens, 0
}
