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

	"github.com/egoist/mygo"
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
