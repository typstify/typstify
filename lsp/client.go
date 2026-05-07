package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oligo/gvcode"
	"github.com/pkg/errors"
	"golang.org/x/exp/jsonrpc2"
	"looz.ws/typstify/lsp/protocol"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst"
)

var (
	StartTimeout         = 5 * time.Second
	ConnectTimeout       = 2000 * time.Millisecond
	CommunicationTimeout = 500 * time.Millisecond
)

type Client struct {
	server *Server
	mu     sync.Mutex
	logger *slog.Logger

	jsonConn        *jsonrpc2.Connection
	connReady       atomic.Bool
	lspInitialized  atomic.Bool
	lspCapabilities *protocol.ServerCapabilities

	docCache *documentCache
	// Messages: they should be reset whenever they have been consumed.
	diagnostics []*DocDiagnostics
}

func newClient(server *Server) *Client {
	return &Client{
		server:   server,
		logger:   slog.Default(),
		docCache: newDocumentCache(),
	}
}

func (c *Client) ServerCapabilities() *protocol.ServerCapabilities {
	return c.lspCapabilities
}

// Start tinymist as a server, on `Client.Address()` port. It is started
// asynchronously (so `Start()` returns immediately) and is followed up
// by automatically connecting to it.
//
// While it is not started the various services return empty results.
func (c *Client) Start(ctx context.Context, setting *settings.Settings) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.server.IsRunning() {
		return nil
	}

	err := c.server.Start(ctx)
	if err != nil {
		return err
	}

	c.Connect(ctx, setting)
	return nil
}

func (c *Client) Connect(ctx context.Context, setting *settings.Settings) {
	initOpts := c.buildInitOptions(setting)

	go func() {
		onDeadline := time.After(StartTimeout)
		ctx := context.Background()
		for {
			// Wait a bit for server to startup, and try to connect.
			select {
			case <-onDeadline:
				// Still failing to connect, kill job.
				c.logger.Info("started server, but failed to connect, stopping it.", "timeout", StartTimeout)
				c.Stop()
				return
			case <-time.After(ConnectTimeout):
				// Wait before trying to connect (again)
			}

			c.logger.Info("trying to connect to server")
			err := c.connectServer(ctx, initOpts)
			if err != nil {
				c.logger.Error("failed to connect to LSP server", "error", err)
			} else {
				c.logger.Info("connected to server")
				return
			}
		}
	}()
}

const (
	TypstMarkup protocol.MarkupKind = "typc"
)

func (c *Client) buildInitOptions(setting *settings.Settings) map[string]any {
	typstExtraArgs := []string{}

	typstSettings := setting.Typst()

	if typstSettings.UseSysInputs != 0 {
		inputsMap, _ := typst.LoadInputs(c.server.Workspace(), false)
		sysInputs := make([]string, 0)
		for k, v := range inputsMap {
			sysInputs = append(sysInputs, fmt.Sprintf("--input=%s=%s", k, v))
		}
		typstExtraArgs = append(typstExtraArgs, sysInputs...)
	}

	if typstSettings.PackageDir != "" {
		typstExtraArgs = append(typstExtraArgs, "--package-path", typstSettings.PackageDir)
	}
	if typstSettings.PackageCacheDir != "" {
		typstExtraArgs = append(typstExtraArgs, "--package-cache-path", typstSettings.PackageCacheDir)
	}

	fontPaths := []string{c.server.Workspace()}
	if typstSettings.ExtraFontPath != "" {
		fontPaths = append(fontPaths, typstSettings.ExtraFontPath)
	}

	// TODO: Does not support yet
	// if typstSettings.IgnoreEmbeddedFonts == 1 {
	// 	typstExtraArgs = append(typstExtraArgs, "--ignore-embedded-fonts")
	// }

	syntaxOnly := "disable"
	if setting.General().EnablePowerSaving != 0 {
		syntaxOnly = "enable" // or use "onPowerSaving" to let tinymist decide?
	}

	opts := map[string]any{
		"trace": map[string]any{
			"server": "verbose",
		},
		"lint": map[string]any{
			"enabled": false,
			"when":    "onType",
		},
		"rootPath":       c.server.Workspace(),
		"fontPaths":      fontPaths,
		"systemFonts":    typstSettings.IgnoreSystemFonts == 0,
		"syntaxOnly":     syntaxOnly,
		"typstExtraArgs": typstExtraArgs,
	}

	return opts
}

