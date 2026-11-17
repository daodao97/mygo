// Package push receives push notifications on phones: APNs on iOS, and Firebase
// Cloud Messaging on Android. It tells the token that the app's server sends to.
//
// iOS needs the Push Notifications capability in the app's provisioning
// profile. Android needs a Firebase project: give Configure its values, as in
// its google-services.json. Elsewhere its functions report
// errors.ErrUnsupported.
package push

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/pushhub"
)

const pluginID = "push"

// Firebase is what Android's messaging needs of the app's Firebase project.
type Firebase struct {
	ProjectID string `json:"projectId"`
	AppID     string `json:"appId"`
	APIKey    string `json:"apiKey"`
	SenderID  string `json:"senderId"`
}

// Configure sets up Firebase, on Android. Call it before Register. iOS ignores
// it.
func Configure(ctx context.Context, f Firebase) error {
	return mygo.Native(pluginID).Call(ctx, "configure", f, nil)
}

// Register asks the user to allow notifications, and returns the token that
// the app's server sends them to, which may change: see OnToken. Call it off
// the main goroutine, as it waits for the user.
func Register(ctx context.Context) (string, error) {
	var r struct {
		Token string `json:"token"`
	}
	err := mygo.Native(pluginID).Call(ctx, "register", nil, &r)
	return r.Token, err
}

// OnToken calls fn on the main thread as the token changes.
func OnToken(fn func(token string)) (off func()) {
	return mygo.Native(pluginID).On("token", func(data []byte) {
		var s string
		if json.Unmarshal(data, &s) == nil {
			fn(s)
		}
	})
}

// OnMessage calls fn on the main thread as a push arrives while the app runs,
// with what it carries, as JSON.
func OnMessage(fn func(data []byte)) (off func()) { return mygo.Native(pluginID).On("message", fn) }

// OnOpened calls fn on the main thread as the user taps a push's notification, with
// what it carries, as JSON: also the tap that launched the app, once this listens,
// so call it as the app is ready.
func OnOpened(fn func(data []byte)) (off func()) {
	off = mygo.Native(pluginID).On("opened", fn)
	mygo.App.WhenReady(func() {
		go mygo.Native(pluginID).Call(context.Background(), "listenOpened", nil, nil)
	})
	return off
}

// SetPresentation decides, on the main thread, whether a notification that
// arrives while the app is in front shows (iOS): show is given what it
// carries, as JSON, the app's own notifications' Data included. It is the
// fork's (FORK.md); nil shows them all.
func SetPresentation(show func(data []byte) bool) {
	var fn func(pushhub.Event) bool
	if show != nil {
		fn = func(e pushhub.Event) bool { return show(dataOf(e)) }
	}
	mu.Lock()
	present = fn
	mu.Unlock()
	native := mygo.Native(pluginID)
	presentation.Do(func() { native.On("present", presentNative) })
	on := show != nil
	mygo.App.WhenReady(func() {
		go native.Call(context.Background(), "setPresentation", map[string]bool{"on": on}, nil)
	})
}

var presentation sync.Once

// presentNative answers the host's question of whether a notification shows.
func presentNative(raw []byte) {
	var m struct {
		Key    int            `json:"key"`
		ID     string         `json:"id"`
		Remote bool           `json:"remote"`
		Data   map[string]any `json:"data"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	e := pushhub.Event{ID: m.ID, Remote: m.Remote, Data: stringsOf(m.Data)}
	if !e.Remote && len(e.Data) == 0 && pushhub.LocalData != nil {
		e.Data = pushhub.LocalData(e.ID)
	}
	show := shows(e)
	go mygo.Native(pluginID).Call(context.Background(), "presented", map[string]any{"key": m.Key, "show": show}, nil)
}

// stringsOf is what a notification carries, as strings, as Data holds it.
func stringsOf(m map[string]any) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		} else if b, err := json.Marshal(v); err == nil {
			out[k] = string(b)
		} else {
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

// dataOf is what e carries, as a JSON object.
func dataOf(e pushhub.Event) []byte {
	if len(e.Data) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(e.Data)
	if err != nil {
		return []byte("{}")
	}
	return b
}
