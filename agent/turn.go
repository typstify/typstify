package agent

import (
	"sync"
	"sync/atomic"

	"github.com/coder/acp-go-sdk"
)

type PromptTurn struct {
	// ongoing tool calls. Map toolcall id to a session cancel notify channel.
	ongoingToolCalls map[acp.ToolCallId]chan acp.ToolCallId
	canceled         atomic.Bool
	mu               sync.Mutex
	broker           *Broker[acp.ToolCallId]
}

func NewPromptTurn() *PromptTurn {
	broker := NewBroker[acp.ToolCallId]()
	go broker.Start()
	return &PromptTurn{
		ongoingToolCalls: make(map[acp.ToolCallId]chan acp.ToolCallId),
		broker:           broker,
	}
}

func (t *PromptTurn) UpdateToolCall(update any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	removeFunc := func(toolcallID acp.ToolCallId) {
		cancelNotifyChan := t.ongoingToolCalls[toolcallID]
		if cancelNotifyChan != nil {
			t.broker.Unsubscribe(cancelNotifyChan)
		}
		delete(t.ongoingToolCalls, toolcallID)

	}

	switch update := update.(type) {
	case ToolCall:
		if update.Status == acp.ToolCallStatusPending || update.Status == acp.ToolCallStatusInProgress {
			if _, exists := t.ongoingToolCalls[update.ToolCallId]; !exists {
				t.ongoingToolCalls[update.ToolCallId] = t.broker.Subscribe()
			}
		} else {
			removeFunc(update.ToolCallId)
		}
	case ToolCallUpdate:
		if update.Status == nil {
			return
		}

		if *update.Status == acp.ToolCallStatusPending || *update.Status == acp.ToolCallStatusInProgress {
			if _, exists := t.ongoingToolCalls[update.ToolCallId]; !exists {
				t.ongoingToolCalls[update.ToolCallId] = t.broker.Subscribe()
			}
		} else {
			removeFunc(update.ToolCallId)
		}
	}
}

func (t *PromptTurn) Cancel() {
	if t.canceled.CompareAndSwap(false, true) {
		t.mu.Lock()
		for tc := range t.ongoingToolCalls {
			t.broker.Publish(tc)
		}
		t.mu.Unlock()
	}
}

func (t *PromptTurn) IsCanceled() bool {
	return t.canceled.Load()
}

func (t *PromptTurn) CancelChan(toolCallID acp.ToolCallId) <-chan acp.ToolCallId {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ongoingToolCalls[toolCallID]
}

func (t *PromptTurn) Close() {
	if t.broker != nil {
		t.broker.Stop()
	}
}
