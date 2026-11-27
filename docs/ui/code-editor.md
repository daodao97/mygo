# Code editor

<!-- preview code-editor 320 -->

Package `ui/editor` is a code editor and a [diff viewer](diff-viewer.md)
for native UI, designed after Zed's: syntax highlighting, several carets
and selections, soft wraps, folding, finding and replacing, completions,
hovers, diagnostics and inlay hints from the app or a language server,
ranges shown as something else, and the commands and keys of code editors. It lays out and paints only the
lines in view, so a file of a million lines scrolls and edits as one of
ten.

```go
import "github.com/egoist/mygo/ui/editor"

// Kept by the app, as its other state.
buf := editor.NewBuffer(source)
buf.SetLanguage(editor.LanguageFor("main.go"))
app.code = &editor.State{Buffer: buf, WrapGuides: []int{100}}

// In the view:
editor.View(c, app.code).Grow(1).FillWidth()
```

A `State` is the editor's state, kept across frames as a text input's
string is: its buffer, its options, and what the editor keeps of its own,
as its selections, folds and scroll position. Its options are fields set
before the editor is built; their zero values are the defaults, those of
Zed. `View` builds the editor of a state, which is a focusable element as
any other: give it a size, and `AutoFocus` to type in it at once.

The [code editor example](../../examples/code-editor) edits files of
several languages, with completions of the words of the file, hovers,
diagnostics of TODO comments, the changes in the gutter and their diff.

## Buffers

A `Buffer` is the text an editor edits, with its undo history and its
language. Several editors, and both sides of a diff, can show one buffer:
an edit in one shows in the others, their carets moving with the text
around them, and undo takes back the buffer's last edit, whichever editor
made it.

```go
buf.Edit(editor.Edit{Start: 0, End: 0, Text: "// Generated.\n"})
buf.Transact(func() { // one step of undo
	buf.Edit(editor.Edit{Start: a, End: b, Text: name})
	buf.Edit(editor.Edit{Start: c, End: d, Text: name})
})
if buf.Modified() {
	_, err := buf.WriteTo(file)
	if err == nil {
		buf.MarkSaved()
	}
}
```

Offsets count bytes of UTF-8, and a `Pos` is a line and the bytes into it,
both from 0: `Pos` and `Offset` convert between them, and `Line`, `Slice`
and `String` read the text. `Set` replaces the text as an edit, which undo
takes back, and `Reset` as a new document, without history.

`EditRemote` applies edits made elsewhere, as another device's or the
result of a merge, outside the history. Undo and redo go on taking back and
making again the buffer's own steps, on the text with those edits:

```go
buf.Edit(editor.Edit{Start: 0, End: 0, Text: "// Mine.\n"})
buf.EditRemote(editor.Edit{Start: end, End: end, Text: "// Theirs.\n"})
buf.Undo() // takes back "// Mine.", and "// Theirs." stays
```

A step that overlaps a remote edit can no longer be taken back or made
again alone: it leaves the history, the text staying as it is, and the
steps around it stay. Inserting at the edge of a step's text does not overlap it, nor does
replacing the text next to it. So pass the ranges that changed, from a diff
of the two texts, rather than the whole text, which overlaps every step.
Carets, selections, folds, diagnostics and inlays move through a remote edit
as through any, and `Modified` still compares with the saved text: call
`MarkSaved` when the new text is what is on disk. A remote edit moves every
step of the history, around a millisecond for a full one of 10,000 steps,
whatever the size of the text.

Lines end in `\n`, as in Zed: a text with `\r\n` or `\r` line endings is
kept with `\n` ones, and the buffer remembers the ending of each line,
which `WriteTo` writes, so that a file saves as it was, mixed endings and
all. An edit changes only the endings of the lines it breaks or joins: a
line broken in two, as with Enter, ends both halves as it ended, deleting
a line break keeps the one after it, and line breaks pasted take the
endings of those they replace, then the ending of the line they go in,
whatever the clipboard had; moving lines up or down moves their endings
with them. A line ending in `\r` followed by an empty line ending in `\n`
would be written `\r\n`, which reads back as one line break, so the empty
line's break becomes `\r\n`. Undo and redo put endings back with the text.
`LineEnding` says which ending most lines have, `MixedLineEndings`
whether the lines' endings differ, and `SetLineEnding` converts every
line to one. Both read counts the buffer keeps, without a scan of the
text, so a status bar can show "Mixed" or the ending, as Notepad++'s does,
and offer the conversion:

