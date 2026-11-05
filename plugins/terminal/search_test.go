package terminal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/wait"
)

func searchTerminal(t *testing.T, cols, rows int) *Terminal {
	t.Helper()
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	term.Resize(cols, rows)
	t.Cleanup(func() { term.Close() })
	return term
}

// settledSearch drives the search as a view's frames do, until it has gone
// through the scrollback.
func settledSearch(t *testing.T, term *Terminal) SearchStatus {
	t.Helper()
	wait.Until(t, "search to complete", func() bool {
		term.mu.Lock()
		defer term.mu.Unlock()
		return term.term == nil || !term.find.drive(term.term)
	})
	return term.SearchStatus()
}

func TestSearchHistoryNavigationAndLiveOutput(t *testing.T) {
	term := searchTerminal(t, 32, 4)
	for i := range 80 {
		term.Feed([]byte(fmt.Sprintf("row %02d needle\r\n", i)))
	}
	before := term.Text()
	term.Search("NEEDLE")
	if s := settledSearch(t, term); s.Total != 80 || s.Selected != 1 {
		t.Fatalf("initial search %+v", s)
	}
	for range 79 {
		term.SearchNext()
	}
	if s := settledSearch(t, term); s.Selected != 80 {
		t.Fatalf("oldest match %+v", s)
	}
	if sb := term.term.Scrollbar(); sb.Offset != 0 {
		t.Fatalf("oldest match did not scroll into view: %+v", sb)
	}
	term.SearchNext()
	if s := settledSearch(t, term); s.Selected != 1 {
		t.Fatalf("next did not wrap: %+v", s)
	}
	term.SearchPrevious()
	if s := settledSearch(t, term); s.Selected != 80 {
		t.Fatalf("previous did not wrap: %+v", s)
	}
	term.Feed([]byte("another needle\r\n"))
	if s := settledSearch(t, term); s.Total != 81 {
		t.Fatalf("live output missing: %+v", s)
	}
	if !strings.HasPrefix(term.Text(), before) {
		t.Fatal("search changed terminal contents")
	}
	term.Search("")
	if term.find.s != nil || term.SearchStatus() != (SearchStatus{}) {
		t.Fatal("ending the search kept its state")
	}
}

func TestSearchWrappedUnicodeAndResize(t *testing.T) {
	term := searchTerminal(t, 10, 5)
	term.Feed([]byte("12345678中文搜索🙂abcd\r\n"))
	term.Search("中文搜索🙂")
	if s := settledSearch(t, term); s.Total != 1 || s.Selected != 1 {
		t.Fatalf("Unicode match %+v", s)
	}
	term.mu.Lock()
	spans, _ := term.viewportMatches(nil, nil, 10, 5)
	term.mu.Unlock()
	if len(spans) != 2 || spans[0].x0 != 8 || !spans[0].selected {
		t.Fatalf("wrapped highlight %+v", spans)
	}
	term.Resize(24, 5)
	if s := settledSearch(t, term); s.Total != 1 {
		t.Fatalf("search lost after reflow %+v", s)
	}
	// A reflow keeps the match but not its selection; the next one selects it.
	term.SearchNext()
	if s := settledSearch(t, term); s.Selected != 1 {
		t.Fatalf("match not selectable after reflow %+v", s)
	}
	term.Search("missing")
	if s := settledSearch(t, term); s.Total != 0 || s.Selected != 0 {
		t.Fatalf("stale match %+v", s)
	}
	term.Search("abcd")
	if s := settledSearch(t, term); s.Total != 1 {
		t.Fatalf("replacement query %+v", s)
	}
}

func TestSearchClearAndAlternateScreen(t *testing.T) {
	term := searchTerminal(t, 40, 4)
	term.Feed([]byte("primary target\r\n"))
	term.Search("target")
	if s := settledSearch(t, term); s.Total != 1 {
		t.Fatalf("%+v", s)
	}
	term.Feed([]byte("\x1b[?1049hfullscreen target target"))
	if s := settledSearch(t, term); s.Total != 2 {
		t.Fatalf("alternate screen %+v", s)
	}
	term.Feed([]byte("\x1b[?1049l"))
	if s := settledSearch(t, term); s.Total != 1 {
		t.Fatalf("primary screen %+v", s)
	}
	term.Feed([]byte("\x1b[H\x1b[2J\x1b[3J"))
	if s := settledSearch(t, term); s.Total != 0 {
		t.Fatalf("cleared content still matched %+v", s)
	}
	term.mu.Lock()
	_, selected := term.term.SelectionText()
	term.mu.Unlock()
	if selected {
		t.Fatal("search overwrote the terminal's normal selection")
	}
}

// A match wrapping through more rows than the viewport shows takes the
// whole of each row it shows on.
func TestSearchHighlightsClipWrappedMatch(t *testing.T) {
	term := searchTerminal(t, 5, 2)
	term.Feed([]byte("abcdefghijklmnopqrst\r\n"))
	term.Search("abcdefghijklmnopqrst")
	settledSearch(t, term)
	term.mu.Lock()
	term.term.ScrollToRow(1)
	spans, _ := term.viewportMatches(nil, nil, 5, 2)
	term.mu.Unlock()
	if len(spans) != 2 {
		t.Fatalf("partial viewport match %+v", spans)
	}
	for _, s := range spans {
		if s.x0 != 0 || s.x1 != 4 {
			t.Fatalf("partial match %+v", s)
		}
	}
}
