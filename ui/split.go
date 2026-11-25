package ui

// Split lays out first and second side by side, with a divider between
// them that the user drags to resize them, or moves with the arrows once
// it has the keyboard focus: *size is the width of first, which the
// divider keeps 40 DIPs from either edge. With less room than that, first
// gives way, keeping *size for when there is room again. Changed reports a
// move. Right to left (Direction), first is on the right. SplitReset makes
// a double click on the divider put first back at a size the app chooses,
// and SplitSecond makes *size the width of second instead, as a panel at
// the end of a window takes. SplitGap spaces the panes apart by a number
// of DIPs without drawing the 1 DIP divider line, including 0 for flush
// panes, and SplitLimits gives either pane a minimum and a maximum size.
//
//	ui.Split(c, &app.sidebar, app.files, app.editor).Fill()
func coreSplit(c *context, size *float32, first, second func()) *node {
	return split(c, size, first, second, false, false)
}

// SplitVertical is Split with first above second, *size its height.
func coreSplitVertical(c *context, size *float32, first, second func()) *node {
	return split(c, size, first, second, true, false)
}

// SplitPercent is Split with *p the percentage of the room first takes,
// from 0 to 100, so that both panes grow and shrink with the room, in
// proportion.
//
//	ui.SplitPercent(c, &app.sidebarPercent, app.files, app.editor).Fill()
func coreSplitPercent(c *context, p *float32, first, second func()) *node {
	return split(c, p, first, second, false, true)
}

// SplitVerticalPercent is SplitPercent with first above second.
func coreSplitVerticalPercent(c *context, p *float32, first, second func()) *node {
	return split(c, p, first, second, true, true)
}

// splitResetKey is the local that keeps a split's SplitReset settings.
type splitResetKey struct{}

// splitResetSettings are what SplitReset says of a split in this build,
// and what it said in the build before, which the input since applies by:
// the split the user double-clicked.
type splitResetSettings struct {
	now, shown       float32
	nowSet, shownSet bool
}

// SplitReset makes a double click on the divider of a Split put the first
// pane at v, or the second with SplitSecond, in the split's own unit: DIPs
// for Split and SplitVertical, a percentage for SplitPercent and
// SplitVerticalPercent. A split without it keeps its size on a double
// click. The value is kept even when the window is too small to show it,
// so the pane takes it back when there is room again.
//
//	ui.Split(c, &app.sidebar, app.files, app.editor).SplitReset(220)
func (e *node) SplitReset(v float32) *node {
	if e.widget != "Split" {
		return e
	}
	s := coreLocal(e, splitResetKey{}, func() splitResetSettings { return splitResetSettings{} })
	s.now, s.nowSet = v, true
	return e
}

// splitResetOf returns the SplitReset settings of a split, or nil when it
// has none.
func splitResetOf(e *node) *splitResetSettings {
	if v, ok := e.st.locals[splitResetKey{}]; ok {
		if s, ok := v.(*splitResetSettings); ok {
			return s
		}
	}
	return nil
}

// splitSecondKey is the local that keeps a split's SplitSecond settings.
type splitSecondKey struct{}

// splitSecondSettings are whether SplitSecond made *size the second pane's
// size in this build, which lays the panes out, and in the build before,
// which the input since applies by: the panes the user saw.
type splitSecondSettings struct {
	now, shown bool
}

// SplitSecond makes *size of a Split the size of its second pane rather
// than its first's, the pane on the right of Split (on the left right to
// left) and at the bottom of SplitVertical, as a chat's list of members or
// an inspector at the end of a window, or a terminal under an editor: the
// first pane then takes the rest of the room. The divider, its arrows and
// the 40 DIPs it keeps from either edge stay as they are; with less room,
// the first pane gives way down to its 40 DIPs, then the second, keeping
// *size for when there is room again. SplitReset puts the second pane at
// its value. Of SplitPercent and SplitVerticalPercent, *p is the second
// pane's percentage of the room.
//
//	ui.Split(c, &app.membersWidth, app.chat, app.members).SplitSecond().Fill()
func (e *node) SplitSecond() *node {
	if e.widget != "Split" {
		return e
	}
	s := coreLocal(e, splitSecondKey{}, func() splitSecondSettings { return splitSecondSettings{} })
	s.now = true
	if sb := e.c.splitBuilt(e); sb != nil && !sb.second {
		sb.second = true
		sb.apply()
	}
	return e
}