```go
ending := map[string]string{"\n": "LF", "\r\n": "CRLF", "\r": "CR"}[buf.LineEnding()]
if buf.MixedLineEndings() {
	ending = "Mixed"
}
```

The text is a persistent tree of chunks, as Zed's rope: an edit copies a
few kilobytes and a path of the tree, whatever the size of the document,
and each version shares the rest with the one before. Undo keeps the edits
that take each step back, and a `Snapshot` is a version that stays as it is
while the buffer changes, which other goroutines read without locks.
Buffers' methods are safe from any goroutine; change them with
`Window.Update` so the window shows the change.

## Editing

The editor edits as Zed does, with the commands bound to the same keys
(Control replaces Command on Linux and Windows, as in Visual Studio Code):

- **Several carets.** Cmd+D selects the next occurrence of the selection,
  or the word at the caret; Cmd+Shift+L all of them; Cmd+Alt+Up and Down
  add a caret on the line above or below; Alt+click adds a caret, and
  Alt+Shift+drag selects a column. Typing, deleting and every command act
  at each caret, and Escape goes back to one. Cmd+U undoes a change of the
  selections.
- **Selections by structure.** Ctrl+Shift+Right selects the word, then
  the string or brackets around it, and on out; Ctrl+Shift+Left goes back.
