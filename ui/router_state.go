package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"slices"
	"strings"
)

// routerSnapshot is the persistent navigation state, separate from view,
// element, focus and animation state. Include forward history and the cursor
// so saving after Back does not change what Forward does after relaunch.
type routerSnapshot struct {
	Version int      `json:"version"`
	Entries []string `json:"entries"`
	Index   int      `json:"index"`
}

// MarshalJSON saves the full history and current index. A Router can be a
// field in state registered with mygo.PersistState; no history synchronization
// or manual replay is needed. View state and configuration are not serialized.
// Call on the UI thread, as with the other Router methods.
func (r *Router) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	r.current()
	s := routerSnapshot{Version: 1, Index: r.at, Entries: make([]string, len(r.entries))}
	for i, e := range r.entries {
		s.Entries[i] = e.loc
	}
	return json.Marshal(s)
}

// UnmarshalJSON restores navigation atomically, including forward history.
// A fresh Router uses NewRouter's defaults; an initialized Router keeps its
// configuration and window attachment. Editing and scroll values should be
// persisted separately. Legacy arrays from History() are also accepted, with
// the last location current. Invalid snapshots leave the Router unchanged.
// Shared layouts are rediscovered by Route.View on the first build, including
// for older snapshots; no layout registry or persisted element state is needed.
func (r *Router) UnmarshalJSON(data []byte) error {
	var s routerSnapshot
	if data = bytes.TrimSpace(data); len(data) > 0 && data[0] == '[' {
		if err := json.Unmarshal(data, &s.Entries); err != nil {
			return err
		}
		s.Version, s.Index = 1, len(s.Entries)-1
	} else if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s.Version != 1 {
		return fmt.Errorf("ui: unsupported router state version %d", s.Version)
	}
	if len(s.Entries) == 0 || len(s.Entries) > maxHistory || s.Index < 0 || s.Index >= len(s.Entries) {
		return fmt.Errorf("ui: invalid router history length %d or index %d", len(s.Entries), s.Index)
	}
	next := Router{InteractiveBack: runtime.GOOS == "ios", pages: r.pages, rt: r.rt, view: r.view}
	if len(r.entries) > 0 || r.rt != nil {
		next.Transition, next.InteractiveBack = r.Transition, r.InteractiveBack
	}
	for _, location := range s.Entries {
		u, err := url.Parse(location)
		if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Fragment != "" || !strings.HasPrefix(location, "/") {
			return fmt.Errorf("ui: invalid router location %q", location)
		}
		loc, p, ok := next.resolve(location)
		if !ok || loc != location {
			return fmt.Errorf("ui: non-canonical router location %q", location)
		}
		e := &routeEntry{loc: loc, restoring: true}
		if len(next.entries) > 0 {
			e.pages, e.layouts = shared(next.entries[len(next.entries)-1], p)
		}
		next.pageAt(e, 0)
		next.entries = append(next.entries, e)
	}
	next.at = s.Index
	*r = next
	r.changed()
	return nil
}

// restoreSharing reconstructs page identities from the views the app builds,
// so older location-only snapshots retain the same shared layouts as Push.
// A nonnegative layout is the path prefix consumed by Route.View; -1 shares
// only an identical full path (query changes). History branches remain
// separate: sharing stops at a different prefix or enclosing page identity.
func (r *Router) restoreSharing(e *routeEntry, level, layout int) {
	at := slices.Index(r.entries, e)
	if at < 0 {
		return // the restore loop has not inserted this entry yet
	}
	path := pathOf(e.loc)
	parts := pathParts(path)
	for _, step := range []int{-1, 1} {
		for i := at + step; i >= 0 && i < len(r.entries); i += step {
			n := r.entries[i]
			if level > 0 && (len(n.pages) < level || n.pages[level-1] != e.pages[level-1]) {
				break
			}
			other := pathOf(n.loc)
			if layout < 0 {
				if other != path {
					break
				}
			} else {
				nparts := pathParts(other)
				if layout > len(nparts) || !slices.Equal(parts[:layout], nparts[:layout]) {
					break
				}
			}
			if !n.restoring {
				// Already-built state is authoritative. Query variants can
				// adopt it; a distinct built layout must not be overwritten.
				if layout < 0 && len(n.pages) > level {
					e.pages[level] = n.pages[level]
				} else if len(n.pages) <= level || n.pages[level] != e.pages[level] {
					break
				}
				continue
			}
			if len(n.pages) > level && n.pages[level] != e.pages[level] {
				n.pages = n.pages[:level]
				n.layouts = n.layouts[:min(level, len(n.layouts))]
			}
			if len(n.pages) == level {
				n.pages = append(n.pages, e.pages[level])
			}
			if layout >= 0 {
				for len(n.layouts) <= level {
					n.layouts = append(n.layouts, -1)
				}
				n.layouts[level] = layout
			}
		}
	}
}
