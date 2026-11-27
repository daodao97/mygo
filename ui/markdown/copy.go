package markdown

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// Copying: as text, a view joins its paragraphs with blank lines, a code
// block's lines and a tight list's items with newlines and a table's
// cells with tabs; as Markdown, it copies the source between the
// selection's ends, with the syntax of emphasis, code and links cut by
// them completed.

func (st *viewState) elem(id uint64) (elemInfo, bool) {
	for _, e := range st.elems {
		if e.id == id {
			return e, true
		}
	}
	return elemInfo{}, false
}

// copyText returns the text of the selected parts.
func (st *viewState) copyText(parts []ui.SelectedText) string {
	var b strings.Builder
	var prev elemInfo
	for k, p := range parts {
		inf, ok := st.elem(p.ID)
		if k > 0 {
			if ok {
				b.WriteString(st.separator(prev, inf))
			} else {
				b.WriteString("\n")
			}
		}
		if ok {
			st.writeText(&b, inf, p)
		} else {
			b.WriteString(runeSlice(p.Text, p.Start, p.End))
		}
		prev = inf
	}
	return b.String()
}

// writeText writes the text of a selected part, with the source of the
// math drawn as objects, as $x_i$, in place of their characters, and
// without links' icons, which have no source, or their joiners.
func (st *viewState) writeText(b *strings.Builder, inf elemInfo, p ui.SelectedText) {
	text := runeSlice(p.Text, p.Start, p.End)
	if inf.line != -1 || inf.block >= len(st.paras) || !st.paras[inf.block].ok || len(st.paras[inf.block].p.objects) == 0 {
		b.WriteString(text)
		return
	}
	objects := st.paras[inf.block].p.objects
	src := st.doc.Source()
	r := p.Start
	for _, c := range text {
		if c == linkIconJoiner {
			if k, found := slices.BinarySearchFunc(objects, r-1, func(o mathObject, r int) int { return o.rune - r }); found && objects[k].src == objects[k].end {
				r++
				continue
			}
		}
		k, found := slices.BinarySearchFunc(objects, r, func(o mathObject, r int) int { return o.rune - r })
		if found && c == '\uFFFC' && objects[k].end <= len(src) {
			b.WriteString(src[objects[k].src:objects[k].end])
		} else {
			b.WriteRune(c)
		}
		r++
	}
}

// copyData returns the selection as Copy puts it on the clipboard: as
// text, as HTML and as Markdown.
func (st *viewState) copyData(parts []ui.SelectedText) transfer.Data {
	md := st.copyMarkdown(parts)
	d := parser.New(st.doc.Extensions())
	d.Set(md)
	return transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.Text, []byte(st.copyText(parts))),
		transfer.Bytes(transfer.HTML, []byte(`<meta charset="utf-8">`+d.HTML())),
		transfer.Bytes(transfer.Markdown, []byte(md)),
	))
}

// markdownData returns Markdown as Copy as Markdown puts it on the
// clipboard: as text, and as Markdown.
func markdownData(md string) transfer.Data {
	return transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.Text, []byte(md)),
		transfer.Bytes(transfer.Markdown, []byte(md)),
	))
}

func runeSlice(s string, from, to int) string {
	i, n := 0, 0
	a, z := len(s), len(s)
	for i < len(s) {
		if n == from {
			a = i
		}
		if n == to {
			z = i
			break
		}
		_, w := utf8.DecodeRuneInString(s[i:])
		i += w
		n++
	}
	if from >= n && a == len(s) {
		a = len(s)
	}
	return s[a:max(a, z)]
}

// separator returns what goes between the text of two blocks copied.
func (st *viewState) separator(a, b elemInfo) string {
	d := st.doc
	if a.block == b.block {
		return "\n"
	}
	ab, bb := d.Block(a.block), d.Block(b.block)
	if ab.Kind == parser.TableCell && bb.Kind == parser.TableCell {
		if ab.Parent == bb.Parent {
			return "\t"
		}
		return "\n"
	}
	// Items of the same tight list.
	if l := st.listOf(b.block); l > 0 && d.Block(l).Tight && st.listOf(a.block) == l || st.listOf(a.block) > 0 && d.Block(st.listOf(a.block)).Tight && st.within(b.block, st.listOf(a.block)) {
		return "\n"
	}
	return "\n\n"
}

// listOf returns the nearest list around block i, or 0.
func (st *viewState) listOf(i int) int {
	for p := st.doc.Block(i).Parent; p > 0; p = st.doc.Block(p).Parent {
		if st.doc.Block(p).Kind == parser.List {
			return p
		}
	}
	return 0
}

func (st *viewState) within(i, anc int) bool {
	for p := st.doc.Block(i).Parent; p >= 0; p = st.doc.Block(p).Parent {
		if p == anc {
			return true
		}
	}
	return false
}