- **Lines.** Alt+Up and Down move lines, Alt+Shift+Up and Down duplicate
  them, Cmd+Shift+K deletes them, Ctrl+J joins them, Cmd+/ comments them
  out, Cmd+] and Cmd+[ indent them; Cmd+Enter opens a line below.
- **Typing.** Brackets and quotes close as they are typed, and wrap the
  selection; Enter keeps the indentation, indents after an opening bracket
  and between a pair, and continues line comments; a closing bracket, or
  a word such as `end`, outdents its line as it is typed. Undo takes back
  typing a burst at a time.
- **Folding.** The gutter's markers fold the lines indented under a line,
  up to its closing bracket, or a run of imports, with the blank lines
  between them; Cmd+Alt+[ and ] fold and unfold, Cmd+K
  Cmd+0 folds everything and Cmd+K Cmd+J unfolds it, Cmd+K Cmd+1 to 9 by
  level.
- **Finding.** Cmd+F opens the find bar, with case, whole words, regular
  expressions and finding in the selection; Cmd+Alt+F replaces. Cmd+G goes
  to the next match, and Ctrl+G to a line. The bar is no wider than the
  editor: in a narrow one it leaves out the count of matches, then the
  options and the buttons that go through the matches or replace them,
  keeping the fields, the button that shows or hides the replacement and
  the close button. Its field turns red for a regular expression that is
  not one.
- **The pointer.** A double click selects a word, a triple click a line,
  and dragging extends by them; dragging near the edges scrolls; Cmd+click
  goes to a definition.

`Run` runs a command, as a menu item or the app's own key does, and
`Bindings` binds keys: Mods and Key, then ThenMods and ThenKey for chords
such as Cmd+K Cmd+0. An app's bindings come before the defaults
(`DefaultBindings`), and a binding to `NoCommand` takes a key away from
the editor, for the app's own shortcut. `CommandNamed` finds a command by
the name keymaps give it, as "selectNext".

```go
app.code.Bindings = []editor.Binding{
	{Mods: ui.Cmd, Key: ui.KeyP, Command: editor.NoCommand}, // the app's
	{Mods: ui.Cmd | ui.Shift, Key: ui.KeyD, Command: editor.DuplicateSelection},
}
if ui.Button(c, "Format").Clicked() {
	app.code.Run(editor.SortLines)
}
```

`Selections`, `SetSelections`, `Select`, `Cursor` and `CursorPos` read
and set the carets and selections, `Insert` types at each of them, `GoTo`
moves the caret to a position and `ScrollTo` shows a line. `Find` opens
the find bar on a query and `FindMatches` counts its matches; `Fold`,
`Unfold` and `Folded` fold lines, and `FoldImports` the first run of
imports, as an editor can on opening a file; `Focused` reports whether the editor has
the keyboard focus, and `VisibleLines` the first and last lines the last
frame showed, as a half page of Vim's moves by. `ScrollY` and
`SetScrollY` read and set how far the lines are scrolled, in DIPs,
`ViewportHeight` how high the part of them in view is, and `LineTop`
where a line starts, below the first, `LineTop` of the count of lines
being the height of them all: editors side by side scroll together with
them, an app draws what joins their lines, as the bands between the
panes of a merge, and a minimap beside the editor shows where its view
is. The scroll bars' thumbs show while the pointer is over the editor, or
drags one, as a scroll container's do, or all the time with
`KeepScrollbars`; the marks of changes, matches and diagnostics on the
vertical one show all the time. The horizontal one shows while a line
the editor has shown is wider than it, and goes once an edit, inlays,
replacements or another tab size make the lines narrower, the text
scrolling back to fit. `NoScrollbars` takes the bars and their marks
away, and gives their room to the text.

`OnInput` sees each input event before the editor does: keys, the text
typed, edit commands and the pointer. Returning true takes the event, and
the editor leaves it; the others go on as without it. An app's own mode of
editing, as Vim's normal mode, takes keys this way while the editor still
shows and moves the carets. Letters typed at the carets come as
`InputText`: on some systems their keys come first as `InputKeyDown` too,
and taking a key doesn't take its text, so a mode reads letters from
`InputText`. What an input method composes, commits or replaces, as the
accents of macOS's press and hold replace the letter before the caret,
goes to the editor alone. The key after the first of a chord of
`Bindings`, as Cmd+K Cmd+0, is the chord's, as VS Code resolves chords
before an extension's keys.

`NoInputMethod` turns the system's input methods off while the editor has
the keyboard, as Neovide does in Vim's normal mode: keys type the
characters of the keyboard's layout, which come to `OnInput` as
`InputText`, and nothing is composed, so j is j with a Chinese input
method on. Set it in the modes that read letters as commands and clear it
in insert mode. Browsers cannot turn input methods off: there they compose
in the text still.

```go
app.code.CursorShape = editor.CursorBlock // normal mode
app.code.NoInputMethod = true
app.code.OnInput = func(ev ui.InputEvent) bool {
	if ev.Kind == ui.InputText {
		return app.vim.normalKeys(ev.Text) // j, k, dd, …
	}
	return false
}
```

## Options

- `SoftWrap` wraps lines at the editor's width, or at `WrapColumn` columns
  when narrower; wrapped rows keep the line's indentation. CJK text wraps
  between any two characters, but closing punctuation such as `，` and `。`
  starts no row, and opening punctuation such as `（` and `「` ends none.
- `LineNumbers` shows the gutter's numbers, absolute, relative to the
  caret's line, or not at all; `GutterMarks` put dots of a color by lines,
  as breakpoints, and `OnGutterClick` reports clicks on numbers.
- `LineBackgrounds` fill lines, from `First` to `Last`, with a color under
  the text and the selections, as the statement a key would run; the
  gutter keeps its own, and lines a fold hides take no room to fill. They
  keep their line numbers as the text is edited, unlike diagnostics and
  inlays, which move with it: compute them again when the buffer's
  `Version` changes.
- `TextBackgrounds` fill ranges of the text, bytes `Start` to `End`, with
  a color under the text and the selections and over line backgrounds, as
  the words a merge changed within its lines. They follow the editor's
  layout: the rows a line wraps into, tabs, wide characters and scrolling
  sideways; lines a fold hides show none. As line backgrounds, they stay
  where they are as the text is edited, cut to its end until computed
  again when the buffer's `Version` changes.
- `TabSize` and `Indent` set indentation: tabs or spaces, as the
  language's files usually have them when zero (Go's tabs).
- `Whitespace` shows spaces and tabs as dots and arrows: within selections
  by default, everywhere, at the ends of lines, or nowhere.
- `WrapGuides` draws guides at columns, as 80 and 100; `NoIndentGuides`
  hides those of indentation.
- `ReadOnly` lets the text be selected and copied, not edited.
- `FitContent` makes the editor as high as its lines, rather than
  scrolling them, to show code in a scroll view or a list. The containers
  around it scroll it, so its find bar and go to line bring their text
  into view through those containers, and the bars themselves stick to
  the top of the part of the text those containers show.
- `KeepScrollbars` keeps the scroll bars' thumbs showing with the pointer
  away, as a scroll container's `KeepScrollbars`; `NoScrollbars` takes
  the bars away, with their room, the wheel and the keys still
  scrolling.