// splitGapKey is the local that keeps a split's SplitGap settings.
type splitGapKey struct{}

// splitGapSettings are what SplitGap says of a split in this build, and
// what it said in the build before, which the input since applies by.
type splitGapSettings struct {
	now, shown       float32
	nowSet, shownSet bool
}

// SplitLimits bounds the sizes of a Split's panes, in DIPs, for all four
// kinds of split: the width of each pane in Split and SplitPercent, its
// height in SplitVertical and SplitVerticalPercent. A zero field sets no
// limit, and a minimum under the 40 DIPs every pane keeps changes nothing.
type SplitLimits struct {
	// FirstMin and FirstMax bound the first pane: on the left (on the
	// right, right to left) or on top.
	FirstMin, FirstMax float32
	// SecondMin and SecondMax bound the second pane, which takes the rest
	// of the room, or with SplitSecond is the one *size measures.
	SecondMin, SecondMax float32
}

// SplitLimits gives the panes of a Split a minimum and a maximum size, in
// DIPs whichever the split's unit: the divider stops where a pane would
// leave its limits, as it stops 40 DIPs from either edge, both as the user
// drags it and as the arrows move it, and the panes keep within them as
// the room changes. Where the limits can't all hold, or don't fill the
// room:
//
//   - a pane's minimum wins over its maximum;
//   - when the room is smaller than both minimums together, both panes
//     keep them and overflow the end of the split, which the window, or
//     a Clip around it, cuts: give the window a minimum size that holds
//     them;
//   - when the room is larger than both maximums together, both panes
//     keep them and the rest of the room, after the second, stays empty.
//
// A size outside the limits is kept, and shown within them, until the
// user moves the divider. As with SplitReset, the input applies the limits
// of the build before, and the layout those of this build.
//
//	// A side panel between 280 and 480 DIPs wide, after a chat no
//	// narrower than 360.
//	ui.Split(c, &app.chatWidth, app.chat, app.panel).SplitLimits(ui.SplitLimits{
//		FirstMin: 360, SecondMin: 280, SecondMax: 480,
//	})
func (e *node) SplitLimits(l SplitLimits) *node {
	if e.widget != "Split" {
		return e
	}
	l.FirstMin, l.FirstMax = max(l.FirstMin, 0), max(l.FirstMax, 0)
	l.SecondMin, l.SecondMax = max(l.SecondMin, 0), max(l.SecondMax, 0)
	s := coreLocal(e, splitLimitsKey{}, func() splitLimitsSettings { return splitLimitsSettings{} })
	s.now, s.set = l, true
	if sb := e.c.splitBuilt(e); sb != nil {
		sb.lim = l
		sb.apply()
	}
	return e
}

// splitLimitsKey is the local that keeps a split's SplitLimits settings.
type splitLimitsKey struct{}

// splitLimitsSettings are what SplitLimits says of a split in this build,
// which the next build's input applies by.
type splitLimitsSettings struct {
	now SplitLimits
	set bool
}

// splitLimitsOf returns the SplitLimits settings of a split, or nil when it
// has none.
func splitLimitsOf(e *node) *splitLimitsSettings {
	if v, ok := e.st.locals[splitLimitsKey{}]; ok {
		if s, ok := v.(*splitLimitsSettings); ok {
			return s
		}
	}
	return nil
}

// splitSlack is how much a pane in DIPs grows, against the other's 1, so
// that it takes only the room the other can't, at its maximum: too little
// to move it by a fraction of a DIP otherwise, and a power of two, which
// the flexible lengths divide by exactly.
const splitSlack = 0x1p-64

