// Package sensors reads the motion of a phone or a tablet: Core Motion on iOS,
// and SensorManager on Android. Elsewhere its functions report
// errors.ErrUnsupported.
package sensors

import (
	"context"
	"encoding/json"
	"time"

	"github.com/egoist/mygo"
)

const pluginID = "sensors"

// Sensor is a sensor to read.
type Sensor string

const (
	// Accelerometer reads acceleration, in m/s², with gravity.
	Accelerometer Sensor = "accelerometer"
	// Gyroscope reads rotation, in radians a second.
	Gyroscope Sensor = "gyroscope"
	// Magnetometer reads the magnetic field, in microteslas.
	Magnetometer Sensor = "magnetometer"
)

// Reading is a sensor's three axes at a time: X to the right of the screen, Y
// to its top, and Z out of it.
type Reading struct {
	X, Y, Z float64
	Time    time.Time
}

// Watch calls fn on the main thread with what a sensor reads, every interval
// (at the least what the system allows), until stop is called.
func Watch(ctx context.Context, s Sensor, interval time.Duration, fn func(Reading)) (stop func(), err error) {
	p := mygo.Native(pluginID)
	off := p.On(string(s), func(data []byte) {
		var r struct {
			X, Y, Z float64
			T       int64
		}
		if json.Unmarshal(data, &r) == nil {
			fn(Reading{r.X, r.Y, r.Z, time.UnixMilli(r.T)})
		}
	})
	args := map[string]any{"sensor": string(s), "intervalMs": interval.Milliseconds()}
	if err := p.Call(ctx, "start", args, nil); err != nil {
		off()
		return nil, err
	}
	return func() {
		off()
		go p.Call(context.Background(), "stop", args, nil)
	}, nil
}
