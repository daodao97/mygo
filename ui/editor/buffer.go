package editor

import (
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/internal/diff"
	"github.com/egoist/mygo/internal/rope"
	"github.com/egoist/mygo/ui"
)

// Buffer is the text an editor edits, with its undo history and its
// language. Several editors, and both sides of a DiffView, can show one
// buffer: an edit in one shows in the others, their carets and
// selections moving with the text around them, and undo takes back the
// buffer's last edit, whichever editor made it, as in Zed.
//
// The text is a persistent tree of chunks: an edit copies a few
// kilobytes and a path of the tree, whatever the size of the document,
// and each version shares the rest with the one before, so frames and
// other goroutines read a version (Snapshot) without locks. Undo history
// keeps the edits that take each step back, which EditRemote moves past
// edits made elsewhere. Offsets count bytes of UTF-8, and a Pos a line
// and the bytes into it.
//
// Lines end in \n, as in Zed: a text set with \r\n or \r line endings
// keeps \n ones, and the buffer remembers the ending each line had, which
// WriteTo writes back, so that a file opened and saved stays as it was,
// mixed endings and all, and an edit changes the endings of only the
// lines it breaks or joins. LineEnding says which ending most lines have.
//
// Methods are safe from any goroutine; schedule changes with
// Window.Update so the window shows them.
type Buffer struct {
	mu      sync.Mutex
	text    rope.Text
	version uint64
	// log is the changes of the last versions, and the text each made,
	// which editors, the highlighter and diffs follow to keep their state
	// of the text.
	log      []logEntry
	language *Language
	saved    rope.Text
	undo     []transaction
	redo     []transaction
	// open counts the Transact calls running, which group edits into
	// openTx.
	open     int
	openTx   *transaction
	lastEdit time.Time
	// ends are the line endings of text, and savedEnds of saved.
	ends, savedEnds lineEnds
	syntax          *syntaxState
	// restarts are the lines that start in state 0 in the default
	// highlighter, the first lines of a patch's hunks (setRestarts),
	// which each version moves (record) into a new slice, as snapshots
	// share them; restartsText is the text they are of.
	restarts     []int
	restartsText rope.Text
	// wake are the windows showing the buffer, which a highlighter
	// working in the background has draw again.
	wake []ui.Services
}

// Edit replaces the bytes Start to End of a Buffer's text with Text.
type Edit struct {
	Start, End int
	Text       string
}

// Pos is a place in a text: a line, from 0, and a column in bytes from
// the start of the line.
type Pos struct{ Line, Col int }

// Less reports whether p comes before q.
func (p Pos) Less(q Pos) bool { return p.Line < q.Line || p.Line == q.Line && p.Col < q.Col }

// change is a range of the text an edit replaced, in bytes and lines,
// before (old) and after (new) the edit. The changes of a version are in
// order and do not overlap; newLine is oldLine moved by the lines the
// changes before it added.
type change struct {
	oldStart, oldEnd, newStart, newEnd int
	oldLine, oldEndLine                int
	newLine, newEndLine                int
}

type logEntry struct {
	version  uint64
	text     rope.Text
	ends     lineEnds
	changes  []change
	restarts []int // the buffer's restarts in text
}

// transaction is a step of undo or redo: the edits that take it back,
// on the undo stack, or make it again, on the redo stack, and where the
// editor making it had its selections before and after it.
type transaction struct {
	// edits are a list for each version the step made, applied from the
	// last: each list is of the text the list after it makes, as Edit
	// takes them. Undo and redo log a version for each list, so that each
	// entry's text is its changes' (as tree-sitter needs), and keep the
	// edits taking each back, in the order they applied them, as the step
	// that makes it again (applyStep). Being edits rather than texts, the
	// steps move past an edit outside the history (EditRemote, rebase).
	edits [][]Edit
	// ends are the line ending codes of the edits' line breaks, a string
	// for each edit of edits, which put back the endings the step took
	// away.
	ends      [][]string
	owner     any
	selBefore any
	selAfter  any
	kind      editKind
}

// editKind groups edits into undo steps: typing in a row is one step,
// as Zed groups edits within 300 ms.
type editKind uint8

const (
	editOther editKind = iota
	editTyping
	editDeleting
	// editCompose is the text of an input method's composition and its
	// commit, which make one step however long they take.
	editCompose
)

const (
	logMax       = 256
	undoMax      = 10000
	groupTimeout = 300 * time.Millisecond
)

// NewBuffer returns a buffer holding text, which is its saved text.
func NewBuffer(text string) *Buffer {
	t, ends := newLineEnds(text)
	b := &Buffer{text: rope.New(t), ends: ends}
	b.saved, b.savedEnds = b.text, b.ends
	return b
}

