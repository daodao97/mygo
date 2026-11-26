package richtext

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/rope"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// ParseMarkdown returns the attributed text of Markdown, CommonMark with
// GitHub's extensions and footnotes, as Markdown writes it: headings,
// lists and task lists, quotations, blocks of code, bold, italic,
// strikethrough, code, links, images, and the HTML tags of these marks
// and of underlines, <u>. A list item's paragraphs after its first are
// indented under it; a table's rows are paragraphs of their cells apart
// by tabs, the header's bold. Math, which the editor does not show, stays
// as written: \[ is an escaped bracket, as everywhere but in math.
func ParseMarkdown(s string) Text {
	// Invalid UTF-8 becomes U+FFFD: the halves of a broken sequence,
	// apart in the source, would meet in the text as one rune, which the
	// runs count as two.
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	d := parser.New(parser.GFM | parser.ExtFootnotes)
	d.Set(s)
	m := mdReader{d: d}
	m.b.buf.Grow(len(s))
	m.blocks(0, ParagraphStyle{}, -1)
	return m.b.text()
}

// mdReader projects the blocks and inlines of a Markdown document
// directly into the text buffer and run slab of a Text.
type mdReader struct {
	d *parser.Document
	b textBuilder
	// html counts the inline HTML tags open, by the attribute they turn
	// on: <u>, <s>, <b> and <i> and their synonyms.
	html [4]int
}

// textBuilder builds the paragraphs of a Text in one byte buffer and
// fixed-size slabs of attrRuns, avoiding intermediate []Paragraph and
// []Run copies.
type textBuilder struct {
	buf   strings.Builder
	paras []richPara
	slab  []attrRun
	from  int // where the open paragraph's runs start in slab
	open  bool
	style ParagraphStyle
	n     int        // runes of the open paragraph
	last  Attributes // attributes of its last text, which its end takes
	// label is a footnote's prefix, prepended to the first paragraph of
	// its definition.
	label string
}

// slabRuns caps a run slab's size: each paragraph's runs are a bounded
// subslice of a slab, which is replaced rather than grown so earlier
// paragraphs' runs are never copied.
const slabRuns = 4096

// start opens a paragraph of style st.
func (b *textBuilder) start(st ParagraphStyle) {
	if b.open {
		b.end()
	}
	if len(b.paras) > 0 {
		b.buf.WriteByte('\n')
	}
	b.open, b.style, b.n, b.last = true, st, 0, Attributes{}
	b.from = len(b.slab)
	if b.label != "" {
		b.addLine(b.label, Attributes{})
		b.label = ""
	}
}

// end closes the open paragraph, its end taking the attributes of its
// last run, extending.
func (b *textBuilder) end() {
	b.endWith(b.last.extending())
}

func (b *textBuilder) endWith(a Attributes) {
	b.room(1)
	b.slab = appendRunFrom(b.slab, b.from, attrRun{1, a})
	runs := b.slab[b.from:len(b.slab):len(b.slab)]
	b.paras = append(b.paras, richPara{id: paraIDs.Add(1), style: b.style.normalized(), n: b.n, runs: runs})
	b.open = false
}

// add appends s with attributes a to the open paragraph: a newline in s
// starts another paragraph of the same style, matching TextOf.
func (b *textBuilder) add(s string, a Attributes) {
	b.last = a
	for {
		line, rest, br := strings.Cut(s, "\n")
		b.addLine(line, a)
		if !br {
			return
		}
		b.endWith(a.extending())
		st := b.style
		st.ID = ""
		b.start(st)
		b.last = a
		s = rest
	}
}

// addLine appends line (without newlines) with attributes a to the open
// paragraph.
func (b *textBuilder) addLine(s string, a Attributes) {
	if s == "" {
		return
	}
	b.buf.WriteString(s)
	b.last = a
	if a.Attachment == nil {
		n := rope.CountRunes(s)
		b.n += n
		b.room(1)
		b.slab = appendRunFrom(b.slab, b.from, attrRun{n, a})
		return
	}
	plain := a
	plain.Attachment = nil
	for _, r := range s {
		b.n++
		b.room(1)
		if r == ObjectReplacement {
			b.slab = appendRunFrom(b.slab, b.from, attrRun{1, a})
		} else {
			b.slab = appendRunFrom(b.slab, b.from, attrRun{1, plain})
		}
	}
}

