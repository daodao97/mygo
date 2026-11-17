package mobile

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const wheelRow = 36

// WheelState is how a Wheel shows, kept in the element.
type wheelState struct {
	offset ui.Point
	// value is the value it shows, and ticked the item it last ticked for.
	value, ticked int
	once          bool
}

// Wheel shows a wheel of items a finger spins to choose one, as UIPickerView,
// choosing *value's item, with a tick as the choice changes. Changed reports a
// new choice.
//
//	mobile.Wheel(c, &app.size, []string{"S", "M", "L", "XL"})
func Wheel(c *ui.Context, value *int, items []string) ui.Element {
	t := c.Theme()
	box := ui.Box(c).Height(wheelRow * 5).Clip().Role(ui.RoleSlider)
	s := ui.Local(box, wheelKey{}, func() wheelState { return wheelState{} })
	n := len(items)
	if n == 0 {
		return box
	}
	*value = max(0, min(*value, n-1))
	at := func(i int) ui.Point { return ui.Point{Y: -float32(i) * wheelRow} }
	if !s.once || s.value != *value {
		s.once, s.value, s.ticked = true, *value, *value
		s.offset = at(*value)
	}
	box.Children(func() {
		// The band of the choice.
		ui.Box(c).Absolute().Left(4).Right(4).Top(wheelRow * 2).Height(wheelRow).Radius(8).Background(t.Surface)
		points := make([]ui.Point, n)
		for i := range points {
			points[i] = at(i)
		}
		track := ui.Column(c).Absolute().Left(0).Right(0).Top(wheelRow * 2).Height(float32(n) * wheelRow)
		track.Draggable(&s.offset, ui.DraggableOptions{
			Axis:   ui.Vertical,
			Bounds: &ui.Rect{Y: -float32(n-1) * wheelRow, H: float32(n-1) * wheelRow},
			Snap:   ui.Nearest(points...),
		})
		shown := track.DragOffset().Y
		cur := max(0, min(int(-shown/wheelRow+0.5), n-1))
		track.Children(func() {
			first, last := max(0, cur-4), min(n-1, cur+4)
			if first > 0 {
				ui.Box(c).Height(float32(first) * wheelRow).Shrink(0)
			}
			for i := first; i <= last; i++ {
				d := -shown/wheelRow - float32(i)
				if d < 0 {
					d = -d
				}
				alpha := max(0.2, 1-d*0.35)
				ui.Row(c.Key(i)).Height(wheelRow).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					ui.Text(c, items[i]).TextColor(t.Text.Alpha(alpha)).NoWrap()
				})
			}
		})
		if cur != s.ticked {
			s.ticked = cur
			mygo.Haptics.Selection()
		}
		if !track.Transitioning() && !track.Dragging() {
			if p := max(0, min(int(-s.offset.Y/wheelRow+0.5), n-1)); p != *value {
				*value, s.value = p, p
				box.SetChanged()
			}
		}
	})
	return box
}

type wheelKey struct{}

// numbers returns the labels of the numbers from lo to hi, formatted.
func numbers(lo, hi int, format string) []string {
	out := make([]string, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		out = append(out, fmt.Sprintf(format, i))
	}
	return out
}

// TimePicker shows wheels of hours and minutes choosing *value's time of day,
// which keeps its date, as UIDatePicker's time mode: 24 hours or 12 as the
// user's region says. Changed reports a new time.
func TimePicker(c *ui.Context, value *time.Time) ui.Element {
	row := ui.Row(c).AlignItems(ui.Center).Gap(4)
	h24 := c.Region().HourCycle != 12
	row.Children(func() {
		hour, minute := value.Hour(), value.Minute()
		changed := false
		if h24 {
			changed = Wheel(c.Key("h"), &hour, numbers(0, 23, "%02d")).Grow(1).Basis(0).Changed()
		} else {
			// 12 is the first of a half of the day's, shown as the wheel's last.
			h12 := (hour + 11) % 12
			changed = Wheel(c.Key("h"), &h12, numbers(1, 12, "%d")).Grow(1).Basis(0).Changed()
			hour = (h12 + 1) % 12
		}
		if Wheel(c.Key("m"), &minute, numbers(0, 59, "%02d")).Grow(1).Basis(0).Changed() {
			changed = true
		}
		if !h24 {
			pm := 0
			if value.Hour() >= 12 {
				pm = 1
			}
			if Wheel(c.Key("a"), &pm, []string{"AM", "PM"}).Grow(1).Basis(0).Changed() {
				changed = true
			}
			hour += 12 * pm
		}
		if changed {
			v := *value
			*value = time.Date(v.Year(), v.Month(), v.Day(), hour, minute, v.Second(), v.Nanosecond(), v.Location())
			row.SetChanged()
		}
	})
	return row
}

// DatePicker chooses *value's date, which keeps its time of day: wheels of
// month, day and year on Cupertino, as UIDatePicker's wheels, the month's
// calendar on Material. Changed reports a new date.
func DatePicker(c *ui.Context, value *time.Time) ui.Element {
	if Current() == Material {
		return ui.Calendar(c, value)
	}
	row := ui.Row(c).AlignItems(ui.Center).Gap(4)
	row.Children(func() {
		v := *value
		year0 := v.Year() - 100
		month, day, year := int(v.Month())-1, v.Day()-1, v.Year()-year0
		months := make([]string, 12)
		for i := range months {
			months[i] = time.Month(i + 1).String()
		}
		last := time.Date(v.Year(), v.Month()+1, 0, 0, 0, 0, 0, v.Location()).Day()
		changed := Wheel(c.Key("mo"), &month, months).Grow(2).Basis(0).Changed()
		if Wheel(c.Key("d"), &day, numbers(1, last, "%d")).Grow(1).Basis(0).Changed() {
			changed = true
		}
		if Wheel(c.Key("y"), &year, numbers(year0, year0+200, "%d")).Grow(1).Basis(0).Changed() {
			changed = true
		}
		if changed {
			y, m := year0+year, time.Month(month+1)
			d := min(day+1, time.Date(y, m+1, 0, 0, 0, 0, 0, v.Location()).Day())
			*value = time.Date(y, m, d, v.Hour(), v.Minute(), v.Second(), v.Nanosecond(), v.Location())
			row.SetChanged()
		}
	})
	return row
}