// splitBuild holds the panes, the divider and the handle built for a Split
// in the current pass, so that SplitSecond, SplitGap and SplitLimits lay
// them out in the same pass, without positional walking or an extra
// rebuild pass.
type splitBuild struct {
	split, a, div, b, handle *node
	size                     *float32
	total, minPane, grip     float32
	// gap is the room between the panes (SplitGap).
	gap                         float32
	vertical, inPercent, second bool
	lim                         SplitLimits
}

// splitBuilt returns what the current pass built for the split e, or nil.
func (c *context) splitBuilt(e *node) *splitBuild {
	for i := len(c.splits) - 1; i >= 0; i-- {
		if c.splits[i].split == e {
			return &c.splits[i]
		}
	}
	return nil
}

// along returns n's length, minimum and maximum along the split.
func (sb *splitBuild) along(n *node) (*length, *bound, *bound) {
	if sb.vertical {
		return &n.height, &n.minH, &n.maxH
	}
	return &n.width, &n.minW, &n.maxW
}

// apply lays the panes out, by what this pass says of the split so far:
// the pane *size measures, the first or with SplitSecond the second, at
// that size or share and the other with the rest, both within their
// limits, as each of SplitSecond and SplitLimits calls it again. The
// layout sizes them, not splitShown: that uses the last frame's room, and
// the layout this frame's.
func (sb *splitBuild) apply() {
	sized, rest := sb.a, sb.b
	if sb.second {
		sized, rest = sb.b, sb.a
	}
	for _, n := range [2]*node{sized, rest} {
		n.width, n.height, n.basis, n.grow = length{}, length{}, length{}, 0
		n.minW, n.minH, n.maxW, n.maxH = bound{}, bound{}, bound{}, bound{}
	}
	// l names the pane *size measures first, as fit does.
	l := sb.limits()
	limited := l != SplitLimits{}
	minPane := sb.minPane
	firstMin, secondMin := max(l.FirstMin, minPane), max(l.SecondMin, minPane)
	firstMax, secondMax := max(l.FirstMax, firstMin), max(l.SecondMax, secondMin)
	main, minMain, maxMain := sb.along(sized)
	_, restMin, restMax := sb.along(rest)
	if sb.inPercent {
		// The panes grow in proportion, and either takes the room the
		// other can't, at its maximum.
		share := min(max(*sb.size, 0), 100) / 100
		sized.Grow(share)
		rest.Grow(1 - share)
		minMain.setPx(firstMin)
		if l.SecondMax > 0 && sized.grow == 0 {
			sized.grow = splitSlack
		}
		if l.FirstMax > 0 && rest.grow == 0 {
			rest.grow = splitSlack
		}
	} else if !limited {
		*main = px(*sb.size)
		minMain.setPx(min(*sb.size, minPane))
		rest.Grow(1)
	} else {
		// As large as *size, within its limits, giving way to the other's
		// minimum down to its own.
		v := *sb.size
		if l.FirstMax > 0 {
			v = min(v, firstMax)
		}
		*main = px(max(v, firstMin))
		minMain.setPx(firstMin)
		rest.Grow(1)
		if l.SecondMax > 0 {
			// It takes the room the other can't.
			sized.grow = splitSlack
		}
	}
	if l.FirstMax > 0 {
		maxMain.setPx(firstMax)
	}
	restMin.setPx(secondMin)
	if l.SecondMax > 0 {
		restMax.setPx(secondMax)
	}
	sb.acc()
}

// limits returns the split's limits with the pane *size measures first,
// as fit takes them.
func (sb *splitBuild) limits() SplitLimits {
	if sb.second {
		return sb.lim.swapped()
	}
	return sb.lim
}

// swapped returns l with its panes' limits swapped.
func (l SplitLimits) swapped() SplitLimits {
	return SplitLimits{FirstMin: l.SecondMin, FirstMax: l.SecondMax, SecondMin: l.FirstMin, SecondMax: l.FirstMax}
}

