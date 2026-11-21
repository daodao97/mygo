package editor

import (
	"fmt"
	"math/rand/v2"
	"path"
	"strings"
	"testing"
	"time"
)

var tokenNames = [...]string{"text", "kw", "type", "fn", "var", "prop", "const", "num", "str", "esc", "re", "comment", "op", "punct",
	"tag", "attr", "meta", "heading", "em", "strong", "link", "code", "ins", "del", "invalid"}

// lexLines tokenizes lines and writes each token as kind(text), leaving
// out spaces and plain variables.
func lexLines(t Tokenizer, lines ...string) string {
	var b strings.Builder
	var state uint64
	for i, line := range lines {
		if i > 0 {
			b.WriteString(" / ")
		}
		var toks []Token
		toks, state = t.Tokenize(line, state, nil)
		from := 0
		for _, tk := range toks {
			if tk.End <= from || tk.End > len(line) {
				panic(fmt.Sprintf("token %+v after %d in %q", tk, from, line))
			}
			s := line[from:tk.End]
			from = tk.End
			if strings.TrimSpace(s) == "" || tk.Kind == TokenText && len(strings.Fields(s)) == 0 {
				continue
			}
			fmt.Fprintf(&b, "%s(%s)", tokenNames[tk.Kind], strings.TrimSpace(s))
		}
		if from != len(line) {
			panic(fmt.Sprintf("tokens end at %d of %q", from, line))
		}
	}
	return b.String()
}