// LineEnding returns the line ending most of the text's lines have, \r\n,
// \r or \n, the first line's on a tie, and \n for a text of one line.
// WriteTo writes each line with its own ending.
func (b *Buffer) LineEnding() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ends.dominant()
}

// MixedLineEndings reports whether the text's line breaks don't all have
// the same ending, some \r\n and some \n, say; a text with one line, or
// with every break alike, is not mixed. It reads counts the buffer keeps,
// as quick for a large text as for a small one. With LineEnding, it tells
// an editor's status bar what to show, "Mixed" or the ending all lines
// have, and SetLineEnding converts a mixed text to one ending.
func (b *Buffer) MixedLineEndings() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ends.mixed()
}

// SetLineEnding converts the ending of every line to ending, \r\n, \r
// or \n, as an editor's command converting a file's line endings: lines
// it breaks later end so too. It is not a step of undo, and undo and redo
// make lines with it after it; the buffer is modified until saved.
func (b *Buffer) SetLineEnding(ending string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	code := endCode(ending)
	b.ends = b.ends.convert(code)
	for _, stack := range [][]transaction{b.undo, b.redo} {
		for i := range stack {
			for _, ends := range stack[i].ends {
				for j, e := range ends {
					ends[j] = strings.Repeat(string(code), len(e))
				}
			}
		}
	}
}

// snapshot returns the current text and version.
func (b *Buffer) snapshot() (rope.Text, uint64) {
	if b == nil {
		return rope.Text{}, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text, b.version
}

// Snapshot returns the text as it is now, which stays as it is while
// the buffer changes: read it from any goroutine, as to save the text or
// to send it to a language server.
func (b *Buffer) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.snapshotLocked()
}

// snapshotLocked returns the current Snapshot; b.mu is held.
func (b *Buffer) snapshotLocked() Snapshot {
	return Snapshot{t: b.text, version: b.version, ends: b.ends, restarts: b.restarts}
}

// String returns the text.
func (b *Buffer) String() string { r, _ := b.snapshot(); return r.String() }

// WriteTo writes the text to w, a chunk at a time, as for saving it, each
// line with its own line ending.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	s := b.Snapshot()
	return s.WriteTo(w)
}

// Len returns the length of the text in bytes.
func (b *Buffer) Len() int { r, _ := b.snapshot(); return r.Len() }

// LineCount returns how many lines the text has: one more than its
// newlines.
func (b *Buffer) LineCount() int { r, _ := b.snapshot(); return r.LineCount() }

// Line returns line i, without its newline.
func (b *Buffer) Line(i int) string { r, _ := b.snapshot(); return r.Line(i) }

// Slice returns the bytes start to end of the text.
func (b *Buffer) Slice(start, end int) string { r, _ := b.snapshot(); return r.Slice(start, end) }

// Pos returns the line and column of byte offset.
func (b *Buffer) Pos(offset int) Pos { r, _ := b.snapshot(); return posOf(r, offset) }

// Offset returns the byte offset of p, its column clamped to its line.
func (b *Buffer) Offset(p Pos) int { r, _ := b.snapshot(); return offsetOf(r, p) }

// Version returns a number that changes with every change of the text.
func (b *Buffer) Version() uint64 { _, v := b.snapshot(); return v }

// Language returns the buffer's language, nil for plain text.
func (b *Buffer) Language() *Language {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.language
}

// SetLanguage sets the language that highlights the text, and that
// comments, brackets and indentation follow; nil is plain text.
func (b *Buffer) SetLanguage(l *Language) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.language != l {
		b.language = l
		b.syntax = nil
	}
}

// Modified reports whether the text, or its line endings, differ from
// the saved one: the text the buffer was made or Reset with, or had when
// MarkSaved was called. Undoing back to it makes the buffer unmodified
// again.
func (b *Buffer) Modified() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !sameText(b.text, b.saved) || !sameText(b.ends.codes, b.savedEnds.codes)
}

// sameText reports whether a and b hold the same bytes, comparing them
// a chunk at a time rather than copying them out.
func sameText(a, b rope.Text) bool {
	if a.Same(b) {
		return true
	}
	if a.Len() != b.Len() {
		return false
	}
	same, off := true, 0
	a.Chunks(0, a.Len(), func(s string) bool {
		n := len(s)
		b.Chunks(off, off+n, func(t string) bool {
			same = s[:len(t)] == t
			s = s[len(t):]
			return same
		})
		off += n
		return same
	})
	return same
}

// MarkSaved makes the current text the saved one.
func (b *Buffer) MarkSaved() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.saved, b.savedEnds = b.text, b.ends
}

