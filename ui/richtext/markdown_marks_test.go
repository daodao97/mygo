package richtext

import (
	"testing"
)

func TestSameMarkRuns(test *testing.T) {
	bold := Attributes{Weight: 700}
	underlined := Attributes{Underline: true}
	strike := Attributes{Strikethrough: true}
	code := Attributes{Code: true}
	linked := Attributes{Link: "https://example.com"}
	attachmentAttributes := func(name, url string) Attributes {
		return Attributes{Attachment: &Attachment{Name: name, URL: url}}
	}
	tests := []struct {
		name        string
		left, right []Run
		want        bool
	}{
		{"empty", nil, nil, true},
		{"empty runs", []Run{{}, {Text: "  "}}, nil, true},
		{"trailing spaces", []Run{{Text: "a"}, {Text: "  "}}, []Run{{Text: "a"}}, true},
		{"trailing spaces across runs", []Run{{Text: "a"}, {Text: " "}, {Text: "b  "}}, []Run{{Text: "a b"}}, true},
		{"trailing spaces in code", []Run{{Text: "a"}, {Text: "  ", Attributes: code}}, []Run{{Text: "a"}}, false},
		{"trailing spaces after code", []Run{{Text: "a"}, {Text: "  ", Attributes: code}, {Text: "  "}}, []Run{{Text: "a"}, {Text: "  ", Attributes: code}}, true},
		{"code run after trailing spaces", []Run{{Text: "a  "}, {Text: " ", Attributes: code}}, []Run{{Text: "a  "}, {Text: " ", Attributes: code}}, true},
		{"code run after stripped spaces", []Run{{Text: "a  "}, {Text: " ", Attributes: code}}, []Run{{Text: "a"}, {Text: " ", Attributes: code}}, false},
		{"empty code run between trailing spaces", []Run{{Text: "a  "}, {Text: "", Attributes: code}, {Text: "  "}}, []Run{{Text: "a"}}, true},
		{"trailing tab", []Run{{Text: "a"}, {Text: " \t "}}, []Run{{Text: "a"}, {Text: " \t"}}, true},
		{"trailing tab with marks", []Run{{Text: "a"}, {Text: " \t ", Attributes: bold}}, []Run{{Text: "a"}, {Text: " \t", Attributes: bold}}, true},
		{"whitespace marks ignored", []Run{{Text: "a"}, {Text: " ", Attributes: bold}, {Text: "b"}}, []Run{{Text: "a b"}}, true},
		{"code whitespace marks kept", []Run{{Text: "a"}, {Text: " ", Attributes: code}, {Text: "b"}}, []Run{{Text: "a b"}}, false},
		{"attachment name", []Run{{Text: string(ObjectReplacement), Attributes: attachmentAttributes("cat", "cat.png")}}, []Run{{Text: string(ObjectReplacement), Attributes: attachmentAttributes("dog", "cat.png")}}, false},
		{"attachment URL", []Run{{Text: string(ObjectReplacement), Attributes: attachmentAttributes("cat", "cat.png")}}, []Run{{Text: string(ObjectReplacement), Attributes: attachmentAttributes("cat", "dog.png")}}, false},
		{"attachment equal", []Run{{Text: string(ObjectReplacement), Attributes: attachmentAttributes("cat", "cat.png")}}, []Run{{Text: string(ObjectReplacement), Attributes: attachmentAttributes("cat", "cat.png")}}, true},
		{"run boundary", []Run{{Text: "a", Attributes: bold}, {Text: "b", Attributes: bold}}, []Run{{Text: "ab", Attributes: bold}}, true},
		{"mark changes at run boundary", []Run{{Text: "a", Attributes: bold}, {Text: "b"}}, []Run{{Text: "ab", Attributes: bold}}, false},
		{"split rune", []Run{{Text: "\xc3"}, {Text: "\xa9"}}, []Run{{Text: "é"}}, false},
		{"replacement rune encodings", []Run{{Text: "\ufffd"}}, []Run{{Text: "\xff"}}, true},
		{"rune mismatch", []Run{{Text: "a"}}, []Run{{Text: "b"}}, false},
		{"length mismatch", []Run{{Text: "a"}}, []Run{{Text: "a"}, {Text: "b"}}, false},
		{"mark mismatch", []Run{{Text: "a", Attributes: bold}}, []Run{{Text: "a"}}, false},
		{"link mismatch", []Run{{Text: "a", Attributes: linked}}, []Run{{Text: "a", Attributes: Attributes{Link: "https://other.example"}}}, false},
		{"code mismatch", []Run{{Text: "a", Attributes: code}}, []Run{{Text: "a"}}, false},
		{"all marks", []Run{
			{Text: "a ", Attributes: underlined},
			{Text: "bold", Attributes: Attributes{Weight: 700, Italic: true, Underline: true, Strikethrough: true, Link: "https://example.com"}},
			{Text: " "},
			{Text: "code", Attributes: code},
			{Text: " "},
			{Text: string(ObjectReplacement), Attributes: attachmentAttributes("cat", "cat.png")},
		}, []Run{
			{Text: "a ", Attributes: strike},
			{Text: "bold", Attributes: Attributes{Weight: 700, Italic: true, Underline: true, Strikethrough: true, Link: "https://example.com"}},
			{Text: " "},
			{Text: "code", Attributes: code},
			{Text: " "},
			{Text: string(ObjectReplacement), Attributes: attachmentAttributes("cat", "cat.png")},
		}, false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if got := sameMarkRuns(testCase.left, testCase.right); got != testCase.want {
				test.Errorf("sameMarkRuns = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestMarkdownReadsAs(test *testing.T) {
	bold := Attributes{Weight: 700}
	tests := []struct {
		name   string
		source string
		runs   []Run
		want   bool
	}{
		{"bold", "**a**", []Run{{Text: "a", Attributes: bold}}, true},
		{"bold with trailing spaces", "**a**  ", []Run{{Text: "a", Attributes: bold}}, true},
		{"plain", "**a**", []Run{{Text: "a"}}, false},
		{"multiple paragraphs", "a\n\nb", []Run{{Text: "a"}, {Text: "b"}}, false},
		{"empty", "", nil, true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if got := readsAs(testCase.source, testCase.runs); got != testCase.want {
				test.Errorf("readsAs = %v, want %v", got, testCase.want)
			}
		})
	}
}

// Tabs at the end of a paragraph, which Markdown drops as it drops spaces,
// leave the marks before them in Markdown, not HTML.
func TestMarkdownTrailingTabs(test *testing.T) {
	for _, testCase := range []struct {
		runs []Run
		md   string
	}{
		{[]Run{{Text: "a", Attributes: Attributes{Italic: true}}, {Text: " \t"}}, "*a* \t"},
		{[]Run{{Text: "a\t", Attributes: Attributes{Strikethrough: true}}}, "~~a~~\t"},
		{[]Run{{Text: "b", Attributes: Attributes{Weight: 700}}, {Text: "\t\t"}}, "**b**\t\t"},
	} {
		text := TextOf(Paragraph{Runs: testCase.runs})
		if md := text.Markdown(); md != testCase.md {
			test.Errorf("%s as Markdown is %q, want %q", dump(text), md, testCase.md)
		}
	}
}

func TestSameMarkRunsAllocs(test *testing.T) {
	left := readsAsRuns()
	right := readsAsRuns()
	var equal bool
	allocs := testing.AllocsPerRun(100, func() {
		equal = sameMarkRuns(left, right)
	})
	if !equal {
		test.Fatal("runs with the same marks compare unequal")
	}
	if allocs != 0 {
		test.Errorf("got %v allocations, want 0", allocs)
	}
}

func BenchmarkSameMarkRuns(bench *testing.B) {
	left := readsAsRuns()
	right := readsAsRuns()
	if !sameMarkRuns(left, right) {
		bench.Fatal("runs with the same marks compare unequal")
	}
	bench.ReportAllocs()
	for bench.Loop() {
		if !sameMarkRuns(left, right) {
			bench.Fatal("runs with the same marks compare unequal")
		}
	}
}