func TestTokenizers(t *testing.T) {
	for _, c := range []struct {
		lang  string
		lines []string
		want  string
	}{
		{"go", []string{`func main() { fmt.Println("hi\n", 42) } // done`},
			`kw(func)fn(main)punct(())punct({)var(fmt)punct(.)fn(Println)punct(()str("hi)esc(\n)str(")punct(,)num(42)punct())punct(})comment(// done)`},
		{"go", []string{"x := `raw", "still` + 1 /* a", "b */"},
			"var(x)op(:=)str(`raw) / str(still`)op(+)num(1)comment(/* a) / comment(b */)"},
		{"ts", []string{"const s = `a${b + `c${d}`}e`"},
			"kw(const)var(s)op(=)str(`a)esc(${)var(b)op(+)str(`c)esc(${)var(d)esc(})str(`)esc(})str(e`)"},
		{"js", []string{"x = /a[/]b/g.test(y)"},
			"var(x)op(=)re(/a[/]b/g)punct(.)fn(test)punct(()var(y)punct())"},
		{"python", []string{`@app.route("/")`, `def f(x): return f"{x!r}" + r'\d'  # c`},
			`meta(@app.route)punct(()str("/")punct()) / kw(def)fn(f)punct(()var(x)punct())op(:)kw(return)str(f")esc({)var(x)op(!)var(r)esc(})str(")op(+)str(r'\d')comment(# c)`},
		// Each of Python's f-strings, the single-quoted ones last of all.
		{"python", []string{`x = f'{a}' + F'{a}{b}' + rf'{c}' + Fr"""{d}"""`},
			`var(x)op(=)str(f')esc({)var(a)esc(})str(')op(+)str(F')esc({)var(a)esc(}{)var(b)esc(})str(')op(+)str(rf')esc({)var(c)esc(})str(')op(+)str(Fr""")esc({)var(d)esc(})str(""")`},
		{"json", []string{`{"a": [1, true, "b"]}`},
			`punct({)prop("a")op(:)punct([)num(1)punct(,)const(true)punct(,)str("b")punct(]})`},
		{"json", []string{`{"a\nb" : "c\nd", "e":{"f":1}}`},
			`punct({)prop("a\nb")op(:)str("c)esc(\n)str(d")punct(,)prop("e")op(:)punct({)prop("f")op(:)num(1)punct(}})`},
		{"rust", []string{"/* a /* b */ c */ fn x() {}"},
			"comment(/* a /* b */ c */)kw(fn)fn(x)punct(())punct({})"},
		{"c", []string{"#include <stdio.h>", "int x = 0x1fu;"},
			"meta(#include)op(<)var(stdio)punct(.)prop(h)op(>) / type(int)var(x)op(=)num(0x1fu)punct(;)"},
		{"css", []string{".a:hover, #b { color: red; margin: -4px; }"},
			"type(.a)kw(:hover)punct(,)type(#b)punct({)prop(color)punct(:)const(red)punct(;)prop(margin)punct(:)num(-4px)punct(;)punct(})"},
		{"html", []string{`<div class="x">a &amp; b<script>let y = 1</script>`},
			`punct(<)tag(div)attr(class)op(=)str("x")punct(>)text(a)esc(&amp;)text(b)punct(<)tag(script)punct(>)kw(let)var(y)op(=)num(1)punct(</)tag(script)punct(>)`},
		// ERB's Ruby in tags, in attribute values and across lines, its
		// comments, and the HTML around them.
		{"erb", []string{`<p class="<%= cls %>"><% if x -%>`, `<%# a`, `b %><%= link_to "a",`, `  root_path %></p>`},
			`punct(<)tag(p)attr(class)op(=)str(")meta(<%=)var(cls)meta(%>)str(")punct(>)meta(<%)kw(if)var(x)meta(-%>) / comment(<%# a) / ` +
				`comment(b %>)meta(<%=)var(link_to)str("a")punct(,) / var(root_path)meta(%>)punct(</)tag(p)punct(>)`},
		// ERB's <%% is a literal <%, opening no tag.
		{"erb", []string{`<p><%% x %></p><%= y %>`},
			`punct(<)tag(p)punct(>)text(<%% x %>)punct(</)tag(p)punct(>)meta(<%=)var(y)meta(%>)`},
		// Jinja's expressions, statements with whitespace control,
		// comments over lines, and the plain text around them.
		{"jinja", []string{`ExecStart={{ script | default('/bin/x') }}`, `{%- for r in rules if r.port > 0 +%}`, `{# a`, `b #}x{{ none }}`},
			`text(ExecStart=)meta({{)var(script)op(|)fn(default)punct(()str('/bin/x')punct())meta(}}) / ` +
				`meta({%-)kw(for)var(r)kw(in)var(rules)kw(if)var(r)punct(.)prop(port)op(>)num(0)meta(+%}) / comment({# a) / comment(b #})text(x)meta({{)const(none)meta(}})`},
		// Svelte's expressions in text, tags and values, its blocks, and a
		// TypeScript script.
		{"svelte", []string{`<p class="a {b}" {c}>{#if x > 1}{y}{/if}</p>`},
			`punct(<)tag(p)attr(class)op(=)str("a)punct({)var(b)punct(})str(")punct({)var(c)punct(}>{)kw(#if)var(x)op(>)num(1)punct(}{)var(y)punct(}{)kw(/if)punct(}</)tag(p)punct(>)`},
		{"svelte", []string{`{fn({a: 1,`, `b})}<i>`, `<script lang="ts">let x: number</script>`},
			`punct({)fn(fn)punct(({)var(a)op(:)num(1)punct(,) / var(b)punct(})}<)tag(i)punct(>) / ` +
				`punct(<)tag(script)attr(lang)op(=)str("ts")punct(>)kw(let)var(x)op(:)type(number)punct(</)tag(script)punct(>)`},
		// Astro's frontmatter in TypeScript after blank lines, its template's
		// expressions, a --- there that is text, and a TypeScript script.
		{"astro", []string{``, `---`, `const { a } = Astro.props as Props`, `---`, `<p class={a}>{a}</p>`, `---`, `<script>let x: number</script>`, `<style>`, `</style>`},
			` / punct(---) / kw(const)punct({)var(a)punct(})op(=)type(Astro)punct(.)prop(props)kw(as)type(Props) / punct(---) / ` +
				`punct(<)tag(p)attr(class)op(=)punct({)var(a)punct(}>{)var(a)punct(}</)tag(p)punct(>) / text(---) / ` +
				`punct(<)tag(script)punct(>)kw(let)var(x)op(:)type(number)punct(</)tag(script)punct(>) / punct(<)tag(style)punct(>) / punct(</)tag(style)punct(>)`},
		// Without frontmatter, the template starts at once.
		{"astro", []string{`<h1>{title}</h1>`, `---`},
			`punct(<)tag(h1)punct(>{)var(title)punct(}</)tag(h1)punct(>) / text(---)`},
		// HCL's blocks, attributes, interpolations, comments and heredocs.
		{"hcl", []string{`resource "aws_s3_bucket" "b" {`, `  count = var.n > 0 ? 1 : 0 # c`, `  tags = { Name = "x-${var.env}" } // d`,
			`  policy = <<-EOT`, `    {"a": "${b}"}`, `  EOT`, `  type = list(string) /* e */`, `}`},
			`kw(resource)str("aws_s3_bucket")str("b")punct({) / prop(count)op(=)var(var)punct(.)prop(n)op(>)num(0)op(?)num(1)op(:)num(0)comment(# c) / ` +
				`prop(tags)op(=)punct({)prop(Name)op(=)str("x-)esc(${)var(var)punct(.)prop(env)esc(})str(")punct(})comment(// d) / ` +
				`prop(policy)op(=)str(<<-EOT) / str({"a": "${b}"}) / str(EOT) / prop(type)op(=)type(list)punct(()type(string)punct())comment(/* e */) / punct(})`},
		// A block word before a dot names a value, like var.
		{"hcl", []string{`data "z" "a" {`, `  id = data.z.a.id`, `  x = module.m.out`, `}`},
			`kw(data)str("z")str("a")punct({) / prop(id)op(=)var(data)punct(.)prop(z)punct(.)prop(a)punct(.)prop(id) / ` +
				`prop(x)op(=)var(module)punct(.)prop(m)punct(.)prop(out) / punct(})`},
		// A heredoc's opener in a string or comment opens none.
		{"hcl", []string{`a = "<<EOT"`, `b = 1 # <<EOT`, `c = 2`},
			`prop(a)op(=)str("<<EOT") / prop(b)op(=)num(1)comment(# <<EOT) / prop(c)op(=)num(2)`},
		{"markdown", []string{"# Title", "Some *em* and **strong** `code` [link](url)", "```go", "func x() {}", "```"},
			"heading(# Title) / text(Some)em(*em*)text(and)strong(**strong**)code(`code`)link([link])str((url)) / punct(```)type(go) / kw(func)fn(x)punct(())punct({}) / punct(```)"},
		{"diff", []string{"@@ -1,2 +1,2 @@ func x", "-a", "+b", " c"},
			"kw(@@ -1,2 +1,2 @@)fn(func x) / del(-a) / ins(+b) / text(c)"},
		{"yaml", []string{"key: 'v' # c", "- true"},
			"prop(key)op(:)str('v')comment(# c) / op(-)const(true)"},
		{"toml", []string{"[server]", "port = 8080"},
			"type([server]) / prop(port)op(=)num(8080)"},
		{"sh", []string{`echo "$HOME/x" $1 # c`},
			`fn(echo)str(")var($HOME)str(/x")var($1)comment(# c)`},
		{"sql", []string{"SELECT id FROM users WHERE x = 'a'"},
			"kw(SELECT)var(id)kw(FROM)var(users)kw(WHERE)var(x)op(=)str('a')"},
		{"dart", []string{`final s = r'\d' + "a${b}c$d"; // c`, "/* a /* b */ c */ var t = '''x", "y''';"},
			`kw(final)var(s)op(=)str(r'\d')op(+)str("a)esc(${)var(b)esc(})str(c)var($d)str(")punct(;)comment(// c) / ` +
				"comment(/* a /* b */ c */)kw(var)var(t)op(=)str('''x) / str(y''')punct(;)"},
		{"scala", []string{`val s = s"Hi $name ${a + 1}" + raw"\d" // c`, `/* a /* b */ c */ def f[T](x: T): Unit = """multi`, `line"""`},
			`kw(val)var(s)op(=)str(s"Hi)var($name)esc(${)var(a)op(+)num(1)esc(})str(")op(+)str(raw"\d")comment(// c) / ` +
				`comment(/* a /* b */ c */)kw(def)var(f)punct([)type(T)punct(]()var(x)op(:)type(T)punct())op(:)type(Unit)op(=)str("""multi) / str(line""")`},
		{"r", []string{`x <- c(1L, NA) %in% r"(a\b)" # c`, "if (TRUE) df$col.name |> is.na()"},
			`var(x)op(<-)fn(c)punct(()num(1L)punct(,)const(NA)punct())op(%in%)str(r"(a\b)")comment(# c) / ` +
				"kw(if)punct(()const(TRUE)punct())var(df)op($)var(col.name)op(|>)fn(is.na)punct(())"},
		{"graphql", []string{`query Get($id: ID!) @cached { user(id: $id) { ...F ... on User { name } } } # c`, `"""Doc`, `"""`},
			`kw(query)type(Get)punct(()var($id)op(:)type(ID)op(!)punct())meta(@cached)punct({)var(user)punct(()prop(id)op(:)var($id)punct())punct({)` +
				`punct(...)type(F)punct(...)kw(on)type(User)punct({)var(name)punct(})punct(})punct(})comment(# c) / str("""Doc) / str(""")`},
		{"nix", []string{`{ pkgs ? import <nixpkgs> {} }: let s = "a${pkgs.hello-world}\n"; # c`, `in rec { x = '' echo ${s}`, `''; y = s // { z = true; }; /* d */ }`, `inputs.x."y".${s} = a.b == c;`},
			`punct({)var(pkgs)op(?)fn(import)op(<)var(nixpkgs)op(>)punct({})punct(})op(:)kw(let)prop(s)op(=)str("a)esc(${)var(pkgs)punct(.)prop(hello-world)esc(}\n)str(")punct(;)comment(# c) / ` +
				`kw(in)kw(rec)punct({)prop(x)op(=)str('' echo)esc(${)var(s)esc(}) / str('')punct(;)prop(y)op(=)var(s)op(//)punct({)prop(z)op(=)const(true)punct(;)punct(};)comment(/* d */)punct(}) / ` +
				`prop(inputs)punct(.)prop(x)punct(.)str("y")punct(.)text($)punct({)var(s)punct(})op(=)var(a)punct(.)prop(b)op(==)var(c)punct(;)`},
	} {
		l := LanguageNamed(c.lang)
		if l == nil {
			t.Fatalf("no language %q", c.lang)
		}
		if got := lexLines(l.Tokenizer, c.lines...); got != c.want {
			t.Errorf("%s %q:\n got %s\nwant %s", c.lang, c.lines, got, c.want)
		}
	}
}

