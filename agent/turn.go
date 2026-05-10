package agent

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/coder/acp-go-sdk"
)

type PromptTurn struct {
	// ongoing tool calls.
	ongoingToolCalls []acp.ToolCallId
	canceled         atomic.Bool
	mu               sync.Mutex
	cancelChan       chan acp.ToolCallId
}

func NewPromptTurn() *PromptTurn {
	return &PromptTurn{
		cancelChan: make(chan acp.ToolCallId),
	}
}

func (t *PromptTurn) UpdateToolCall(update any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	removeFunc := func(toolcallID acp.ToolCallId) {
		t.ongoingToolCalls = slices.DeleteFunc(t.ongoingToolCalls, func(id acp.ToolCallId) bool {
			return id == toolcallID
		})
	}

	switch update := update.(type) {
	case ToolCall:
		if update.Status == acp.ToolCallStatusPending || update.Status == acp.ToolCallStatusInProgress {
			t.ongoingToolCalls = append(t.ongoingToolCalls, update.ToolCallId)

		} else {
			removeFunc(update.ToolCallId)
		}
	case ToolCallUpdate:
		if update.Status == nil {
			return
		}

		if *update.Status == acp.ToolCallStatusPending || *update.Status == acp.ToolCallStatusInProgress {
			t.ongoingToolCalls = append(t.ongoingToolCalls, update.ToolCallId)
		} else {
			removeFunc(update.ToolCallId)
		}
	}
}

func (t *PromptTurn) Cancel() {
	if t.canceled.CompareAndSwap(false, true) {
		for _, tc := range t.ongoingToolCalls {
			t.cancelChan <- tc
		}
	}

}

func (t *PromptTurn) IsCanceled() bool {
	return t.canceled.Load()
}

func (t *PromptTurn) CancelChan() <-chan acp.ToolCallId {
	return t.cancelChan
}

func (t *PromptTurn) Close() {
	close(t.cancelChan)
}