- `Scrollbar` styles the scroll bars, as a theme's `ScrollbarStyle`: their
  track and thumb colors, widths, corners and shortest thumb. Its zero
  fields follow the theme's; its `TrackWidth` is 10 DIPs when 0, unless the
  theme's style sets one, as the editor's bar has always been. As VS
  Code's editor:

  ```go
  st.Scrollbar = ui.ScrollbarStyle{
      TrackWidth: 14, ThumbWidth: 14, Square: true, Flush: true, MinLength: 20,
      Thumb: ui.RGBA(121, 121, 121, 0.4),
      Hover: ui.RGBA(100, 100, 100, 0.7),
      Active: ui.RGBA(191, 191, 191, 0.4),
  }
  ```
- `Font` and `LineHeight` set the text: the system's monospaced font of
  13 DIPs, 1.5 times as high, by default.
- `NoAutoClose`, `NoFolding`, `NoActiveLine` and `NoScrollPastEnd` turn
  those behaviors off. `Placeholder` shows while the buffer is empty.
- `ScrollMargin` sets how many lines to keep above and below the
  caret when scrolling it into view. Zero uses three lines, a positive
  value sets that many lines, and a negative value removes the margin.
  The margin shrinks to fit the viewport and stops at the scroll limits.

  ```go
  st.ScrollMargin = -1 // no margin
  ```
- `CursorShape` paints carets as bars (`CursorBar`), as blocks over the
  character after them (`CursorBlock`), as Vim's normal mode does,
  translucent so that the character shows and outlined while the editor
  hasn't the keyboard, or as underlines (`CursorUnderline`), as its
  pending operators do.
- `DiffBase` is the text the buffer's changes show against in the gutter,
  as the file in Git: bars mark the lines added and changed, and a mark
  where lines were deleted. Cmd+F8 goes to the next change, and Cmd+Alt+Z
  reverts the one at the caret.
- `ContextMenu` adds items to the editor's context menu, after its own.

## Languages and highlighting

`LanguageFor` finds a file's language by its name, `LanguageForText` by
its name or else the `#!` line its text opens with, as
`#!/usr/bin/env ruby`, and `LanguageNamed` by its own; `Languages` lists
them. The editor knows Go, JavaScript,
TypeScript, JSON, Python, Rust, C, C++, Java, Kotlin, Scala, Swift, Dart,
C#, shell scripts, YAML, TOML, SQL, R, Ruby, Lua, PHP, CSS, HTML, ERB,
Svelte, Astro, XML, Markdown, diffs, Dockerfiles, Makefiles, Protocol Buffers,
GraphQL, Zig, Nix, Jinja templates and HCL (Terraform).
A template's text is in the language its name gives before the template's
extension: `LanguageFor` highlights `compose.yaml.j2` as Jinja in YAML,
`config.json.erb` as ERB in JSON, and `notice.text.erb` as ERB in plain
text. Without such a name, ERB's text is HTML and Jinja's plain text.
A `Language` says how to highlight, comment, match brackets and indent its
text, and with `Imports`, a pattern of a line that imports (`^import\b` for
Go), which lines fold as a run: build one for another language, or copy
one and change it.

A language highlights its text with a `Tokenizer` by default: a function
from a line, and the state the line before it ended in, to the line's
tokens and the state it ends in, as TextMate grammars and Monaco's
tokenizers work. The editor tokenizes the lines it shows, and after an
edit only the lines below it until one starts in the state it started in
before, a frame's time budget at a time.

```go
// Lines starting with # are comments.
lang := &editor.Language{
	Name:        "Notes",
	Extensions:  []string{".notes"},
	LineComment: "#",
	Tokenizer: editor.TokenizerFunc(func(line string, state uint64, tokens []editor.Token) ([]editor.Token, uint64) {
		kind := editor.TokenText
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			kind = editor.TokenComment
		}
		return append(tokens, editor.Token{End: len(line), Kind: kind}), 0
	}),
}
```

A token is a run of a line of one `TokenKind`, which the theme colors:
keywords, types, functions, strings, comments and the others.

### Highlighters

A language can instead bring a highlighter of its own, as one of a parser
such as tree-sitter: `Language.Highlighter` makes the `Highlighter` of each
buffer of the language, and the editor uses it in place of the tokenizer.
The tokenizer itself is one (`NewTokenHighlighter`), so a highlighter
plugs in without the editor changing. The text of a patch's side
(`DiffState.SetPatch`) is its hunks' lines one after another: the
tokenizer starts each hunk in state 0, while a highlighter of its own
sees one text, and highlights it as it sees fit.

