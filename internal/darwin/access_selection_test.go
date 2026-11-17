//go:build darwin && !ios

package darwin

import (
	"sync"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo/internal/platform"
)

var (
	accessInfoTestOnce sync.Once
	accessInfoTestCall uintptr
	accessInfoTestHook func(obj id, name string, info id)
)

// observeAccessInfo has hook see the notifications posted with a userInfo
// (NSAccessibilityPostNotificationWithUserInfo) in place of AppKit.
func observeAccessInfo(t *testing.T, hook func(obj id, name string, info id)) {
	t.Helper()
	loadAccess()
	accessInfoTestOnce.Do(func() {
		accessInfoTestCall = purego.NewCallback(func(obj, name, info uintptr) {
			if accessInfoTestHook != nil {
				accessInfoTestHook(id(obj), stringOf(id(name)), id(info))
			}
		})
	})
	previous := postAccessInfo
	postAccessInfo, accessInfoTestHook = accessInfoTestCall, hook
	t.Cleanup(func() { postAccessInfo, accessInfoTestHook = previous, nil })
}

// A caret moving down a text field, its text the same, posts
// AXSelectedTextChanged with how it moved, a line down, which VoiceOver
// reads the new line at; an edit posts none of it.
func TestAccessibilitySelectionMoveNotification(t *testing.T) {
	s := accessFixture(t)
	field := func(value string, sel int) *platform.AccessTree {
		return &platform.AccessTree{Nodes: []platform.AccessNode{
			{ID: 1, Parent: -1, Role: platform.RoleTextField, Label: "Code", Value: value, SelStart: sel, SelEnd: sel, States: platform.AccessMultiline},
		}}
	}
	withPool(func() { s.UpdateAccessibility(field("one\ntwo\nthree", 1)) })
	type note struct {
		obj                   id
		name                  string
		typ, dir, granularity int
		element               id
	}
	var notes []note
	observeAccessInfo(t, func(obj id, name string, info id) {
		value := func(key string) id { return send(info, "objectForKey:", uintptr(nsString(key))) }
		notes = append(notes, note{obj, name, sendInt(value("AXTextStateChangeType"), "intValue"),
			sendInt(value("AXTextSelectionDirection"), "intValue"), sendInt(value("AXTextSelectionGranularity"), "intValue"),
			value("AXTextChangeElement")})
	})
	withPool(func() { s.UpdateAccessibility(field("one\ntwo\nthree", 5)) })
	obj := s.elements[1].obj
	want := note{obj, "AXSelectedTextChanged", platform.TextChangeMove, platform.TextDirectionNext, platform.TextGranularityLine, obj}
	if len(notes) != 1 || notes[0] != want {
		t.Fatalf("moving the caret down posted %+v, want %+v", notes, want)
	}
	notes = nil
	withPool(func() { s.UpdateAccessibility(field("one\ntwo!\nthree", 8)) })
	if len(notes) != 0 {
		t.Fatalf("an edit posted %+v", notes)
	}
	withPool(func() { s.UpdateAccessibility(field("one\ntwo!\nthree", 8)) })
	if len(notes) != 0 {
		t.Fatalf("a frame changing nothing posted %+v", notes)
	}
}
