package editors

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/lsp"
	appIcons "looz.ws/typstify/widgets/icons"
)

var compileErrorIcon = appIcons.NewSvgIcon(appIcons.CircleX)

// layoutCompileStatus shows result of latest compilation reported by tinymist, so errors are visible even when preview is not updated.
func (te *TypstEditor) layoutCompileStatus(gtx C, th *theme.Theme) D {
	if te.lspClient == nil {
		return D{}
	}

	status := te.lspClient.CompileStatus()
	if status == nil {
		return D{}
	}

	var labelText string
	textColor := th.Fg
	switch status.Status {
	case lsp.CompileStateSuccess:
		if status.PageCount == 1 {
			labelText = i18n.Translate("1 page")
		} else {
			labelText = i18n.Translate("%d pages", status.PageCount)
		}
	case lsp.CompileStateError:
		labelText = i18n.Translate("Compile error")
		textColor = color.NRGBA{R: 255, A: 255}
	default:
		return D{}
	}

	return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				if status.Status != lsp.CompileStateError {
					return D{}
				}

				return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
					return compileErrorIcon.Layout(gtx, textColor, th.TextSize)
				})
			}),
			layout.Rigid(func(gtx C) D {
				label := material.Label(th.Theme, th.TextSize*0.9, labelText)
				label.Color = textColor
				return label.Layout(gtx)
			}),
		)
	})
}