// Reset replaces the text and forgets the undo history, as for a file
// loaded anew, and makes it the saved text.
func (b *Buffer) Reset(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	old := b.text
	t, ends := newLineEnds(text)
	b.text, b.ends = rope.New(t), ends
	b.saved, b.savedEnds = b.text, b.ends
	b.undo, b.redo, b.openTx = nil, nil, nil
	b.record([]change{wholeChange(old, b.text)})
}

// Set replaces the whole text as one edit, which Undo takes back.
func (b *Buffer) Set(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.edit([]Edit{{Start: 0, End: b.text.Len(), Text: text}}, editOther, nil, nil)
}

// Edit applies edits to the text as one step of undo. Their offsets are
// in the text before any of them; overlapping edits are cut to follow
// the one before.
func (b *Buffer) Edit(edits ...Edit) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.edit(edits, editOther, nil, nil)
}

// Transact groups the edits fn makes, through Edit and the editors
// showing the buffer, into one step of undo.
func (b *Buffer) Transact(fn func()) {
	b.mu.Lock()
	b.open++
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.open--
		if b.open == 0 {
			b.openTx = nil
		}
		b.mu.Unlock()
	}()
	fn()
}

// CanUndo and CanRedo report whether Undo and Redo have a step to take.
func (b *Buffer) CanUndo() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.undo) > 0
}

func (b *Buffer) CanRedo() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.redo) > 0
}

// Undo takes back the last step of edits, and reports whether there was
// one.
func (b *Buffer) Undo() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.undoStep()
	return ok
}

// Redo makes the step Undo took back again, and reports whether there
// was one.
func (b *Buffer) Redo() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.redoStep()
	return ok
}

// undoStep takes back the last transaction; b.mu is held.
func (b *Buffer) undoStep() (transaction, bool) {
	n := len(b.undo)
	if n == 0 {
		return transaction{}, false
	}
	tx := b.undo[n-1]
	b.undo = b.undo[:n-1]
	b.openTx = nil
	b.lastEdit = time.Time{}
	redo := tx
	redo.edits, redo.ends = b.applyStep(tx.edits, tx.ends)
	b.redo = append(b.redo, redo)
	return tx, true
}

// redoStep makes the last transaction undone again; b.mu is held.
func (b *Buffer) redoStep() (transaction, bool) {
	n := len(b.redo)
	if n == 0 {
		return transaction{}, false
	}
	tx := b.redo[n-1]
	b.redo = b.redo[:n-1]
	b.openTx = nil
	b.lastEdit = time.Time{}
	undo := tx
	undo.edits, undo.ends = b.applyStep(tx.edits, tx.ends)
	b.undo = append(b.undo, undo)
	return tx, true
}

// applyStep applies the edits of a step, from the last list, each list
// a version, with the line endings of their breaks, and returns the edits
// taking them back, in the order it applied them, and their endings: the
// step that makes it again. b.mu is held.
func (b *Buffer) applyStep(edits [][]Edit, ends [][]string) ([][]Edit, [][]string) {
	back := make([][]Edit, 0, len(edits))
	backEnds := make([][]string, 0, len(edits))
	for i := len(edits) - 1; i >= 0; i-- {
		before, beforeEnds := b.text, b.ends
		var codes []string
		if i < len(ends) {
			codes = ends[i]
		}
		text, newEnds, changes, kept := b.replace(edits[i], codes)
		if changes == nil {
			continue
		}
		b.text, b.ends = text, newEnds
		b.record(changes)
		e, c := inverse(before, beforeEnds, changes, kept)
		back, backEnds = append(back, e), append(backEnds, c)
	}
	return back, backEnds
}

// inverse returns the edits taking changes back, of the text they made
// of before, and the line ending codes of their breaks, of ends, before's
// line endings, as replace takes them: each followed by the code kept
// says the break after the change had, when it changed it.
func inverse(before rope.Text, ends lineEnds, changes []change, kept []byte) ([]Edit, []string) {
	out := make([]Edit, len(changes))
	codes := make([]string, len(changes))
	for i, c := range changes {
		out[i] = Edit{Start: c.newStart, End: c.newEnd, Text: before.Slice(c.oldStart, c.oldEnd)}
		if c.oldEndLine > c.oldLine {
			codes[i] = ends.codes.Slice(c.oldLine, c.oldEndLine)
		}
		if kept != nil && kept[i] != 0 {
			codes[i] += string(kept[i])
		}
	}
	return out, codes
}