// acc tells assistive technology the divider's range and where it is from
// the start, so that increasing it moves it toward the end, whichever pane
// *size measures.
func (sb *splitBuild) acc() {
	l := sb.limits()
	at := splitShown(*sb.size, sb.total, sb.gap, sb.minPane, sb.inPercent, l)
	lo, hi := l.fit(0, sb.total, sb.gap, sb.minPane), l.fit(sb.total, sb.total, sb.gap, sb.minPane)
	if sb.second && sb.total > 0 {
		room := sb.total - sb.gap
		at, lo, hi = room-at, room-hi, room-lo
	}
	sb.div.accRange = [3]float64{float64(lo), float64(hi), float64(at)}
}

// splitShown returns the size of the pane size measures, in DIPs or with
// inPercent a percentage, as a split total long with gap between its panes
// and limits l (that pane first) shows it: not a size the room can't, which
// is kept for when there is room again.
func splitShown(size, total, gap, minPane float32, inPercent bool, l SplitLimits) float32 {
	switch {
	case inPercent && total > 0:
		share := min(max(size, 0), 100) / 100
		return l.fit(share*(total-gap), total, gap, minPane)
	case !inPercent && total > 0 && (size > minPane || l != SplitLimits{}):
		return l.fit(size, total, gap, minPane)
	}
	return size
}

// SplitGap spaces the two panes of a Split apart by v DIPs without drawing
// the 1 DIP divider line between them, as two cards side by side or panes
// that draw their own edges: SplitGap(0) puts them flush with no line, and
// a positive v leaves that room between them, where the resize handle sits.
//
//	ui.Split(c, &app.sidebar, app.files, app.editor).SplitGap(8).Fill()
func (e *node) SplitGap(v float32) *node {
	if e.widget != "Split" {
		return e
	}
	if !(v > 0) {
		v = 0
	}
	s := coreLocal(e, splitGapKey{}, func() splitGapSettings { return splitGapSettings{} })
	s.now, s.nowSet = v, true
	if sb := e.c.splitBuilt(e); sb != nil {
		sb.div.paintFn = nil
		handleSize := max(sb.grip, v)
		if sb.vertical {
			sb.div.Height(v)
			sb.handle.Top(-(handleSize - v) / 2).Height(handleSize)
		} else {
			sb.div.Width(v)
			sb.handle.Start(-(handleSize - v) / 2).Width(handleSize)
		}
		sb.gap = v
		sb.acc()
	}
	return e
}

// splitGapOf returns the SplitGap settings of a split, or nil when it has
// none.
func splitGapOf(e *node) *splitGapSettings {
	if v, ok := e.st.locals[splitGapKey{}]; ok {
		if s, ok := v.(*splitGapSettings); ok {
			return s
		}
	}
	return nil
}

