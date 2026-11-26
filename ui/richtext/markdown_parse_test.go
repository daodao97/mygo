package richtext

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// dump writes the paragraphs of t, their styles and runs, to compare.
func dump(t Text) string {
	var b strings.Builder
	for _, p := range t.Paragraphs() {
		fmt.Fprintf(&b, "%+v:", p.Style)
		for _, r := range p.Runs {
			a := r.Attributes
			at := a.Attachment
			a.Attachment = nil
			fmt.Fprintf(&b, " %q%+v", r.Text, a)
			if at != nil {
				fmt.Fprintf(&b, "[%q %q]", at.Name, at.URL)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestParseMarkdown(t *testing.T) {
	want := sample()
	// Markdown has no alignment.
	var paras []Paragraph
	for _, p := range want.Paragraphs() {
		p.Style.Align = ui.Start
		paras = append(paras, p)
	}
	want = TextOf(paras...)
	if got := ParseMarkdown(sample().Markdown()); dump(got) != dump(want) {
		t.Errorf("got\n%s\nwant\n%s", dump(got), dump(want))
	}

	para := func(st ParagraphStyle, runs ...Run) Paragraph { return Paragraph{Style: st, Runs: runs} }
	bold := Attributes{Weight: 700}
	for _, c := range []struct {
		src  string
		want Text
	}{
		{"- item\n\n  more\n\n      code\n- next", TextOf(
			para(ParagraphStyle{List: Bullets}, Run{Text: "item"}),
			para(ParagraphStyle{Indent: 1}, Run{Text: "more"}),
			para(ParagraphStyle{Indent: 1, CodeBlock: true}, Run{Text: "code"}),
			para(ParagraphStyle{List: Bullets}, Run{Text: "next"}),
		)},
		{"> - [x] done\n>   - nested", TextOf(
			para(ParagraphStyle{List: Tasks, Checked: true, Quote: true}, Run{Text: "done"}),
			para(ParagraphStyle{List: Bullets, Indent: 1, Quote: true}, Run{Text: "nested"}),
		)},
		{"a  \nb", TextOf(para(ParagraphStyle{}, Run{Text: "a"}), para(ParagraphStyle{}, Run{Text: "b"}))},
		{"1. a\\\n   b", TextOf(para(ParagraphStyle{List: Numbers}, Run{Text: "a"}), para(ParagraphStyle{Indent: 1}, Run{Text: "b"}))},
		{"| a | b |\n|---|---|\n| 1 | **2** |", TextOf(
			para(ParagraphStyle{}, Run{Text: "a\tb", Attributes: bold}),
			para(ParagraphStyle{}, Run{Text: "1\t"}, Run{Text: "2", Attributes: bold}),
		)},
		{"x <u>under *it*</u> <b>b</b>", TextOf(para(ParagraphStyle{}, Run{Text: "x "}, Run{Text: "under ", Attributes: Attributes{Underline: true}},
			Run{Text: "it", Attributes: Attributes{Underline: true, Italic: true}}, Run{Text: " "}, Run{Text: "b", Attributes: bold}))},
		{"![a *cat*](cat.png \"t\") [x](javascript:alert(1))", TextOf(para(ParagraphStyle{},
			Run{Text: string(ObjectReplacement), Attributes: Attributes{Attachment: &Attachment{Name: "a cat", URL: "cat.png"}}},
			Run{Text: " x"}))},
		{"see $$x^2$$, \\[a\\] and[^1]\n\n[^1]: The note.", TextOf(
			para(ParagraphStyle{}, Run{Text: "see $$x^2$$, [a] and[1]"}),
			para(ParagraphStyle{}, Run{Text: "[1] The note."}),
		)},
		{"", Text{}},
	} {
		if got := ParseMarkdown(c.src); dump(got) != dump(c.want) {
			t.Errorf("%q:\ngot\n%s\nwant\n%s", c.src, dump(got), dump(c.want))
		}
	}
}

// TestMarkdownRoundTrip writes random texts as Markdown and reads them
// back: what Markdown keeps comes back the same.
func TestMarkdownRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	words := []string{"foo", "bar", "a", "x1", ".", ",", "!", "?", "(", ")", "-", "#", "*", "_", "`", "~", "[", "]", "<", ">", "&", "\\", "1.", "é", "\"", "'", ":", "/"}
	pick := func() Attributes {
		var a Attributes
		if rng.IntN(3) == 0 {
			a.Weight = 700
		}
		if rng.IntN(3) == 0 {
			a.Italic = true
		}
		if rng.IntN(5) == 0 {
			a.Strikethrough = true
		}
		if rng.IntN(6) == 0 {
			a.Underline = true
		}
		if rng.IntN(6) == 0 {
			a.Link = "https://e.org/" + fmt.Sprint(rng.IntN(3))
		}
		if rng.IntN(7) == 0 {
			a = Attributes{Code: true}
		}
		return a
	}
	failures := 0
	for range 3000 {
		var runs []Run
		for range 1 + rng.IntN(5) {
			var s strings.Builder
			for k := range 1 + rng.IntN(3) {
				if k > 0 && rng.IntN(2) == 0 {
					s.WriteByte(' ')
				}
				s.WriteString(words[rng.IntN(len(words))])
			}
			runs = append(runs, Run{Text: s.String(), Attributes: pick()})
		}
		// Only what shows the same: spaces at the ends of a paragraph are
		// not shown, and a code span's are its own.
		want := TextOf(Paragraph{Runs: runs})
		want = TextOf(normalize(want)...)
		md := want.Markdown()
		if got := ParseMarkdown(md); dump(got) != dump(want) {
			failures++
			if failures <= 10 {
				t.Errorf("%q as Markdown %q reads\n%s want\n%s", want.String(), md, dump(got), dump(want))
			}
		}
	}
	if failures > 0 {
		t.Errorf("%d of 3000 failed", failures)
	}
}

// normalize returns the paragraphs of t as Markdown can keep them: no
// spaces at their ends, and none at the ends of code, which a code span
// takes as its padding.
func normalize(t Text) []Paragraph {
	var out []Paragraph
	for _, p := range t.Paragraphs() {
		var runs []Run
		for i, r := range p.Runs {
			if i == 0 {
				r.Text = strings.TrimLeft(r.Text, " ")
			}
			if i == len(p.Runs)-1 {
				r.Text = strings.TrimRight(r.Text, " ")
			}
			if r.Attributes.Code {
				r.Text = strings.Trim(r.Text, " ")
			}
			if r.Text == "" {
				continue
			}
			if n := len(runs); n > 0 && runs[n-1].Attributes == r.Attributes {
				runs[n-1].Text += r.Text
				continue
			}
			runs = append(runs, r)
		}
		p.Runs = runs
		out = append(out, p)
	}
	return out
}

// TestPasteFromMarkdownView copies a Markdown view's selection and pastes
// it into the editor with its formatting, through HTML, and pastes the
// Markdown that Markdown apps copy.
func TestPasteFromMarkdownView(t *testing.T) {
	src := "## Plan\n\nSome **bold** and `code`.\n\n- [x] done\n"
	view := ui.NewTester(func(c *ui.Context) { markdown.View(c, src, nil) }, 500, 300)
	view.Click("Plan")
	view.Key(ui.Cmd, ui.KeyA)
	view.Key(ui.Cmd, ui.KeyC)
	data := view.ClipboardData()
	for _, f := range []transfer.Format{transfer.Text, transfer.HTML, transfer.Markdown} {
		if _, err := data.Read(f); err != nil {
			t.Fatalf("no %s copied", f)
		}
	}
	want := TextOf(
		Paragraph{Style: ParagraphStyle{Heading: 2}, Runs: []Run{{Text: "Plan"}}},
		Paragraph{Runs: []Run{{Text: "Some "}, {Text: "bold", Attributes: Attributes{Weight: 700}}, {Text: " and "}, {Text: "code", Attributes: Attributes{Code: true}}, {Text: "."}}},
		Paragraph{Style: ParagraphStyle{List: Tasks, Checked: true}, Runs: []Run{{Text: "done"}}},
	)
	for _, only := range []transfer.Format{transfer.HTML, transfer.Markdown} {
		b, _ := data.Read(only)
		doc := NewDocument(Text{})
		tt, _ := richTester(t, doc)
		tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(only, b), transfer.Bytes(transfer.Text, []byte("plain")))))
		tt.Key(ui.Cmd, ui.KeyV)
		if dump(doc.Text()) != dump(want) {
			t.Errorf("pasted from %s %q:\n%s\nwant\n%s", only, b, dump(doc.Text()), dump(want))
		}
	}
}

