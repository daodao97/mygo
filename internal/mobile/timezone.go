package mobile

import (
	"time"

	"github.com/egoist/mygo/internal/mobile/wire"
)

// Go finds no local time zone on phones, whose /etc/localtime it can't reach
// or doesn't look at, and has every time in UTC: time.Local comes from the
// host, and follows it as the phone changes zone. Go's own files of zones
// aren't there either (timezone_phone.go embeds them).

// setTimeZone makes the zone named by id, as "Asia/Shanghai", time.Local. What
// Go doesn't know stays as it was.
func setTimeZone(id string) {
	if id == "" {
		return
	}
	if loc, err := time.LoadLocation(id); err == nil {
		time.Local = loc
	}
}

// askTimeZone sets the zone before the app's main function runs, on the
// thread that starts Go. The host may answer on any thread.
func askTimeZone(h interface{ callHere(msg []byte) []byte }) {
	var w wire.Writer
	(&wire.TimeZone{}).Encode(&w)
	var r wire.TimeZoneReply
	r.Decode(&wire.Reader{Buf: h.callHere(w.Buf)})
	setTimeZone(r.ID)
}
