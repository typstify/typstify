package view

import (
	"context"
	"fmt"
	"log"
	"sync"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/coder/acp-go-sdk"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	gvwidget "github.com/oligo/gioview/widget"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/i18n"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var _ agent.SessionUpdateSubsciber = (*AgentChatView)(nil)

// permissionState tracks a pending permission request with clickable option buttons.
type permissionState struct {
	request  agent.PermissionGrantRequest
	buttons  []widget.Clickable
	resolved bool
}

type messageStyle interface {
	Layout(gtx C, th *theme.Theme) D
}

// AgentChatView renders a chat conversation with an ACP agent.
type AgentChatView struct {
	session   *agent.ACPSession
	ctxCancel context.CancelFunc

	messages      []chatMessage
	mu            sync.Mutex
	invalidate    func()
	messageStyles []messageStyle

	list        widget.List
	inputEditor gvwidget.TextField
	sendBtn     widget.Clickable

	pendingPerm *permissionState

	MaxWidth unit.Dp
}

func (v *AgentChatView) SetInvalidator(fn func()) {
	v.invalidate = fn
}

func (v *AgentChatView) Layout(gtx C, th *theme.Theme) D {
	// Handle submit events from the input editor.
	if ok := v.inputEditor.Submitted(); ok {
		v.doSend()
	}

	if v.sendBtn.Clicked(gtx) {
		if v.isPromptRunning() {
			go v.session.Cancel(context.Background())
		} else {
			v.doSend()
		}
	}

	width := gtx.Constraints.Max.X
	padding := unit.Dp(12)

	if width-2*gtx.Dp(padding) > gtx.Dp(v.MaxWidth) {
		paddingVal := float32(width-gtx.Dp(v.MaxWidth)) / 2.0
		padding = unit.Dp(paddingVal / gtx.Metric.PxPerDp)
	}

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, th.Bg2, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return v.layoutHeader(gtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Flexed(1, func(gtx C) D {
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					return v.layoutMessages(gtx, th, padding)
				}),
				layout.Rigid(func(gtx C) D {
					return layout.Inset{
						Left:  padding,
						Right: padding,
					}.Layout(gtx, func(gtx C) D {
						return v.layoutPermission(gtx, th)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Spacer{Height: unit.Dp(4)}.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			return v.layoutInput(gtx, th)
		}),
	)

}

func (v *AgentChatView) layoutHeader(gtx C, th *theme.Theme) D {
	agentName := v.agentDisplayName()
	sessionTitle := v.session.Title()
	mode := v.session.CurrentModeID()

	return widget.Border{
		Color: misc.WithAlpha(th.Fg, 0x30),
		Width: unit.Dp(0.5),
	}.Layout(gtx, func(gtx C) D {
		return layout.Inset{
			Left:   unit.Dp(8),
			Right:  unit.Dp(8),
			Top:    unit.Dp(6),
			Bottom: unit.Dp(6),
		}.Layout(gtx, func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
				Spacing:   layout.SpaceBetween,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{
						Axis:      layout.Horizontal,
						Alignment: layout.Baseline,
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize, agentName)
							label.Color = th.Fg
							label.Font.Weight = font.Black
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx C) D {
							if mode == "" {
								return D{}
							}
							return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
								modeLabel := material.Label(th.Theme, th.TextSize*0.8, mode)
								modeLabel.Color = misc.WithAlpha(th.Fg, 0xb0)
								return modeLabel.Layout(gtx)
							})
						}),
					)
				}),
				layout.Rigid(func(gtx C) D {
					if sessionTitle == "" {
						return D{}
					}
					st := material.Label(th.Theme, th.TextSize*0.8, sessionTitle)
					st.Color = misc.WithAlpha(th.Fg, 0xb0)
					return st.Layout(gtx)
				}),
			)
		})
	})
}

func (v *AgentChatView) agentDisplayName() string {
	if !v.session.Active() {
		return "Agent"
	}

	info := v.session.Conn().AgentInfo
	if info.Title != nil && *info.Title != "" {
		return *info.Title
	}
	if info.Name != "" {
		return fmt.Sprintf("%s/%s", info.Name, info.Version)
	}

	return "Agent"
}