// room ensures space for k more runs in the current slab, moving only
// the open paragraph's runs to a fresh slab when full.
func (b *textBuilder) room(k int) {
	if len(b.slab)+k <= cap(b.slab) {
		return
	}
	open := b.slab[b.from:]
	next := min(max(cap(b.slab)*2, 16), slabRuns)
	slab := make([]attrRun, 0, max(next, 2*(len(open)+k)))
	b.slab = append(slab, open...)
	b.from = 0
}

// appendRunFrom is appendRun scoped to the open paragraph's runs from from.
func appendRunFrom(runs []attrRun, from int, r attrRun) []attrRun {
	if r.n <= 0 {
		return runs
	}
	if k := len(runs) - 1; k >= from && runs[k].a == r.a && r.a.Attachment == nil {
		runs[k].n += r.n
		return runs
	}
	return append(runs, r)
}

func (b *textBuilder) text() Text {
	if b.open {
		b.end()
	}
	if len(b.paras) == 0 {
		return Text{}
	}
	return Text{text: rope.Build(b.buf.String()), paras: seqBuild(b.paras)}
}

// blocks adds the children of block i in style st; depth is the level of
// the list they are in, -1 outside lists.
func (m *mdReader) blocks(i int, st ParagraphStyle, depth int) {
	for k := range m.d.Children(i) {
		m.block(k, st, depth)
	}
}

func (m *mdReader) block(i int, st ParagraphStyle, depth int) {
	b := m.d.Block(i)
	switch b.Kind {
	case parser.Paragraph:
		m.inlinePara(st, m.d.Inlines(i))
	case parser.Heading:
		// A heading is no item, as SetHeading and ParseHTML make it.
		st.Heading, st.List, st.Checked = b.Level, NoList, false
		m.inlinePara(st, m.d.Inlines(i))
	case parser.CodeBlock:
		st.CodeBlock, st.Heading = true, 0
		n := m.d.LineCount(i)
		for k := range n {
			line, _ := m.d.Line(i, k)
			m.b.start(st)
			m.b.add(line, Attributes{})
		}
		if n == 0 {
			m.b.start(st)
		}
	case parser.HTMLBlock:
		for _, p := range ParseHTML(m.d.Lines(i)).Paragraphs() {
			depth := p.Style.quoteDepth() + st.quoteDepth()
			p.Style.Quote = depth > 0
			if depth > 1 {
				p.Style.QuoteDepth = depth
			}
			p.Style.Indent += st.Indent
			m.b.start(p.Style)
			for _, r := range p.Runs {
				m.b.add(r.Text, r.Attributes)
			}
		}
	case parser.Quote:
		if st.Quote {
			st.QuoteDepth = st.quoteDepth() + 1
		}
		st.Quote = true
		m.blocks(i, st, depth)
	case parser.List:
		kind := Bullets
		if b.Ordered {
			kind = Numbers
		}
		for it := range m.d.Children(i) {
			ist := ParagraphStyle{List: kind, Indent: depth + 1, Quote: st.Quote, QuoteDepth: st.QuoteDepth}
			if task := m.d.Block(it).Task; task != parser.NoTask {
				ist.List, ist.Checked = Tasks, task == parser.Checked
			} else if checked, ok := m.bareTask(it); ok {
				ist.List, ist.Checked = Tasks, checked
			}
			m.item(it, ist, depth+1)
		}
	case parser.Table:
		for row := range m.d.Children(i) {
			m.b.start(st)
			var a Attributes
			if m.d.Block(row).Header {
				a.Weight = 700
			}
			first := true
			for cell := range m.d.Children(row) {
				if !first {
					m.b.add("\t", a)
				}
				first = false
				m.html = [4]int{}
				in := m.d.Inlines(cell)
				m.inlines(in, 0, len(in), a, m.b.add)
			}
		}
	case parser.FootnoteDefinition:
		// The label goes before the first paragraph, after the labels of
		// the definitions it is in.
		prev := m.b.label
		m.b.label = prev + "[" + b.Label() + "] "
		m.blocks(i, st, depth)
		if m.b.label != "" {
			// No paragraph took it: the next one takes the outer labels.
			m.b.label = prev
		}
	}
}