// srcAt returns the source offset of rune r of an element's text, for
// the end of a selection when end.
func (st *viewState) srcAt(inf elemInfo, r int, end bool) int {
	d := st.doc
	b := d.Block(inf.block)
	switch b.Kind {
	case parser.CodeBlock, parser.HTMLBlock, parser.MathBlock:
		// Lines from inf.line on, each followed by a newline.
		n := d.LineCount(inf.block)
		for k := max(inf.line, 0); k < n; k++ {
			line, at := d.Line(inf.block, k)
			c := utf8.RuneCountInString(line)
			if r <= c || k == n-1 {
				// Spaces standing for a tab partly taken by indentation
				// are not the source's.
				pad := linePad(d.Source(), line, at)
				off := at
				for j := pad; j < r && off < len(d.Source()); j++ {
					_, w := utf8.DecodeRuneInString(d.Source()[off:])
					off += w
				}
				return off
			}
			r -= c + 1
		}
		return b.Start
	}
	if inf.block < len(st.paras) && st.paras[inf.block].ok {
		return st.paras[inf.block].p.sourceAt(d.Source(), r, end)
	}
	return b.Start
}

// copyMarkdown returns the Markdown of the selected parts, or of the
// whole document when none is selected. The parts of views sharing the
// selection (Options.ShareSelection) built within this one's blocks copy
// as those views copy them, each as the Markdown of its own source,
// between this one's.
func (st *viewState) copyMarkdown(parts []ui.SelectedText) string {
	if len(parts) == 0 {
		return st.doc.Source()
	}
	runs := st.runs(parts)
	if len(runs) == 1 && runs[0].st == st {
		// This view's alone, with what it does not know at its ends.
		return st.ownMarkdown(parts)
	}
	if len(runs) == 0 {
		return st.copyText(parts)
	}
	var b strings.Builder
	for k, r := range runs {
		if k > 0 {
			b.WriteString(st.separator(runs[k-1].last, r.first))
		}
		b.WriteString(r.st.copyMarkdown(parts[r.from:r.to]))
	}
	return b.String()
}

// mdRun is a run of selected parts that one view copies as Markdown:
// this one, or a view sharing its selection within it. first and last
// are where the run starts and ends among this view's blocks.
type mdRun struct {
	st          *viewState
	from, to    int
	first, last elemInfo
}

// runs splits the selected parts into runs of the views that know them.
// Parts no view knows within a run (the text of blocks built by
// Options.Block, Math or HTML) stay in it, and those between runs or at
// the ends go.
func (st *viewState) runs(parts []ui.SelectedText) []mdRun {
	var runs []mdRun
	for k, p := range parts {
		owner, at, ok := st.owner(p.ID)
		if !ok {
			continue
		}
		if n := len(runs); n > 0 && runs[n-1].st == owner {
			runs[n-1].to, runs[n-1].last = k+1, at
			continue
		}
		runs = append(runs, mdRun{st: owner, from: k, to: k + 1, first: at, last: at})
	}
	return runs
}

// owner returns the view that copies element id, this one or a view
// sharing its selection within it, and where the element is among this
// view's blocks.
func (st *viewState) owner(id uint64) (*viewState, elemInfo, bool) {
	if inf, ok := st.elem(id); ok {
		return st, inf, true
	}
	for _, n := range st.nested {
		if n.st.knows(id) {
			return n.st, elemInfo{id: id, block: n.block, line: -1}, true
		}
	}
	return nil, elemInfo{}, false
}

// knows reports whether the view, or a view sharing its selection within
// it, built element id.
func (st *viewState) knows(id uint64) bool {
	_, _, ok := st.owner(id)
	return ok
}

