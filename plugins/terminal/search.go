package terminal

import (
	"time"

	"github.com/egoist/mygo/plugins/terminal/internal/vt"
)

// SearchStatus is how a search of the terminal goes (Terminal.Search,
// Options.OnSearch).
type SearchStatus struct {
	// Text is what the search looks for: "" when none goes.
	Text string
	// Total counts the matches found so far, on the screen that shows: a
	// full-screen program's, or the main screen's with its scrollback.
	// Selected is the selected match's place among them, 1 the newest, as
	// a find bar's "1 of 3"; 0 when none is.
	Total, Selected int
	// Searching tells that the search has more of the scrollback to go
	// through: Total may grow.
	Searching bool
}

// search is the terminal's search, while one goes; t.mu guards it.
type search struct {
	s *vt.Search
	// pick selects the newest match once one is found, as a search
	// starts.
	pick   bool
	status SearchStatus
	rects  []vt.MatchRect
}

// searchBudget bounds what a frame spends going through the scrollback; a
// search of more goes on in the frames after it.
const searchBudget = 4 * time.Millisecond

// Search searches the screen and the scrollback for text, as Ghostty's
// find does, and highlights the matches (Theme.Search) as they are found,
// following the output that comes; the newest match is selected and
// shown, scrolling to it. Matching is exact but for the case of ASCII
// letters, which it ignores. Searching for the text searched already keeps
// the selected match; "" ends the search, as EndSearch. A view drives the
// search, which goes on in the frames that show the terminal.
func (t *Terminal) Search(text string) { _ = t.SetSearch(text) }

// SetSearch is the error-reporting form of Search, kept for existing callers.
func (t *Terminal) SetSearch(text string) error {
	if text == "" {
		t.EndSearch()
		return nil
	}
	t.mu.Lock()
	if t.term == nil {
		t.mu.Unlock()
		return nil
	}
	f := &t.find
	if f.s == nil {
		native, err := vt.NewSearch(t.term)
		if err != nil {
			t.mu.Unlock()
			return err
		}
		f.s = native
	}
	t.search = f.s
	if f.status.Text != text {
		f.s.SetText(text)
		f.status, f.pick = SearchStatus{Text: text, Searching: true}, true
		t.searchMoves = 0
	}
	t.mu.Unlock()
	t.redraw()
	return nil
}

// SearchNext selects the next match of the search toward older output, up
// the screen and into the scrollback, wrapping around past the oldest, and
// unless it shows scrolls it to the middle of the view, clear of a find bar
// over an edge; SearchPrevious goes the other way.
func (t *Terminal) SearchNext(direction ...int) {
	if len(direction) == 0 {
		t.selectMatch(false)
		return
	}
	t.mu.Lock()
	if t.find.s != nil {
		if direction[0] < 0 {
			t.searchMoves--
		} else {
			t.searchMoves++
		}
	}
	t.mu.Unlock()
	t.redraw()
}
func (t *Terminal) SearchPrevious() { t.selectMatch(true) }

func (t *Terminal) selectMatch(back bool) {
	t.mu.Lock()
	if f := &t.find; f.s != nil && t.term != nil {
		// A match picked by hand is the one to keep.
		f.pick = false
		f.s.Select(back)
		f.show(t.term)
		f.update()
	}
	t.mu.Unlock()
	t.redraw()
}

// EndSearch ends the search, and its highlights.
func (t *Terminal) EndSearch() {
	t.mu.Lock()
	ended := t.find.s != nil
	t.endSearch()
	t.mu.Unlock()
	if ended {
		t.redraw()
	}
}

// endSearch frees the search; t.mu is held.
func (t *Terminal) endSearch() {
	if t.find.s != nil {
		t.find.s.Free()
	}
	t.find = search{}
	t.search = nil
	t.searchMoves = 0
}

// SearchStatus returns how the search goes.
func (t *Terminal) SearchStatus() SearchStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.find.status
}