// assertExactText checks that got and want match in invariants, plain text,
// paragraph styles, runs, and trailing end-of-paragraph attributes.
func assertExactText(t *testing.T, name string, got, want Text) {
	t.Helper()
	checkInvariants(t, got)
	if got.String() != want.String() || dump(got) != dump(want) {
		t.Fatalf("%s:\ngot:\n%s\nwant:\n%s", name, dump(got), dump(want))
	}
	ws := want.seq()
	seqEach(got.seq(), 0, func(i int, p *richPara) bool {
		q := seqAt(ws, i)
		if p.style != q.style || p.n != q.n || len(p.runs) != len(q.runs) {
			t.Fatalf("%s: para %d: got style=%+v n=%d runs=%d, want style=%+v n=%d runs=%d",
				name, i, p.style, p.n, len(p.runs), q.style, q.n, len(q.runs))
		}
		if cap(p.runs) != len(p.runs) {
			t.Fatalf("%s: para %d: cap(runs)=%d, want len=%d (must not alias slab)", name, i, cap(p.runs), len(p.runs))
		}
		for k := range p.runs {
			a, b := p.runs[k], q.runs[k]
			aAtt, bAtt := a.a.Attachment, b.a.Attachment
			a.a.Attachment, b.a.Attachment = nil, nil
			if a != b || (aAtt == nil) != (bAtt == nil) || (aAtt != nil && (aAtt.Name != bAtt.Name || aAtt.URL != bAtt.URL)) {
				t.Fatalf("%s: para %d run %d: got %+v att=%+v, want %+v att=%+v",
					name, i, k, a, aAtt, b, bAtt)
			}
		}
		return true
	})
}