// Connect to the LSP server. It also starts a goroutine to monitor receiving requests.
func (c *Client) connectServer(ctx context.Context, initOpts map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connReady.Load() {
		c.logger.Warn("already connected to LSP server")
		return nil
	}

	c.lspInitialized.Store(false)

	jsonConn, err := c.server.Connect(c)
	if err != nil {
		return err
	}
	c.jsonConn = jsonConn
	c.connReady.Store(true)

	go func() {
		c.jsonConn.Wait()
		if c.connReady.CompareAndSwap(true, false) {
			c.closeConn()
		}
		c.logger.Info("- LSP server connection stopped")
	}()

	trace := protocol.Messages
	// Send the LSP initialize request.
	initResult := protocol.InitializeResult{}
	initParams := protocol.InitializeParams{
		XInitializeParams: protocol.XInitializeParams{
			RootURI: protocol.URIFromPath(c.server.Workspace()),
			ClientInfo: &protocol.ClientInfo{
				Name: "Typstify",
			},
			Capabilities: protocol.ClientCapabilities{
				TextDocument: protocol.TextDocumentClientCapabilities{
					Synchronization: &protocol.TextDocumentSyncClientCapabilities{
						DidSave:             true,
						DynamicRegistration: false,
						WillSave:            false,
					},
					Completion: protocol.CompletionClientCapabilities{
						CompletionItem: protocol.ClientCompletionItemOptions{
							SnippetSupport:       true,
							InsertReplaceSupport: true,
							InsertTextModeSupport: &protocol.ClientCompletionItemInsertTextModeOptions{
								ValueSet: []protocol.InsertTextMode{
									protocol.AsIs,
									protocol.AdjustIndentation,
								},
							},
						},
					},
					Hover: &protocol.HoverClientCapabilities{
						ContentFormat: []protocol.MarkupKind{protocol.Markdown, protocol.PlainText},
					},
					PublishDiagnostics: &protocol.PublishDiagnosticsClientCapabilities{
						VersionSupport: false,
						DiagnosticsCapabilities: protocol.DiagnosticsCapabilities{
							TagSupport: &protocol.ClientDiagnosticsTagOptions{
								ValueSet: []protocol.DiagnosticTag{protocol.Deprecated},
							},
						},
					},
				},
				Workspace: protocol.WorkspaceClientCapabilities{
					ExecuteCommand: &protocol.ExecuteCommandClientCapabilities{
						DynamicRegistration: false,
					},
				},
			},
			InitializationOptions: initOpts,
			Trace:                 &trace,
		},
		WorkspaceFoldersInitializeParams: protocol.WorkspaceFoldersInitializeParams{
			WorkspaceFolders: []protocol.WorkspaceFolder{
				{
					URI:  string(protocol.URIFromPath(c.server.Workspace())),
					Name: path.Base(c.server.Workspace()),
				},
			},
		},
	}

	err = c.jsonConn.Call(ctx, protocol.RPCMethodInitialize, initParams).Await(ctx, &initResult)

	if err != nil {
		c.closeConn()
		return errors.Wrapf(err, "failed to call initialize call")
	}

	c.lspCapabilities = &initResult.Capabilities

	//c.logger.Info("server registered commands", "commands", c.lspCapabilities.ExecuteCommandProvider.Commands)

	err = c.jsonConn.Notify(ctx, protocol.RPCMethodInitialized, protocol.InitializedParams{})
	if err != nil {
		c.closeConn()
		return errors.Wrapf(err, "failed to send initialized notification to LSP server")
	}

	c.lspInitialized.Store(true)
	return nil
}

