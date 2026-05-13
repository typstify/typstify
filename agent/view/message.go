package view

import (
	"fmt"
	"strings"

	"gioui.org/widget"
	"github.com/coder/acp-go-sdk"
	"looz.ws/typstify/agent"
)

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

// Helpers

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
	result := strings.Builder{}
	for i, entry := range plan.Entries {
		statusIcon := "[ ]"
		switch entry.Status {
		case "in_progress":
			statusIcon = "[>]"
		case "completed":
			statusIcon = "[x]"
		}
		result.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, statusIcon, entry.Content))
	}
	return result.String()
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
