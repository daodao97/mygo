package mygo

import (
	"errors"
	"github.com/egoist/mygo/push/apns"
)

// PushProvider opens the APNs sender configured by `mygo push setup` for this
// application's PathUserData. Use on a desktop or server, not in the iOS app.
// It works before Run and in headless worker processes. Retain the returned
// provider and call Reload when credentials may have changed.
func (a *Application) PushProvider() (*apns.Provider, error) {
	dir, err := a.Path(PathUserData)
	if err != nil {
		return nil, err
	}
	p, err := apns.OpenProvider(dir)
	if err != nil {
		return nil, err
	}
	if info, ok := packageInfo(); ok && info.Identifier != "" && p.Topic() != info.Identifier && !(IsDev() && info.Identifier == p.Topic()+".dev") {
		return nil, errors.New("mygo: push provider belongs to another application")
	}
	return p, nil
}