func (v *AgentChatView) layoutMessages(gtx C, th *theme.Theme, padding unit.Dp) D {
	v.mu.Lock()
	msgs := v.messages
	v.mu.Unlock()

	if len(msgs) == 0 {
		return layout.Center.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, "Start a conversation")
			label.Color = misc.WithAlpha(th.Fg, 0xb0)
			return label.Layout(gtx)
		})
	}

	return v.list.Layout(gtx, len(msgs), func(gtx C, index int) D {
		return layout.Inset{
			Left:  padding,
			Right: padding,
		}.Layout(gtx, func(gtx C) D {
			return v.layoutMessage(gtx, th, index, msgs[index])
		})

	})
}

func (v *AgentChatView) layoutMessage(gtx C, th *theme.Theme, index int, msg chatMessage) D {
	if len(v.messageStyles) <= index {
		switch msg.Kind {
		case msgUser:
			v.messageStyles = append(v.messageStyles, &UserMsgStyle{msg: &msg})
		case msgAgent:
			v.messageStyles = append(v.messageStyles, &AgentMsgStyle{msg: &msg})
		case msgThought:
			v.messageStyles = append(v.messageStyles, &ThoughtMsgStyle{msg: &msg})
		case msgToolCall:
			v.messageStyles = append(v.messageStyles, &ToolCallStyle{msg: &msg})
		case msgPlan:
			v.messageStyles = append(v.messageStyles, &PlanMsgStyle{msg: &msg})
		default:
			log.Panicf("unknown message: %v", msg.Kind)
		}
	}

	return v.messageStyles[index].Layout(gtx, th)

}

func (v *AgentChatView) layoutInput(gtx C, th *theme.Theme) D {
	return layout.Inset{
		Left:   unit.Dp(4),
		Right:  unit.Dp(4),
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				v.inputEditor.SingleLine = false
				return v.inputEditor.Layout(gtx, th, i18n.Translate("Type a message..."))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx C) D {
				if !v.canSend() || !v.isPromptRunning() {
					gtx = gtx.Disabled()
				}

				label := i18n.Translate("Send")
				bgColor := th.ContrastBg
				if v.isPromptRunning() {
					bgColor = misc.WithAlpha(bgColor, 0x40)
					label = i18n.Translate("Stop")
				}
				btn := material.Button(th.Theme, &v.sendBtn, label)
				btn.Background = bgColor
				return btn.Layout(gtx)
			}),
		)
	})
}

func (v *AgentChatView) layoutPermission(gtx C, th *theme.Theme) D {
	v.mu.Lock()
	perm := v.pendingPerm
	v.mu.Unlock()

	if perm == nil || perm.resolved {
		return D{}
	}

	// Check for button clicks and respond.
	for i := range perm.buttons {
		if perm.buttons[i].Clicked(gtx) {
			perm.resolved = true
			optionID := perm.request.Req.Options[i].OptionId
			go func() {
				perm.request.ResponseChan <- optionID
			}()
			// Clear pending permission after resolution.
			v.mu.Lock()
			v.pendingPerm = nil
			v.mu.Unlock()
			v.invalidate()
			return D{}
		}
	}

	return PermissionGrantPopup{Perm: perm}.Layout(gtx, th)
}

func (v *AgentChatView) isPromptRunning() bool {
	return v.session.HasOngoingTurn()
}

func (v *AgentChatView) canSend() bool {
	return len(v.inputEditor.Text()) > 0
}

func (v *AgentChatView) doSend() {
	text := v.inputEditor.Text()
	if text == "" {
		return
	}

	v.inputEditor.SetText("")

	// Echo the user message locally so it appears immediately.
	v.mu.Lock()
	v.messages = append(v.messages, chatMessage{
		Kind:    msgUser,
		Content: text,
	})
	v.mu.Unlock()
	v.invalidate()

	go func() {
		block := acp.TextBlock(text)
		_, err := v.session.Prompt(context.Background(), block)
		if err != nil {
			log.Printf("chat: prompt error: %v", err)
		}
	}()
}

func (v *AgentChatView) Session() *agent.ACPSession {
	return v.session
}

// Close cancels the subscription context.
func (v *AgentChatView) Close() {
	v.ctxCancel()
}

// NewAgentChat creates a chat view and subscribes to session updates.
func NewAgentChat(session *agent.ACPSession) *AgentChatView {
	ctx, cancel := context.WithCancel(context.Background())

	chat := &AgentChatView{
		session:   session,
		ctxCancel: cancel,
		list: widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		MaxWidth: unit.Dp(760),
	}

	// Start with a no-op invalidator; callers should set a real one.
	chat.invalidate = func() {}
	session.SubscribeUpdates(ctx, chat)
	return chat
}