// show scrolls the selected match to the middle of the viewport unless
// all of it shows, as browsers do, rather than to an edge, where an app's
// find bar may cover it; t.mu is held.
func (f *search) show(term *vt.Terminal) {
	m, ok := f.s.SelectedMatch()
	if !ok {
		return
	}
	a, b := m.Ends()
	_, rows := term.Size()
	_, y0, ok0 := term.ViewportPoint(a)
	_, y1, ok1 := term.ViewportPoint(b)
	if ok0 && ok1 && y0 < rows && y1 < rows {
		return
	}
	if y, ok := term.ScreenRow(a); ok {
		term.ScrollToRow(y - rows/2)
	}
}

// drive goes on with the search for a frame, and reports whether it has
// more to go through; t.mu is held.
func (f *search) drive(term *vt.Terminal) bool {
	if f.s == nil {
		return false
	}
	f.s.Feed()
	deadline := time.Now().Add(searchBudget)
	for st := f.s.Tick(); st != vt.SearchComplete; st = f.s.Tick() {
		if st == vt.SearchFeedRequired {
			f.s.Feed()
		}
		if time.Now().After(deadline) {
			break
		}
	}
	if f.pick && f.s.Total() > 0 {
		f.pick = false
		f.s.Select(false)
		f.show(term)
	}
	f.update()
	return f.status.Searching
}

// update reads the status of the search; t.mu is held.
func (f *search) update() {
	st := SearchStatus{Text: f.status.Text, Total: f.s.Total(), Searching: f.s.Status() != vt.SearchComplete}
	if i, ok := f.s.Selected(); ok {
		st.Selected = i + 1
	}
	f.status = st
}

// matchSpan is a match's cells in a row of the viewport, x0 to x1
// inclusive.
type matchSpan struct {
	y, x0, x1 int
	selected  bool
}

// viewportMatches returns the cells of the matches the viewport shows, in
// spans' memory, rows rows tall and cols wide; t.mu is held.
func (t *Terminal) viewportMatches(spans []matchSpan, buf []vt.Selection, cols, rows int) ([]matchSpan, []vt.Selection) {
	spans = spans[:0]
	f := &t.find
	if f.s == nil || f.status.Total == 0 {
		return spans, buf
	}
	f.s.Feed()
	// Share the coordinate conversion with the compatibility API: endpoints
	// name a grapheme's leading cell, so wide glyphs include their trailing cell.
	buf, f.rects = f.s.Highlights(t.term, buf, f.rects)
	for _, r := range f.rects {
		if r.Row >= 0 && r.Row < rows && r.Start < cols && r.End > 0 {
			spans = append(spans, matchSpan{r.Row, max(r.Start, 0), min(r.End, cols) - 1, r.Selected})
		}
	}
	return spans, buf
}

// SearchState is the original find-bar status, ordered newest to oldest.
type SearchState struct {
	Total, Current int
	Busy           bool
	Err            error
}

// SearchState advances a bounded amount of the search for clients without a view.
func (t *Terminal) SearchState() SearchState {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.driveSearch()
	s := t.find.status
	return SearchState{Total: s.Total, Current: s.Selected, Busy: s.Searching}
}

// driveSearch also applies legacy directional navigation once the scan is complete.
// The terminal lock is held.
func (t *Terminal) driveSearch() bool {
	f := &t.find
	if f.s == nil || t.term == nil {
		return false
	}
	busy := f.drive(t.term)
	if !busy && f.s.Total() > 0 {
		if _, ok := f.s.Selected(); !ok {
			f.s.Select(false)
			f.show(t.term)
		}
		moves := t.searchMoves % f.s.Total()
		t.searchMoves = 0
		for moves != 0 {
			back := moves < 0
			f.s.Select(back)
			if back {
				moves++
			} else {
				moves--
			}
			f.show(t.term)
		}
		f.update()
	}
	return busy
}