func TestParseMarkdownEdgeCases(t *testing.T) {
	para := func(st ParagraphStyle, runs ...Run) Paragraph { return Paragraph{Style: st, Runs: runs} }
	bold := Attributes{Weight: 700}
	italic := Attributes{Italic: true}
	code := Attributes{Code: true}

	cases := []struct {
		name string
		src  string
		want Text
	}{
		{
			name: "nested_quotes_and_list_hard_break",
			src:  "> l1\n>\n> > - **first**  \n> >   second\n> >\n> > > l3",
			want: TextOf(
				para(ParagraphStyle{Quote: true}, Run{Text: "l1"}),
				para(ParagraphStyle{List: Bullets, Quote: true, QuoteDepth: 2}, Run{Text: "first", Attributes: bold}),
				para(ParagraphStyle{Indent: 1, Quote: true, QuoteDepth: 2}, Run{Text: "second"}),
				para(ParagraphStyle{Quote: true, QuoteDepth: 3}, Run{Text: "l3"}),
			),
		},
		{
			name: "empty_and_multiline_code_and_list_start",
			src:  "```\n```\n\n- ```go\n  a\n\n  b\n  ```\n- ",
			want: TextOf(
				para(ParagraphStyle{CodeBlock: true}),
				para(ParagraphStyle{List: Bullets}),
				para(ParagraphStyle{Indent: 1, CodeBlock: true}, Run{Text: "a"}),
				para(ParagraphStyle{Indent: 1, CodeBlock: true}),
				para(ParagraphStyle{Indent: 1, CodeBlock: true}, Run{Text: "b"}),
				para(ParagraphStyle{List: Bullets}),
			),
		},
		{
			name: "hard_breaks_and_trailing_marks",
			src:  "**a<br>b** and `c`<br>**d**<br>",
			want: TextOf(
				para(ParagraphStyle{}, Run{Text: "a", Attributes: bold}),
				para(ParagraphStyle{}, Run{Text: "b", Attributes: bold}, Run{Text: " and "}, Run{Text: "c", Attributes: code}),
				para(ParagraphStyle{}, Run{Text: "d", Attributes: bold}),
				para(ParagraphStyle{}),
			),
		},
		{
			name: "table_breaks_in_header_and_cells",
			src:  "| **a<br>b** | c |\n|---|---|\n| **x**<br> | *y* |\n| | |",
			want: TextOf(
				para(ParagraphStyle{}, Run{Text: "a\nb\tc", Attributes: bold}),
				para(ParagraphStyle{}, Run{Text: "x", Attributes: bold}, Run{Text: "\n\t"}, Run{Text: "y", Attributes: italic}),
				para(ParagraphStyle{}, Run{Text: "\t"}),
			),
		},
		{
			name: "html_block_in_quote",
			src:  "> <blockquote><p>inner <b>bold</b></p></blockquote>",
			want: TextOf(
				para(ParagraphStyle{Quote: true, QuoteDepth: 2}, Run{Text: "inner "}, Run{Text: "bold", Attributes: bold}),
			),
		},
		{
			name: "footnotes_code_br_and_empty",
			src:  "See[^a][^b][^empty].\n\n[^a]: first<br>second\n\n[^b]:\n    ```\n    ```\n\n[^empty]:\n",
			want: TextOf(
				para(ParagraphStyle{}, Run{Text: "See[a][b][empty]."}),
				para(ParagraphStyle{}, Run{Text: "[a] first"}),
				para(ParagraphStyle{}, Run{Text: "second"}),
				para(ParagraphStyle{CodeBlock: true}, Run{Text: "[b] "}),
			),
		},
		{
			name: "nested_footnote_definitions",
			src:  "x[^a]\n\n[^a]:[^b]: inner\n\n[^c]: [^d]:\n\n    after",
			want: TextOf(
				para(ParagraphStyle{}, Run{Text: "x[a]"}),
				para(ParagraphStyle{}, Run{Text: "[a] [b] inner"}),
				para(ParagraphStyle{}, Run{Text: "[c] after"}),
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertExactText(t, tc.name, ParseMarkdown(tc.src), tc.want)
		})
	}
}