// EditRemote applies edits made outside the buffer's history, as
// another device's or the result of a merge, which undo does not take
// back: undo and redo take back and make again the buffer's own steps
// as if they had been made on the text with these edits, moving them
// past the edits as collaborative editors do (rebase). Their offsets
// are as Edit's. A step whose edits overlap them can no longer be taken
// back or made again alone and leaves the history, the text staying as
// it is; the steps around it stay, and the redo steps too, which a local
// edit drops. Pass the ranges that changed rather than the whole text,
// which overlaps every step: a diff of the old and new texts gives them.
//
// Carets, selections, folds, diagnostics and inlays move through the
// edits as through any edit, and Modified still compares with the saved
// text: call MarkSaved when the text is what was saved, as edits read
// from the file on disk.
func (b *Buffer) EditRemote(edits ...Edit) {
	b.mu.Lock()
	defer b.mu.Unlock()
	text, ends, changes, _ := b.replace(edits, nil)
	if changes == nil {
		return
	}
	b.text, b.ends = text, ends
	b.record(changes)
	c := make([]span, len(changes))
	for i, ch := range changes {
		c[i] = span{ch.oldStart, ch.oldEnd, ch.newEnd - ch.newStart}
	}
	var top bool
	b.undo, top = rebase(b.undo, c, false)
	b.redo, _ = rebase(b.redo, c, true)
	if !top {
		// The step edits were going into left: the next makes its own,
		// rather than joining the one under it.
		b.openTx = nil
		b.lastEdit = time.Time{}
	} else if b.openTx != nil {
		b.openTx = &b.undo[len(b.undo)-1]
	}
}

// edit applies edits as a transaction, or into the open one; b.mu is
// held. It returns the changes, nil when nothing changed.
func (b *Buffer) edit(edits []Edit, kind editKind, owner, selBefore any) []change {
	return b.editEnds(edits, nil, kind, owner, selBefore)
}

// editEnds is edit, with the line ending codes of the edits' breaks as
// replace takes them, nil for those lineEnds.made gives.
func (b *Buffer) editEnds(edits []Edit, codes []string, kind editKind, owner, selBefore any) []change {
	before, beforeEnds := b.text, b.ends
	text, ends, changes, kept := b.replace(edits, codes)
	if changes == nil {
		return nil
	}
	b.text, b.ends = text, ends
	b.record(changes)
	b.redo = nil
	back, backEnds := inverse(before, beforeEnds, changes, kept)
	now := time.Now()
	switch tx := b.openTx; {
	case tx != nil:
		tx.edits = append(tx.edits, back)
		tx.ends = append(tx.ends, backEnds)
	case b.open == 0 && kind != editOther && len(b.undo) > 0 && b.undo[len(b.undo)-1].kind == kind &&
		b.undo[len(b.undo)-1].owner == owner && (now.Sub(b.lastEdit) < groupTimeout || kind == editCompose && !b.lastEdit.IsZero()):
		tx := &b.undo[len(b.undo)-1]
		tx.edits = append(tx.edits, back)
		tx.ends = append(tx.ends, backEnds)
	default:
		b.undo = append(b.undo, transaction{edits: [][]Edit{back}, ends: [][]string{backEnds}, owner: owner, selBefore: selBefore, kind: kind})
		if len(b.undo) > undoMax {
			b.undo = slices.Delete(b.undo, 0, len(b.undo)-undoMax)
		}
		if b.open > 0 {
			b.openTx = &b.undo[len(b.undo)-1]
		}
	}
	b.lastEdit = now
	return changes
}