// A minified JSON file's one line, of many keys, lexes in time linear in
// its length.
func TestJSONLongLine(t *testing.T) {
	line := "{" + strings.Repeat(`"key":[1.5,"v"],`, 100_000) + `"end":0}`
	start := time.Now()
	toks, _ := LanguageNamed("json").Tokenizer.Tokenize(line, 0, nil)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("a line of %d keys took %v to lex", 100_000, d)
	}
	if n := len(toks); n < 700_000 {
		t.Errorf("%d tokens, want each key's, number's and string's", n)
	}
}

// countingTokenizer is t, counting the lines and bytes it tokenizes and
// the longest line.
type countingTokenizer struct {
	t                  Tokenizer
	lines, bytes, most int
}

func (c *countingTokenizer) Tokenize(line string, state uint64, dst []Token) ([]Token, uint64) {
	c.lines++
	c.bytes += len(line)
	c.most = max(c.most, len(line))
	return c.t.Tokenize(line, state, dst)
}

// Highlight reads the clock after long lines, not only every 64 lines: a
// pass of a minified file's lines, short enough to color, stops near its
// budget rather than megabytes later. A negative budget is spent at the
// first look at the clock.
func TestHighlightLooksAtTheClockAfterLongLines(t *testing.T) {
	line := strings.Repeat("abc(def, 12); ", 640) // 8,960 bytes
	text := strings.Repeat(line+"\n", 200)
	c := &countingTokenizer{t: LanguageNamed("js").Tokenizer}
	h := NewTokenHighlighter(c, NewBuffer(text).Snapshot())
	if h.Highlight(199, -1) {
		t.Fatal("a pass with no budget highlighted every line")
	}
	if want := highlightCheckBytes/len(line) + 1; c.lines > want {
		t.Errorf("a pass with no budget tokenized %d lines of %d bytes, want at most %d", c.lines, len(line), want)
	}
	// Each pass still tokenizes a line, so the passes finish, and they
	// color as one pass does.
	for passes := 1; !h.Highlight(199, -1); passes++ {
		if passes > 200 {
			t.Fatal("passes with no budget never finish")
		}
	}
	one := NewTokenHighlighter(LanguageNamed("js").Tokenizer, NewBuffer(text).Snapshot())
	if !one.Highlight(199, time.Hour) {
		t.Fatal("one pass didn't finish")
	}
	for _, i := range []int{0, 1, 63, 64, 199} {
		want, _ := one.Tokens(i, line, nil)
		got, ok := h.Tokens(i, line, nil)
		if !ok || fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("line %d: %v (final %v), want %v", i, got, ok, want)
		}
	}
}

