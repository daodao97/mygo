// Package location tells where the device is, on phones and tablets: Core
// Location on iOS, and LocationManager on Android, without Google's services.
// Importing it adds the permissions it needs to the app, with a usage
// description on iOS that ios.infoPlist in the app's configuration replaces:
//
//	"NSLocationWhenInUseUsageDescription": "Shows nearby stores."
//
// Elsewhere its functions report errors.ErrUnsupported.
package location

import (
	"context"
	"encoding/json"
	"time"

	"github.com/egoist/mygo"
)

const pluginID = "location"

// Fix is a place the device was at.
type Fix struct {
	// Latitude and Longitude are in degrees.
	Latitude, Longitude float64
	// Altitude is in meters above sea level, Speed in meters a second, and
	// Heading in degrees clockwise from north; negative where not known.
	Altitude, Speed, Heading float64
	// Accuracy is how far off it may be, in meters.
	Accuracy float64
	// Time is when it was measured.
	Time time.Time
}

type wireFix struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude"`
	Speed     float64 `json:"speed"`
	Heading   float64 `json:"heading"`
	Accuracy  float64 `json:"accuracy"`
	TimeMS    int64   `json:"time"`
}

func (w wireFix) fix() Fix {
	return Fix{w.Latitude, w.Longitude, w.Altitude, w.Speed, w.Heading, w.Accuracy, time.UnixMilli(w.TimeMS)}
}

// RequestPermission asks the user to let the app know where the device is,
// the first time, and reports whether it may. Call it off the main goroutine,
// as it waits for the user.
func RequestPermission(ctx context.Context) (bool, error) {
	var r struct {
		Granted bool `json:"granted"`
	}
	err := mygo.Native(pluginID).Call(ctx, "requestPermission", nil, &r)
	return r.Granted, err
}

// Current finds where the device is now, asking for permission if it must.
func Current(ctx context.Context) (Fix, error) {
	var w wireFix
	if err := mygo.Native(pluginID).Call(ctx, "current", nil, &w); err != nil {
		return Fix{}, err
	}
	return w.fix(), nil
}

// Watch calls fn on the main thread as the device moves, until stop is called.
func Watch(ctx context.Context, fn func(Fix)) (stop func(), err error) {
	p := mygo.Native(pluginID)
	off := p.On("fix", func(data []byte) {
		var w wireFix
		if json.Unmarshal(data, &w) == nil {
			fn(w.fix())
		}
	})
	if err := p.Call(ctx, "start", nil, nil); err != nil {
		off()
		return nil, err
	}
	return func() {
		off()
		go p.Call(context.Background(), "stop", nil, nil)
	}, nil
}
