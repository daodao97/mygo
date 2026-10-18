// Package timeseries samples monotonically increasing counters into bounded
// rate histories. It is independent of rendering, persistence and any protocol.
package timeseries

import (
	"fmt"
	"time"
)

type Options struct {
	Window                         time.Duration
	MaxSamples                     int
	MinInterval, MaxGap, ClockSkew time.Duration
}

func (o Options) normalized() Options {
	if o.Window == 0 {
		o.Window = time.Minute
	}
	if o.MaxSamples == 0 {
		o.MaxSamples = 121
	}
	if o.MinInterval == 0 {
		o.MinInterval = 500 * time.Millisecond
	}
	if o.MaxGap == 0 {
		o.MaxGap = 3500 * time.Millisecond
	}
	if o.ClockSkew == 0 {
		o.ClockSkew = time.Second
	}
	if o.Window <= 0 || o.MaxSamples < 1 || o.MaxSamples > 1_000_000 || o.MinInterval <= 0 || o.MaxGap < o.MinInterval || o.ClockSkew < 0 {
		panic("timeseries: invalid options")
	}
	return o
}

type Sample struct {
	At    int64
	Rates []float64
	Gap   bool
}
type Counter struct {
	options  Options
	at       time.Time
	counters []uint64
	samples  []Sample
}

func NewCounter(options Options, at time.Time, counters ...uint64) *Counter {
	if len(counters) == 0 {
		panic("timeseries: no counters")
	}
	return &Counter{options: options.normalized(), at: at, counters: append([]uint64(nil), counters...)}
}

// Counter is single-owner. External synchronization is needed if shared.
// Observe keeps clock gaps and counter resets as explicit missing samples.
func (c *Counter) Observe(now time.Time, values ...uint64) bool {
	if len(values) != len(c.counters) {
		panic(fmt.Sprintf("timeseries: expected %d counters", len(c.counters)))
	}
	dt := now.Sub(c.at)
	if dt >= 0 && dt < c.options.MinInterval {
		return false
	}
	at := now.UnixMilli()
	wall := time.Duration(at-c.at.UnixMilli()) * time.Millisecond
	skew := wall - dt
	if skew < 0 {
		skew = -skew
	}
	s := Sample{At: at, Rates: make([]float64, len(values))}
	if at <= c.at.UnixMilli() || skew > c.options.ClockSkew {
		c.samples = c.samples[:0]
		s.Gap = true
	} else if dt > c.options.MaxGap {
		s.Gap = true
	}
	for i, v := range values {
		if v < c.counters[i] {
			s.Gap = true
		}
	}
	if !s.Gap {
		for i, v := range values {
			s.Rates[i] = float64(v-c.counters[i]) / dt.Seconds()
		}
	}
	c.samples = append(c.samples, s)
	first := 0
	for first < len(c.samples) && at-c.samples[first].At > c.options.Window.Milliseconds() {
		first++
	}
	c.samples = append(c.samples[:0], c.samples[first:]...)
	if len(c.samples) > c.options.MaxSamples {
		c.samples = append(c.samples[:0], c.samples[len(c.samples)-c.options.MaxSamples:]...)
	}
	c.at = now
	copy(c.counters, values)
	return true
}
func (c *Counter) Samples() []Sample {
	out := make([]Sample, len(c.samples))
	for i, v := range c.samples {
		out[i] = v
		out[i].Rates = append([]float64(nil), v.Rates...)
	}
	return out
}
