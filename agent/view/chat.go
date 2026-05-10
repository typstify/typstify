package view

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"sync"

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
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var _ agent.SessionUpdateSubsciber = (*AgentChatView)(nil)

type msgKind int

const (
	msgUser msgKind = iota
	msgAgent
	msgThought
	msgToolCall
	msgPlan
	msgPermission
)

type chatMessage struct {
	Kind      msgKind
	Content   string
	MessageID string
	ToolCall  *agent.ToolCall
	Plan      *agent.Plan
}

// permissionState tracks a pending permission request with clickable option buttons.
type permissionState struct {
	request  agent.PermissionGrantRequest
	buttons  []widget.Clickable
	resolved bool
}

// AgentChatView renders a chat conversation with an ACP agent.
type AgentChatView struct {
	session   *agent.ACPSession
	ctxCancel context.CancelFunc

	messages   []chatMessage
	mu         sync.Mutex
	invalidate func()

	list        widget.List
	inputEditor widget.Editor
	sendBtn     widget.Clickable

	pendingPerm *permissionState
}

func (v *AgentChatView) SetInvalidator(fn func()) {
	v.invalidate = fn
}

// --- SessionUpdateSubsciber ---

func (v *AgentChatView) OnUserMessage(chunk agent.UserMessageChunk) {
	text := extractText(chunk.Content)
	v.mu.Lock()
	if len(v.messages) > 0 {
		last := &v.messages[len(v.messages)-1]
		if last.Kind == msgUser {
			if sameID(last.MessageID, chunk.MessageId) {
				// Same message, append chunk.
				last.Content += text
			} else if last.MessageID == "" {
				// Local echo from doSend(); merge with first server chunk.
				last.MessageID = derefStr(chunk.MessageId)
				if text != "" {
					last.Content = text
				}
			} else {
				// New user message from server.
				v.messages = append(v.messages, chatMessage{
					Kind:      msgUser,
					Content:   text,
					MessageID: derefStr(chunk.MessageId),
				})
			}
			v.mu.Unlock()
			v.invalidate()
			return
		}
	}
	v.messages = append(v.messages, chatMessage{
		Kind:      msgUser,
		Content:   text,
		MessageID: derefStr(chunk.MessageId),
	})
	v.mu.Unlock()
	v.invalidate()
}

func (v *AgentChatView) OnAgentMessage(chunk agent.AgentMessageChunk) {
	text := extractText(chunk.Content)
	v.mu.Lock()
	if len(v.messages) > 0 {
		last := &v.messages[len(v.messages)-1]
		if last.Kind == msgAgent && sameID(last.MessageID, chunk.MessageId) {
			last.Content += text
			v.mu.Unlock()
			v.invalidate()
			return
		}
	}
	v.messages = append(v.messages, chatMessage{
		Kind:      msgAgent,
		Content:   text,
		MessageID: derefStr(chunk.MessageId),
	})
	v.mu.Unlock()
	v.invalidate()
}

func (v *AgentChatView) OnAgentThought(chunk agent.AgentThoughtChunk) {
	text := extractText(chunk.Content)
	v.mu.Lock()
	if len(v.messages) > 0 {
		last := &v.messages[len(v.messages)-1]
		if last.Kind == msgThought && sameID(last.MessageID, chunk.MessageId) {
			last.Content += text
			v.mu.Unlock()
			v.invalidate()
			return
		}
	}
	v.messages = append(v.messages, chatMessage{
		Kind:      msgThought,
		Content:   text,
		MessageID: derefStr(chunk.MessageId),
	})
	v.mu.Unlock()
	v.invalidate()
}

func (v *AgentChatView) OnToolCallInit(toolCall agent.ToolCall) {
	v.mu.Lock()
	// Deduplicate by ToolCallId.
	for i := len(v.messages) - 1; i >= 0; i-- {
		if v.messages[i].Kind == msgToolCall && v.messages[i].ToolCall != nil &&
			string(v.messages[i].ToolCall.ToolCallId) == string(toolCall.ToolCallId) {
			v.mu.Unlock()
			return
		}
	}
	tc := toolCall
	v.messages = append(v.messages, chatMessage{
		Kind:      msgToolCall,
		Content:   tc.Title,
		MessageID: string(tc.ToolCallId),
		ToolCall:  &tc,
	})
	v.mu.Unlock()
	v.invalidate()
}