func (c *Client) IsReady() bool {
	return c.connReady.Load() && c.lspInitialized.Load()
}

func (c *Client) CloseConn() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connReady.CompareAndSwap(true, false) {
		return
	}

	c.closeConn()
}

func (c *Client) closeConn() {
	if c.jsonConn != nil {
		_ = c.jsonConn.Close()
		c.jsonConn = nil
	}
}

func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeConn()
	c.connReady.CompareAndSwap(true, false)
	c.server.Stop()
}

func (c *Client) Diagnostics(filePath string) *DocDiagnostics {
	idx := slices.IndexFunc(c.diagnostics, func(d *DocDiagnostics) bool {
		return d.Match(filePath)
	})
	if idx < 0 {
		return nil
	}

	return c.diagnostics[idx]

}

func (c *Client) updateDiagnostics(diagnostics DocDiagnostics) {
	idx := slices.IndexFunc(c.diagnostics, func(d *DocDiagnostics) bool {
		return d.URI == diagnostics.URI
	})
	if idx < 0 && len(diagnostics.Diagnostics) > 0 {
		c.diagnostics = append(c.diagnostics, &diagnostics)
	} else {
		c.diagnostics[idx] = &diagnostics
	}
}

// Handler implements jsonrpc2.Handler, and receives messages initiated by tinymist.
func (c *Client) Handle(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
	switch req.Method {
	case protocol.RPCMethodLogTrace:
		var params protocol.LogTraceParams
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			c.logger.Error("Failed to parse LogTraceParams", "error", err)
			return nil, err
		}
		c.logger.Info("[tinymist trace]", "message", params.Message)
	case protocol.RPCMethodLogMessage:
		var p protocol.LogMessageParams
		json.Unmarshal(req.Params, &p)
		c.logger.Info("[Tinymist log]", "message", p.Message)

	case protocol.RPCMethodPublishDiagnostics:
		var params protocol.PublishDiagnosticsParams
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			c.logger.Error("Failed to parse PublishDiagnosticsParams", "error", err)
			return nil, err
		}
		docDiagnostics := DocDiagnostics{URI: params.URI, Diagnostics: params.Diagnostics, refreshed: true}
		c.updateDiagnostics(docDiagnostics)
	default:
		c.logger.Debug("LSP notification not handled", "method", req.Method)
	}
	return nil, nil
}

// OnEditorUpdated should be called when editor loads a file, or content changed, or when the editor closed.
func (c *Client) OnEditorUpdated(filePath string, state *gvcode.Editor) {
	c.docCache.Update(filePath, state.GetReader())
	if !c.IsReady() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), CommunicationTimeout)
	defer cancel()

	err := c.notifyDidOpenOrChange(ctx, filePath)
	if err != nil {
		c.logger.Info("notifyDidOpenOrChange failed", "error", err)
	}
}

func (c *Client) OnEditorClosed(filePath string) {
	c.docCache.Remove(filePath)
	//TODO: if connection is not ready, the closing notification may never fire.
	if c.IsReady() {
		ctx, cancel := context.WithTimeout(context.Background(), CommunicationTimeout)
		defer cancel()
		c.notifyDidOpenOrChange(ctx, filePath)
	}
}

func (c *Client) OnEditorSaved(filePath string) {
	if !c.IsReady() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), CommunicationTimeout)
	defer cancel()

	doc, err := c.docCache.Get(filePath)
	if err != nil {
		return
	}

	params := &protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: doc.URI,
		},
	}

	// Notify the server that the file is safe on disk
	c.jsonConn.Notify(ctx, protocol.RPCMethodDidSave, params)
}

