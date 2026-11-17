//go:build darwin && !ios

package darwin

import (
	"runtime"
	"sync"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo/internal/platform"
)

var (
	accessClassTestOnce sync.Once
	accessNoteTestOnce  sync.Once
	accessNoteTestCall  uintptr
	accessNoteTestHook  func(id)
)

// A native accessibility notification can synchronously call back into MyGo.
// Keep one callback for its ABI, rather than leaking one per test or element.
func observeAccessNotes(t *testing.T, hook func(id)) {
	t.Helper()
	loadAccess()
	accessNoteTestOnce.Do(func() {
		accessNoteTestCall = purego.NewCallback(func(obj, _ uintptr) {
			if accessNoteTestHook != nil {
				accessNoteTestHook(id(obj))
			}
		})
	})
	previous := postAccessNote
	postAccessNote, accessNoteTestHook = accessNoteTestCall, hook
	t.Cleanup(func() { postAccessNote, accessNoteTestHook = previous, nil })
}

func accessFixture(t *testing.T) *surface {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	load()
	accessClassTestOnce.Do(registerAccessClass)
	b := &Backend{byAccess: map[id]*accessElement{}}
	s := &surface{w: &window{b: b}}
	withPool(func() { s.view = send(send(class("NSView"), "alloc"), "init") })
	previous := theBackend
	theBackend = b
	t.Cleanup(func() {
		withPool(func() {
			s.w.closed = true
			s.destroyAccess()
			release(s.view)
		})
		theBackend = previous
	})
	return s
}

func accessPair(child uint64) *platform.AccessTree {
	return &platform.AccessTree{Nodes: []platform.AccessNode{
		{ID: 1, Parent: -1, Role: platform.RoleGroup},
		{ID: child, Parent: 0, Role: platform.RoleButton, Label: "Button"},
	}}
}

func TestAccessibilityNotificationQueuesLatestTree(t *testing.T) {
	s := accessFixture(t)
	s.UpdateAccessibility(accessPair(2))
	removed, root := s.elements[2].obj, s.elements[1].obj
	called := false
	latest := accessPair(5)
	latest.Nodes[0].Controls = []uint64{5}
	observeAccessNotes(t, func(obj id) {
		if obj == removed && !called {
			called = true
			s.UpdateAccessibility(accessPair(4))
			s.UpdateAccessibility(latest)
		}
	})
	// Removing node 2 posts AXUIElementDestroyed. A nested update used to
	// remove node 3 from the outer transaction and panic at el.linked.
	s.UpdateAccessibility(accessPair(3))
	if !called || len(s.elements) != 2 || s.elements[5] == nil || s.elements[3] != nil || s.elements[4] != nil {
		t.Fatal("nested updates did not settle on the latest complete tree")
	}
	if len(s.access.Nodes) != 2 || s.access.Nodes[1].ID != 5 || s.updating {
		t.Fatal("the published accessibility tree does not match its native elements")
	}
	if links := s.elements[1].linked; s.elements[1].obj != root || len(links) != 1 || links[0] != s.elements[5].obj || len(s.w.b.byAccess) != 2 {
		t.Fatal("the latest tree lost its control links or leaked retired native elements")
	}
}

func TestAccessibilityNotificationCanDestroySurface(t *testing.T) {
	s := accessFixture(t)
	s.UpdateAccessibility(accessPair(2))
	removed := s.elements[2].obj
	called := false
	observeAccessNotes(t, func(obj id) {
		if obj == removed && !called {
			called = true
			s.UpdateAccessibility(accessPair(4))
			s.w.closed = true
			s.destroyAccess()
		}
	})
	s.UpdateAccessibility(accessPair(3))
	if !called || len(s.elements) != 0 || len(s.w.b.byAccess) != 0 || s.topLevel != 0 || len(s.access.Nodes) != 0 || s.updating {
		t.Fatal("surface destruction left accessibility objects or resumed a stale update")
	}
	// A late frame cannot resurrect a destroyed surface's native elements.
	s.UpdateAccessibility(accessPair(4))
	if len(s.elements) != 0 {
		t.Fatal("late update resurrected destroyed accessibility elements")
	}
}

func TestAccessibilityDestroyNotificationIsReentrant(t *testing.T) {
	s := accessFixture(t)
	s.UpdateAccessibility(accessPair(2))
	calls := 0
	observeAccessNotes(t, func(obj id) {
		calls++
		s.destroyAccess()
		s.UpdateAccessibility(accessPair(3))
	})
	s.destroyAccess()
	if calls != 2 || len(s.elements) != 0 || len(s.w.b.byAccess) != 0 || s.topLevel != 0 || len(s.access.Nodes) != 0 {
		t.Fatal("recursive destruction released an element twice or allowed it to be recreated")
	}
}
