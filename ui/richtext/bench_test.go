package richtext

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// longText returns n paragraphs of formatted text: a heading every
// fiftieth, items of lists, and runs of bold, italic and links.
func longText(n int) Text {
	paras := make([]Paragraph, n)
	for i := range paras {
		st := ParagraphStyle{}
		switch {
		case i%50 == 0:
			st.Heading = 2
		case i%7 == 3:
			st.List = Bullets
		}
		paras[i] = Paragraph{Style: st, Runs: []Run{
			{Text: fmt.Sprintf("Paragraph %d has ", i)},
			{Text: "bold words", Attributes: Attributes{Weight: 700}},
			{Text: ", an "},
			{Text: "italic one", Attributes: Attributes{Italic: true}},
			{Text: " and "},
			{Text: "a link", Attributes: Attributes{Link: "https://example.com"}},
			{Text: " in a line long enough to wrap in a narrow window."},
		}}
	}
	return TextOf(paras...)
}

// benchParagraphs are the sizes of the documents of the benchmarks.
var benchParagraphs = []int{1000, 10000, 100000}

// BenchmarkTypeFrame types a character in the middle of a document and
// draws the frame that shows it, as typing does.
func BenchmarkTypeFrame(b *testing.B) {
	for _, n := range benchParagraphs {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			doc := NewDocument(longText(n))
			tt := ui.NewTester(func(c *ui.Context) { Editor(c, doc).Fill() }, 1000, 800)
			tt.Press(500, 400)
			tt.Release(500, 400)
			mid := doc.Text().Len() / 2
			doc.SetSelection(mid, mid)
			tt.Frame()
			b.ReportAllocs()
			for b.Loop() {
				tt.Type("x")
			}
		})
	}
}

// BenchmarkReplace replaces a rune in the middle of a document's text,
// the edit a keystroke makes, without drawing.
func BenchmarkReplace(b *testing.B) {
	for _, n := range benchParagraphs {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			t := longText(n)
			mid := t.Len() / 2
			b.ReportAllocs()
			for b.Loop() {
				t = t.ReplaceString(mid, mid+1, "y", Attributes{Weight: 700})
			}
		})
	}
}

// BenchmarkFormat makes a thousand runes bold in the middle of a document.
func BenchmarkFormat(b *testing.B) {
	for _, n := range benchParagraphs {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			t := longText(n)
			mid := t.Len() / 2
			on := false
			b.ReportAllocs()
			for b.Loop() {
				on = !on
				t = t.Format(mid, mid+1000, func(a *Attributes) { a.Italic = on })
			}
		})
	}
}

// BenchmarkScrollFrame scrolls a long document by a wheel's step a frame.
func BenchmarkScrollFrame(b *testing.B) {
	doc := NewDocument(longText(100000))
	tt := ui.NewTester(func(c *ui.Context) { Editor(c, doc).Fill() }, 1000, 800)
	tt.Frame()
	b.ReportAllocs()
	for b.Loop() {
		tt.Scroll(500, 400, 0, 120)
	}
}

// shortParagraphs returns n short paragraphs, as a note's lines or a
// list's items are, many to a chunk of the text: their words run plain,
// bold and plain, with a multi-byte rune in each.
func shortParagraphs(n int) Text {
	paras := make([]Paragraph, n)
	for i := range paras {
		paras[i] = Paragraph{Runs: []Run{
			{Text: fmt.Sprintf("Line %d — ", i)},
			{Text: "bold", Attributes: Attributes{Weight: 700}},
			{Text: "."},
		}}
	}
	return TextOf(paras...)
}

// benchShort are the sizes of the documents of short paragraphs.
var benchShort = []int{100, 300, 1000, 3000}

// BenchmarkParagraphs walks the paragraphs of a document of short ones,
// as saving or searching it does.
func BenchmarkParagraphs(b *testing.B) {
	for _, n := range benchShort {
		t := shortParagraphs(n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for range t.Paragraphs() {
				}
			}
		})
	}
}

// BenchmarkRuns walks the runs of the whole of a document of short
// paragraphs, as copying it does.
func BenchmarkRuns(b *testing.B) {
	for _, n := range benchShort {
		t := shortParagraphs(n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for range t.Runs(0, t.Len()) {
				}
			}
		})
	}
}

// readsAsRuns returns a paragraph with every mark Markdown writes.
func readsAsRuns() []Run {
	bold := Attributes{Weight: 700}
	italic := Attributes{Italic: true}
	underlined := Attributes{Underline: true}
	strike := Attributes{Strikethrough: true}
	code := Attributes{Code: true}
	linked := Attributes{Link: "https://example.com"}
	attachment := &Attachment{Name: "a cat", URL: "cat.png"}
	var runs []Run
	for repeat := 0; repeat < 32; repeat++ {
		runs = append(runs,
			Run{Text: "a é "},
			Run{Text: "bold", Attributes: bold},
			Run{Text: " "},
			Run{Text: "italic", Attributes: italic},
			Run{Text: " "},
			Run{Text: "under", Attributes: underlined},
			Run{Text: " "},
			Run{Text: "strike", Attributes: strike},
			Run{Text: " "},
			Run{Text: "code", Attributes: code},
			Run{Text: " "},
			Run{Text: "link", Attributes: linked},
			Run{Text: " "},
			Run{Text: string(ObjectReplacement), Attributes: Attributes{Attachment: attachment}},
			Run{Text: " "},
		)
	}
	runs = append(runs, Run{Text: "  "})
	return runs
}

// BenchmarkReadsAs measures reading a written paragraph back, which
// compares every rune and its marks.
func BenchmarkReadsAs(bench *testing.B) {
	runs := readsAsRuns()
	var builder strings.Builder
	writeMarkdownRuns(&builder, runs)
	markdown := builder.String()
	if !readsAs(markdown, runs) {
		bench.Fatal("Markdown does not read back")
	}
	bench.ReportAllocs()
	for bench.Loop() {
		if !readsAs(markdown, runs) {
			bench.Fatal("Markdown does not read back")
		}
	}
}

// benchMarkdown returns a Markdown document of roughly n blocks: headings,
// formatted paragraphs, lists, nested quotes, code blocks and tables.
func benchMarkdown(n int) string {
	var b strings.Builder
	for i := range n {
		switch i % 10 {
		case 0:
			fmt.Fprintf(&b, "## Section %d\n\n", i)
		case 3:
			fmt.Fprintf(&b, "- Item %d with **bold**, *italic*, `code` and [link](https://example.com/%d)\n\n", i, i)
		case 5:
			fmt.Fprintf(&b, "> > Quoted paragraph %d with ~~strike~~ and <u>underline</u>.\n\n", i)
		case 7:
			fmt.Fprintf(&b, "```go\nfunc f%d() int { return %d }\n```\n\n", i, i)
		case 9:
			fmt.Fprintf(&b, "| Col A | Col B |\n|---|---|\n| %d | **val %d** |\n\n", i, i)
		default:
			fmt.Fprintf(&b, "Paragraph %d has **bold words**, an *italic one*, `inline code` and [a link](https://example.com/%d) in a line long enough to wrap.\n\n", i, i)
		}
	}
	return b.String()
}

// BenchmarkParseMarkdown parses Markdown documents of varying sizes into Text.
func BenchmarkParseMarkdown(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		src := benchMarkdown(n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for b.Loop() {
				if ParseMarkdown(src).Len() == 0 {
					b.Fatal("empty")
				}
			}
		})
	}
}
