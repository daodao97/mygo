package timeseries

import (
	"testing"
	"time"
)

func TestRatesGapsLimitsAndOwnership(t *testing.T) {
	at := time.Unix(1700000000, 0)
	c := NewCounter(Options{MaxSamples: 2}, at, 100, 200)
	if c.Observe(at.Add(100*time.Millisecond), 110, 220) {
		t.Fatal("too frequent")
	}
	c.Observe(at.Add(time.Second), 200, 500)
	s := c.Samples()
	if len(s) != 1 || s[0].Rates[0] != 100 || s[0].Rates[1] != 300 || s[0].Gap {
		t.Fatal(s)
	}
	s[0].Rates[0] = 999
	if c.Samples()[0].Rates[0] != 100 {
		t.Fatal("snapshot aliases")
	}
	c.Observe(at.Add(2*time.Second), 10, 600)
	c.Observe(at.Add(8*time.Second), 100, 900)
	s = c.Samples()
	if len(s) != 2 || !s[0].Gap || !s[1].Gap || s[1].Rates[0] != 0 {
		t.Fatal(s)
	}
	c.Observe(at.Add(9*time.Second), 200, 1000)
	if c.Samples()[1].Rates[0] != 100 {
		t.Fatal(c.Samples())
	}
	c.Observe(at.Add(-time.Second), 300, 1100)
	s = c.Samples()
	if len(s) != 1 || !s[0].Gap {
		t.Fatal("clock rollback", s)
	}
	c.Observe(at, 400, 1200)
	if c.Samples()[1].Rates[0] != 100 {
		t.Fatal("baseline after rollback", c.Samples())
	}
}
func TestWindowAndCounterShape(t *testing.T) {
	at := time.Unix(1700000000, 0)
	c := NewCounter(Options{Window: 2 * time.Second}, at, 0)
	for i := 1; i < 10; i++ {
		c.Observe(at.Add(time.Duration(i)*time.Second), uint64(i))
	}
	if len(c.Samples()) != 3 {
		t.Fatal(c.Samples())
	}
	defer func() {
		if recover() == nil {
			t.Fatal("shape accepted")
		}
	}()
	c.Observe(at, 1, 2)
}