```go
type Highlighter interface {
	// Edit follows an edit of the text, old and new its text before and
	// after, and changes what it replaced, in order.
	Edit(old, new editor.Snapshot, changes []editor.Change)
	// Highlight works toward the tokens of the lines up to last, for at
	// most budget, and reports whether they are ready.
	Highlight(last int, budget time.Duration) bool
	// Tokens appends the tokens of line i to dst, and reports whether
	// they are final rather than a guess made before Highlight is done.
	Tokens(i int, text string, dst []editor.Token) ([]editor.Token, bool)
}
```

The editor calls these methods on the main thread: `Edit` as the buffer
changes, `Highlight` before it paints, with the lines it shows, and again
in later frames while it reports false, and `Tokens` for each line it
paints. A `Change` gives the ranges an edit replaced in the old text and
the new, what an incremental parser takes to edit its tree. A parser can
work in the background instead: snapshots are safe to read from any
goroutine, `Chunks` reads them a piece at a time, and the `invalidate`
function the highlighter was made with draws the editors showing the
buffer again once a parse is done.

A highlighter that knows the structure of the text can tell the editor
more, by implementing any of these interfaces too:

- `SyntaxSelector`: `Enclosing` returns the smallest node holding more
  than a range, which Select Larger selects, rather than the word, string
  or brackets around it.
- `Folder`: `FoldEnd` returns the last line folding a line hides, rather
  than what indentation and brackets say.
- `BracketMatcher`: `EnclosingBrackets` returns the innermost pair around
  a range, which the editor highlights and moves between, as `do` and
  `end`.
- `Indenter`: `IndentAfter` returns how many levels more a line Enter
  opens is indented.

## Completions, hovers and diagnostics

The editor shows what the app or a language server knows of the code; the
app answers its requests, at once or later from another goroutine through
`Window.Update`:

```go
app.code.OnCompletion = func(r editor.CompletionRequest) {
	go func() {
		items := app.lsp.Complete(r.Offset) // a language server's
		app.win.Update(func() { app.code.ShowCompletions(r.Offset-len(r.Prefix), items) })
	}()
}
app.code.OnHover = func(r editor.HoverRequest) {
	if doc, start, end, ok := app.lsp.Hover(r.Offset); ok {
		app.code.ShowHover(start, end, doc, nil)
	}
}
app.code.SetDiagnostics(buf.Version(), app.lsp.Diagnostics())
```

- `OnCompletion` asks for completions as a word is typed, or Ctrl+Space
  pressed. `ShowCompletions` lists them below the caret, filtered and
  ranked as typing goes on by the word typed, with their kinds, details
  and documentation; Enter or Tab inserts the one chosen.
- `OnHover` asks what to show as the pointer rests on the text
  (`HoverDelay`, 600 ms by default), or for Cmd+K Cmd+I at the caret.
  `NoPointerHover` turns hovers from the resting pointer off, diagnostics'
  too, as VS Code's `editor.hover.enabled`; the keys still show one.
  `ShowHover` shows text in a popover over the range, or what a function
  builds there. The pointer can go into the
  hover, which stays while it is there, to select and copy its text; the
  hover goes a moment after the pointer leaves the text and the hover,
  and Escape closes it.
- `SetDiagnostics` underlines problems with wavy lines of their severity,
  shows their messages in the hover and marks them on the scroll bar. The
  version is the buffer's the diagnostics are of: the editor moves them
  through the edits made since.
- `SetInlays` shows inlay hints, text in the lines that is not theirs, as
  the types of variables; `SetSuggestion` shows text after the caret as a
  suggestion, which Tab accepts, as an AI's.
- `OnDefinition` is called for Cmd+click and F12 on a word. With Cmd
  held, the word under the pointer shows as a link.
  `NoDefinitionClick` leaves Cmd+click to the editor's other clicks, link
  and all, and F12 and the context menu still go to the definition;
  `NoDefinitionLink` keeps Cmd+click but shows no link.
- `ShowPopup` shows a panel the app builds below the caret at an offset,
  as the usages or implementations a go to finds, to choose one from. It
  takes the keyboard as it opens, for the app's own keys
  (`panel.Shortcut`); Escape, a press outside it, an edit or `ClosePopup`
  closes it, and the keyboard goes back to the editor. An offset outside
  the text is taken as its nearest end.
