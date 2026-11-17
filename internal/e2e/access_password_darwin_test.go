//go:build darwin && !ios

package e2e

import (
	"slices"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestAccessPasswordWrite sets a password field's value and types into
// it through accessibility, as a password manager fills it: the text
// reaches the app, but no assistive app reads it back, as none reads
// NSSecureTextField's.
func TestAccessPasswordWrite(t *testing.T) {
	var frames atomic.Int32
	secret := ""
	view := func(c *ui.Context) {
		frames.Add(1)
		ui.Column(c).Fill().Padding(20).Children(func() {
			ui.TextInput(c, &secret).Label("Password").Password()
		})
	}
	w := newWindow(t, mygo.WindowOptions{Title: "Password", Width: 400, Height: 200, Content: ui.View(view)})
	eventually(t, "a frame", func() bool { return frames.Load() > 0 })
	field := func() (accessNode, bool) {
		nodes, _ := accessibility(w)
		i := slices.IndexFunc(nodes, func(n accessNode) bool { return n.label == "Password" })
		if i < 0 {
			return accessNode{}, false
		}
		return nodes[i], true
	}
	eventually(t, "the password field", func() bool { _, ok := field(); return ok })
	read := func() string {
		var s string
		mygo.RunOnMain(func() { s = secret })
		return s
	}
	if !accessPerform(w, "Password", "value", "hunter2") {
		t.Fatal("cannot set the password field's value")
	}
	eventually(t, "the password set", func() bool { return read() == "hunter2" })
	if !accessPerform(w, "Password", "insert", "!") {
		t.Fatal("cannot insert text into the password field")
	}
	eventually(t, "the text inserted", func() bool { return read() == "hunter2!" })
	if n, _ := field(); n.value != "" {
		t.Errorf("the password field's value reads %q, want it empty", n.value)
	}
	if v, _, _, _ := axAttribute(w, "Password", "AXSelectedText"); v != "" {
		t.Errorf("the password field's selected text reads %q, want it empty", v)
	}
}
