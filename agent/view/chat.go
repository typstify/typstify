package view

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"slices"
	"strings"
	"sync"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/coder/acp-go-sdk"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/i18n"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var _ agent.SessionUpdateSubsciber = (*AgentChat)(nil)

// permissionState tracks a pending permission request with clickable option buttons.
type permissionState struct {
	request  agent.PermissionGrantRequest
	buttons  []widget.Clickable
	resolved bool
}

type messageStyle interface {
	Layout(gtx C, th *theme.Theme, msg chatMessage) D
}

// AgentChat renders a chat conversation with an ACP agent.
type AgentChat struct {
	session   *agent.ACPSession
	ctxCancel context.CancelFunc

	messages      []chatMessage
	mu            sync.Mutex
	invalidate    func()
	messageStyles []messageStyle

	list        widget.List
	scroll      bool
	inputEditor *InputBox
	sendBtn     widget.Clickable

	pendingPerm *permissionState
	configStyle SessionConfigStyle

	MaxWidth unit.Dp
}

func (v *AgentChat) SetInvalidator(fn func()) {
	v.invalidate = fn
}

func (v *AgentChat) Layout(gtx C, th *theme.Theme) D {
	// Handle submit events from the input editor.
	if ok := v.inputEditor.Update(gtx); ok {
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
			return v.layoutMessages(gtx, th, padding)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{
				Left:  padding,
				Right: padding,
			}.Layout(gtx, func(gtx C) D {
				return layout.Flex{
					Axis: layout.Vertical,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return v.layoutPermission(gtx, th)
					}),
					layout.Rigid(func(gtx C) D {
						return v.layoutInput(gtx, th)
					}),
				)
			})
		}),
	)

}

func (v *AgentChat) layoutHeader(gtx C, th *theme.Theme) D {
	agentName := v.agentDisplayName()
	sessionTitle := v.session.Title()

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
				Alignment: layout.Baseline,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize, agentName)
					label.Color = th.Fg
					label.Font.Weight = font.Black
					return label.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Flexed(1, func(gtx C) D {
					return layout.Center.Layout(gtx, func(gtx C) D {
						if sessionTitle == "" {
							return D{}
						}
						st := material.Label(th.Theme, th.TextSize*0.9, sessionTitle)
						return st.Layout(gtx)
					})
				}),
			)
		})
	})
}

func (v *AgentChat) agentDisplayName() string {
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

func (v *AgentChat) layoutMessages(gtx C, th *theme.Theme, padding unit.Dp) D {
	v.mu.Lock()
	msgs := v.messages
	scroll := v.scroll
	v.scroll = false
	v.mu.Unlock()

	if len(msgs) == 0 {
		return layout.Center.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, i18n.Translate("Type to start a conversation, press Shift+Enter to send"))
			label.Color = misc.WithAlpha(th.Fg, 0xb0)
			return label.Layout(gtx)
		})
	}

	// recorded normal layout first, checks Position.OffsetLast, and only enables
	// ScrollToEnd when the content actually overflows the viewport. If the content
	// fits, it keeps the list top-aligned.
	if scroll {
		return v.layoutMessagesWithScroll(gtx, th, padding, msgs)
	}
	if v.list.ScrollToEnd {
		return v.layoutMessagesResetShortList(gtx, th, padding, msgs)
	}

	return v.layoutMessageList(gtx, th, padding, msgs)
}

func (v *AgentChat) layoutMessagesWithScroll(gtx C, th *theme.Theme, padding unit.Dp, msgs []chatMessage) D {
	v.list.ScrollToEnd = false

	macro := op.Record(gtx.Ops)
	dims := v.layoutMessageList(gtx, th, padding, msgs)
	call := macro.Stop()

	if !v.messagesOverflow() {
		call.Add(gtx.Ops)
		return dims
	}

	v.list.ScrollToEnd = true
	v.list.Position.BeforeEnd = false
	return v.layoutMessageList(gtx, th, padding, msgs)
}

func (v *AgentChat) layoutMessagesResetShortList(gtx C, th *theme.Theme, padding unit.Dp, msgs []chatMessage) D {
	macro := op.Record(gtx.Ops)
	dims := v.layoutMessageList(gtx, th, padding, msgs)
	call := macro.Stop()

	if v.list.Position.OffsetLast <= 0 {
		call.Add(gtx.Ops)
		return dims
	}

	v.list.ScrollToEnd = false
	return v.layoutMessageList(gtx, th, padding, msgs)
}

func (v *AgentChat) messagesOverflow() bool {
	pos := v.list.Position
	return pos.First > 0 || pos.BeforeEnd || pos.OffsetLast < 0
}

func (v *AgentChat) layoutMessageList(gtx C, th *theme.Theme, padding unit.Dp, msgs []chatMessage) D {
	return v.list.Layout(gtx, len(msgs), func(gtx C, index int) D {
		return layout.Inset{
			Left:  padding,
			Right: padding,
		}.Layout(gtx, func(gtx C) D {
			return v.layoutMessage(gtx, th, msgs, index)
		})

	})
}

