package widgets

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"slices"

	"gioui.org/f32"
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
	"gioui.org/widget/material"
	"gioui.org/x/component"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/widgets/icons"
	"looz.ws/typstify/widgets/menu"
)

var (
	arrowRightIcon = icons.NewSvgIcon(icons.ChevronRight)
	expandMoreIcon = icons.NewSvgIcon(icons.ChevronDown)
)

type Dropdown struct {
	BorderRadus unit.Dp
	Inset       layout.Inset
	// MaxHeightRatio set the height of the popup to ensure
	// it will not be cliped.
	MaxHeight    unit.Dp
	optionList   layout.List
	optionLabels []*InteractiveLabel
	optionArea   menu.ContextArea

	labels        map[string]any
	labelIdx      []string
	selected      string
	selectChanged bool
	expanded      bool
	pendingOpen   bool
	click         gesture.Click
	hovering      bool
}

func NewDropDown(optionLabels map[string]any) *Dropdown {
	return &Dropdown{
		labels:      optionLabels,
		labelIdx:    slices.Collect(maps.Keys(optionLabels)),
		BorderRadus: unit.Dp(4),
		Inset:       layout.UniformInset(unit.Dp(8)),
		MaxHeight:   unit.Dp(100),
	}
}

func (d *Dropdown) Layout(gtx C, th *theme.Theme) D {
	d.Update(gtx)

	gtx.Constraints.Min.Y = 0

	macro := op.Record(gtx.Ops)
	dims := layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			macro := op.Record(gtx.Ops)
			boxDims := d.layout(gtx, th, th.TextSize, d.labels[d.selected])
			callOp := macro.Stop()

			macro2 := op.Record(gtx.Ops)
			func() {
				if d.pendingOpen {
					d.optionArea.PositionHint = layout.N
					d.optionArea.Show(f32.Pt(0, float32(boxDims.Size.Y)))
					d.pendingOpen = false
				}
				d.optionArea.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Min = image.Point{}
					gtx.Constraints.Max.X = boxDims.Size.X
					gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(d.MaxHeight))
					return d.layoutOptions(gtx, th)
				})
			}()

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
	if !d.hovering && !d.expanded {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	rect := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max})
	paint.FillShape(gtx.Ops, misc.WithAlpha(th.Fg, th.HoverAlpha), rect.Op())
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func (d *Dropdown) layoutOptions(gtx C, th *theme.Theme) D {
	if !d.expanded || !d.optionArea.Active() {
		return D{}
	}
	surface := component.Surface(th.Theme)
	surface.Fill = misc.WithAlpha(th.Bg, 0xff)
	surface.CornerRadius = unit.Dp(4)
	d.optionList.Axis = layout.Vertical

	return surface.Layout(gtx, func(gtx C) D {
		return d.optionList.Layout(gtx, len(d.labels), func(gtx C, index int) D {
			if len(d.optionLabels) <= index {
				d.optionLabels = append(d.optionLabels, &InteractiveLabel{})
			}

			return d.layoutOption(gtx, th, d.optionLabels[index], d.labelIdx[index])
		})
	})
}

func (d *Dropdown) layoutOption(gtx C, th *theme.Theme, state *InteractiveLabel, optKey string) D {
	return state.Layout(gtx, th, func(gtx C, _ color.NRGBA) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{
			Top:    unit.Dp(3),
			Bottom: unit.Dp(3),
			Left:   unit.Dp(12),
			Right:  unit.Dp(12),
		}.Layout(gtx, func(gtx C) D {
			return material.Label(th.Theme, th.TextSize, fmt.Sprintf("%s", d.labels[optKey])).Layout(gtx)
		})
	})
}

// Update dropdown states and report if the selection changed.
func (d *Dropdown) Update(gtx C) bool {
	d.optionArea.Update(gtx)
	if d.optionArea.Dismissed() {
		d.expanded = false
	}

	if d.selected == "" && len(d.labelIdx) > 0 {
		// pick a inital value
		d.selected = d.labelIdx[0]
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
			if d.expanded {
				d.expanded = false
				d.optionArea.Dismiss()
			} else {
				d.expanded = true
				d.pendingOpen = true
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}

	for idx, lb := range d.optionLabels {
		if lb.Update(gtx) {
			d.selectChanged = d.selected != d.labelIdx[idx]
			d.selected = d.labelIdx[idx]
			d.expanded = false
			d.optionArea.Dismiss()
			gtx.Execute(op.InvalidateCmd{})
		} else {
			lb.Unselect()
		}
	}

	defer func() { d.selectChanged = false }()
	return d.selectChanged
}

func (d *Dropdown) Value() string {
	return d.selected
}
