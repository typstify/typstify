package menu

import (
	"image"
	"image/color"

	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	defaultOptionInset = layout.Inset{
		Left:   unit.Dp(24),
		Right:  unit.Dp(24),
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
	}
)

type ContextMenu struct {
	state       MenuState
	contextArea ContextArea
	// position hint
	PositionHint layout.Direction

	menuItems      []layout.FlexChild
	optionsUpdated bool

	// Background color of the menu. If unset, bg2 of theme will be used.
	Background color.NRGBA
	// Inset applied around the rendered contents of the state's Options field.
	OptionInset layout.Inset
	// Max width of the menu.
	MaxWidth unit.Dp
}

func NewContextMenu() *ContextMenu {
	return &ContextMenu{
		state: MenuState{},
	}
}

func (m *ContextMenu) Show(pos f32.Point, options [][]MenuOption) {
	m.state.SetOptions(options)
	m.optionsUpdated = false
	m.contextArea.Show(pos)
}

func (m *ContextMenu) Dismiss() {
	m.contextArea.Dismiss()
}

func (m *ContextMenu) Layout(gtx C, th *theme.Theme) D {
	m.Update(gtx)

	return m.layout(gtx, th, m.contextArea.Layout)
}

// Update the state and reports if the menu is active.
func (m *ContextMenu) Update(gtx C) bool {
	m.contextArea.Update(gtx)

	m.contextArea.PositionHint = layout.E
	if m.contextArea.Activated() {
		m.onActivated(gtx)
	}

	if m.state.RequestDismiss {
		m.contextArea.Dismiss()
		m.state.RequestDismiss = false
	}

	if m.contextArea.Active() {
		m.update(gtx)
		return true
	}

	return false
}

func (m *ContextMenu) onActivated(gtx C) {
	gtx.Execute(key.FocusCmd{Tag: m})
	m.state.FocusedOption = -1
}

func (m *ContextMenu) buildMenus(th *theme.Theme, options [][]MenuOption) {
	if m.optionsUpdated {
		return
	}

	m.menuItems = m.menuItems[:0]
	idx := 0
	for i, group := range options {
		if i != 0 {
			m.menuItems = append(m.menuItems, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return misc.Divider(layout.Horizontal, unit.Dp(1)).Layout(gtx, th)
			}))
		}

		for _, opt := range group {
			state := m.state.OptionStates[idx]
			idx++
			m.menuItems = append(m.menuItems, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return m.layoutOption(gtx, th, state, &opt)
			}))
		}
	}
	m.optionsUpdated = false
}

func (m *ContextMenu) layout(gtx C, th *theme.Theme, surface func(gtx C, w layout.Widget) D) D {
	if len(m.state.Options) <= 0 {
		return D{}
	}

	m.buildMenus(th, m.state.Options)

	macro := op.Record(gtx.Ops)
	gtx.Constraints.Min = gtx.Constraints.Max
	dims := surface(gtx, func(gtx C) D {
		gtx.Constraints.Min = image.Point{}
		return m.layoutOptions(gtx, th)
	})
	menuOps := macro.Stop()

	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	menuOps.Add(gtx.Ops)
	event.Op(gtx.Ops, m)

	return dims
}

func (m *ContextMenu) precomputeWidth(gtx C, th *theme.Theme) int {
	maxWidth := gtx.Dp(m.MaxWidth)
	if maxWidth > 0 {
		return maxWidth
	}

	var fakeOps op.Ops
	originalOps := gtx.Ops
	gtx.Ops = &fakeOps

	idx := 0
	for _, group := range m.state.Options {
		for _, opt := range group {
			state := m.state.OptionStates[idx]
			idx++

			dims := m.layoutOption(gtx, th, state, &opt)
			if dims.Size.X > maxWidth {
				maxWidth = dims.Size.X
			}
		}
	}

	gtx.Ops = originalOps
	return maxWidth
}

// layoutOptions renders the menu option list.
func (m *ContextMenu) layoutOptions(gtx C, th *theme.Theme) D {
	surface := component.Surface(th.Theme)
	surface.Fill = th.Bg
	surface.CornerRadius = unit.Dp(4)

	maxWidth := m.precomputeWidth(gtx, th)

	return surface.Layout(gtx, func(gtx C) D {
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
				Top:    unit.Dp(8),
				Bottom: unit.Dp(8),
			}.Layout(gtx, func(gtx C) D {
				if maxWidth > 0 {
					gtx.Constraints.Max.X = maxWidth
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
				}
				return layout.Flex{
					Axis: layout.Vertical,
				}.Layout(gtx, m.menuItems...)
			})

		call := macro.Stop()
		defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
		if m.Background == (color.NRGBA{}) {
			paint.ColorOp{Color: th.Bg2}.Add(gtx.Ops)
		} else {
			paint.ColorOp{Color: m.Background}.Add(gtx.Ops)
		}
		paint.PaintOp{}.Add(gtx.Ops)
		call.Add(gtx.Ops)

		return dims
	})

}

func (m *ContextMenu) layoutOption(gtx C, th *theme.Theme, state *widget.Clickable, opt *MenuOption) D {
	if state.Clicked(gtx) {
		opt.OnClicked(gtx)
		m.state.RequestDismiss = true
		gtx.Execute(op.InvalidateCmd{})
	}

	if m.OptionInset == (layout.Inset{}) {
		m.OptionInset = defaultOptionInset
	}

	return layout.Inset{
		// list scrollbar on the right side has width of 10px or 20px in HiDP system ,
		Bottom: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return material.Clickable(gtx, state, func(gtx C) D {
			macro := op.Record(gtx.Ops)
			dims := m.OptionInset.Layout(gtx, func(gtx C) D {
				return opt.Layout(gtx, th)
			})
			callOp := macro.Stop()

			defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
			if m.state.FocusedOption >= 0 && m.state.FocusedOption < len(m.state.OptionStates) && m.state.OptionStates[m.state.FocusedOption] == state {
				paint.ColorOp{Color: misc.WithAlpha(th.Fg, th.HoverAlpha)}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
			}

			callOp.Add(gtx.Ops)
			return dims
		})
	})
}

func (m *ContextMenu) update(gtx C) {
	if !gtx.Focused(m) {
		gtx.Execute(key.FocusCmd{Tag: m})
	}

	for {
		e, ok := gtx.Event(
			key.FocusFilter{Target: m},
			key.Filter{Focus: m, Name: key.NameUpArrow},
			key.Filter{Focus: m, Name: key.NameDownArrow},
			key.Filter{Focus: m, Name: key.NameEnter},
			key.Filter{Focus: m, Name: key.NameReturn},
			key.Filter{Focus: m, Name: key.NameEscape},
		)

		if !ok {
			break
		}

		switch e := e.(type) {
		case key.Event:
			if e.Name == key.NameDownArrow && e.State == key.Release {
				m.state.FocusedOption++
				if m.state.FocusedOption >= len(m.menuItems) {
					m.state.FocusedOption = 0
				}
			}
			if e.Name == key.NameUpArrow && e.State == key.Release {
				m.state.FocusedOption--
				if m.state.FocusedOption < 0 {
					m.state.FocusedOption = len(m.menuItems) - 1
				}
			}
			if (e.Name == key.NameEnter || e.Name == key.NameReturn) && e.State == key.Release {
				if m.state.FocusedOption >= 0 {
					// simulate a mouse click
					m.state.OptionStates[m.state.FocusedOption].Click()
				}
			}

			if e.Name == key.NameEscape && e.State == key.Release {
				m.state.RequestDismiss = true
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}

}