func (v *AgentChatView) OnToolCallUpdate(update agent.ToolCallUpdate) {
	v.mu.Lock()
	for i := len(v.messages) - 1; i >= 0; i-- {
		if v.messages[i].Kind == msgToolCall && v.messages[i].ToolCall != nil &&
			string(v.messages[i].ToolCall.ToolCallId) == string(update.ToolCallId) {
			tc := v.messages[i].ToolCall
			if update.Title != nil {
				tc.Title = *update.Title
				v.messages[i].Content = *update.Title
			}
			if update.Status != nil {
				tc.Status = *update.Status
			}
			if update.Kind != nil {
				tc.Kind = *update.Kind
			}
			if update.Content != nil {
				tc.Content = update.Content
			}
			if update.RawInput != nil {
				tc.RawInput = update.RawInput
			}
			if update.RawOutput != nil {
				tc.RawOutput = update.RawOutput
			}
			v.mu.Unlock()
			v.invalidate()
			return
		}
	}
	v.mu.Unlock()
}

func (v *AgentChatView) OnPlan(plan agent.Plan) {
	v.mu.Lock()
	// Replace latest plan message or insert new one.
	for i := len(v.messages) - 1; i >= 0; i-- {
		if v.messages[i].Kind == msgPlan {
			p := plan
			v.messages[i].Plan = &p
			v.messages[i].Content = formatPlan(&p)
			v.mu.Unlock()
			v.invalidate()
			return
		}
	}
	p := plan
	v.messages = append(v.messages, chatMessage{
		Kind:    msgPlan,
		Content: formatPlan(&p),
		Plan:    &p,
	})
	v.mu.Unlock()
	v.invalidate()
}

func (v *AgentChatView) OnRequestPermission(params agent.PermissionGrantRequest) {
	v.mu.Lock()
	buttons := make([]widget.Clickable, len(params.Req.Options))
	v.pendingPerm = &permissionState{
		request:  params,
		buttons:  buttons,
		resolved: false,
	}
	v.mu.Unlock()
	v.invalidate()
}

// --- Layout ---

func (v *AgentChatView) Layout(gtx C, th *theme.Theme) D {
	// Handle submit events from the input editor.
	for {
		ev, ok := v.inputEditor.Update(gtx)
		if !ok {
			break
		}
		if _, isSubmit := ev.(widget.SubmitEvent); isSubmit {
			v.doSend()
		}
	}
	if v.sendBtn.Clicked(gtx) {
		v.doSend()
	}

	return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return v.layoutHeader(gtx, th)
			}),
			layout.Rigid(func(gtx C) D {
				return layout.Spacer{Height: unit.Dp(4)}.Layout(gtx)
			}),
			layout.Flexed(1, func(gtx C) D {
				return v.layoutMessages(gtx, th)
			}),
			layout.Rigid(func(gtx C) D {
				return v.layoutPermission(gtx, th)
			}),
			layout.Rigid(func(gtx C) D {
				return layout.Spacer{Height: unit.Dp(4)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				return v.layoutInput(gtx, th)
			}),
		)
	})
}

func (v *AgentChatView) layoutHeader(gtx C, th *theme.Theme) D {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, misc.WithAlpha(th.Bg2, 0xe8), clip.Rect{Max: gtx.Constraints.Max}.Op())

	agentName := v.agentDisplayName()
	sessionTitle := v.session.Title()
	mode := v.session.CurrentModeID()

	return layout.Inset{
		Left: unit.Dp(8), Right: unit.Dp(8),
		Top: unit.Dp(4), Bottom: unit.Dp(4),
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
						label.Font.Weight = 600
						return label.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						if mode == "" {
							return D{}
						}
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
							modeLabel := material.Label(th.Theme, th.TextSize*0.75, mode)
							modeLabel.Color = misc.WithAlpha(th.Fg, 0x50)
							return modeLabel.Layout(gtx)
						})
					}),
				)
			}),
			layout.Rigid(func(gtx C) D {
				if sessionTitle == "" {
					return D{}
				}
				st := material.Label(th.Theme, th.TextSize*0.85, sessionTitle)
				st.Color = misc.WithAlpha(th.Fg, 0x70)
				return st.Layout(gtx)
			}),
		)
	})
}

func (v *AgentChatView) agentDisplayName() string {
	conn := v.session.Conn()
	if conn == nil {
		return "Agent"
	}
	info := conn.AgentInfo
	if info.Title != nil && *info.Title != "" {
		return *info.Title
	}
	if info.Name != "" {
		return info.Name
	}
	return "Agent"
}