// A line too long to color isn't tokenized to highlight the lines after
// it either: they start as it did, here in a comment.
func TestHighlightSkipsLinesTooLongToColor(t *testing.T) {
	long := strings.Repeat("x", maxTokenize+1)
	lines := []string{"/* a comment", long, "still */ x"}
	c := &countingTokenizer{t: LanguageNamed("js").Tokenizer}
	h := NewTokenHighlighter(c, NewBuffer(strings.Join(lines, "\n")).Snapshot())
	if !h.Highlight(2, time.Hour) {
		t.Fatal("not highlighted")
	}
	if c.most > maxTokenize {
		t.Errorf("tokenized a line of %d bytes, longer than %d", c.most, maxTokenize)
	}
	if got := lexLines(TokenizerFunc(func(line string, state uint64, dst []Token) ([]Token, uint64) {
		dst, _ = h.Tokens(2, line, dst)
		return dst, 0
	}), lines[2]); !strings.HasPrefix(got, "comment(still */)") {
		t.Errorf("the line after: %s, want it to start in the comment", got)
	}
}

// A string with interpolations is one the lexer's stack can hold: its
// index in five bits.
func TestInterpolatedStringsFit(t *testing.T) {
	for _, l := range Languages() {
		lx, ok := l.Tokenizer.(*lexer)
		if !ok {
			continue
		}
		for k, s := range lx.strings {
			if s.interpOpen != "" && k+1 >= 1<<5 {
				t.Errorf("%s's string %s%s… is number %d, past the stack's 31", l.Name, s.prefix, s.open, k)
			}
		}
	}
}

