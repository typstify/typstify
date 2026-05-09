package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/coder/acp-go-sdk"
)

type (
	PromptResponse = acp.PromptResponse

	UserMessageChunk        = acp.SessionUpdateUserMessageChunk
	AgentMessageChunk       = acp.SessionUpdateAgentMessageChunk
	AgentThoughtChunk       = acp.SessionUpdateAgentThoughtChunk
	ToolCall                = acp.SessionUpdateToolCall
	ToolCallUpdate          = acp.SessionToolCallUpdate
	Plan                    = acp.SessionUpdatePlan
	AvailableCommandsUpdate = acp.SessionAvailableCommandsUpdate
	CurrentModeUpdate       = acp.SessionCurrentModeUpdate
	ConfigOptionUpdate      = acp.SessionConfigOptionUpdate
	SessionInfoUpdate       = acp.SessionSessionInfoUpdate
	UsageUpdate             = acp.SessionUsageUpdate
)

type SessionUpdateSubsciber interface {
	OnUserMessage(chunk UserMessageChunk)

	OnAgentMessage(chunk AgentMessageChunk)

	OnAgentThought(chunk AgentThoughtChunk)

	OnToolCallInit(toolCall ToolCall)

	OnToolCallUpdate(toolCallUpdate ToolCallUpdate)

	OnPlan(plan Plan)

	// OnAvailableCommandsUpdate(commands AvailableCommandsUpdate)

	// OnModeUpdate(modeUpdate CurrentModeUpdate)

	// OnConfigOptionUpdate(update ConfigOptionUpdate)

	// OnSessionInfoUpdate(sessionInfo SessionInfoUpdate)

	// OnUsageUpdate(usage UsageUpdate)
}

type ACPSession struct {
	SessionID string
	Cwd       string
	conn      *AgentConn
	// Human-readable title for the session
	title string
	// ISO 8601 timestamp of last activity
	updatedAt string
	modeState acp.SessionModeState
	// Available slash commands supported by the Agent.
	commands      []acp.AvailableCommand
	configOptions []acp.SessionConfigOption
	usage         UsageUpdate
	mu            sync.Mutex

	updateChan chan any
}

func NewACPSession(sessionID string, cwd string) *ACPSession {
	return &ACPSession{
		SessionID:  sessionID,
		Cwd:        cwd,
		updateChan: make(chan any, 1),
	}
}

func (sn *ACPSession) Active() bool {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	return sn.conn != nil
}

func (sn *ACPSession) SetConn(conn *AgentConn) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	if sn.conn != nil {
		panic("Cannot set connection of active session")
	}
	sn.conn = conn
}

func (sn *ACPSession) Conn() *AgentConn {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return sn.conn
}

func (sn *ACPSession) Title() string {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	return sn.title
}

func (sn *ACPSession) UpdatedAt() string {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return sn.updatedAt
}

func (sn *ACPSession) AvailableModes() []acp.SessionMode {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return sn.modeState.AvailableModes
}

func (sn *ACPSession) CurrentModeID() string {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return string(sn.modeState.CurrentModeId)
}

func (sn *ACPSession) AvailableCommands() []acp.AvailableCommand {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	return sn.commands
}

func (sn *ACPSession) ConfigOptions() []acp.SessionConfigOption {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	return sn.configOptions
}

func (sn *ACPSession) UpdateInfo(title string, updatedAt string) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.title = title
	sn.updatedAt = updatedAt
}

func (sn *ACPSession) SetMode(modeState acp.SessionModeState) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.modeState = modeState
}

func (sn *ACPSession) UpdateMode(modeID string) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.modeState.CurrentModeId = acp.SessionModeId(modeID)
}

func (sn *ACPSession) SetCommands(commands []acp.AvailableCommand) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.commands = commands
}

func (sn *ACPSession) SetConfigOptions(options []acp.SessionConfigOption) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.configOptions = options
}

func (sn *ACPSession) UpdateUsage(usage UsageUpdate) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.usage = usage
}

// Prompt sends content blocks to Agent. If there are no pending tool calls,
// the turn ends and the Agent respond with a StopReason and optional Usage.
func (sn *ACPSession) Prompt(ctx context.Context, contents ...acp.ContentBlock) (PromptResponse, error) {
	if !sn.Active() {
		return PromptResponse{}, errors.New("invalid session")
	}

	// Validate the content structure and kind.
	//
	// ACP: As a baseline, all Agents MUST support ContentBlock::Text and
	// ContentBlock::ResourceLink in session/prompt requests.
	for _, content := range contents {
		if err := content.Validate(); err != nil {
			return PromptResponse{}, err
		}

		isAudio := content.Audio != nil
		isImage := content.Image != nil
		isEmbeddedContext := content.Resource != nil

		if !sn.conn.AgentCapabilities.PromptCapabilities.Audio && isAudio {
			return PromptResponse{}, fmt.Errorf("unsupported content block: %s", content.Audio.Type)
		}

		if !sn.conn.AgentCapabilities.PromptCapabilities.Image && isImage {
			return PromptResponse{}, fmt.Errorf("unsupported content block: %s", content.Image.Type)
		}

		if !sn.conn.AgentCapabilities.PromptCapabilities.EmbeddedContext && isEmbeddedContext {
			return PromptResponse{}, fmt.Errorf("unsupported content block: %s", content.Resource.Type)
		}
	}

	resp, err := sn.conn.Conn.Prompt(ctx, acp.PromptRequest{
		SessionId: acp.SessionId(sn.SessionID),
		Prompt:    contents,
	})

	err = checkACPErr(err)
	if err != nil {
		return PromptResponse{}, err
	}

	return resp, nil
}

func (sn *ACPSession) PublishUpdate(update any) {
	sn.mu.Lock()
	if sn.updateChan == nil {
		sn.updateChan = make(chan any, 3)
	}
	sn.mu.Unlock()

	sn.updateChan <- update
}

func (sn *ACPSession) SubscribeUpdates(ctx context.Context, sub SessionUpdateSubsciber) {
	if sub == nil {
		return
	}

	go func() {
		for {
			select {
			case update := <-sn.updateChan:
				switch update := update.(type) {
				case UserMessageChunk:
					sub.OnUserMessage(update)
				case AgentMessageChunk:
					sub.OnAgentMessage(update)
				case AgentThoughtChunk:
					sub.OnAgentThought(update)
				case ToolCall:
					sub.OnToolCallInit(update)
				case ToolCallUpdate:
					sub.OnToolCallUpdate(update)
				case Plan:
					sub.OnPlan(update)
				case AvailableCommandsUpdate:
					sn.SetCommands(update.AvailableCommands)
				case CurrentModeUpdate:
					sn.UpdateMode(string(update.CurrentModeId))
				case ConfigOptionUpdate:
					sn.SetConfigOptions(update.ConfigOptions)
				case SessionInfoUpdate:
					sn.UpdateInfo(*update.Title, *update.UpdatedAt)
				case UsageUpdate:
					sn.UpdateUsage(update)
				default:
					log.Panicf("unknown update object: %v", update)
				}
			case <-ctx.Done():
				log.Println("session subscriber closed")
				return
			}
		}
	}()
}