// ownMarkdown returns the Markdown of the selected parts of this view's
// own text.
func (st *viewState) ownMarkdown(parts []ui.SelectedText) string {
	d := st.doc
	first, last := parts[0], parts[len(parts)-1]
	fi, ok1 := st.elem(first.ID)
	li, ok2 := st.elem(last.ID)
	if !ok1 || !ok2 {
		return st.copyText(parts)
	}
	fb, lb := d.Block(fi.block), d.Block(li.block)
	a := st.srcAt(fi, first.Start, false)
	b := st.srcAt(li, last.End, true)
	atStart := first.Start == 0 && fi.line <= 0
	atEnd := last.End >= utf8.RuneCountInString(last.Text)
	if lb.Kind == parser.CodeBlock && atEnd {
		// Only the last chunk of lines ends the block.
		atEnd = li.line+chunkLines >= d.LineCount(li.block)
	}
	// From a block's start, or to its end, its syntax too: a heading's #,
	// a list's marker, a quote's >, a table row's pipes, a code block's
	// fences when it is selected whole.
	switch {
	case !atStart:
	case fb.Kind == parser.TableCell:
		// A row's pipes go with it when cells after it are selected too.
		if fi.block == fb.Parent+1 && li.block != fi.block {
			a = lineStartOf(d.Source(), fb.Start)
		}
	case fb.Kind == parser.CodeBlock:
		if fb.Fenced && atEnd && fi.block == li.block {
			a = lineStartOf(d.Source(), fb.Start)
		}
	default:
		a = lineStartOf(d.Source(), fb.Start)
	}
	switch {
	case !atEnd:
	case lb.Kind == parser.TableCell:
		if lb.Next == d.Block(lb.Parent).Next && li.block != fi.block {
			b = lineEndOf(d.Source(), lb.End)
		}
	case lb.Kind == parser.CodeBlock:
		if lb.Fenced && !lb.Open && atStart && fi.block == li.block {
			b = lb.End
		}
	default:
		b = lineEndOf(d.Source(), lb.End)
	}
	if b < a {
		a, b = b, a
	}
	body := d.Source()[a:b]
	prefix := st.openAt(fi.block, a)
	suffix := st.closeAt(li.block, b)
	// Emphasis's delimiters open and close nothing next to whitespace,
	// as **bold **: what the selection has at its ends goes outside them.
	var lead, trail string
	if st.inEmphasis(fi.block, a) {
		rest := strings.TrimLeftFunc(body, unicode.IsSpace)
		lead, body = body[:len(body)-len(rest)], rest
	}
	if st.inEmphasis(li.block, b) {
		rest := strings.TrimRightFunc(body, unicode.IsSpace)
		trail, body = body[len(rest):], rest
	}
	if body == "" {
		return lead + trail
	}
	return lead + prefix + body + suffix + trail
}

// inEmphasis reports whether the innermost span of block i whose syntax
// copying completes, that offset at falls inside, is emphasis, strong or
// strikethrough.
func (st *viewState) inEmphasis(i, at int) bool {
	in := st.doc.Inlines(i)
	inner := -1
	for k := range in {
		if n := &in[k]; n.Start < at && at < n.End && st.openSyntax(in, k) != "" {
			inner = k
		}
	}
	if inner == -1 {
		return false
	}
	switch in[inner].Kind {
	case parser.Emphasis, parser.Strong, parser.Strikethrough:
		return true
	}
	return false
}

func lineEndOf(src string, off int) int {
	for off < len(src) && src[off] != '\n' && src[off] != '\r' {
		off++
	}
	return off
}

// openAt returns the opening syntax of the inline spans of block i that
// offset at falls inside, outermost first.
func (st *viewState) openAt(i, at int) string {
	in := st.doc.Inlines(i)
	var b strings.Builder
	for k := range in {
		n := &in[k]
		if n.Start < at && at < n.End {
			if open := st.openSyntax(in, k); open != "" {
				b.WriteString(open)
			}
		}
	}
	return b.String()
}

// closeAt returns the closing syntax of the inline spans of block i that
// offset at falls inside, innermost first.
func (st *viewState) closeAt(i, at int) string {
	in := st.doc.Inlines(i)
	var closes []string
	for k := range in {
		n := &in[k]
		if n.Start < at && at < n.End {
			if c := st.closeSyntax(in, k); c != "" {
				closes = append(closes, c)
			}
		}
	}
	slices.Reverse(closes)
	return strings.Join(closes, "")
}

func (st *viewState) openSyntax(in []parser.Inline, k int) string {
	n := &in[k]
	src := st.doc.Source()
	switch n.Kind {
	case parser.Emphasis, parser.Strong, parser.Strikethrough, parser.Link:
		if k+1 < n.Next {
			return src[n.Start:in[k+1].Start]
		}
	case parser.Code:
		return src[n.Start:codeStart(src, n)]
	}
	return ""
}

func (st *viewState) closeSyntax(in []parser.Inline, k int) string {
	n := &in[k]
	src := st.doc.Source()
	switch n.Kind {
	case parser.Emphasis, parser.Strong, parser.Strikethrough, parser.Link:
		if k+1 < n.Next {
			// The end of its last child.
			end := in[k+1].End
			for j := k + 1; j < n.Next; j = in[j].Next {
				end = in[j].End
			}
			return src[end:n.End]
		}
	case parser.Code:
		ticks := codeStart(src, n) - n.Start
		return src[n.End-ticks : n.End]
	}
	return ""
}

// linePad returns how many spaces a code line starts with that are not
// the source's at, standing for a tab partly taken by indentation.
func linePad(src, line string, at int) int {
	pad := 0
	for pad < len(line) && line[pad] == ' ' && !strings.HasPrefix(src[at:], line[pad:]) {
		pad++
	}
	return pad
}

func lineStartOf(src string, off int) int {
	for off > 0 && src[off-1] != '\n' && src[off-1] != '\r' {
		off--
	}
	return off
}

// isEscapable reports whether a backslash escapes c: ASCII punctuation.
func isEscapable(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
