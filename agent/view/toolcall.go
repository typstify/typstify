package view

import (
	"encoding/json"
	"slices"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/coder/acp-go-sdk"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/rogpeppe/go-internal/diff"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/widgets/icons"
)

var (
	toolcallIcon = icons.NewSvgIcon(icons.Hammer)
)

type ToolCallStyle struct {
	session        *agent.ACPSession
	msg            *chatMessage
	parsedDiff     []byte
	titleSelection widget.Selectable
	selection      widget.Selectable
	mdBock         markdownBlock
	card           CardStyle
	terminal       TerminalStyle
	inputCard      CardStyle
	outputCard     CardStyle
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

func (t *ToolCallStyle) layoutHeader(gtx C, th *theme.Theme) D {
	tc := t.msg.ToolCall
	if tc == nil {
		return D{}
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Flexed(1, func(gtx C) D {
			title := material.Label(th.Theme, th.TextSize, t.msg.Content)
			title.MaxLines = 1
			title.State = &t.titleSelection
			return title.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx C) D {
			return toolcallStatusWidget(gtx, th, tc.Status)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
	)

}

func (t *ToolCallStyle) layoutExtraContent(gtx C, th *theme.Theme) D {
	tc := t.msg.ToolCall
	if tc == nil {
		return D{}
	}

	children := []layout.FlexChild{}

	// if len(tc.Locations) > 0 {
	// 	children = append(children, layout.Rigid(func(gtx C) D {
	// 		locations := make([]layout.FlexChild, 0, len(tc.Locations))
	// 		for _, loc := range tc.Locations {
	// 			locations = append(locations, layout.Rigid(func(gtx C) D {
	// 				label := material.Label(th.Theme, th.TextSize*0.8, loc.Path)
	// 				label.Color = misc.WithAlpha(th.Fg, 0xb0)
	// 				return label.Layout(gtx)
	// 			}))
	// 		}
	// 		return layout.Flex{
	// 			Axis: layout.Vertical,
	// 		}.Layout(gtx, locations...)
	// 	}),
	// 	)
	// }

	if tc.RawInput != nil {
		rawBytes, err := json.MarshalIndent(tc.RawInput, "", "  ")
		if err == nil {
			rawInput := string(rawBytes)
			children = append(children,
				layout.Rigid(func(gtx C) D {
					return t.inputCard.Layout(gtx, th,
						func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize*0.8, i18n.Translate("Raw Input", rawInput))
							label.Color = misc.WithAlpha(th.Fg, 0xb0)
							return label.Layout(gtx)
						},
						func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize*0.8, rawInput)
							label.Color = misc.WithAlpha(th.Fg, 0xb0)
							return label.Layout(gtx)
						},
					)
				}),
			)
		}
	}

	if tc.RawOutput != nil {
		rawBytes, err := json.MarshalIndent(tc.RawOutput, "", "  ")
		if err == nil {
			rawOutput := string(rawBytes)
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx C) D {
					return t.outputCard.Layout(gtx, th,
						func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize*0.8, i18n.Translate("Raw Output", rawOutput))
							label.Color = misc.WithAlpha(th.Fg, 0xb0)
							return label.Layout(gtx)
						},
						func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize*0.8, rawOutput)
							label.Color = misc.WithAlpha(th.Fg, 0xb0)
							return label.Layout(gtx)
						},
					)
				}),
			)
		}
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

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx, children...)
}

var (
	markdownContentTools = []acp.ToolKind{
		acp.ToolKindFetch,
	}
)

func (t *ToolCallStyle) layoutContentBlock(gtx C, th *theme.Theme, content *acp.ToolCallContentContent) D {
	if content == nil {
		return D{}
	}
	text := extractText(content.Content)
	if text == "" {
		return D{}
	}

	if slices.Contains(markdownContentTools, t.msg.ToolCall.Kind) {
		return t.mdBock.Layout(gtx, th, []byte(text))
	}

	label := material.Label(th.Theme, th.TextSize, text)
	label.State = &t.selection
	label.LineHeightScale = 1.5
	return label.Layout(gtx)
}

func (t *ToolCallStyle) layoutContentDiff(gtx C, th *theme.Theme, content *acp.ToolCallContentDiff) D {
	if content == nil {
		return D{}
	}

	if t.parsedDiff == nil {
		var oldText, newText string
		if content.OldText != nil {
			oldText = *content.OldText
		}
		newText = content.NewText

		t.parsedDiff = diff.Diff("old", []byte(oldText), "new", []byte(newText))

	}

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, content.Path)
			label.Color = misc.WithAlpha(th.Fg, 0xb0)
			return label.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx C) D {
			if t.parsedDiff != nil {
				label := material.Label(th.Theme, th.TextSize, string(t.parsedDiff))
				label.State = &t.selection
				label.LineHeightScale = 1.5
				return label.Layout(gtx)
			}
			return D{}
		}),
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (t *ToolCallStyle) layoutTerminal(gtx C, th *theme.Theme, content *acp.ToolCallContentTerminal) D {
	if content == nil {
		return D{}
	}

	if t.session == nil {
		label := material.Label(th.Theme, th.TextSize, i18n.Translate("Terminal: %s", content.TerminalId))
		label.Color = misc.WithAlpha(th.Fg, 0x60)
		return label.Layout(gtx)
	}

	terminal := t.session.GetTerminal(content.TerminalId)

	if terminal == nil {
		label := material.Label(th.Theme, th.TextSize, i18n.Translate("Terminal: %s", content.TerminalId))
		label.Color = misc.WithAlpha(th.Fg, 0x60)
		return label.Layout(gtx)
	}

	return t.terminal.Layout(gtx, th, terminal)
}

const progressSpinnerFrameDuration = 150 * time.Millisecond

var progressSpinnerFrames = [...]string{"◐", "◓", "◑", "◒"}

// helper to render the tool call status.
func toolcallStatusWidget(gtx C, th *theme.Theme, status acp.ToolCallStatus) D {

	textSymbol := func(statusStr string) D {
		status := material.Label(th.Theme, th.TextSize*0.9, statusStr)
		status.Color = misc.WithAlpha(th.Fg, 0xb0)
		return status.Layout(gtx)
	}

	switch status {
	case acp.ToolCallStatusPending:
		return textSymbol("\u29D6")
	case acp.ToolCallStatusInProgress:
		gtx.Execute(op.InvalidateCmd{
			At: gtx.Now.Add(progressSpinnerFrameDuration),
		})

		frame := int(gtx.Now.UnixMilli()/progressSpinnerFrameDuration.Milliseconds()) % len(progressSpinnerFrames)
		return textSymbol(progressSpinnerFrames[frame])
	case acp.ToolCallStatusCompleted:
		return textSymbol("\u2713")
	case acp.ToolCallStatusFailed:
		return textSymbol("\u2717")
	default:
		return textSymbol(string(status))

	}
}