func (v *AgentChat) layoutMessage(gtx C, th *theme.Theme, msgs []chatMessage, index int) D {
	// Because of auto-scrolling to the end when list overflows, the index may not start from 0,
	// so we need to use a loop to append to messageStyles to catch up with the index.
	for len(v.messageStyles) <= index {
		idx := len(v.messageStyles)
		msgKind := msgs[idx].Kind

		switch msgKind {
		case msgUser:
			v.messageStyles = append(v.messageStyles, &UserMsgStyle{})
		case msgAgent:
			v.messageStyles = append(v.messageStyles, &AgentMsgStyle{})
		case msgThought:
			v.messageStyles = append(v.messageStyles, &ThoughtMsgStyle{})
		case msgToolCall:
			v.messageStyles = append(v.messageStyles, &ToolCallStyle{session: v.session})
		case msgPlan:
			v.messageStyles = append(v.messageStyles, &PlanMsgStyle{})
		default:
			log.Panicf("unknown message: %v", msgKind)
		}
	}

	msg := msgs[index]
	return v.messageStyles[index].Layout(gtx, th, msg)
}

func (v *AgentChat) layoutInput(gtx C, th *theme.Theme) D {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			rr := gtx.Dp(unit.Dp(6))
			borderRect := image.Rectangle{Max: gtx.Constraints.Min}
			defer clip.UniformRRect(borderRect, rr).Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, th.Bg)
			// Border stroke.
			defer clip.Stroke{
				Path:  clip.UniformRRect(borderRect, rr).Path(gtx.Ops),
				Width: float32(gtx.Dp(unit.Dp(1))),
			}.Op().Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, misc.WithAlpha(th.Fg, 0x12))
			return D{Size: borderRect.Max}
		}),
		layout.Stacked(func(gtx C) D {
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx C) D {
						return v.inputEditor.Layout(gtx, th)
					})
				}),
				layout.Rigid(func(gtx C) D {
					return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
						// if no options, the status bar still occupies the entire width.
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Flex{
							Axis:      layout.Horizontal,
							Spacing:   layout.SpaceBetween,
							Alignment: layout.Middle,
						}.Layout(gtx,
							layout.Flexed(1, func(gtx C) D {
								return v.configStyle.Layout(gtx, th)
							}),

							layout.Rigid(func(gtx C) D {
								return layout.Flex{
									Axis:      layout.Horizontal,
									Alignment: layout.Middle,
								}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										usage := v.session.Usage()
										if usage.Size == 0 {
											return D{}
										}
										label := material.Label(th.Theme, th.TextSize*0.8, fmt.Sprintf("Tokens: %d/%d", usage.Used, usage.Size))
										label.Color = misc.WithAlpha(th.Fg, 0x60)
										return label.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Rigid(func(gtx C) D {
										if !v.canSend() && !v.isPromptRunning() {
											gtx = gtx.Disabled()
										}

										label := i18n.Translate("Send")
										var bgColor = misc.WithAlpha(th.ContrastBg, 0xb6)
										if v.isPromptRunning() {
											bgColor = th.ContrastBg
											label = i18n.Translate("Stop")
										}
										btn := material.Button(th.Theme, &v.sendBtn, label)
										btn.Background = bgColor
										btn.Color = th.Fg
										btn.Inset = layout.UniformInset(unit.Dp(2))
										return btn.Layout(gtx)
									}),
								)
							}),
						)
					})
				}),
			)

		}),
	)
}

func (v *AgentChat) layoutPermission(gtx C, th *theme.Theme) D {
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

func (v *AgentChat) scrollToEnd() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.scroll = true
}

func (v *AgentChat) isPromptRunning() bool {
	return v.session.HasOngoingTurn()
}

func (v *AgentChat) canSend() bool {
	return len(v.inputEditor.Text()) > 0
}

func (v *AgentChat) doSend() {
	text := strings.TrimSpace(v.inputEditor.Text())
	if text == "" {
		return
	}

	blocks := v.inputEditor.Blocks()
	v.inputEditor.SetText("")

	textBlkIdx := slices.IndexFunc(blocks, func(blk acp.ContentBlock) bool { return blk.Text != nil })
	if textBlkIdx < 0 {
		panic("expects at least one text block")
	}

	// Echo the user message locally so it appears immediately.
	v.mu.Lock()
	v.messages = append(v.messages, chatMessage{
		Kind:    msgUser,
		Content: blocks[textBlkIdx].Text.Text,
	})
	v.mu.Unlock()

	v.scrollToEnd()
	v.invalidate()

	go func() {
		resp, err := v.session.Prompt(context.Background(), blocks...)
		if err != nil && !errors.Is(err, agent.ErrPromptBuffered) {
			v.mu.Lock()
			v.messages = append(v.messages, chatMessage{Kind: msgAgent, Content: fmt.Sprintf("[System Error] %s", err.Error())})
			v.mu.Unlock()
			log.Printf("chat: prompt error: %v", err)
		} else if errors.Is(err, agent.ErrPromptBuffered) {
			log.Printf("prompt buffered")
		} else {
			log.Printf("prompt turn finished, reason: %s", resp.StopReason)
		}
	}()
}

func (v *AgentChat) Session() *agent.ACPSession {
	return v.session
}

// Close cancels the subscription context.
func (v *AgentChat) Close() {
	if v.ctxCancel != nil {
		v.ctxCancel()
	}
}

// NewAgentChat creates a chat view and subscribes to session updates.
func NewAgentChat(session *agent.ACPSession) *AgentChat {
	ctx, cancel := context.WithCancel(context.Background())

	chat := &AgentChat{
		session:   session,
		ctxCancel: cancel,
		list: widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		MaxWidth:    unit.Dp(760),
		configStyle: SessionConfigStyle{Session: session},
		inputEditor: newInputBox(session),
	}

	// Start with a no-op invalidator; callers should set a real one.
	chat.invalidate = func() {}
	session.SubscribeUpdates(ctx, chat)
	return chat
}