- `Rect` returns where the caret at a byte offset shows, in DIPs from the
  top left of the editor's box as the last frame laid it out, and whether
  its line shows (a line a fold hides does not). An app draws what belongs
  to a range of the text from the `Rect` of its first and last offsets, as a
  frame around it, in an element's `DrawOver`. `TextX` is where the text
  starts, right of the gutter; unlike a `Rect`, it stays as the text
  scrolls sideways, for what belongs beside a line rather than in it, as a
  mark by its number.

## Replacing ranges

`SetReplacements` shows ranges of the text as something else, as Zed's
folds show their placeholders: `Text`, the element `Content` builds, or
nothing. The text stays the buffer's: it saves, copies, finds and undoes
as it is, and assistive technology reads it. A Markdown editor's live
preview hides the marks and draws check boxes, but on the lines with a
caret, whose source shows to edit:

```go
var rs []editor.Replacement
for _, m := range marks(buf) { // the app's parser
	if !onCaretLine(m) {
		rs = append(rs, editor.Replacement{Start: m.Start, End: m.End})
	}
}
rs = append(rs, editor.Replacement{
	Start: box.Start, End: box.End, Label: "Not done", Interactive: true,
	Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
		e.Children(func() {
			if ui.ButtonBase(c).Fill().KeepFocus().Clicked() {
				buf.Edit(editor.Edit{Start: r.End - 2, End: r.End - 1, Text: "x"})
			}
		})
	},
})
app.code.SetReplacements(rs)
editor.View(c, app.code).Grow(1).FillWidth()
```

Replacements move with edits, as inlays do, leaving out what is typed at
their edges, and go when their text does. Set them again as the text or
the selections change: laying out again only the lines whose replacements
changed, this is cheap in every frame.

- **Carets** step over what a replacement shows as over one character,
  and deleting next to it deletes all of it. A range showing nothing
  within a line is as if it were not there: no key press is spent on it,
  and Backspace takes the character before it, not the hidden text.
- **`Text`** shows in the editor's font, in `Color`, else in the color of
  the text it replaces.
