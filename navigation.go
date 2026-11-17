package mygo

import (
	"errors"

	"github.com/egoist/mygo/internal/platform"
)

// TabItem is a tab of Tabs: its title, its icon and the window it shows.
type TabItem struct {
	Title string
	// Symbol names an SF Symbol, as "house" or "gearshape", which iOS
	// draws, filled as the tab is chosen.
	Symbol string
	// Icon is a PNG drawn as a template, its alpha in the bar's colors:
	// Android's icon, and iOS's without a Symbol. About 24 DIPs, at the
	// screen's scale (72×72 pixels draws sharp on every phone).
	Icon []byte
	// Window is what the tab shows, a window of a page or of native UI,
	// which windows pushed over it (Window.Push) cover.
	Window *Window
}

// Tabs is a bar of tabs at the bottom of a phone's screen, the system's
// own: UIKit's tab bar on iOS, Material's bottom navigation on Android.
// Each tab shows a window, and keeps the windows pushed over it as the
// user goes from tab to tab:
//
//	home := mygo.NewWindow(mygo.WindowOptions{Title: "Home", Content: ui.View(app.home), Hidden: true})
//	web := mygo.NewWindow(mygo.WindowOptions{Title: "Web", URL: "/", Hidden: true})
//	tabs, err := mygo.NewTabs(
//		mygo.TabItem{Title: "Home", Symbol: "house", Icon: homePNG, Window: home},
//		mygo.TabItem{Title: "Web", Symbol: "globe", Icon: globePNG, Window: web},
//	)
//
// On desktops, which have no tab bar, the tabs' windows stay windows:
// Select shows and focuses one.
type Tabs struct {
	items    []TabItem
	selected int
	native   platform.TabBar
	onSelect listeners[func(int)]
}

// NewTabs shows a bar of tabs, over the windows shown before, with the
// first tab chosen. Call it once the app is ready (App.WhenReady), with
// windows made Hidden: the tabs show them.
func NewTabs(items ...TabItem) (*Tabs, error) {
	needsApp("NewTabs")
	if len(items) == 0 {
		return nil, errors.New("mygo: NewTabs needs a tab")
	}
	for _, it := range items {
		if it.Window == nil {
			return nil, errors.New("mygo: a tab needs a window")
		}
	}
	t := &Tabs{items: append([]TabItem(nil), items...)}
	var err error
	onMain(func() {
		nb, ok := backend().(platform.NavigationBackend)
		if !ok {
			// Desktops: the windows are windows; the first shows.
			t.items[0].Window.Show()
			return
		}
		tabs := make([]platform.Tab, len(t.items))
		for i, it := range t.items {
			if it.Window.native == nil {
				err = errors.New("mygo: a tab's window is closed")
				return
			}
			tabs[i] = platform.Tab{Title: it.Title, Symbol: it.Symbol, Icon: it.Icon, Window: it.Window.native}
			it.Window.shown = true
		}
		t.native, err = nb.NewTabs(tabs, tabsHandler{t})
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

type tabsHandler struct{ t *Tabs }

func (h tabsHandler) TabSelected(i int) {
	if i == h.t.selected {
		return
	}
	h.t.selected = i
	fire1(&h.t.onSelect, i)
}

// Selected returns the index of the tab chosen.
func (t *Tabs) Selected() int { return onMainValue(func() int { return t.selected }) }

// Select chooses tab i, as the user would, though without OnSelect.
func (t *Tabs) Select(i int) {
	onMain(func() {
		if i < 0 || i >= len(t.items) {
			return
		}
		t.selected = i
		if t.native != nil {
			t.native.Select(i)
		} else {
			t.items[i].Window.Show()
		}
	})
}

// SetBadge shows text on tab i, as a count of what is new there; "" takes
// it away.
func (t *Tabs) SetBadge(i int, text string) {
	onMain(func() {
		if t.native != nil && i >= 0 && i < len(t.items) {
			t.native.SetBadge(i, text)
		}
	})
}

// OnSelect calls fn on the main thread when the user chooses another tab,
// with its index.
func (t *Tabs) OnSelect(fn func(i int)) (off func()) { return t.onSelect.add(fn, false) }

// Push shows child over w, as phones go deeper into an app: the system's
// stack of screens, with a bar at the top holding child's title and a
// back button (and on iOS a swipe from the left edge) that takes it away
// again, which closes it. child is made Hidden. On desktops, which have
// no such stack, child shows as a window.
func (w *Window) Push(child *Window) {
	onMain(func() {
		if w.native == nil || child.native == nil {
			return
		}
		nb, ok := backend().(platform.NavigationBackend)
		if !ok {
			child.native.Show()
			child.shown = true
			return
		}
		child.shown = true
		nb.PushWindow(w.native, child.native, true)
	})
}

// Pop takes a window Push showed off its stack, as its back button does,
// which closes it.
func (w *Window) Pop() {
	onMain(func() {
		if w.native == nil {
			return
		}
		if nb, ok := backend().(platform.NavigationBackend); ok {
			nb.PopWindow(w.native, true)
			return
		}
		w.destroy()
	})
}

// Popped closes a window the user took off its stack.
func (h *windowHandler) Popped() { h.w.destroy() }
