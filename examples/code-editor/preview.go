package main

import (
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

var (
	markdownEmphasis = regexp.MustCompile("(\\*\\*|\\*|`)([^*`]+)(\\*\\*|\\*|`)")
	markdownLink     = regexp.MustCompile(`(\[)[^\]]+(\]\([^)]*\))`)
	markdownHeading  = regexp.MustCompile(`^#{1,6} `)
	markdownBullet   = regexp.MustCompile(`^\s*(-) `)
	markdownTask     = regexp.MustCompile(`^\s*(- \[[ x]\])`)
	markdownImage    = regexp.MustCompile(`^!\[([^\]]*)\]\([^)]*\)\s*$`)
)

// livePreview shows Markdown as it reads, the file's text staying as it
// is: the marks hide, bullets show as dots and tasks as check boxes,
// images show under their lines and tables in place of theirs, but on
// the lines with a caret, whose source shows to edit.
func livePreview(st *editor.State) ([]editor.Replacement, []editor.Block) {
	b := st.Buffer
	carets := map[int]bool{}
	for _, s := range st.Selections() {
		for line := b.Pos(s.Start()).Line; line <= b.Pos(s.End()).Line; line++ {
			carets[line] = true
		}
	}
	var out []editor.Replacement
	var blocks []editor.Block
	fenced := false
	for i := 0; i < b.LineCount(); i++ {
		line := b.Line(i)
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		start := b.Offset(editor.Pos{Line: i})
		// A table's lines, in place of which it shows.
		if strings.HasPrefix(line, "|") {
			last, touched := i, carets[i]
			for last+1 < b.LineCount() && strings.HasPrefix(b.Line(last+1), "|") {
				last++
				touched = touched || carets[last]
			}
			if !touched && last+1 < b.LineCount() {
				out = append(out, editor.Replacement{Start: start, End: b.Offset(editor.Pos{Line: last + 1})})
				blocks = append(blocks, tableBlock(b, i, last))
			}
			i = last
			continue
		}
		if m := markdownImage.FindStringSubmatch(line); m != nil {
			blocks = append(blocks, imageBlock(i, m[1]))
			continue
		}
		if carets[i] {
			continue
		}
		if m := markdownHeading.FindStringIndex(line); m != nil {
			out = append(out, editor.Replacement{Start: start, End: start + m[1]})
		}
		if m := markdownTask.FindStringSubmatchIndex(line); m != nil {
			out = append(out, taskBox(st, start+m[2], start+m[3]))
		} else if m := markdownBullet.FindStringSubmatchIndex(line); m != nil {
			out = append(out, editor.Replacement{Start: start + m[2], End: start + m[3], Text: "•"})
		}
		for _, m := range markdownEmphasis.FindAllStringSubmatchIndex(line, -1) {
			if line[m[2]:m[3]] == line[m[6]:m[7]] {
				out = append(out, editor.Replacement{Start: start + m[2], End: start + m[3]},
					editor.Replacement{Start: start + m[6], End: start + m[7]})
			}
		}
		for _, m := range markdownLink.FindAllStringSubmatchIndex(line, -1) {
			out = append(out, editor.Replacement{Start: start + m[2], End: start + m[3]},
				editor.Replacement{Start: start + m[4], End: start + m[5]})
		}
	}
	return out, blocks
}

// imageBlock shows a picture under its line: here a card with its text,
// as high as a real one would lay out.
func imageBlock(line int, alt string) editor.Block {
	return editor.Block{Line: line, Height: 96, Label: alt,
		Content: func(c *ui.Context, e ui.Element, b editor.Block) {
			t := c.Theme()
			e.Padding(4, 0).Children(func() {
				ui.Box(c).Width(220).Height(88).Center().Background(t.Surface).Radius(8).Children(func() {
					ui.Text(c, alt).TextColor(t.TextMuted)
				})
			})
		}}
}

// tableBlock shows the table of lines first to last in place of them,
// its second line being the one under its head.
func tableBlock(b *editor.Buffer, first, last int) editor.Block {
	var rows [][]string
	for i := first; i <= last; i++ {
		if i == first+1 {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(b.Line(i)), "|"), "|")
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		rows = append(rows, cells)
	}
	return editor.Block{Line: first, Height: float32(len(rows)) * 28, Label: "Table",
		Content: func(c *ui.Context, e ui.Element, _ editor.Block) {
			t := c.Theme()
			e.Padding(4, 0).Children(func() {
				ui.Column(c).Width(360).Border(1, t.Border).Radius(6).Children(func() {
					for i, row := range rows {
						ui.Row(c).Padding(4, 8).Children(func() {
							for _, cell := range row {
								text := ui.Text(c, cell).Grow(1).Width(0)
								if i == 0 {
									text.Bold()
								}
							}
						})
					}
				})
			})
		}}
}

// taskBox draws a check box in place of a task's "- [ ]", which a click
// toggles in the text.
func taskBox(st *editor.State, start, end int) editor.Replacement {
	done := strings.Contains(st.Buffer.Slice(start, end), "x")
	label, mark := "Not done", "☐"
	if done {
		label, mark = "Done", "☑"
	}
	return editor.Replacement{Start: start, End: end, Label: label, Interactive: true,
		Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
			e.Children(func() {
				b := ui.ButtonBase(c).Fill().Center().KeepFocus().Cursor(ui.CursorPointer).Label(label)
				b.Children(func() { ui.Text(c, mark) })
				if b.Clicked() {
					text := "x"
					if done {
						text = " "
					}
					st.Buffer.Edit(editor.Edit{Start: r.End - 2, End: r.End - 1, Text: text})
				}
			})
		}}
}

// headingStyles shows the headings in larger type, by their level, on the
// lines with a caret too, as their marks show there.
func headingStyles(b *editor.Buffer) []editor.LineStyle {
	var out []editor.LineStyle
	fenced := false
	for i := range b.LineCount() {
		line := b.Line(i)
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if m := markdownHeading.FindString(line); m != "" && !fenced {
			level := len(m) - 1
			out = append(out, editor.LineStyle{Line: i, Font: ui.Font{Size: float32(26 - 3*level), Weight: 700}})
		}
	}
	return out
}