// split shares the room between first and second: *size is the size in
// DIPs of first, or of second with SplitSecond, or with inPercent its
// percentage of the room.
func split(c *context, size *float32, first, second func(), vertical, inPercent bool) *node {
	t := c.theme
	minPane, grip := t.Space(10), t.Space(1.5)
	stack := func() *node {
		// One element or the other: creating one adds it to the parent.
		if vertical {
			return coreColumn(c)
		}
		return coreRow(c)
	}
	setLength := func(n *node, v float32) {
		if vertical {
			n.Height(v)
		} else {
			n.Width(v)
		}
	}
	minLength := func(n *node, v float32) {
		if vertical {
			n.MinHeight(v)
		} else {
			n.MinWidth(v)
		}
	}
	e := stack().AlignItems(Stretch)
	e.widget = "Split"
	// reset is what SplitReset said of the split in the build before, which
	// the input since applies by; this build's SplitReset says what the
	// next build's input applies by.
	reset := splitResetOf(e)
	if reset != nil {
		reset.shown, reset.shownSet = reset.now, reset.nowSet
		reset.now, reset.nowSet = 0, false
	}
	// side is SplitSecond's, as reset is SplitReset's: the input moves the
	// pane the build before sized, and the layout the one this build does,
	// which SplitSecond chooses after the panes were built.
	side := coreLocal(e, splitSecondKey{}, func() splitSecondSettings { return splitSecondSettings{} })
	side.shown, side.now = side.now, false
	gap := float32(1)
	if gapCfg := splitGapOf(e); gapCfg != nil {
		gapCfg.shown, gapCfg.shownSet = gapCfg.now, gapCfg.nowSet
		gapCfg.now, gapCfg.nowSet = 0, false
		if gapCfg.shownSet {
			gap = gapCfg.shown
		}
	}
	// lim is what SplitLimits said of the split in the build before, which
	// the input since applies by, as reset, with the pane *size measures
	// first; this build's SplitLimits sets what this frame lays out by.
	var lim SplitLimits
	if s := splitLimitsOf(e); s != nil {
		if s.set {
			lim = s.now
		}
		s.now, s.set = SplitLimits{}, false
	}
	if side.shown {
		lim = lim.swapped()
	}
	// toward is +1 when the divider moving toward the end (down) widens the
	// pane *size measures, as for the first pane, and -1 for the second.
	toward := float32(1)
	if side.shown {
		toward = -1
	}
	e.Children(func() {
		// The panes meet at a line one DIP thick (unless SplitGap spaces
		// them otherwise), with nothing between them and it. The handle the
		// pointer drags spans a few DIPs over either pane, above them: it
		// is placed in this box, which has no padding, whatever the split's.
		in := stack().Grow(1).AlignItems(Stretch)
		minLength(in, 0)
		// The room to share, as the last frame laid it out.
		total := in.st.w
		if vertical {
			total = in.st.h
		}
		// shown is the size of the pane *size measures as the last frame
		// laid it out. Drags and keys move the divider from there, not from
		// *size, which a split too small for it does not show.
		shown := splitShown(*size, total, gap, minPane, inPercent, lim)
		set := func(v float32) {
			v = lim.fit(v, total, gap, minPane)
			if inPercent {
				if total <= gap {
					return
				}
				v = v * 100 / (total - gap)
			}
			if v != *size {
				*size = v
				e.st.markChanged()
				c.rt.consumed = true
			}
		}
		// resetTo puts the pane *size measures at the size SplitReset
		// chose, in the split's own unit. Unlike set, it keeps a size the
		// room cannot show, as the divider's press does, so that the pane
		// takes it back when there is room again.
		resetTo := func(v float32) {
			if inPercent {
				v = min(max(v, 0), 100)
			} else {
				v = lim.fit(v, 0, gap, minPane)
			}
			if v != *size {
				*size = v
				e.st.markChanged()
				c.rt.consumed = true
			}
		}
		in.Children(func() {
			// splitBuild.apply sizes the panes.
			a := coreBox(c).Shrink(1).Clip()
			a.Children(first)

			// The line takes the focus between the panes, in their order.
			div := coreBox(c).Shrink(0).Focusable().Role(RoleSplitter).Label(c.texts().Divider())
			div.widget = "Divider"
			div.flags |= flagOwnRing
			setLength(div, 1)
			// The arrows move the line the way they point.
			back, forth := div.arrows()
			if vertical {
				back, forth = KeyUp, KeyDown
			}
			div.afterInput(func() {
				if div.Shortcut(0, back) {
					set(shown - toward*t.Space(2.5))
				}
				if div.Shortcut(0, forth) {
					set(shown + toward*t.Space(2.5))
				}
			})
			// Assistive technology hears where the divider is from the
			// start (splitBuild.acc), so that increasing it moves it toward
			// the end, as Right does.
			div.hasRange = true
			div.accStep = float64(t.Space(2.5))

			b := coreBox(c).Shrink(1).Clip()
			b.Children(second)

			// placeOn keeps the handle on the divider where this frame lays
			// it out, also when a pane gives way.
			handle := coreBox(c).Absolute()
			handle.placeOn = div
			handle.flags |= flagDraggable | flagHover
			if vertical {
				handle.Left(0).Right(0).Top(-(grip - 1) / 2).Height(grip).Cursor(CursorResizeNS)
			} else {
				handle.Top(0).Bottom(0).Start(-(grip - 1) / 2).Width(grip).Cursor(CursorResizeEW)
			}
			c.splits = append(c.splits, splitBuild{
				split:     e,
				a:         a,
				div:       div,
				b:         b,
				handle:    handle,
				size:      size,
				total:     total,
				minPane:   minPane,
				grip:      grip,
				gap:       gap,
				vertical:  vertical,
				inPercent: inPercent,
			})
			c.splits[len(c.splits)-1].apply()
			// A press there focuses the line, as a press on it would. Only a
			// move changes the size, so a press keeps *size for when there is
			// room again. A double click restores the size SplitReset chose.
			dx, dy, held := handle.Dragged()
			handle.afterInput(func() {
				if reset != nil && reset.shownSet && handle.DoubleClicked() {
					div.focus(false)
					resetTo(reset.shown)
					return
				}
				if !held {
					// The next drag starts with the divider under the
					// pointer.
					if o, ok := handle.st.locals[splitOvershoot{}].(*float32); ok {
						*o = 0
					}
					return
				}
				div.focus(false)
				if dx == 0 && dy == 0 {
					return
				}
				d := dx
				switch {
				case vertical:
					d = dy
				case handle.rtl():
					d = -dx
				}
				d *= toward
				// The pointer goes on past a pane's limit (its 40 DIPs, or
				// SplitLimits'), where the divider stops: what it went past
				// is kept, so that coming back moves the divider only once
				// the pointer is over it again.
				o, _ := keepLocal(handle.st, splitOvershoot{}, func() float32 { return 0 })
				want := shown + d + *o
				*o = want - lim.fit(want, total, gap, minPane)
				set(want)
			})
			div.Draw(func(p *Painter, r Rect) {
				line := t.Border
				if held || handle.Hovered() {
					line = t.Accent
				}
				p.Fill(r, line, 0)
			})
			handle.Draw(func(p *Painter, r Rect) {
				if div.paintFn == nil && (held || handle.Hovered()) {
					line := Rect{r.X + (r.W-1)/2, r.Y, 1, r.H}
					if vertical {
						line = Rect{r.X, r.Y + (r.H-1)/2, r.W, 1}
					}
					p.Fill(line, t.Accent, 0)
				}
				if div.FocusVisible() {
					p.focusRingOf(div, r, [4]float32{})
				}
			})
		})
	})
	return e
}

// splitOvershoot keys a split's handle's local value: how far the pointer
// dragging the divider went past where the divider stopped, at a pane's
// limit.
type splitOvershoot struct{}

// fit clamps v, the first pane's size, to leave minPane on either side of
// the divider, gap long, in total, the room as last laid out (0 before the
// first layout, for limits that don't depend on the room), and to keep
// both panes within l, as the layout does: a minimum wins over a maximum
// and over the room. SplitSecond's limits come swapped (splitBuild.limits),
// so that v is the size of the pane *size measures.
func (l SplitLimits) fit(v, total, gap, minPane float32) float32 {
	firstMin, secondMin := max(l.FirstMin, minPane), max(l.SecondMin, minPane)
	if l.FirstMax > 0 {
		v = min(v, max(l.FirstMax, firstMin))
	}
	if total > 0 {
		room := total - gap
		if l.SecondMax > 0 {
			// First takes the room second can't, up to its own maximum.
			at := room - max(l.SecondMax, secondMin)
			if l.FirstMax > 0 {
				at = min(at, max(l.FirstMax, firstMin))
			}
			v = max(v, at)
		}
		v = min(v, room-secondMin)
	}
	return max(v, firstMin)
}