// item adds a list item: its first paragraph in the item's style ist, and
// what follows indented under it.
func (m *mdReader) item(i int, ist ParagraphStyle, depth int) {
	under := ParagraphStyle{Indent: depth + 1, Quote: ist.Quote, QuoteDepth: ist.QuoteDepth}
	first := true
	for k := range m.d.Children(i) {
		b := m.d.Block(k)
		if first {
			first = false
			if _, ok := m.bareTask(i); ok {
				m.b.start(ist)
				continue
			}
			if b.Kind == parser.Paragraph || b.Kind == parser.Heading {
				m.block(k, ist, depth)
				continue
			}
			// An item starting with code or a list has an empty line of
			// its own, as it shows.
			m.b.start(ist)
		}
		m.block(k, under, depth)
	}
	if first {
		m.b.start(ist)
	}
}

// bareTask reports whether list item i is an empty task, [ ] or [x] with
// nothing after, as Markdown writes one: GitHub reads it as text, as it
// asks for text after the check box.
func (m *mdReader) bareTask(i int) (checked, ok bool) {
	if m.d.Block(i).Task != parser.NoTask {
		return false, false
	}
	for k := range m.d.Children(i) {
		if m.d.Block(k).Kind != parser.Paragraph || m.d.LineCount(k) != 1 {
			return false, false
		}
		switch line, _ := m.d.Line(k, 0); strings.TrimSpace(line) {
		case "[ ]":
			return false, true
		case "[x]", "[X]":
			return true, true
		}
		return false, false
	}
	return false, false
}

// inlinePara adds a paragraph of inlines in style st; a hard line break
// starts another, indented under a list item rather than an item of its
// own.
func (m *mdReader) inlinePara(st ParagraphStyle, in []parser.Inline) {
	m.b.start(st)
	m.html = [4]int{}
	m.inlines(in, 0, len(in), Attributes{}, func(s string, a Attributes) {
		for {
			line, rest, br := strings.Cut(s, "\n")
			m.b.addLine(line, a)
			if !br {
				return
			}
			m.b.end()
			if st.List != NoList {
				st = ParagraphStyle{Indent: st.Indent + 1, Quote: st.Quote, QuoteDepth: st.QuoteDepth}
			}
			m.b.start(st)
			s = rest
		}
	})
}

func (m *mdReader) inlines(in []parser.Inline, from, to int, a Attributes, emit func(string, Attributes)) {
	add := func(s string, a Attributes) {
		if m.html[0] > 0 {
			a.Underline = true
		}
		if m.html[1] > 0 {
			a.Strikethrough = true
		}
		if m.html[2] > 0 {
			a.Weight = 700
		}
		if m.html[3] > 0 {
			a.Italic = true
		}
		emit(s, a)
	}
	for k := from; k < to; k = in[k].Next {
		n := &in[k]
		x := a
		switch n.Kind {
		case parser.Text:
			add(n.Text, a)
		case parser.SoftBreak:
			add(" ", a)
		case parser.LineBreak:
			add("\n", a)
		case parser.Code:
			x.Code = true
			add(n.Text, x)
		case parser.Emphasis:
			x.Italic = true
			m.inlines(in, k+1, n.Next, x, emit)
		case parser.Strong:
			x.Weight = 700
			m.inlines(in, k+1, n.Next, x, emit)
		case parser.Strikethrough:
			x.Strikethrough = true
			m.inlines(in, k+1, n.Next, x, emit)
		case parser.Link:
			x.Link = safeLink(n.Dest)
			m.inlines(in, k+1, n.Next, x, emit)
		case parser.Image:
			x = a.extending()
			x.Attachment = &Attachment{Name: parser.PlainText(in[k+1 : n.Next]), URL: safeLink(n.Dest)}
			emit(string(ObjectReplacement), x)
		case parser.HTML:
			m.tag(n.Text, a, emit)
		case parser.FootnoteRef:
			add("["+n.Text+"]", a)
		}
	}
}

// tag applies an inline HTML tag: the opening and closing tags of
// underlines, strikethrough, bold and italic, and line breaks. Other
// tags are left out, as a browser shows none.
func (m *mdReader) tag(s string, a Attributes, emit func(string, Attributes)) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(s, "<") || !strings.HasSuffix(s, ">") {
		return
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "<"), ">"))
	closing := strings.HasPrefix(s, "/")
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	name, _, _ := strings.Cut(strings.TrimSpace(s), " ")
	which := -1
	switch name {
	case "br":
		emit("\n", a)
		return
	case "u", "ins":
		which = 0
	case "s", "strike", "del":
		which = 1
	case "b", "strong":
		which = 2
	case "i", "em":
		which = 3
	}
	if which < 0 {
		return
	}
	if closing {
		m.html[which] = max(m.html[which]-1, 0)
	} else {
		m.html[which]++
	}
}
