package widgets

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	wg "gioui.org/widget"
	"gioui.org/widget/material"
)

// Editable is an editable label that layouts an editor in responds to clicking.
type Editable struct {
	Text     string
	TextSize unit.Sp
	Color    color.NRGBA

	//The callback to be called when Text is changed.
	OnChanged func(text string) error
	// OnCancel is called when editing is cancelled (Escape or focus loss).
	OnCancel func()

	editor        wg.Editor
	hovering      bool
	editorFocused bool
	editing       bool
}

func EditableLabel(text string) *Editable {
	return &Editable{
		Text: text,
	}
}

func (e *Editable) SetEditing(editing bool) {
	e.editing = editing
	e.editor.SetText(e.Text)
	e.editor.SetCaret(0, e.editor.Len())
}

func (e *Editable) IsEditing() bool {
	return e.editing
}

func (e *Editable) Update(gtx C) {
	e.editor.SingleLine = true
	e.editor.Submit = true

	for {
		event, ok := gtx.Event(
			key.Filter{Focus: &e.editor, Name: key.NameEscape},
			pointer.Filter{Target: e, Kinds: pointer.Enter | pointer.Leave | pointer.Cancel},
		)
		if !ok {
			break
		}

		switch event := event.(type) {
		case key.Event:
			if event.Name == key.NameEscape {
				e.quit()
			}
		case pointer.Event:
			switch event.Kind {
			case pointer.Enter:
				e.hovering = true
			case pointer.Leave, pointer.Cancel:
				e.hovering = false
			}
		}
	}

	if e.editing && !e.editorFocused {
		gtx.Execute(key.FocusCmd{Tag: &e.editor})
	}

	if gtx.Focused(&e.editor) {
		e.editorFocused = true
	} else if e.editorFocused {
		// editor not focused and but was focused, that is it lost focus.
		defer e.quit()
	}

	// handle editor events:
	if ev, ok := e.editor.Update(gtx); ok {
		if _, ok := ev.(wg.SubmitEvent); ok {
			text := e.editor.Text()
			if e.OnChanged != nil {
				if err := e.OnChanged(text); err == nil {
					e.editing = false
					e.Text = text
				}
			} else {
				e.editing = false
				e.Text = text
			}
		}
	}
}

func (e *Editable) quit() {
	e.editing = false
	e.editorFocused = false
	if e.OnCancel != nil {
		e.OnCancel()
	}
}

func (e *Editable) Layout(gtx C, th *material.Theme) D {
	e.Update(gtx)

	textSize := e.TextSize
	if textSize <= 0 {
		textSize = th.TextSize
	}

	if e.editing {
		return wg.Border{
			Color:        th.ContrastBg,
			Width:        unit.Dp(1),
			CornerRadius: unit.Dp(4),
		}.Layout(gtx, func(gtx C) D {
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
				editor := material.Editor(th, &e.editor, "")
				editor.TextSize = textSize
				editor.Color = e.Color
				return editor.Layout(gtx)
			})
		})
	}

	macro := op.Record(gtx.Ops)
	dims := func() D {
		lb := material.Label(th, textSize, e.Text)
		lb.Color = e.Color
		return lb.Layout(gtx)
	}()
	callOp := macro.Stop()

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, e)
	callOp.Add(gtx.Ops)

	return dims
}