// A line whose expressions all close leaves the state of plain text.
func TestSvelteClosedExpressionsLeaveNoState(t *testing.T) {
	tok := LanguageNamed("svelte").Tokenizer
	for _, line := range []string{`<p class="a {b}">`, `<p class='a {b}'>`, `<p {c}>`, `{x}`, `<p>{#if a}{b}{/if}</p>`} {
		if _, state := tok.Tokenize(line, 0, nil); state != 0 {
			t.Errorf("%q leaves state %#x", line, state)
		}
	}
}

func TestLanguageFor(t *testing.T) {
	for name, want := range map[string]string{"main.go": "Go", "a/b/Makefile": "Makefile", "x.TSX": "TypeScript", "Dockerfile": "Dockerfile", "x.unknown": "",
		"main.dart": "Dart", "schema.graphql": "GraphQL", "q.gql": "GraphQL", "plot.r": "R", "plot.R": "R", "Main.scala": "Scala", "build.sc": "Scala", "flake.nix": "Nix",
		"user.rbs": "Ruby", "show.html.erb": "ERB", "model.rb.tt": "ERB", "nginx.conf.j2": "Jinja", "page.jinja2": "Jinja",
		"App.svelte": "Svelte", "x.vue": "HTML", "index.astro": "Astro",
		"main.tf": "HCL", "prod.tfvars": "HCL", "config.hcl": "HCL"} {
		l := LanguageFor(name)
		got := ""
		if l != nil {
			got = l.Name
		}
		if got != want {
			t.Errorf("LanguageFor(%q) = %q, want %q", name, got, want)
		}
	}
}