- **`Content`** is given the element the editor places, `Width` DIPs wide
  (a square as high as a row when 0) and as high as a row, as parts are
  (see [Bars and popups](#bars-and-popups)). The width is the app's to
  say, so that soft wraps count it in lines not laid out yet. A press on
  it goes to the text, unless `Interactive`; `Label` names it for
  assistive technology.
- **Ranges across lines** show on the line they start on: the lines they
  cover from their start through their newline take no rows, as folded
  ones, but the first when it shows something; the line they end in shows
  what follows them on a row of its own.

Empty ranges, and ranges overlapping one before them, are left out.
`Replacements` returns them as the edits moved them. Only the lines in
view are laid out and only their elements built, so a long text may have
as many as it needs.

The [code editor example](../../examples/code-editor) has a live preview
of its README.md.

## Blocks between lines

`SetBlocks` shows elements between the lines of the text, as Zed's blocks:
an image under the line naming it, or a table in place of its source.
They are no part of the text, which stays the buffer's.

```go
blocks := []editor.Block{{
	Line: img.Line, Height: 200, Label: img.Alt,
	Content: func(c *ui.Context, e ui.Element, b editor.Block) {
		e.Children(func() { ui.Image(c, img.Source).Width(320) })
	},
}}
app.code.SetBlocks(blocks)
editor.View(c, app.code).Grow(1).FillWidth()
```

- **`Line`** and **`Above`** say where it goes: below the line, or above
  it. Blocks move with their line's text. Deleting that line attaches
  the block to the surviving line at that position, with `Above` unchanged.
  Set them again as the text changes: those at the place of one shown
  keep their elements and heights.
- **`Content`** is given the element the editor places, using the text
  layout's width, including its `WrapColumn` limit and minimum width, as
  parts are (see [Bars and popups](#bars-and-popups)). It stays in place
  as the text scrolls sideways.
- **The height** is what the element lays out to. Only the blocks in view
  are built, so a block not shown yet is taken to be `Height` high, or a
  row, and measured as it shows, the lines in view staying where they are.
- **Presses** on it go to the text, the caret to the line next to it,
  unless `Interactive`; `Label` names it for assistive technology.

A block on a line a [replacement](#replacing-ranges) hides shows in place
of the lines the replacement covers, for a range showing nothing from the
start of a line through a newline. A live preview does this for a table:

```go
if !hasCaret(table) {
	rs = append(rs, editor.Replacement{Start: table.Start, End: table.End})
	blocks = append(blocks, editor.Block{Line: table.FirstLine, Content: drawTable})
}
```

Carets stop on such a block once, at the start of the range, showing
before it, and a press on it puts the caret there: an app showing the
source of what has a caret shows it then. `CursorShape` applies over the
whole stand-in: a bar or block spans its height, and an underline sits
at its bottom. The block and underline fit in the margin before the
content; the bar keeps its insertion point. Without focus, only the
block's outline stays visible.
Selecting across it tints it,
and deleting next to it deletes all of it. A fold hides the blocks of its
lines.

The [code editor example](../../examples/code-editor)'s live preview shows
an image and a table this way.

## Line styles

`SetLineStyles` draws lines in other fonts, as a Markdown editor's headings
in larger type. The text stays the buffer's; only how its lines look
changes.

```go
var styles []editor.LineStyle
for _, h := range headings(buf) { // the app's parser
	styles = append(styles, editor.LineStyle{
		Line: h.Line, Font: ui.Font{Size: 28 - 3*float32(h.Level), Weight: 700},
	})
}
app.code.SetLineStyles(styles)
```

- **`Font`**'s zero fields take the editor's, so `Size` alone keeps its
  family. **`LineHeight`** is a multiple of the font's size, as
  `State.LineHeight`; 0 takes the editor's.
- **Rows** of a styled line are as high as its font makes them, and it
  wraps in its own font. Carets, selections, presses, find's matches and a
  replacement's element follow; an element without a `Width` is as wide as
  the line's rows are high.
- **The gutter** numbers styled lines in the editor's font, on the baseline
  of their first row.
- **Only the lines in view** are laid out still: the height of a styled
  line's rows comes from its font's metrics, without shaping it, and how
  many rows a wrapped one takes is measured as for any line.

Styles move with edits, a style going with its line's text, and set again,
the same styles cost nothing and a change lays out only the lines whose
style changed. `LineStyles` returns them as the edits moved them. A diff's
sides take none. The [code editor example](../../examples/code-editor)'s
live preview shows its README.md's headings this way.

## Code snippets

A completion's `Insert` is always plain text, even with
`Kind: editor.CompletionSnippet` (which chooses an icon). To insert a
code template with editable placeholders, parse one and set `Snippet`:

```go
snippet, err := editor.ParseSnippet("for ${1:index}, ${2:value} := range ${3:items} {\n\t$0\n}")
if err != nil {
    // Offer a plain-text completion instead when a server uses
    // snippet syntax the editor does not support.
    return err
}
code.ShowCompletions(start, []editor.Completion{{
    Label: "forrange", Kind: editor.CompletionSnippet, Snippet: snippet,
}})
// Or insert at every current selection, outside the completion list:
code.InsertSnippet(snippet)
```

Parse templates once; a `Snippet` is immutable and can be shared by
editors. `Text()` returns its initial text. The parser supports `$1`,
`${1}`, `${1:default}`, escaped dollars, braces and backslashes, and `$0`
as the final stop (the end when omitted). Tab visits positive numbers in
numeric order, with 0 last; Shift+Tab goes back, stopping at the first
field. Repeated numbers share their default, including forward references,
and edit together through the editor's normal multiple selections.
Coincident empty stops of the same number coalesce as carets do.
Scrolling and input methods follow the first occurrence at the original
primary insertion site.

A completion list takes Tab before the snippet's navigation, so a field
can itself be completed. Escape ends the session after closing any open
completion list or other editor overlay. Moving outside the active
fields, editing the buffer externally, undo/redo or changing buffers
ends the session too. Each field's typing is a separate undo group;
undoing the insertion restores the old text and selections. Redo restores
the text and selections without restarting snippet navigation. Input
methods compose and commit at mirrored fields as at ordinary carets.
A new snippet replaces the previous session.

This is the tab-stop subset of TextMate/LSP syntax: nested placeholders,
variables, choices, transformations, conflicting defaults for one number,
and invalid UTF-8 return errors. Text keeps the template's indentation;
CRLF is normalized to LF. `InsertSnippet` leaves a read-only editor
unchanged. State methods, including snippet insertion, run on the UI
thread as the rest of editor interaction does.

Run `go run ./examples/code-editor`, type `funcdef` or `forrange` in a Go
file, then press Tab to try it.

## Styling

### Themes

`Theme` colors the editor: its background, text, gutter, selections,
carets, guides, diagnostics, changes and each kind of token. Colors left
zero take those of `DefaultTheme`, which follows the window's theme and
appearance, light or dark.

```go
th := editor.DefaultTheme(c.Theme())
th.Tokens[editor.TokenKeyword] = editor.TokenStyle{Color: ui.RGB(0xc6, 0x78, 0xdd), Bold: true}
app.code.Theme = th
```

### Bars and popups

The find bar, the go to line bar, the completions and the hovers are
parts the app can build itself, as it styles ui's
[bases](custom-widgets.md): `RenderFindBar`, `RenderGoToLine`,
`RenderCompletions` and `RenderHover` are each given the element the
editor places (a toolbar over its top right corner, a bar at its top, a
list below the caret, a card above the text hovered), which keeps the
presses from the text under it and the keyboard where the part needs it
(in the editor while the completions show, in the hover as its text is
selected), and a handle to what the part shows and does. Style the element and build its
content; its position can change too. The find bar comes with margins
that keep it inside the editor, so `bar.Right(0)` leaves it the theme's
`Space(4)` from the right edge; `bar.Margin(0)` takes them off. An editor
as high as its lines (`FitContent`) has no scroll of its own: the bars
are built in a row over the whole editor, and stick to the top of the
part of the text the containers around it show, so a bar an app builds
inside `bar` moves with it. The editor's own looks are
`DefaultFindBar`, `DefaultGoToLine`, `DefaultCompletions` and
`DefaultHover`, which a render function can call and then restyle.

```go
app.code.RenderFindBar = func(c *ui.Context, bar ui.Element, f *editor.FindBar) {
	t := c.Theme()
	bar.Row().Gap(8).Padding(6).Background(t.Surface).Radius(12).Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.2))
	bar.Children(func() {
		f.QueryInput(ui.TextInputBase(c, f.Query()).Placeholder("Search").Width(180))
		ui.Toggle(c, f.Regexp(), ".*").KeepFocus()
		ui.Text(c, f.Status()).TextColor(t.TextMuted)
		if ui.Button(c, "Next").KeepFocus().Clicked() {
			f.Next()
		}
	})
}
app.code.RenderHover = func(c *ui.Context, panel ui.Element, h *editor.HoverCard) {
	editor.DefaultHover(c, panel, h) // the editor's look, on another background
	panel.Background(app.hoverColor)
}
```

- `FindBar` binds fields and toggles: `Query` and `Replacement`, and
  `CaseSensitive`, `WholeWord`, `Regexp` and `InSelection`, are pointers,
  and the matches follow them as they change. `QueryInput` and
  `ReplacementInput` give inputs the bar's keys: the query takes the
  focus as the bar opens, Enter and Shift+Enter go through the matches,
  Alt+Enter selects them all, Enter and Cmd+Enter in the replacement
  replace one or all, and Escape closes. `Matches` and `Status` count the
  matches, and `Next`, `Previous`, `SelectAll`, `Replace`, `ReplaceAll`
  and `Close` act; `Replacing` says whether the bar replaces too, as
  Cmd+Alt+F opens it.
- `GoToLineBar` has `Text` for its field, which `Input` gives the bar's
  keys, `Cursor`, `LineCount` and `Target` to tell where it goes, and
  `Go` and `Close`.
- `CompletionMenu` has the `Items` shown, best first, and the one
  `Selected`. `Item(i)` makes the row of an item, selected as the arrows
  move, scrolled into view and inserting the item when clicked; style it
  and give it content. `Font`, `LineHeight` and `Theme` show code as the
  editor does, and a `CompletionKind` has an `Icon` and the `Token` whose
  color tints it.
- `HoverCard` has the `Diagnostics` of the text hovered, the `Text` the
  app gave `ShowHover`, and `Content`, which builds what its function
  builds.

## Accessibility

Assistive technology sees a multi-line text field, named by `Label`, with
the text of the buffer, replaced ranges among it, and the newest selection,
which it moves, and where its lines in view are, so that VoiceOver moves
through it line by line and outlines the line it reads; the elements of
replacements are named by their `Label`; the
completions are a list, and the find bar a toolbar that tells the count
of matches, except in a narrow editor, where its query field still tells
an invalid regular expression. The editor's menu, its find and go to line bars and their
names are in the window's language
([MyGo's own strings](../i18n.md#mygos-own-strings)). Input methods compose in the editor as in other text fields,
with their candidate window at the caret.
