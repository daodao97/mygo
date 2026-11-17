// Package pushhub carries what the system tells of push notifications from
// package mygo, which the backends tell, to plugins/push, which apps use.
// It holds only hooks, so that apps that import no plugins/push link none
// of it: the plugin sets Received and Registered as it initializes, and
// mygo calls them on the main thread.
package pushhub

// Event is a notification the system delivered: in front of the app
// (Clicked false) or tapped. Remote tells a push from a notification the
// app scheduled.
type Event struct {
	ID      string
	Data    map[string]string
	Clicked bool
	Remote  bool
	Action  string
}

var (
	// Received is told of every notification the system delivers, and
	// reports, for one in front of the app, whether it shows.
	Received func(Event) (show bool)
	// Registered is told the token pushes are sent to, as it changes, or
	// why registering failed.
	Registered func(token string, err error)
	// LocalData returns the Data of a notification the app showed, by its ID,
	// which the core keeps for the notifications that have it, as phones'
	// hosts carry no Data. Set as the first such notification shows.
	LocalData func(id string) map[string]string
)