// replace returns the text edits make of the buffer's, its line endings
// and their changes, nil when they change nothing, recording nothing;
// b.mu is held. The edits are as Edit takes them. codes, when not nil,
// are the line ending codes of each edit's line breaks, as undo and redo
// put back, and maybe one more, the code of the break after the edit's
// text, which ends its last line; without them the breaks end as
// lineEnds.made says, whatever their text has, \r\n, \r or \n.
//
// kept, nil when none, is for each change the code the break after it had
// when the edits changed it (fixCR, or codes' one more), else 0: what
// inverse puts back.
func (b *Buffer) replace(edits []Edit, codes []string) (_ rope.Text, _ lineEnds, _ []change, kept []byte) {
	if len(edits) == 0 {
		return b.text, b.ends, nil, nil
	}
	if !slices.IsSortedFunc(edits, func(x, y Edit) int { return x.Start - y.Start }) {
		// Undo's edits, which come with codes, are in order.
		edits = slices.Clone(edits)
		slices.SortStableFunc(edits, func(x, y Edit) int { return x.Start - y.Start })
		codes = nil
	}
	cloned := false
	var own []string // the codes of the edits' texts, "" for all \n
	for i, e := range edits {
		if strings.IndexByte(e.Text, '\r') >= 0 {
			if !cloned {
				edits, cloned = slices.Clone(edits), true
				own = make([]string, len(edits))
			}
			edits[i].Text, own[i] = readEnds(e.Text)
		}
	}
	before, beforeEnds := b.text, b.ends
	text, ends := before, beforeEnds
	changes := make([]change, 0, len(edits))
	var after []byte // codes' one more of each change, 0 for none
	delta, lineDelta, last := 0, 0, 0
	for i, e := range edits {
		start := max(last, min(e.Start, before.Len()))
		end := max(start, min(e.End, before.Len()))
		last = end
		if start == end && e.Text == "" || end-start == len(e.Text) && before.Slice(start, end) == e.Text {
			continue
		}
		c := change{oldStart: start, oldEnd: end}
		c.oldLine = before.LineOf(start)
		c.oldEndLine = c.oldLine
		if end > start {
			c.oldEndLine = before.LineOf(end)
		}
		c.newStart = start + delta
		c.newEnd = c.newStart + len(e.Text)
		c.newLine = c.oldLine + lineDelta
		c.newEndLine = c.newLine + strings.Count(e.Text, "\n")
		text = text.Replace(c.newStart, c.newStart+end-start, e.Text)
		// The line breaks the edit takes away are those of its old lines,
		// and those it puts in end its new ones: the break ending its last
		// line stays as it is.
		if m, k := c.oldEndLine-c.oldLine, c.newEndLine-c.newLine; m > 0 || k > 0 {
			var add string
			if codes != nil && i < len(codes) && (len(codes[i]) == k || len(codes[i]) == k+1) {
				add = codes[i][:k]
			} else {
				var o string
				if own != nil {
					o = own[i]
				}
				add = beforeEnds.made(c.oldLine, c.oldEndLine, k, o)
			}
			ends = ends.replace(c.newLine, c.newLine+m, add)
		}
		var more byte
		if codes != nil && i < len(codes) && len(codes[i]) == c.newEndLine-c.newLine+1 {
			more = codes[i][len(codes[i])-1]
		}
		after = append(after, more)
		delta += len(e.Text) - (end - start)
		lineDelta += (c.newEndLine - c.newLine) - (c.oldEndLine - c.oldLine)
		changes = append(changes, c)
	}
	if len(changes) == 0 {
		return before, beforeEnds, nil, nil
	}
	// The break after a change is its own when the next change starts on
	// a line after it: the last change on a line has it.
	owns := func(i int) bool {
		c := changes[i]
		return (i+1 == len(changes) || changes[i+1].oldLine > c.oldEndLine) &&
			c.oldEndLine < beforeEnds.codes.Len() && c.newEndLine < ends.codes.Len()
	}
	for i, c := range changes {
		if after[i] != 0 && owns(i) && ends.at(c.newEndLine) != after[i] {
			ends = ends.replace(c.newEndLine, c.newEndLine+1, string(after[i]))
		}
	}
	for _, c := range changes {
		ends = fixCR(text, ends, c.newLine-1, c.newEndLine)
	}
	for i, c := range changes {
		if owns(i) {
			if was := beforeEnds.at(c.oldEndLine); ends.at(c.newEndLine) != was {
				if kept == nil {
					kept = make([]byte, len(changes))
				}
				kept[i] = was
			}
		}
	}
	return text, ends, changes, kept
}

// endGroup ends the group of typing, so that the next edit makes a step
// of undo of its own, as moving the caret does.
func (b *Buffer) endGroup() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastEdit = time.Time{}
}

// setSelectionsAfter notes where the owner of the last transaction has
// its selections after it.
func (b *Buffer) setSelectionsAfter(owner, sel any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n := len(b.undo); n > 0 && b.undo[n-1].owner == owner {
		b.undo[n-1].selAfter = sel
	}
}

// record logs the changes of a new version, made of the current text;
// b.mu is held.
func (b *Buffer) record(changes []change) {
	b.version++
	if len(b.restarts) > 0 {
		b.restarts = moveRestarts(b.restarts, Snapshot{t: b.restartsText}, Snapshot{t: b.text}, publicChanges(changes))
		b.restartsText = b.text
	}
	b.log = append(b.log, logEntry{version: b.version, text: b.text, ends: b.ends, changes: changes, restarts: b.restarts})
	if len(b.log) > logMax {
		b.log = slices.Delete(b.log, 0, len(b.log)-logMax/2)
	}
}

// changesSince returns the changes of each version after since, the
// version they lead to, and false when the log no longer goes back that
// far.
func (b *Buffer) changesSince(since uint64) ([][]change, uint64, bool) {
	entries, version, ok := b.entriesSince(since)
	out := make([][]change, len(entries))
	for i, e := range entries {
		out[i] = e.changes
	}
	return out, version, ok
}

