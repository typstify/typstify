package lsp

import (
	"context"
	"fmt"
	"log"
	"log/slog"

	"gioui.org/io/key"
	"github.com/oligo/gvcode"
	"github.com/sahilm/fuzzy"
	"looz.ws/typstify/lsp/protocol"
)

type LspAutoCompletor struct {
	client     *Client
	filePath   string
	editor     *gvcode.Editor
	candicates []gvcode.CompletionCandidate
	resultBuf  []gvcode.CompletionCandidate
}

func NewLspAutoCompletor(client *Client, filePath string, editor *gvcode.Editor) *LspAutoCompletor {
	if client == nil {
		log.Println("LSP client is not initialized!")
		return nil
	}

	return &LspAutoCompletor{
		filePath: filePath,
		client:   client,
		editor:   editor,
	}
}

func (c *LspAutoCompletor) Trigger() gvcode.Trigger {
	trigger := gvcode.Trigger{
		Characters: []string{},
		KeyBinding: struct {
			Name      key.Name
			Modifiers key.Modifiers
		}{
			Name: "P", Modifiers: key.ModShortcut,
		}}

	if c.client.ServerCapabilities() != nil {
		trigger.Characters = c.client.ServerCapabilities().CompletionProvider.TriggerCharacters
	}
	return trigger
}

func (c *LspAutoCompletor) Suggest(ctx gvcode.CompletionContext) []gvcode.CompletionCandidate {
	// Notify LSP server the editor content has changed:
	c.client.OnEditorUpdated(c.filePath, c.editor.GetReader())

	// Then request the completion result based on the new version of content.
	c.candicates = c.candicates[:0]
	result, err := c.client.Complete(context.Background(), c.filePath, ctx.Position.Line, ctx.Position.Column)
	if err != nil {
		slog.Error("run lsp complete failed", "error", err.Error())
		return c.candicates
	}

	for _, r := range result.Items {
		var start, end protocol.Position

		switch r.TextEdit.Value.(type) {
		case protocol.InsertReplaceEdit:
			log.Println("WOW, got InsertReplaceEdit here!!!")
		case protocol.TextEdit:
			edit := r.TextEdit.Value.(protocol.TextEdit)
			start, end = edit.Range.Start, edit.Range.End
			c.candicates = append(c.candicates, gvcode.CompletionCandidate{
				Label: r.Label,
				TextEdit: gvcode.TextEdit{
					NewText: edit.NewText,
					EditRange: gvcode.EditRange{
						Start: gvcode.Position{Line: int(start.Line), Column: int(start.Character)},
						End:   gvcode.Position{Line: int(end.Line), Column: int(end.Character)},
					},
				},
				Description: r.Detail,
				Kind:        fmt.Sprintf("%s", r.Kind),
				TextFormat:  fmt.Sprintf("%s", r.InsertTextFormat),
			})
		}

	}

	return c.candicates

}

type candicatesSource struct {
	candidates []gvcode.CompletionCandidate
}

func (src *candicatesSource) String(i int) string {
	return src.candidates[i].Label
}

func (src *candicatesSource) Len() int {
	return len(src.candidates)
}

func (c *LspAutoCompletor) FilterAndRank(pattern string, candidates []gvcode.CompletionCandidate) []gvcode.CompletionCandidate {
	if pattern == "" {
		return candidates
	}

	//log.Printf("[%d] filter and rank with pattern: %s", len(candidates), pattern)
	source := &candicatesSource{candidates: candidates}

	matches := fuzzy.FindFrom(pattern, source)

	c.resultBuf = c.resultBuf[:0]
	for _, match := range matches {
		c.resultBuf = append(c.resultBuf, candidates[match.Index])
	}

	return c.resultBuf
}
