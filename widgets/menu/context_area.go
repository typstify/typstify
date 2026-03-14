package menu

import (
	"image"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// copied from gio-x and modified

// ContextArea is a region of the UI that responds to certain
// keyboard input events by displaying a contextual widget.
type ContextArea struct {
	lastUpdate    time.Time
	position      f32.Point
	dims          D
	active        bool
	justActivated bool
	justDismissed bool
	// PositionHint tells the ContextArea the closest edge/corner of the
	// window to where it is being used in the layout. This helps it to
	// position the contextual widget without it overflowing the edge of
	// the window.
	PositionHint layout.Direction
}

func (r *ContextArea) Show(pos f32.Point) {
	r.position = pos
	r.active = true
	r.justActivated = true
}

// Update performs event processing for the context area but does not lay it out.
// It is automatically invoked by Layout() if it has not already been called during
// a given frame.
func (r *ContextArea) Update(gtx C) {
	if gtx.Now.Equal(r.lastUpdate) {
		return
	}
	r.lastUpdate = gtx.Now
	suppressionTag := &r.active

	// Dismiss the contextual widget if the user clicked outside of it.
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: suppressionTag,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		if e.Kind == pointer.Press {
			r.Dismiss()
		}
	}
}

// Layout renders the context area and also the provided widget overlaid using an op.DeferOp.
func (r *ContextArea) Layout(gtx C, w layout.Widget) D {
	r.Update(gtx)

	if !r.active {
		return D{}
	}
	suppressionTag := &r.active
	// dismissTag := &r.dims
	dims := D{Size: gtx.Constraints.Min}

	contextual := func() op.CallOp {
		macro := op.Record(gtx.Ops)
		r.dims = w(gtx)
		return macro.Stop()
	}()

	// adjust position of widget
	if int(r.position.X)+r.dims.Size.X > dims.Size.X {
		if newX := int(r.position.X) - r.dims.Size.X; newX < 0 {
			switch r.PositionHint {
			case layout.E, layout.NE, layout.SE:
				r.position.X = float32(dims.Size.X - r.dims.Size.X)
			case layout.W, layout.NW, layout.SW:
				r.position.X = 0
			}
		} else {
			r.position.X = float32(newX)
		}
	}
	if int(r.position.Y)+r.dims.Size.Y > dims.Size.Y {
		if newY := int(r.position.Y) - r.dims.Size.Y; newY < 0 {
			switch r.PositionHint {
			case layout.S, layout.SE, layout.SW:
				r.position.Y = float32(dims.Size.Y - r.dims.Size.Y)
			case layout.N, layout.NE, layout.NW:
				r.position.Y = 0
			}
		} else {
			r.position.Y = float32(newY)
		}
	}

	// Lay out a transparent scrim to block input to things beneath the
	// contextual widget.
	suppressionScrim := func() op.CallOp {
		macro2 := op.Record(gtx.Ops)
		// Set passOp here so widgets beneath it will get click event at the same time
		// (for example, treeView dismiss the context menu and reactivate a new one during continous right click).
		passStack := pointer.PassOp{}.Push(gtx.Ops)
		pr := clip.Rect(image.Rectangle{Min: image.Point{-1e6, -1e6}, Max: image.Point{1e6, 1e6}})
		stack := pr.Push(gtx.Ops)
		event.Op(gtx.Ops, suppressionTag)
		stack.Pop()
		passStack.Pop()
		return macro2.Stop()
	}()
	op.Defer(gtx.Ops, suppressionScrim)

	// Lay out the contextual widget itself.
	pos := image.Point{
		X: int(math.Round(float64(r.position.X))),
		Y: int(math.Round(float64(r.position.Y))),
	}
	macro := op.Record(gtx.Ops)
	op.Offset(pos).Add(gtx.Ops)
	contextual.Add(gtx.Ops)

	contextual = macro.Stop()
	op.Defer(gtx.Ops, contextual)

	// Capture pointer events in the contextual area.
	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Min}).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, r)

	return dims
}

// Dismiss sets the ContextArea to not be active.
func (r *ContextArea) Dismiss() {
	r.active = false
	r.justDismissed = true
}

// Active returns whether the ContextArea is currently active (whether
// it is currently displaying overlaid content or not).
func (r ContextArea) Active() bool {
	return r.active
}

// Activated returns true if the context area has become active since
// the last call to Activated.
func (r *ContextArea) Activated() bool {
	defer func() {
		r.justActivated = false
	}()
	return r.justActivated
}

// Dismissed returns true if the context area has been dismissed since
// the last call to Dismissed.
func (r *ContextArea) Dismissed() bool {
	defer func() {
		r.justDismissed = false
	}()
	return r.justDismissed
}