func (c *Client) notifyDidOpenOrChange(ctx context.Context, filePath string) (err error) {
	doc, err := c.docCache.Get(filePath)
	if err != nil {
		return err
	}

	if !doc.NeedsSync() {
		return nil
	}

	// If file got deleted since last time.
	if doc.Removed {
		c.logger.Info("NotifyDidOpenOrChange -- file deleted", "file", filePath)
		params := &protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: doc.URI,
			},
		}
		err = c.jsonConn.Notify(ctx, protocol.RPCMethodDidClose, params)
		if err != nil {
			c.logger.Error("failed to call MethodTextDocumentDidClose", "error", err)
			return errors.Wrapf(err, "Failed Client.MethodTextDocumentDidClose notification for %q", filePath)
		}
	}

	currentVersion := doc.Version
	currentContent := doc.Content()

	// else update it.
	if doc.IsNew() {
		// Notify opening a file not previously tracked.
		c.logger.Info("lsp.NotifyDidOpenOrChange -- file opened", "file", filePath)
		params := &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        doc.URI,
				LanguageID: "typst",
				Version:    int32(currentVersion),
				Text:       currentContent,
			}}

		err = c.jsonConn.Notify(ctx, protocol.RPCMethodDidOpen, params)
		if err != nil {
			err = errors.Wrapf(err, "Failed Client.NotifyDidOpenOrChange notification for %q", filePath)
		} else {
			doc.MarkSynced(currentVersion)
		}
		return
	}

	// Update the contents of the file.
	c.logger.Debug("lsp.NotifyDidOpenOrChange -- file changed ", "version", doc.Version)
	params := &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{
				URI: doc.URI,
			},
			Version: int32(currentVersion),
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{
				Text: currentContent,
			},
		},
	}
	err = c.jsonConn.Notify(ctx, protocol.RPCMethodDidChange, params)
	if err != nil {
		err = errors.Wrapf(err, "Failed Client.NotifyDidOpenOrChange::change notification for %q", filePath)
	} else {
		// Only mark this specific version as synced
		doc.MarkSynced(currentVersion)
	}
	return
}

// Complete request auto-complete suggestions from `tinymist`. It returns the completion list, or an error
// if it fails.
func (c *Client) Complete(ctx context.Context, filePath string, line, col int) (items *protocol.CompletionList, err error) {
	ctx, cancel := context.WithTimeout(ctx, CommunicationTimeout)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.IsReady() {
		return nil, errors.New("client is not connected")
	}

	c.notifyDidOpenOrChange(ctx, filePath)

	requestCompletion := func() (*protocol.CompletionList, error) {
		params := &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: protocol.URIFromPath(filePath),
				},
				Position: protocol.Position{
					Line:      uint32(line),
					Character: uint32(col),
				},
			},
			Context: protocol.CompletionContext{
				TriggerKind: protocol.Invoked,
			},
		}
		items = &protocol.CompletionList{}
		err = c.jsonConn.Call(ctx, protocol.RPCMethodCompletion, params).Await(ctx, items)
		if err != nil {
			return nil, errors.Wrapf(err, "failed call to lsp \"complete_request\"")
		}
		return items, nil
	}

	//<-time.After(time.Millisecond * 100)
	items, err = requestCompletion()

	return
}

func (c *Client) Hover(ctx context.Context, filePath string, line, col int) (*Hover, error) {
	ctx, cancel := context.WithTimeout(ctx, CommunicationTimeout)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.IsReady() {
		return nil, errors.New("client is not connected")
	}

	c.notifyDidOpenOrChange(ctx, filePath)

	params := &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.URIFromPath(filePath),
			},
			Position: protocol.Position{
				Line:      uint32(line),
				Character: uint32(col),
			},
		},
	}

	result := Hover{}
	err := c.jsonConn.Call(ctx, protocol.RPCMethodHover, params).Await(ctx, &result)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call %s", protocol.RPCMethodHover)
	}

	return &result, nil
}

