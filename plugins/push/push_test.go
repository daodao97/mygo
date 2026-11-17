package push

import (
	"testing"

	"github.com/egoist/mygo/internal/pushhub"
)

func TestStringsOfAndDataOf(t *testing.T) {
	got := stringsOf(map[string]any{"thread": "7", "n": 3.0, "aps": map[string]any{"badge": 1.0}})
	if got["thread"] != "7" || got["n"] != "3" || got["aps"] != `{"badge":1}` {
		t.Fatalf("stringsOf = %v", got)
	}
	if stringsOf(nil) != nil {
		t.Fatal("empty data made a map")
	}
	if s := string(dataOf(pushhub.Event{})); s != "{}" {
		t.Fatalf("empty dataOf = %s", s)
	}
	if s := string(dataOf(pushhub.Event{Data: map[string]string{"thread": "7"}})); s != `{"thread":"7"}` {
		t.Fatalf("dataOf = %s", s)
	}
}

// What SetPresentation decides with: a tapped one always shows.
func TestShowsFollowsPresentation(t *testing.T) {
	t.Cleanup(func() { mu.Lock(); present = nil; mu.Unlock() })
	mu.Lock()
	present = func(e pushhub.Event) bool { return e.Data["thread"] != "open" }
	mu.Unlock()
	if shows(pushhub.Event{Data: map[string]string{"thread": "open"}}) {
		t.Error("the open thread's notification showed")
	}
	if !shows(pushhub.Event{Data: map[string]string{"thread": "other"}}) {
		t.Error("another thread's notification was hidden")
	}
	if !shows(pushhub.Event{Clicked: true, Data: map[string]string{"thread": "open"}}) {
		t.Error("a tap was hidden")
	}
}