// A template's text is in the language of the name before its extension,
// plain text for a name LanguageFor doesn't know, and the template's own
// without one; its tags stay the template's.
func TestLanguageForTemplateHost(t *testing.T) {
	for _, c := range []struct{ name, host, line, tagged string }{
		{"compose.yaml.j2", "YAML", "image: app # pinned", `meta({{)var(tag)meta(}})`},
		{"config.json.erb", "JSON", `{"a": [1, true]}`, `meta(<%=)var(tag)meta(%>)`},
		{"Dockerfile.j2", "Dockerfile", "FROM ruby AS app", `meta({{)var(tag)meta(}})`},
		{"show.html.erb", "HTML", `<p class="x">a</p>`, `meta(<%=)var(tag)meta(%>)`},
		{"show.erb", "HTML", `<p class="x">a</p>`, `meta(<%=)var(tag)meta(%>)`},
		{"welcome.text.erb", "", `<p class="x">a: 1</p>`, `meta(<%=)var(tag)meta(%>)`},
		{"nginx.conf.j2", "TOML", "listen = 80", `meta({{)var(tag)meta(}})`},
		{"motd.j2", "", "[a] = 1 # b", `meta({{)var(tag)meta(}})`},
	} {
		l := LanguageFor(c.name)
		if l == nil {
			t.Errorf("LanguageFor(%q) = nil", c.name)
			continue
		}
		if again := LanguageFor("x/" + c.name); again != l {
			t.Errorf("LanguageFor(%q) made a new language on each call", c.name)
		}
		want := "text(" + c.line + ")"
		if c.host != "" {
			want = lexLines(LanguageNamed(c.host).Tokenizer, c.line)
		}
		if got := lexLines(l.Tokenizer, c.line); got != want {
			t.Errorf("%s text:\n got %s\nwant %s", c.name, got, want)
		}
		tag := "<%= tag %>"
		if l.Name == "Jinja" {
			tag = "{{ tag }}"
		}
		if got := lexLines(l.Tokenizer, tag); got != c.tagged {
			t.Errorf("%s tag:\n got %s\nwant %s", c.name, got, c.tagged)
		}
	}
	if LanguageFor("show.html.erb") != LanguageNamed("erb") || LanguageFor("motd.j2") != LanguageNamed("jinja") {
		t.Error("a template in its own text language is not the template itself")
	}
	for _, name := range []string{"page.html.erb.j2", "x.j2.erb"} {
		if l := LanguageFor(name); l != LanguageFor("a"+path.Ext(name)) {
			t.Errorf("LanguageFor(%q) = %v, want the template in its own text", name, l)
		}
	}
}

// LanguageNamed finds a language by its name, an extension or an alias,
// ignoring case, and each has a tokenizer.
func TestLanguageForText(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"bin/rails", "#!/usr/bin/env ruby\nrequire \"x\"\n", "Ruby"},
		{"bin/dev", "#!/usr/bin/env bash\n", "Shell"},
		{"setup", "#!/bin/sh -e\n", "Shell"},
		{"tool", "#!/usr/bin/python3.12\n", "Python"},
		{"run", "#!/usr/bin/env -S deno run --allow-read\n", "TypeScript"},
		{"run", "#!/usr/bin/env -i PATH=/bin node", "JavaScript"},
		{"x.rb", "#!/bin/bash\n", "Ruby"}, // the name decides first
		{"README", "#!/bin/bash\n", "Markdown"},
		{"notes", "#!/usr/bin/env perl\n", ""},
		{"notes", "#!\n", ""},
		{"notes", "#!/usr/bin/env\n", ""},
		{"notes", "echo #!/bin/sh\n", ""},
		{"notes", "", ""},
	} {
		got := ""
		if l := LanguageForText(c.name, c.text); l != nil {
			got = l.Name
		}
		if got != c.want {
			t.Errorf("LanguageForText(%q, %q) = %q, want %q", c.name, c.text, got, c.want)
		}
	}
}

func TestLanguageNamed(t *testing.T) {
	for want, names := range map[string][]string{
		"Go":      {"go", "Go", "golang"},
		"Dart":    {"dart", "Dart"},
		"GraphQL": {"graphql", "GraphQL", "gql"},
		"R":       {"r", "R", " r "},
		"Scala":   {"scala", "Scala", "sc"},
		"ERB":     {"erb", "ERB", "eruby"},
		"Jinja":   {"jinja", "Jinja", "jinja2", "j2"},
		"HCL":     {"hcl", "terraform", "tf", "tfvars"},
	} {
		for _, name := range names {
			l := LanguageNamed(name)
			if l == nil || l.Name != want {
				t.Errorf("LanguageNamed(%q) = %v, want %s", name, l, want)
				continue
			}
			if l.Tokenizer == nil {
				t.Errorf("%s has no tokenizer", l.Name)
			}
		}
	}
	if l := LanguageNamed("nope"); l != nil {
		t.Errorf("LanguageNamed(\"nope\") = %s", l.Name)
	}
}

