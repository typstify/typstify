package filetree

import (
	"image"
	"image/color"
	"io"
	"log"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	gv "github.com/oligo/gioview/widget"
)

const (
	// For DnD use.
	mimeDnd = "dnd/filepath"
	// For read from clipboard use.
	mimeText = "application/text"
)

func (fn *FlatNode) Layout(gtx layout.Context, th *theme.Theme, textColor color.NRGBA, tree *TreeView) D {
	fn.Update(gtx, tree)

	inset := layout.Inset{
		Top:    fn.VerticalPadding,
		Bottom: fn.VerticalPadding,
		Left:   unit.Dp(8) + unit.Dp(fn.Depth*int(fn.IndentUnit)),
	}

	macro := op.Record(gtx.Ops)
	//dims := fn.layout(gtx, th, textColor)
	dims := fn.State.Label.Layout(gtx, th, func(gtx C, color color.NRGBA) D {
		return inset.Layout(gtx, func(gtx C) D {
			return layout.W.Layout(gtx, func(gtx C) D {
				return fn.layout(gtx, th, textColor)
			})
		})
	})

	call := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	if fn.State.Cutted {
		defer paint.PushOpacity(gtx.Ops, 0.6).Pop()
	}

	event.Op(gtx.Ops, fn.Node)
	call.Add(gtx.Ops)

	return dims
}

func (fn *FlatNode) layout(gtx layout.Context, th *theme.Theme, textColor color.NRGBA) D {
	if fn.State.Editable == nil {
		fn.State.Editable = gv.EditableLabel(fn.Node.Name(), func(text string) {
			err := fn.Node.UpdateName(text)
			if err != nil {
				log.Println("err: ", err)
			}
		})
	}

	fn.State.Editable.Color = textColor
	fn.State.Editable.TextSize = th.TextSize

	return fn.State.Draggable.Layout(gtx,
		func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					if fn.Icon == nil {
						return layout.Dimensions{}
					}
					return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
						iconColor := th.ContrastBg
						return misc.Icon{Icon: fn.Icon, Color: iconColor, Size: IconSize}.Layout(gtx, th)
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return fn.State.Editable.Layout(gtx, th)
				}),
			)
		},
		func(gtx C) D {
			return fn.layoutDraggingBox(gtx, th)
		},
	)

}

func (fn *FlatNode) layoutDraggingBox(gtx C, th *theme.Theme) D {
	if !fn.State.Draggable.Dragging() {
		return D{}
	}

	offset := fn.State.Draggable.Pos()
	if offset.Round().X == 0 && offset.Round().Y == 0 {
		return D{}
	}

	macro := op.Record(gtx.Ops)
	dims := func(gtx C) D {
		return widget.Border{
			Color:        th.ContrastBg,
			Width:        unit.Dp(1),
			CornerRadius: unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				lb := material.Label(th.Theme, th.TextSize, fn.Node.Name())
				lb.Color = th.ContrastFg
				return lb.Layout(gtx)
			})
		})
	}(gtx)
	call := macro.Stop()

	defer clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(8))).Push(gtx.Ops).Pop()
	paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, 0xb6)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	defer paint.PushOpacity(gtx.Ops, 0.8).Pop()
	call.Add(gtx.Ops)

	return dims
}

func (fn *FlatNode) Update(gtx C, tree *TreeView) error {
	if err := fn.processDndEvents(gtx, fn.State, tree); err != nil {
		return err
	}

	return nil
}

func (fn *FlatNode) processDndEvents(gtx C, state *NodeState, tree *TreeView) error {
	filters := []event.Filter{
		// Detect if pointer is inside of the dir item, so we can highlight it when dropping items to it.
		pointer.Filter{Target: fn.Node, Kinds: pointer.Enter | pointer.Leave},
		transfer.TargetFilter{Target: fn.Node, Type: mimeDnd},
	}

	for {
		ke, ok := gtx.Event(filters...)
		if !ok {
			break
		}

		switch event := ke.(type) {
		case pointer.Event:
			switch event.Kind {
			case pointer.Enter:
				state.Entered = true
				if state.DndInited {
					tree.UpdateDropTarget(fn.Node)
				}
			case pointer.Leave:
				state.Entered = false
				tree.UpdateDropTarget(nil)
			}
		case transfer.InitiateEvent:
			state.DndInited = true

		case transfer.CancelEvent:
			state.DndInited = false
			state.Entered = false
		case transfer.DataEvent:
			// read the clipboard content:
			reader := event.Open()
			defer reader.Close()
			source, err := io.ReadAll(reader)
			if err != nil {
				return err
			}

			log.Printf("flatNode data event, dest: %s, src: %s", fn.Node.Path, string(source))

			if event.Type == mimeDnd {
				tree.OnDropped(fn.Node, string(source))
			}

		}
	}

	//Process transfer.RequestEvent for draggable.
	if state.Draggable.Type == "" {
		state.Draggable.Type = mimeDnd
	}
	if m, ok := state.Draggable.Update(gtx); ok {
		state.Draggable.Offer(gtx, m, newFileNodeReader(fn.Node))
	}

	return nil
}
