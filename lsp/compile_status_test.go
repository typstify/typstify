package lsp

import (
	"context"
	"encoding/json"
	"testing"

	"golang.org/x/exp/jsonrpc2"
)

func sendCompileStatus(t *testing.T, c *Client, params string) {
	t.Helper()

	req, err := jsonrpc2.NewNotification(rpcMethodCompileStatus, json.RawMessage(params))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.Handle(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

func TestHandleCompileStatus(t *testing.T) {
	c := newClient(nil)
	notified := 0
	c.OnCompileStatusChanged(func() { notified++ })

	if c.CompileStatus() != nil {
		t.Fatal("expected no compile status before any notification")
	}

	sendCompileStatus(t, c, `{"status":"compileSuccess","path":"/main.typ","pageCount":3,"wordsCount":null}`)
	status := c.CompileStatus()
	if status == nil || status.Status != CompileStateSuccess || status.Path != "/main.typ" || status.PageCount != 3 {
		t.Fatalf("unexpected compile status: %+v", status)
	}

	// A compilation in progress keeps last finished result.
	sendCompileStatus(t, c, `{"status":"compiling","path":"/main.typ","pageCount":3}`)
	if status := c.CompileStatus(); status.Status != CompileStateSuccess {
		t.Fatalf("expected last finished status to be kept, got %+v", status)
	}

	sendCompileStatus(t, c, `{"status":"compileError","path":"/main.typ","pageCount":0}`)
	if status := c.CompileStatus(); status.Status != CompileStateError || status.PageCount != 0 {
		t.Fatalf("unexpected compile status: %+v", status)
	}

	if notified != 2 {
		t.Fatalf("expected 2 notifications, got %d", notified)
	}
}