// Each language's comments and brackets.
func TestLanguageCommentsAndBrackets(t *testing.T) {
	for name, want := range map[string][3]string{
		"dart": {"//", "/*", "*/"}, "scala": {"//", "/*", "*/"}, "graphql": {"#", "", ""}, "r": {"#", "", ""},
		"jinja": {"", "{#", "#}"},
		"hcl":   {"#", "/*", "*/"},
	} {
		l := LanguageNamed(name)
		if got := [3]string{l.LineComment, l.BlockComment[0], l.BlockComment[1]}; got != want {
			t.Errorf("%s's comments are %q, want %q", l.Name, got, want)
		}
		var pairs []string
		for _, b := range l.Brackets {
			pairs = append(pairs, b.Open+b.Close)
		}
		if got := strings.Join(pairs, " "); got != "() [] {}" {
			t.Errorf("%s's brackets are %s", l.Name, got)
		}
	}
}

// After any edits, the states the syntax keeps are those of tokenizing
// the text from the top.
func TestSyntaxIncremental(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	pieces := []string{"x", "\n", "/*", "*/", "`", "\"", " ", "func", "{", "}", "// c\n", "${", "a\nb"}
	b := NewBuffer(strings.Repeat("func f() {\n\tx := `a\nb` /* c */\n}\n", 20))
	b.SetLanguage(LanguageNamed("go"))
	for round := range 300 {
		if round%3 == 0 {
			b.SetLanguage(LanguageNamed([]string{"go", "javascript", "markdown"}[round/3%3]))
		}
		text, _ := b.snapshot()
		for range r.IntN(3) + 1 {
			at := r.IntN(text.Len() + 1)
			end := min(text.Len(), at+r.IntN(4))
			b.Edit(Edit{Start: at, End: end, Text: pieces[r.IntN(len(pieces))]})
			text, _ = b.snapshot()
		}
		s := b.highlighter().(*tokenHighlighter)
		// Sometimes only part of the text.
		upTo := text.LineCount() - 1
		if r.IntN(2) == 0 {
			upTo = r.IntN(text.LineCount())
		}
		if !s.Highlight(upTo, time.Hour) {
			t.Fatal("advance stopped")
		}
		var state uint64
		tok := b.Language().Tokenizer
		for i := 0; i <= upTo; i++ {
			got, ok := s.stateOf(i)
			if !ok || got != state {
				t.Fatalf("round %d: line %d starts in %x (%v), want %x", round, i, got, ok, state)
			}
			_, state = tok.Tokenize(text.Line(i), state, nil)
		}
	}
}

func BenchmarkTokenizeHTML(b *testing.B) {
	benchmarkTokenize(b, "html", "<div class=\"card\" id='x' data-a=\"{b}\">\n\t<a href=\"/p?q=1&amp;r\">Hi &lt;there&gt;</a> <!-- note -->\n"+
		"\t<script>if (a > 1) { run(`x${y}`) }</script>\n\t<style>.a { color: red; }</style>\n</div>\n")
}

func BenchmarkTokenizeSvelte(b *testing.B) {
	benchmarkTokenize(b, "svelte", "<div class=\"card {cls}\" on:click={() => open(id)}>\n\t{#each items as item (item.id)}\n"+
		"\t\t<a href=\"/p/{item.id}\">{item.name} &amp; {count + 1}</a>\n\t{/each}\n</div>\n")
}

func benchmarkTokenize(b *testing.B, lang, chunk string) {
	src := strings.Repeat(chunk, 200)
	lines := strings.Split(src, "\n")
	tok := LanguageNamed(lang).Tokenizer
	var toks []Token
	b.SetBytes(int64(len(src)))
	for range b.N {
		var state uint64
		for _, l := range lines {
			toks, state = tok.Tokenize(l, state, toks[:0])
		}
	}
}

func BenchmarkTokenizeGo(b *testing.B) {
	src := strings.Repeat("func (s *server) handle(w http.ResponseWriter, r *http.Request) error {\n\tif err := s.check(r.Context(), \"x\\n\", 0x1f); err != nil { // why\n\t\treturn fmt.Errorf(`bad: %w`, err)\n\t}\n\treturn nil\n}\n", 200)
	lines := strings.Split(src, "\n")
	tok := LanguageNamed("go").Tokenizer
	var toks []Token
	b.SetBytes(int64(len(src)))
	for range b.N {
		var state uint64
		for _, l := range lines {
			toks, state = tok.Tokenize(l, state, toks[:0])
		}
	}
}