func (v *AgentChatView) layoutMessages(gtx C, th *theme.Theme) D {
	v.mu.Lock()
	msgs := v.messages
	v.mu.Unlock()

	if len(msgs) == 0 {
		return layout.Center.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize*0.85, "Start a conversation")
			label.Color = misc.WithAlpha(th.Fg, 0x50)
			return label.Layout(gtx)
		})
	}

	return v.list.Layout(gtx, len(msgs), func(gtx C, index int) D {
		return v.layoutMessage(gtx, th, msgs[index])
	})
}

func (v *AgentChatView) layoutMessage(gtx C, th *theme.Theme, msg chatMessage) D {
	switch msg.Kind {
	case msgUser:
		return v.layoutUserBubble(gtx, th, msg.Content)
	case msgAgent:
		return v.layoutAgentText(gtx, th, msg.Content)
	case msgThought:
		return v.layoutThought(gtx, th, msg.Content)
	case msgToolCall:
		return v.layoutToolCall(gtx, th, msg)
	case msgPlan:
		return v.layoutPlan(gtx, th, msg.Content)
	default:
		return D{}
	}
}

func (v *AgentChatView) layoutUserBubble(gtx C, th *theme.Theme, content string) D {
	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
		Left: unit.Dp(40),
	}.Layout(gtx, func(gtx C) D {
		// Measure the label first so we know the background size.
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top: unit.Dp(6), Bottom: unit.Dp(6),
			Left: unit.Dp(8), Right: unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize*0.9, content)
			label.Color = th.Fg
			return label.Layout(gtx)
		})
		call := macro.Stop()

		// Draw background behind the label.
		defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
		paint.FillShape(gtx.Ops, misc.WithAlpha(th.ContrastBg, 0x12), clip.Rect{Max: dims.Size}.Op())

		call.Add(gtx.Ops)
		return dims
	})
}

func (v *AgentChatView) layoutAgentText(gtx C, th *theme.Theme, content string) D {
	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
		Right: unit.Dp(40),
	}.Layout(gtx, func(gtx C) D {
		label := material.Label(th.Theme, th.TextSize*0.9, content)
		label.Color = th.Fg
		return label.Layout(gtx)
	})
}

func (v *AgentChatView) layoutThought(gtx C, th *theme.Theme, content string) D {
	return layout.Inset{
		Top: unit.Dp(2), Bottom: unit.Dp(2),
		Left: unit.Dp(12), Right: unit.Dp(40),
	}.Layout(gtx, func(gtx C) D {
		label := material.Label(th.Theme, th.TextSize*0.8, content)
		label.Color = misc.WithAlpha(th.Fg, 0x60)
		label.Font.Style = 1 // italic
		return label.Layout(gtx)
	})
}

func (v *AgentChatView) layoutToolCall(gtx C, th *theme.Theme, msg chatMessage) D {
	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
		Left: unit.Dp(4), Right: unit.Dp(40),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				statusStr := ""
				if msg.ToolCall != nil {
					statusStr = " [" + string(msg.ToolCall.Status) + "]"
				}
				status := material.Label(th.Theme, th.TextSize*0.85, "Tool: "+msg.Content+statusStr)
				status.Color = misc.WithAlpha(th.Fg, 0x80)
				return status.Layout(gtx)
			}),
		)
	})
}

func (v *AgentChatView) layoutPlan(gtx C, th *theme.Theme, content string) D {
	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
		Left: unit.Dp(4), Right: unit.Dp(40),
	}.Layout(gtx, func(gtx C) D {
		label := material.Label(th.Theme, th.TextSize*0.85, content)
		label.Color = misc.WithAlpha(th.Fg, 0x80)
		return label.Layout(gtx)
	})
}