func TestParseMarkdownSlabInvariants(t *testing.T) {
	t.Run("many_blocks_across_slabs", func(t *testing.T) {
		got := ParseMarkdown(benchMarkdown(500))
		checkInvariants(t, got)
		seqEach(got.seq(), 0, func(i int, p *richPara) bool {
			if cap(p.runs) != len(p.runs) {
				t.Fatalf("para %d: cap(runs)=%d, want len=%d", i, cap(p.runs), len(p.runs))
			}
			return true
		})
	})

	t.Run("single_para_exceeding_slab", func(t *testing.T) {
		// More than slabRuns alternating runs in one paragraph forces room()
		// to move the open paragraph's runs to a larger slab.
		const repeats = slabRuns/2 + 200
		src := strings.Repeat("a **b** ", repeats)
		got := ParseMarkdown(src)
		checkInvariants(t, got)
		if got.ParagraphCount() != 1 {
			t.Fatalf("ParagraphCount=%d, want 1", got.ParagraphCount())
		}
		p := seqAt(got.seq(), 0)
		if cap(p.runs) != len(p.runs) {
			t.Fatalf("cap(runs)=%d, want len=%d", cap(p.runs), len(p.runs))
		}
		wantRuns := make([]Run, 0, 2*repeats)
		bold := Attributes{Weight: 700}
		for i := range repeats {
			if i == 0 {
				wantRuns = append(wantRuns, Run{Text: "a "})
			} else {
				wantRuns = append(wantRuns, Run{Text: " a "})
			}
			wantRuns = append(wantRuns, Run{Text: "b", Attributes: bold})
		}
		// Trailing space in the last repetition is stripped by Markdown parsing.
		want := TextOf(Paragraph{Runs: wantRuns})
		assertExactText(t, "single_para_exceeding_slab", got, want)
	})
}

