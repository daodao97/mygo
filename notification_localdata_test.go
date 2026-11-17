package mygo

import (
	"testing"

	"github.com/egoist/mygo/internal/pushhub"
)

// A notification's Data stays readable by its ID, for push.SetPresentation
// on phones, whose hosts carry no Data.
func TestNotificationLocalData(t *testing.T) {
	n := NewNotification(NotificationOptions{ID: "local-data", Title: "Reply", Data: map[string]string{"thread": "7"}})
	if err := n.Show(); err != nil {
		t.Fatal(err)
	}
	var got, unknown map[string]string
	onMain(func() {
		if pushhub.LocalData != nil {
			got = pushhub.LocalData("local-data")
			unknown = pushhub.LocalData("nothing-shown")
		}
	})
	if got["thread"] != "7" || unknown != nil {
		t.Fatalf("data = %v, unknown = %v", got, unknown)
	}
}
