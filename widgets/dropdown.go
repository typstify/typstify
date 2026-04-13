package widgets

import (
	"fmt"
	"image"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/oligo/gioview/menu"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/widgets/icons"
)

var (
	arrowRightIcon = icons.NewSvgIcon(icons.ChevronRight)
	expandMoreIcon = icons.NewSvgIcon(icons.ChevronDown)
)

type Dropdown struct {
	BorderRadus unit.Dp
	Inset       layout.Inset
	TextSize    unit.Sp

	labels        map[string]any
	selected      string
	selectChanged bool
	expanded      bool
	click         gesture.Click
	hovering      bool
	options       *menu.DropdownMenu
}

func NewDropDown(optionLabels map[string]any) *Dropdown {
	return &Dropdown{
		labels:      optionLabels,
		BorderRadus: unit.Dp(4),
		Inset:       layout.UniformInset(unit.Dp(8)),
	}
}

func (d *Dropdown) Layout(gtx C, th *theme.Theme) D {
	d.Update(gtx)

	gtx.Constraints.Min.Y = 0
	d.options.MaxWidth = unit.Dp(gtx.Constraints.Max.X / int(gtx.Metric.PxPerDp))
	d.options.Background = misc.WithAlpha(th.Bg, 0xb6)

	macro := op.Record(gtx.Ops)
	dims := layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			macro := op.Record(gtx.Ops)
			boxDims := d.layout(gtx, th, th.TextSize, d.labels[d.selected])
			callOp := macro.Stop()

			macro2 := op.Record(gtx.Ops)
			d.options.Layout(gtx, th)
			menuCall := macro2.Stop()

			defer clip.Rect(image.Rectangle{Max: boxDims.Size}).Push(gtx.Ops).Pop()
			callOp.Add(gtx.Ops)
			defer op.Offset(image.Point{Y: boxDims.Size.Y}).Push(gtx.Ops).Pop()
			// draw the dropdown list:
			menuCall.Add(gtx.Ops)

			return boxDims
		}),
		layout.Expanded(func(gtx C) D {
			return d.layoutForeground(gtx, th)
		}),
	)

	callOp := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	// register tag
	event.Op(gtx.Ops, d)
	d.click.Add(gtx.Ops)
	callOp.Add(gtx.Ops)

	return dims

}

func (d *Dropdown) layout(gtx C, th *theme.Theme, textSize unit.Sp, label any) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	borderAlpha := 128
	if d.hovering {
		borderAlpha = 221
	}

	return widget.Border{
		Color:        misc.WithAlpha(th.Fg, uint8(borderAlpha)),
		CornerRadius: d.BorderRadus,
		Width:        unit.Dp(0.5),
	}.Layout(gtx, func(gtx C) D {
		return d.Inset.Layout(gtx, func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
				Spacing:   layout.SpaceBetween,
			}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					return d.layoutText(gtx, th, textSize, fmt.Sprint(label))
				}),
				layout.Rigid(func(gtx C) D {
					icon := arrowRightIcon
					if d.expanded {
						icon = expandMoreIcon
					}

					return icon.Layout(gtx, th.Fg, th.TextSize*1.2)
				}),
			)
		})
	})
}

func (d *Dropdown) layoutText(gtx C, th *theme.Theme, size unit.Sp, txt string) D {
	// gtx.Constraints.Min.X = 0

	textColorMacro := op.Record(gtx.Ops)
	paint.ColorOp{Color: th.Fg}.Add(gtx.Ops)
	textColor := textColorMacro.Stop()

	font := font.Font{
		Typeface: th.Face,
		Weight:   font.Normal,
	}

	tl := widget.Label{
		Alignment: text.Middle,
		MaxLines:  1,
	}

	return tl.Layout(gtx, th.Shaper, font, size, txt, textColor)
}

func (d *Dropdown) layoutForeground(gtx C, th *theme.Theme) D {
	if !d.hovering {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	rect := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max})
	paint.FillShape(gtx.Ops, misc.WithAlpha(th.Fg, th.HoverAlpha), rect.Op())
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

// Update dropdown states and report if the selection changed.
func (d *Dropdown) Update(gtx C) bool {
	if d.options == nil {
		menuOpts := make([]menu.MenuOption, 0)
		for key, opt := range d.labels {
			if d.selected == "" {
				// pick a inital value
				d.selected = key
			}

			menuOpts = append(menuOpts, menu.MenuOption{
				OnClicked: func() error {
					d.selectChanged = d.selected != key
					d.selected = key
					return nil
				},
				Layout: func(gtx C, th *theme.Theme) D {
					return d.layoutText(gtx, th, th.TextSize, fmt.Sprint(opt))
				},
			})
		}
		d.options = menu.NewDropdownMenu([][]menu.MenuOption{menuOpts})
	}

	justDismissed := d.options.Update(gtx)
	if justDismissed {
		d.expanded = false
	}

	for {
		event, ok := gtx.Event(
			pointer.Filter{Target: d, Kinds: pointer.Enter | pointer.Leave | pointer.Cancel},
		)
		if !ok {
			break
		}

		switch event := event.(type) {
		case pointer.Event:
			switch event.Kind {
			case pointer.Enter:
				d.hovering = true
			case pointer.Leave:
				d.hovering = false
			case pointer.Cancel:
				d.hovering = false
			}
		}
	}

	for {
		e, ok := d.click.Update(gtx.Source)
		if !ok {
			break
		}
		if e.Kind == gesture.KindClick {
			d.expanded = !d.expanded
			if d.expanded {
				d.options.ToggleVisibility(gtx)
			}
		}
	}

	defer func() { d.selectChanged = false }()
	return d.selectChanged
}

func (d *Dropdown) Value() string {
	return d.selected
}