// entriesSince returns the log's entries after version since.
// changesTo returns the changes of the versions after since, the version
// they lead to and its text, read at once, so that the text is the
// changes' and not a later one's; ok false when the log no longer
// reaches back to since.
func (b *Buffer) changesTo(since uint64) (changes [][]change, version uint64, text rope.Text, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries, version, ok := b.entriesSinceLocked(since)
	changes = make([][]change, len(entries))
	for i, e := range entries {
		changes[i] = e.changes
	}
	return changes, version, b.text, ok
}

// changesBetween returns the changes from old to new by their lines, for
// an editor too far behind the log to follow the edits themselves.
func changesBetween(old, new rope.Text) []change {
	lines := func(t rope.Text) []string {
		out := make([]string, 0, t.LineCount())
		t.EachLine(0, t.LineCount(), func(_, _ int, s string) bool {
			out = append(out, s)
			return true
		})
		return out
	}
	at := func(t rope.Text, line int) int {
		if line >= t.LineCount() {
			return t.Len()
		}
		return t.LineStart(line)
	}
	var out []change
	for _, h := range diff.Lines(lines(old), lines(new)) {
		c := change{oldStart: at(old, h.A0), oldEnd: at(old, h.A1), newStart: at(new, h.B0), newEnd: at(new, h.B1)}
		c.oldLine, c.oldEndLine = old.LineOf(c.oldStart), old.LineOf(c.oldEnd)
		c.newLine, c.newEndLine = new.LineOf(c.newStart), new.LineOf(c.newEnd)
		out = append(out, c)
	}
	return out
}

func (b *Buffer) entriesSince(since uint64) ([]logEntry, uint64, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.entriesSinceLocked(since)
}

func (b *Buffer) entriesSinceLocked(since uint64) ([]logEntry, uint64, bool) {
	if since == b.version {
		return nil, b.version, true
	}
	if len(b.log) == 0 || b.log[0].version > since+1 || since > b.version {
		return nil, b.version, false
	}
	i, _ := slices.BinarySearchFunc(b.log, since+1, func(e logEntry, v uint64) int {
		switch {
		case e.version < v:
			return -1
		case e.version > v:
			return 1
		}
		return 0
	})
	return slices.Clone(b.log[i:]), b.version, true
}

// watch notes a window showing the buffer, for a highlighter working in
// the background to draw it again.
func (b *Buffer) watch(s ui.Services) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !slices.Contains(b.wake, s) {
		b.wake = append(b.wake, s)
		if len(b.wake) > 8 {
			b.wake = b.wake[1:]
		}
	}
}

// watched reports whether a window showed the buffer, which invalidate
// draws again.
func (b *Buffer) watched() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.wake) > 0
}

// invalidate draws the windows showing the buffer again, from any
// goroutine.
func (b *Buffer) invalidate() {
	b.mu.Lock()
	wake := slices.Clone(b.wake)
	b.mu.Unlock()
	for _, s := range wake {
		s.Invalidate()
	}
}

// wholeChange is the change replacing all of old with new.
func wholeChange(old, new rope.Text) change {
	return change{oldEnd: old.Len(), newEnd: new.Len(), oldEndLine: old.LineCount() - 1, newEndLine: new.LineCount() - 1}
}

// span is a range of a text an edit outside the history replaced, bytes
// start to end, and the length of what replaced it: all moving the
// history past it needs. A text's spans are in order and do not overlap.
type span struct{ start, end, n int }

// rebase moves the steps of a stack, from the last, past spans of the
// text the last applies to, and returns the steps left and whether the
// last is among them. A step whose edits overlap the spans leaves the
// stack, the text staying as it is (with an undo step's edits, without
// a redo step's): the steps below it move past it and the spans
// together. redo says the steps are of the redo stack,
// which makes a step's selections after it of the text its edits make
// rather than of the text they apply to.
func rebase(steps []transaction, c []span, redo bool) ([]transaction, bool) {
	if len(steps) == 0 {
		return steps, false
	}
	c = slices.Clone(c) // c and next take turns holding the spans
	var start, next []span
	var moved []Edit
	dropped := false
	for i := len(steps) - 1; i >= 0; i-- {
		tx := &steps[i]
		// Moved into moved, the lists from the last, and kept only if no
		// list overlaps.
		start = append(start[:0], c...)
		moved = moved[:0]
		ok := true
		for j := len(tx.edits) - 1; j >= 0 && ok; j-- {
			moved, next, ok = rebaseEdits(tx.edits[j], c, moved, next[:0])
			c, next = next, c
		}
		if !ok {
			// The spans become those of the text the step's edits make,
			// which take it back and then make what the spans make.
			c = slices.Clone(start)
			for j := len(tx.edits) - 1; j >= 0; j-- {
				c = composeSpans(madeSpans(tx.edits[j]), c)
			}
			next = nil
			tx.edits, tx.ends = nil, nil
			dropped = true
			continue
		}
		k := 0
		for j := len(tx.edits) - 1; j >= 0; j-- {
			k += copy(tx.edits[j], moved[k:])
		}
		// The selections the step's edits apply to are of start's text,
		// those they make of c's.
		at, made := &tx.selAfter, &tx.selBefore
		if redo {
			at, made = made, at
		}
		*at, *made = moveSaved(*at, start), moveSaved(*made, c)
	}
	if !dropped {
		return steps, true
	}
	top := steps[len(steps)-1].edits != nil
	return slices.DeleteFunc(steps, func(tx transaction) bool { return tx.edits == nil }), top
}

