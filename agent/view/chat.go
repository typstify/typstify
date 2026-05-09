package view

import (
	"gioui.org/layout"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/agent"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var _ agent.SessionUpdateSubsciber = (*AgentChatView)(nil)

type AgentChatView struct {
	// current session.
	session *agent.ACPSession
}

// OnAgentMessage implements [agent.SessionUpdateSubsciber].
func (ac *AgentChatView) OnAgentMessage(chunk agent.AgentMessageChunk) {
	panic("unimplemented")
}

// OnAgentThought implements [agent.SessionUpdateSubsciber].
func (ac *AgentChatView) OnAgentThought(chunk agent.AgentThoughtChunk) {
	panic("unimplemented")
}

// OnPlan implements [agent.SessionUpdateSubsciber].
func (ac *AgentChatView) OnPlan(plan agent.Plan) {
	panic("unimplemented")
}

// OnToolCallInit implements [agent.SessionUpdateSubsciber].
func (ac *AgentChatView) OnToolCallInit(toolCall agent.ToolCall) {
	panic("unimplemented")
}

// OnToolCallUpdate implements [agent.SessionUpdateSubsciber].
func (ac *AgentChatView) OnToolCallUpdate(toolCallUpdate agent.ToolCallUpdate) {
	panic("unimplemented")
}

// OnUserMessage implements [agent.SessionUpdateSubsciber].
func (ac *AgentChatView) OnUserMessage(chunk agent.UserMessageChunk) {
	panic("unimplemented")
}

func (ac *AgentChatView) Layout(gtx C, th *theme.Theme) D {

	return D{}
}

func NewAgentChat(session *agent.ACPSession) *AgentChatView {
	return &AgentChatView{
		session: session,
	}
}
