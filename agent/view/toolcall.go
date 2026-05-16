package view

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/coder/acp-go-sdk"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/widgets/icons"
)

var (
	toolcallIcon = icons.NewSvgIcon(icons.Hammer)
)

type ToolCallStyle struct {
	msg       *chatMessage
	selection widget.Selectable
	mdBock    markdownBlock
	card      CardStyle
}

func (t *ToolCallStyle) Layout(gtx C, th *theme.Theme, msg chatMessage) D {
	if msg.Kind != msgToolCall {
		return D{}
	}

	t.msg = &msg
	t.card.Icon = toolcallIcon
	return t.card.Layout(gtx, th,
		func(gtx C) D {
			return t.layoutHeader(gtx, th)
		},
		func(gtx C) D {
			return t.layoutExtraContent(gtx, th)
		},
	)
}

func (t ToolCallStyle) layoutHeader(gtx C, th *theme.Theme) D {
	tc := t.msg.ToolCall
	if tc == nil {
		return D{}
	}

	statusStr := " [" + string(tc.Status) + "]"

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			title := material.Label(th.Theme, th.TextSize, t.msg.Content)
			title.MaxLines = 1
			title.State = &t.selection
			return title.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx C) D {
			status := material.Label(th.Theme, th.TextSize*0.85, statusStr)
			status.Color = misc.WithAlpha(th.Fg, 0x80)
			return status.Layout(gtx)
		}),
	)

}

func (t ToolCallStyle) layoutExtraContent(gtx C, th *theme.Theme) D {
	tc := t.msg.ToolCall
	if tc == nil {
		return D{}
	}

	children := []layout.FlexChild{}

	if len(tc.Locations) > 0 {
		children = append(children, layout.Rigid(func(gtx C) D {
			locations := make([]layout.FlexChild, 0, len(tc.Locations))
			for _, loc := range tc.Locations {
				locations = append(locations, layout.Rigid(func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize*0.8, loc.Path)
					label.Color = misc.WithAlpha(th.Fg, 0xb0)
					return label.Layout(gtx)
				}))
			}
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx, locations...)
		}),
		)
	}

	if len(tc.Content) > 0 {
		if len(children) > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx C) D {
			contents := make([]layout.FlexChild, 0, len(tc.Content))
			for _, content := range tc.Content {
				if content.Content != nil {
					contents = append(contents, layout.Rigid(func(gtx C) D {
						return t.layoutContentBlock(gtx, th, content.Content)
					}))
				}
				if content.Diff != nil {
					contents = append(contents, layout.Rigid(func(gtx C) D {
						return t.layoutContentDiff(gtx, th, content.Diff)
					}))
				}
				if content.Terminal != nil {
					contents = append(contents, layout.Rigid(func(gtx C) D {
						return t.layoutTerminal(gtx, th, content.Terminal)
					}))
				}
			}
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx, contents...)
		}))
	}

	// if rawInput, ok := tc.RawInput.(string); ok && rawInput != "" {
	// 	children = append(children,
	// 		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
	// 		layout.Rigid(func(gtx C) D {
	// 			label := material.Label(th.Theme, th.TextSize*0.8, i18n.Translate("Input: %s", rawInput))
	// 			label.Color = misc.WithAlpha(th.Fg, 0x60)
	// 			return label.Layout(gtx)
	// 		}),
	// 	)
	// }

	// if rawOutput, ok := tc.RawOutput.(string); ok && rawOutput != "" {
	// 	children = append(children,
	// 		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
	// 		layout.Rigid(func(gtx C) D {
	// 			label := material.Label(th.Theme, th.TextSize*0.8, i18n.Translate("Output: %s", rawOutput))
	// 			label.Color = misc.WithAlpha(th.Fg, 0x60)
	// 			return label.Layout(gtx)
	// 		}),
	// 	)
	// }

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx, children...)
}

func (t ToolCallStyle) layoutContentBlock(gtx C, th *theme.Theme, content *acp.ToolCallContentContent) D {
	if content == nil {
		return D{}
	}
	text := extractText(content.Content)
	if text == "" {
		return D{}
	}

	return t.mdBock.Layout(gtx, th, []byte(text))
}

func (t ToolCallStyle) layoutContentDiff(gtx C, th *theme.Theme, content *acp.ToolCallContentDiff) D {
	if content == nil {
		return D{}
	}
	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, content.Path)
			label.Color = misc.WithAlpha(th.Fg, 0xb0)
			return label.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			if content.OldText != nil {
				label := material.Label(th.Theme, th.TextSize, "- "+*content.OldText)
				return label.Layout(gtx)
			}
			return D{}
		}),
		layout.Rigid(func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, "+ "+content.NewText)
			return label.Layout(gtx)
		}),
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (t ToolCallStyle) layoutTerminal(gtx C, th *theme.Theme, content *acp.ToolCallContentTerminal) D {
	if content == nil {
		return D{}
	}
	label := material.Label(th.Theme, th.TextSize, i18n.Translate("Terminal: %s", content.TerminalId))
	label.Color = misc.WithAlpha(th.Fg, 0x60)
	return label.Layout(gtx)
}
