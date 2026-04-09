package editor

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"github.com/oligo/gvcode"
	"github.com/rivo/uniseg"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/ui/dialog"
	"looz.ws/typstify/widgets/icons"
)

var (
	lockedIcon   = icons.NewSvgIcon(icons.PencilOff)
	unlockedIcon = icons.NewSvgIcon(icons.Pencil)
)

type EditorStatus struct {
	// line and col position of the cursor.
	Pos image.Point
	// selected characters.
	SelectedChars int
	// Indent with tab or spaces. For tab, there is soft tab and hard tab.
	// At present only tab is allowed.
	Indentation string
	Encoding    string
	// End of line sequence, LF or CRLF.
	EndOfLine string
	Language  string
	// Compiler version if any.
	CompilerVer string
	ReadOnly    bool
	SaveErr     error

	indentClick widget.Clickable
}

func (s *EditorStatus) Layout(gtx C, th *theme.Theme, ed *TextEditor, srv *service.ServiceFacade) D {
	if s.indentClick.Clicked(gtx) {
		indentKind, tabWidth := ed.state.TabStyle()
		srv.RequestSwitch(view.Intent{
			Target:      dialog.ChangeIndentationDialogViewID,
			ShowAsModal: true,
			Params: map[string]interface{}{
				"indentation": indentKind,
				"tabWidth":    tabWidth,
				"onConfirm": func(tabKind gvcode.TabStyle, tabWidth int, convertText bool) error {
					ed.state.WithOptions(
						gvcode.WithSoftTab(tabKind == gvcode.Spaces),
						gvcode.WithTabWidth(tabWidth),
					)

					if convertText {
						oldContent := ed.state.Text()
						newContent := ed.convertIndentation(tabKind, tabWidth, oldContent)
						ed.state.SetCaret(0, len([]rune(oldContent)))
						ed.state.Insert(newContent) // use Insert to keep history, so users can undo the convert.
					}
					return nil
				},
			},
		})
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
		Spacing:   layout.SpaceBetween,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if s.SaveErr == nil {
				return D{}
			}

			return layout.Inset{
				Left:  unit.Dp(4),
				Right: unit.Dp(12),
			}.Layout(gtx, func(gtx C) D {
				label := material.Label(th.Theme, th.TextSize*0.9, "Error: "+s.SaveErr.Error())
				label.Color = color.NRGBA{R: 255, A: 255}
				label.Alignment = text.Middle
				return label.Layout(gtx)
			})

		}),

		layout.Rigid(func(gtx C) D {
			lineCol := i18n.Translate("Ln %d, Col %d", s.Pos.Y+1, s.Pos.X+1)
			if s.SelectedChars > 0 {
				lineCol += i18n.Translate(" (%d selected)", s.SelectedChars)
			}

			return layout.E.Layout(gtx, material.Label(th.Theme, th.TextSize*0.9, lineCol).Layout)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),

		layout.Rigid(func(gtx C) D {
			return material.Clickable(gtx, &s.indentClick, func(gtx C) D {
				return layout.Inset{
					Left:   unit.Dp(2),
					Right:  unit.Dp(2),
					Top:    unit.Dp(1),
					Bottom: unit.Dp(1),
				}.Layout(gtx, func(gtx C) D {
					return material.Label(th.Theme, th.TextSize*0.9, s.Indentation).Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),

		layout.Rigid(func(gtx C) D {
			return material.Label(th.Theme, th.TextSize*0.9, s.Encoding).Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),

		layout.Rigid(func(gtx C) D {
			return material.Label(th.Theme, th.TextSize*0.9, s.EndOfLine).Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),

		layout.Rigid(func(gtx C) D {
			return material.Label(th.Theme, th.TextSize*0.9, s.Language).Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),

		layout.Rigid(func(gtx C) D {
			return material.Label(th.Theme, th.TextSize*0.9, s.CompilerVer).Layout(gtx)
		}),

		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx C) D {
			var icon = unlockedIcon
			if s.ReadOnly {
				icon = lockedIcon
			}
			return icon.Layout(gtx, th.Fg, th.TextSize)
		}),
	)
}

func countWords(txt string) int {
	state := -1
	cnt := 0
	var word string
	for len(txt) > 0 {
		word, txt, state = uniseg.FirstWordInString(txt, state)
		if word != "" {
			cnt++
		}
	}

	return cnt
}