// rebaseEdits moves edits es and spans c, both of one text, past each
// other, ties putting c first: it appends to es2 the edits es make of
// the text c makes, and to c2 the spans c make of the text es makes. ok
// is false when an edit and a span overlap, one replacing bytes the
// other replaces or inserting inside them.
func rebaseEdits(es []Edit, c []span, es2 []Edit, c2 []span) (_ []Edit, _ []span, ok bool) {
	i, j, de, dc := 0, 0, 0, 0
	for i < len(es) || j < len(c) {
		switch {
		case j < len(c) && (i == len(es) || c[j].end <= es[i].Start):
			s := c[j]
			c2 = append(c2, span{s.start + dc, s.end + dc, s.n})
			de += s.n - (s.end - s.start)
			j++
		case i < len(es) && (j == len(c) || es[i].End <= c[j].start):
			e := es[i]
			es2 = append(es2, Edit{Start: e.Start + de, End: e.End + de, Text: e.Text})
			dc += len(e.Text) - (e.End - e.Start)
			i++
		default:
			return es2, c2, false
		}
	}
	return es2, c2, true
}

// madeSpans returns the spans of the text edits es make that take it
// back to the text es apply to.
func madeSpans(es []Edit) []span {
	out := make([]span, len(es))
	d := 0
	for i, e := range es {
		out[i] = span{e.Start + d, e.Start + d + len(e.Text), e.End - e.Start}
		d += len(e.Text) - (e.End - e.Start)
	}
	return out
}

// composeSpans returns the spans of a text x making what a makes of it
// and then b of that: b's spans are of the text a makes. Spans that
// overlap or touch in it make one.
func composeSpans(a, b []span) []span {
	out := make([]span, 0, len(a)+len(b))
	// da is what a's spans so far add, which places the next in a's text.
	i, j, da := 0, 0, 0
	for i < len(a) || j < len(b) {
		y0 := 0
		if i < len(a) && (j == len(b) || a[i].start+da <= b[j].start) {
			y0 = a[i].start + da
		} else {
			y0 = b[j].start
		}
		x0, y1, db := y0-da, y0, 0
		for {
			if i < len(a) && a[i].start+da <= y1 {
				y1 = max(y1, a[i].start+da+a[i].n)
				da += a[i].n - (a[i].end - a[i].start)
				i++
			} else if j < len(b) && b[j].start <= y1 {
				y1 = max(y1, b[j].end)
				db += b[j].n - (b[j].end - b[j].start)
				j++
			} else {
				break
			}
		}
		out = append(out, span{x0, y1 - da, y1 - y0 + db})
	}
	return out
}

// moveSaved moves the selections an editor saved with a step through
// spans of their text.
func moveSaved(saved any, c []span) any {
	s, ok := saved.([]sel)
	if !ok || s == nil {
		return saved
	}
	cs := make([]change, len(c))
	d := 0
	for i, x := range c {
		cs[i] = change{oldStart: x.start, oldEnd: x.end, newStart: x.start + d, newEnd: x.start + d + x.n}
		d += x.n - (x.end - x.start)
	}
	out := make([]sel, len(s))
	for i := range s {
		out[i] = mapSel(s[i], cs)
	}
	return out
}

// bias says which side of an insertion at an offset the offset moves
// to.
type bias uint8

const (
	biasLeft  bias = iota // stays before text inserted at it
	biasRight             // moves after text inserted at it
)

// mapOffset moves an offset of the text before changes to the text after
// them. An offset inside a replaced range goes to its start (left) or end
// (right).
func mapOffset(off int, changes []change, b bias) int {
	// The last change starting at or before off.
	i, _ := slices.BinarySearchFunc(changes, off, func(c change, o int) int {
		if c.oldStart <= o {
			return -1
		}
		return 1
	})
	if i == 0 {
		return off
	}
	if b == biasLeft {
		// Before every insertion at off, not only the last.
		for i > 1 && changes[i-2].oldStart == off {
			i--
		}
	}
	c := changes[i-1]
	switch {
	case off > c.oldEnd || off == c.oldEnd && c.oldEnd > c.oldStart:
		return off + c.newEnd - c.oldEnd
	case off == c.oldStart && b == biasLeft:
		return c.newStart
	case off == c.oldStart && c.oldStart == c.oldEnd:
		return c.newEnd
	case b == biasLeft:
		return c.newStart
	}
	return c.newEnd
}