// TestParseMarkdownInvalidUTF8 checks that the halves of a broken UTF-8
// sequence, apart in the source, don't meet in the text as one rune that
// the runs count as two.
func TestParseMarkdownInvalidUTF8(t *testing.T) {
	for _, s := range []string{" ___\xcf_\x8c_", "**\xcf**\x8c", "a\xcf*\x8c*", "\xff`\xfe`"} {
		got := ParseMarkdown(s)
		checkInvariants(t, got)
		if !utf8.ValidString(got.String()) {
			t.Fatalf("%q: text %q is not valid UTF-8", s, got.String())
		}
	}
}

// A code span keeps its spaces, one of spaces alone too, and does not turn
// the marks beside it into HTML.
func TestMarkdownBlankCodeSpan(t *testing.T) {
	for _, c := range []struct {
		runs []Run
		md   string
	}{
		{[]Run{{Text: " ", Attributes: Attributes{Code: true}}, {Text: "0", Attributes: Attributes{Italic: true}}}, "` `*0*"},
		{[]Run{{Text: "a "}, {Text: "  ", Attributes: Attributes{Code: true}}, {Text: " b"}}, "a `  ` b"},
		{[]Run{{Text: " ", Attributes: Attributes{Code: true}}}, "` `"},
		// and its spaces at its ends stay in it
		{[]Run{{Text: "0", Attributes: Attributes{Strikethrough: true}}, {Text: " 0", Attributes: Attributes{Code: true}}}, "~~0~~` 0`"},
		{[]Run{{Text: "a"}, {Text: " b ", Attributes: Attributes{Code: true}}}, "a`  b  `"},
	} {
		want := TextOf(Paragraph{Runs: c.runs})
		md := want.Markdown()
		if md != c.md {
			t.Errorf("%s as Markdown is %q, want %q", dump(want), md, c.md)
		}
		if got := ParseMarkdown(md); dump(got) != dump(want) {
			t.Errorf("%q reads\n%s want\n%s", md, dump(got), dump(want))
		}
	}
}

// A heading of #s alone keeps them all: the first one, escaped so that it
// does not open a heading, is not escaped again as a closing #.
func TestMarkdownHeadingOfHashes(t *testing.T) {
	for _, c := range []struct{ text, md string }{
		{"#", `## \#`},
		{"##", `## \##`},
		{"C#", `## C\#`},
		{`a \#`, `## a \\\#`},
	} {
		want := TextOf(Paragraph{Style: ParagraphStyle{Heading: 2}, Runs: []Run{{Text: c.text}}})
		md := want.Markdown()
		if md != c.md {
			t.Errorf("heading %q as Markdown is %q, want %q", c.text, md, c.md)
		}
		if got := ParseMarkdown(md); dump(got) != dump(want) {
			t.Errorf("%q reads\n%s want\n%s", md, dump(got), dump(want))
		}
	}
}

// TestMarkdownEmptyTask reads back the empty items of a checklist that
// Markdown writes: GitHub asks for text after [ ], but an item just added
// has none.
func TestMarkdownEmptyTask(t *testing.T) {
	task := func(checked bool, indent int, text string) Paragraph {
		p := Paragraph{Style: ParagraphStyle{List: Tasks, Checked: checked, Indent: indent}}
		if text != "" {
			p.Runs = []Run{{Text: text}}
		}
		return p
	}
	for _, want := range []Text{
		TextOf(task(false, 0, "")),
		TextOf(task(true, 0, "")),
		TextOf(task(false, 0, "milk"), task(false, 0, "")),
		TextOf(task(false, 0, ""), task(true, 0, "eggs")),
		TextOf(task(false, 0, "milk"), task(false, 1, ""), task(false, 0, "eggs")),
		TextOf(Paragraph{Style: ParagraphStyle{List: Tasks, Quote: true}}),
		// and a bullet whose text is [ ] stays one
		TextOf(Paragraph{Style: ParagraphStyle{List: Bullets}, Runs: []Run{{Text: "[ ]"}}}),
	} {
		md := want.Markdown()
		if got := ParseMarkdown(md); dump(got) != dump(want) {
			t.Errorf("%q reads\n%s want\n%s", md, dump(got), dump(want))
		}
	}
}
