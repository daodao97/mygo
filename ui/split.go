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
// panes.
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
		sb.place(true)
		sb.acc()
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

// splitBuild holds the divider and handle built for a Split in the current
// pass so SplitGap can size them in the same pass without positional walking
// or an extra rebuild pass.
type splitBuild struct {
	split, div, handle   *node
	size                 *float32
	total, minPane, grip float32
	vertical, inPercent  bool
	// place sizes the panes by *size, the first or the second
	// (SplitSecond), which second says.
	place  func(second bool)
	second bool
	// gap is the room between the panes, and shown the size of the pane
	// *size measures, as the divider's accessible range tells them.
	gap, shown float32
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

// acc tells assistive technology the divider's range and where it is from
// the start, so that increasing it moves it toward the end, whichever pane
// *size measures.
func (sb *splitBuild) acc() {
	at := sb.shown
	if sb.second && sb.total > 0 {
		at = sb.total - sb.gap - sb.shown
	}
	sb.div.accRange = [3]float64{float64(sb.minPane), float64(max(sb.total-sb.gap-sb.minPane, sb.minPane)), float64(at)}
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
		shown := *sb.size
		switch {
		case sb.inPercent && sb.total > 0:
			share := min(max(*sb.size, 0), 100) / 100
			shown = fitSplit(share*(sb.total-v), sb.total, v, sb.minPane)
		case !sb.inPercent && sb.total > 0 && *sb.size > sb.minPane:
			shown = fitSplit(*sb.size, sb.total, v, sb.minPane)
		}
		sb.gap, sb.shown = v, shown
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
		share := min(max(*size, 0), 100) / 100
		// shown is the size of the pane *size measures as the last frame
		// laid it out. Drags and keys move the divider from there, not from
		// *size, which a split too small for it does not show.
		shown := *size
		switch {
		case inPercent && total > 0:
			shown = fitSplit(share*(total-gap), total, gap, minPane)
		case !inPercent && total > 0 && *size > minPane:
			shown = fitSplit(*size, total, gap, minPane)
		}
		set := func(v float32) {
			v = fitSplit(v, total, gap, minPane)
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
				v = max(v, minPane)
			}
			if v != *size {
				*size = v
				e.st.markChanged()
				c.rt.consumed = true
			}
		}
		in.Children(func() {
			// place sizes the panes below.
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

			// place sizes the panes, the first or with second the second
			// by *size and the other with the rest, setting all that the
			// other call set, as SplitSecond calls it again once they were
			// built. The layout sizes them, not shown: shown uses the last
			// frame's room, and the layout uses this frame's.
			place := func(second bool) {
				sized, rest := a, b
				if second {
					sized, rest = b, a
				}
				if inPercent {
					sized.Grow(share)
					rest.Grow(1 - share)
					minLength(sized, minPane)
				} else {
					setLength(sized, *size)
					minLength(sized, min(*size, minPane))
					sized.grow, sized.basis = 0, length{}
					if vertical {
						rest.height = length{}
					} else {
						rest.width = length{}
					}
					rest.Grow(1)
				}
				minLength(rest, minPane)
			}
			place(false)

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
				div:       div,
				handle:    handle,
				size:      size,
				place:     place,
				total:     total,
				minPane:   minPane,
				grip:      grip,
				gap:       gap,
				shown:     shown,
				vertical:  vertical,
				inPercent: inPercent,
			})
			c.splits[len(c.splits)-1].acc()
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
				// The pointer goes on past a pane's limit, where the divider
				// stops: what it went past is kept, so that coming back
				// moves the divider only once the pointer is over it again.
				o, _ := keepLocal(handle.st, splitOvershoot{}, func() float32 { return 0 })
				want := shown + d + *o
				*o = want - fitSplit(want, total, gap, minPane)
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

// fitSplit clamps v, the size of the pane *size measures, to leave
// minPane on either side of the divider in total, the room as last laid
// out (0 before the first layout).
func fitSplit(v, total, gap, minPane float32) float32 {
	if total > 0 {
		v = min(v, total-gap-minPane)
	}
	return max(v, minPane)
}