// mapLine moves a line of the text before changes to the text after
// them; a line the changes replaced goes to where they start.
func mapLine(line int, changes []change) int {
	delta := 0
	for _, c := range changes {
		if line < c.oldLine {
			break
		}
		if line <= c.oldEndLine {
			if line == c.oldLine {
				return c.newLine
			}
			return min(c.newLine+(line-c.oldLine), c.newEndLine)
		}
		delta = c.newEndLine - c.oldEndLine
	}
	return line + delta
}

// posOf returns the line and column, in bytes, of byte off of t.
func posOf(t rope.Text, off int) Pos {
	off = max(0, min(off, t.Len()))
	line := t.LineOf(off)
	return Pos{Line: line, Col: off - t.LineStart(line)}
}

// offsetOf returns the byte of p in t, its column clamped to its line.
func offsetOf(t rope.Text, p Pos) int {
	if p.Line < 0 {
		return 0
	}
	if p.Line >= t.LineCount() {
		return t.Len()
	}
	a := t.LineStart(p.Line)
	return a + max(0, min(p.Col, t.LineEnd(p.Line)-a))
}

// Snapshot is a version of a buffer's text, which stays as it is while
// the buffer changes: read it from any goroutine.
type Snapshot struct {
	t        rope.Text
	version  uint64
	ends     lineEnds
	restarts []int // Restarts, never changed
}

// Version returns the buffer's version the snapshot is of.
func (s Snapshot) Version() uint64 { return s.version }

// Len, LineCount and String return the length in bytes, the number of
// lines and the text of the snapshot.
func (s Snapshot) Len() int       { return s.t.Len() }
func (s Snapshot) LineCount() int { return s.t.LineCount() }
func (s Snapshot) String() string { return s.t.String() }

// Restarts returns the lines, in order, that start as the start of a
// text does, whatever the line before ends in: the first lines of a
// patch's hunks (DiffState.SetPatch), which the edits since have moved.
// A highlighter starts each in its first state, so that a hunk ending
// inside a string or a comment does not run on into the next. It is nil
// for other texts.
func (s Snapshot) Restarts() []int { return slices.Clone(s.restarts) }

// Line returns line i, without its newline.
func (s Snapshot) Line(i int) string { return s.t.Line(i) }

// LineStart and LineEnd return the bytes line i starts and ends at,
// before its newline.
func (s Snapshot) LineStart(i int) int { return s.t.LineStart(i) }
func (s Snapshot) LineEnd(i int) int   { return s.t.LineEnd(i) }

// LineOf returns the line holding byte offset.
func (s Snapshot) LineOf(offset int) int { return s.t.LineOf(offset) }

// Pos returns the line and column of byte offset, and Offset the byte of
// a line and column.
func (s Snapshot) Pos(offset int) Pos { return posOf(s.t, offset) }
func (s Snapshot) Offset(p Pos) int   { return offsetOf(s.t, p) }

// Slice returns bytes start to end.
func (s Snapshot) Slice(start, end int) string { return s.t.Slice(start, end) }

// Chunks calls fn with bytes start to end, a piece at a time, until fn
// returns false: what a parser reads text with.
func (s Snapshot) Chunks(start, end int, fn func(text string) bool) { s.t.Chunks(start, end, fn) }

// WriteTo writes the text to w, each line with its own line ending.
func (s Snapshot) WriteTo(w io.Writer) (int64, error) { return writeText(s.t, s.ends, w) }

// Change is a range of a text an edit replaced: bytes OldStart to OldEnd
// of the text before the edit, which are bytes NewStart to NewEnd of the
// text after it. An edit's changes are in order and do not overlap, so
// NewStart is OldStart moved by what the changes before it added and
// removed. An incremental parser, as tree-sitter's, takes them from the
// last: each starts at OldStart, and ends at OldEnd before and at
// OldStart+NewEnd-NewStart after, as the changes before it have not
// moved it yet.
type Change struct {
	OldStart, OldEnd, NewStart, NewEnd int
}

func publicChanges(cs []change) []Change {
	out := make([]Change, len(cs))
	for i, c := range cs {
		out[i] = Change{c.oldStart, c.oldEnd, c.newStart, c.newEnd}
	}
	return out
}
