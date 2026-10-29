# Combobox

<!-- preview combobox 280 -->

`ui.Combobox` creates a combobox choosing a `*string` among options: a
field showing the choice, whose text filters the options below it as the
user types, those starting with it first, while a click or Down shows them
all; a click showing them selects the choice, for typing to replace. Up
and Down move among them and Enter chooses, as does a click; Escape closes
them, and the text goes back to the choice when the field loses the focus.
`Changed` reports a new choice.

```go
ui.Combobox(c, &app.font, fonts).Label("Font")
```

The field keeps the keyboard focus while the user types and picks.
`Placeholder` shows a text in it while its text is empty.

For a text that may be anything, with suggestions, use
[autocomplete](autocomplete.md); for a short list without typing, a
[select](select.md).

## Without a look

`ui.ComboboxBase` is a combobox without a look: an `Input` whose text
filters the `Item`s of a `Popup` below it, which the arrows highlight and
Enter or a click chooses (`Chosen`): see [custom widgets](custom-widgets.md).

## Accessibility

Assistive technology sees a combo box named by its `Label`, editable as a
text field, which expands; the option the arrows are on has the focus for
it, and reads as which of how many it is.

## Options in the layout

`ComboboxParts.Inline` builds the options in the normal layout instead of a
floating popup, for a command palette or search panel. It shares `Item`,
`Highlighted` and `Chosen` with `Popup`: arrows and Enter choose, a click
keeps the field's focus, and the highlight scrolls into view only when it
changes. The first option is highlighted initially and after typing. Escape
is left to the containing dialog. An empty panel remains visible so it can
show an empty-state message.

```go
choices := ui.ComboboxBase(c, &app.query)
choices.Input.Label("Search commands").AutoFocus()
choices.Inline(func(panel ui.Element) {
    choices.Input.Changed() // apply pending input before filtering
    ui.Scroll(c).MaxHeight(320).Children(func() {
        for _, command := range app.matchingCommands() {
            row := choices.Item(command.ID)
            if row.Highlighted() {
                row.Background(c.Theme().Accent)
            }
            row.Children(func() { ui.Text(c, command.Title) })
        }
    })
})
if id, ok := choices.Chosen(); ok {
    app.runCommand(id)
}
```

Call either `Inline` or `Popup` once per build, then `Chosen`. Applications
own filtering and what a choice does; the component owns input navigation,
option highlighting, scrolling and accessibility.