// TODO: not supported yet!
func (c *Client) PullDiagnostics(ctx context.Context, filePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), CommunicationTimeout)
	defer cancel()

	params := &protocol.DocumentDiagnosticParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.URIFromPath(filePath),
		},
	}

	result := protocol.DocumentDiagnosticReport{}

	err := c.jsonConn.Call(ctx, protocol.RPCMethodDiagnostic, params).Await(ctx, &result)
	if err != nil {
		return errors.Wrapf(err, "failed to call %s", protocol.RPCMethodDiagnostic)
	}

	switch result.Value.(type) {
	case protocol.FullDocumentDiagnosticReport:
		report := result.Value.(protocol.FullDocumentDiagnosticReport)
		docDiagnostics := DocDiagnostics{
			URI:         params.TextDocument.URI,
			Diagnostics: report.Items,
			refreshed:   true,
		}

		c.updateDiagnostics(docDiagnostics)
	}

	return nil
}

func (c *Client) ExecuteCommand(ctx context.Context, cmd string, args []any) (any, error) {
	if !c.IsReady() {
		return nil, errors.New("LSP is not ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), CommunicationTimeout)
	defer cancel()

	arguments := make([]json.RawMessage, 0)
	for _, arg := range args {
		raw, err := json.Marshal(arg)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to parse command args: %v", arg))
		}

		arguments = append(arguments, json.RawMessage(raw))
		//log.Println("arguments for command: ", string(raw))
	}

	params := &protocol.ExecuteCommandParams{
		Command:   cmd,
		Arguments: arguments,
	}

	var result protocol.LSPAny
	err := c.jsonConn.Call(ctx, protocol.RPCMethodExecuteCommand, params).Await(ctx, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) NotifyWorkspaceConfigChanges(ctx context.Context, settings map[string]any) error {
	if !c.IsReady() {
		return errors.New("LSP is not ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), CommunicationTimeout)
	defer cancel()

	err := c.jsonConn.Notify(ctx, protocol.RPCMethodDidChangeConfiguration,
		protocol.DidChangeConfigurationParams{
			Settings: settings,
		})

	if err != nil {
		return errors.Wrapf(err, "failed to update workspace settings")
	}

	return nil
}

func (c *Client) DocumentSymbols(ctx context.Context, filePath string) ([]protocol.DocumentSymbol, error) {
	if !c.IsReady() {
		return nil, errors.New("LSP is not ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), CommunicationTimeout)
	defer cancel()

	c.notifyDidOpenOrChange(ctx, filePath)

	var symbols []protocol.DocumentSymbol

	err := c.jsonConn.Call(ctx, protocol.RPCMethodDocumentSymbol,
		protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.URIFromPath(filePath),
			},
		}).Await(ctx, &symbols)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to get document symbols")
	}

	return symbols, nil
}

func (c *Client) SetServreLogStreamer(dest io.Writer) {
	go func() {
		io.Copy(dest, c.server.LogReader())
		c.logger.Info("LSP server log stream ended!")
	}()
}

// Hover is the result of a hover request.
type Hover struct {
	// Contents is the hover's content
	Contents MarkupContent `json:"contents"`

	// Range an optional range is a range inside a text document
	// that is used to visualize a hover, e.g. by changing the background color.
	Range *protocol.Range `json:"range,omitempty"`
}

type MarkupContent string

// type MarkupContent struct {
// 	// Kind is the type of the Markup
// 	Kind string `json:"kind"`

// 	// Value is the content itself
// 	Value string `json:"value"`
// }

type DocDiagnostics struct {
	URI         protocol.DocumentURI
	Diagnostics []protocol.Diagnostic
	refreshed   bool
}

func (d *DocDiagnostics) Match(filePath string) bool {
	return d.URI.Path() == filePath
}

func (d *DocDiagnostics) Refreshed() bool {
	r := d.refreshed
	d.refreshed = false
	return r
}