func (v *AgentChatView) layoutInput(gtx C, th *theme.Theme) D {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, misc.WithAlpha(th.Bg2, 0xe8), clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Inset{
		Left: unit.Dp(4), Right: unit.Dp(4),
		Top: unit.Dp(4), Bottom: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				editor := material.Editor(th.Theme, &v.inputEditor, "Type a message...")
				editor.TextSize = th.TextSize * 0.9
				return editor.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx C) D {
				if !v.canSend() {
					gtx = gtx.Disabled()
				}
				return v.sendBtn.Layout(gtx, func(gtx C) D {
					return layout.Inset{
						Top: unit.Dp(4), Bottom: unit.Dp(4),
						Left: unit.Dp(8), Right: unit.Dp(8),
					}.Layout(gtx, func(gtx C) D {
						defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
						bgColor := th.ContrastBg
						if !v.canSend() {
							bgColor = misc.WithAlpha(bgColor, 0x40)
						}
						paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
						label := material.Label(th.Theme, th.TextSize*0.85, "Send")
						label.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
						return label.Layout(gtx)
					})
				})
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

	toolTitle := ""
	if perm.request.Req.ToolCall.Title != nil {
		toolTitle = *perm.request.Req.ToolCall.Title
	}
	if toolTitle == "" {
		toolTitle = string(perm.request.Req.ToolCall.ToolCallId)
	}

	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
		Left: unit.Dp(4), Right: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		paint.FillShape(gtx.Ops, misc.WithAlpha(th.ContrastBg, 0x08), clip.Rect{Max: gtx.Constraints.Max}.Op())

		return layout.Inset{
			Top: unit.Dp(8), Bottom: unit.Dp(8),
			Left: unit.Dp(8), Right: unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					prompt := material.Label(th.Theme, th.TextSize*0.85, "Allow "+toolTitle+"?")
					prompt.Color = th.Fg
					prompt.Font.Weight = 600
					return prompt.Layout(gtx)
				}),
				layout.Rigid(func(gtx C) D {
					return layout.Spacer{Height: unit.Dp(6)}.Layout(gtx)
				}),
				layout.Rigid(func(gtx C) D {
					return v.layoutPermissionOptions(gtx, th, perm)
				}),
			)
		})
	})
}

func (v *AgentChatView) layoutPermissionOptions(gtx C, th *theme.Theme, perm *permissionState) D {
	options := perm.request.Req.Options
	if len(options) == 0 {
		return D{}
	}

	children := make([]layout.FlexChild, 0, len(options))
	for i := range options {
		idx := i // capture
		opt := options[idx]
		children = append(children, layout.Rigid(func(gtx C) D {
			return perm.buttons[idx].Layout(gtx, func(gtx C) D {
				return layout.Inset{
					Top: unit.Dp(3), Bottom: unit.Dp(3),
					Left: unit.Dp(8), Right: unit.Dp(8),
				}.Layout(gtx, func(gtx C) D {
					defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
					bg, fg := permissionColors(th, opt.Kind)
					if perm.buttons[idx].Hovered() {
						bg = misc.WithAlpha(bg, 0xcc)
					}
					paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
					label := material.Label(th.Theme, th.TextSize*0.8, permissionLabel(opt.Kind, opt.Name))
					label.Color = fg
					return label.Layout(gtx)
				})
			})
		}))
		if i < len(options)-1 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
		}
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx, children...)
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
	}

	// Start with a no-op invalidator; callers should set a real one.
	chat.invalidate = func() {}
	session.SubscribeUpdates(ctx, chat)
	return chat
}

// --- Helpers ---

func permissionLabel(kind acp.PermissionOptionKind, name string) string {
	if name != "" {
		return name
	}
	switch kind {
	case acp.PermissionOptionKindAllowOnce:
		return "Allow Once"
	case acp.PermissionOptionKindAllowAlways:
		return "Allow Always"
	case acp.PermissionOptionKindRejectOnce:
		return "Reject Once"
	case acp.PermissionOptionKindRejectAlways:
		return "Reject Always"
	default:
		return string(kind)
	}
}

func permissionColors(th *theme.Theme, kind acp.PermissionOptionKind) (bg, fg color.NRGBA) {
	switch kind {
	case acp.PermissionOptionKindAllowOnce, acp.PermissionOptionKindAllowAlways:
		return misc.WithAlpha(color.NRGBA{R: 0, G: 200, B: 100, A: 255}, 0x18),
			color.NRGBA{R: 0, G: 180, B: 80, A: 255}
	case acp.PermissionOptionKindRejectOnce, acp.PermissionOptionKindRejectAlways:
		return misc.WithAlpha(color.NRGBA{R: 220, G: 60, B: 60, A: 255}, 0x18),
			color.NRGBA{R: 200, G: 50, B: 50, A: 255}
	default:
		return th.Bg2, th.Fg
	}
}

func extractText(block acp.ContentBlock) string {
	if block.Text != nil {
		return block.Text.Text
	}
	return ""
}

func sameID(a string, b *string) bool {
	if b == nil {
		return a == ""
	}
	return a == *b
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatPlan(plan *agent.Plan) string {
	if plan == nil {
		return ""
	}
	result := "Plan:\n"
	for i, entry := range plan.Entries {
		statusIcon := "[ ]"
		switch entry.Status {
		case "in_progress":
			statusIcon = "[>]"
		case "completed":
			statusIcon = "[x]"
		}
		result += fmt.Sprintf("%d. %s %s\n", i+1, statusIcon, entry.Content)
	}
	return result
}
