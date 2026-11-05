package lsp

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type PreviewMode string

const (
	DocumentPreviewMode PreviewMode = "document"
	SlidePreviewMode    PreviewMode = "slide"
)

type PreviewOptions struct {
	Mode          PreviewMode
	ProjectRoot   string
	InvertColor   string
	PartialRender bool
}

type previewTask struct {
	opts       PreviewOptions
	serverAddr string
}

// PreviewService starts a global preview server for a project root. Tinymist then
// handles the currently 'focused' file based on LSP didOpen/didChange events. When
// the previewed files changes, the preview server refreshes based on its content,
// so there is not need to start a seperate preview server for each of the file in
// the root dir. This also prevents the issue 'skip compilation for ProjectInsId("primary")
// (or the taskID) due to harmless vfs changes'.
type PreviewService struct {
	client *Client
	task   *previewTask
	mu     sync.Mutex
	// pinnedMainFile resolves the entry file pinned by the user, if any. It is
	// queried every time a preview server is started.
	pinnedMainFile func() string
}

func NewPreviwService(client *Client) *PreviewService {
	return &PreviewService{
		client: client,
	}
}

// SetPinnedMainFileProvider sets the function used to resolve the entry file
// pinned by the user when a preview server is started. It must be set before
// Start is called.
func (p *PreviewService) SetPinnedMainFileProvider(provider func() string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.pinnedMainFile = provider
}

// Start starts a new preview server by calling a Tinymist LSP command.
// The server use a randomly selected port. If there is already one active
// task, it stops it first.
func (p *PreviewService) Start(ctx context.Context, opts PreviewOptions, onFinish func()) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	retry := 10
	for retry > 0 {
		if p.client.IsReady() {
			break
		}

		time.Sleep(1 * time.Second)
		retry--
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Kill existing preveiw first.
	err := p.killLspPreview(ctx)
	if err != nil {
		return err
	}

	// Update tinymist.startDefaultPreview configs via workspace/didChangeConfiguration notification.
	err = p.updatePreview(ctx, opts)
	if err != nil {
		return err
	}

	// Pin the entry file (if any) so that the preview always compiles it no
	// matter which document is focused. This has to happen before the preview
	// is started, otherwise the first compilation uses the focused document.
	p.pinMain(ctx, p.pinnedMain())

	// Lastly start a new preview server.
	previewServerPort, err := p.startPreview(ctx)
	if err != nil {
		return err
	}

	task := &previewTask{
		opts:       opts,
		serverAddr: fmt.Sprintf("http://127.0.0.1:%d", previewServerPort),
	}
	p.task = task
	if onFinish != nil {
		onFinish()
	}
	return nil
}

// pinnedMain returns the entry file pinned by the user, or an empty string if
// no entry file is pinned. p.mu must be held by the caller.
func (p *PreviewService) pinnedMain() string {
	if p.pinnedMainFile == nil {
		return ""
	}

	return p.pinnedMainFile()
}

// PinMain pins path, which must be an absolute file path, as the main file
// compiled by the language server. Passing an empty path unpins the main file
// so the server follows the focused document again.
func (p *PreviewService) PinMain(ctx context.Context, path string) error {
	// tinymist.pinMain accepts a single argument: the path to pin, or null to
	// unpin. See https://github.com/Myriad-Dreamin/tinymist.
	var arg any
	if path != "" {
		arg = path
	}

	_, err := p.client.ExecuteCommand(ctx, "tinymist.pinMain", []any{arg})
	if err != nil {
		return fmt.Errorf("pin main file: %w", err)
	}

	return nil
}

// pinMain is like PinMain, but only logs the error. Failing to pin the main
// file should not prevent the preview from starting.
func (p *PreviewService) pinMain(ctx context.Context, path string) {
	if err := p.PinMain(ctx, path); err != nil {
		log.Println(err)
	}
}

func (p *PreviewService) Address() string {
	if p.task == nil {
		return ""
	}

	return p.task.serverAddr
}

func (p *PreviewService) Destroy(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	err := p.killLspPreview(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (p *PreviewService) updatePreview(ctx context.Context, opts PreviewOptions) error {
	args := []any{
		"--data-plane-host=127.0.0.1:0",
		fmt.Sprintf("--preview-mode=%s", string(opts.Mode)),
		fmt.Sprintf("--invert-colors=%s", opts.InvertColor),
	}

	if opts.PartialRender {
		args = append(args, "--partial-rendering=true")
	}

	args = append(args, "--no-open")

	settings := map[string]any{
		"preview": map[string]any{
			"browsing": map[string]any{
				"args": args,
			},
		},
	}

	return p.client.NotifyWorkspaceConfigChanges(ctx, settings)

}

func (p *PreviewService) startPreview(ctx context.Context) (int, error) {
	result, err := p.client.ExecuteCommand(ctx, "tinymist.startDefaultPreview", nil)
	if err != nil {
		log.Println("start previewer failed: ", err)
		return 0, err
	}

	// Try to open in built-in webview
	cmdResp, ok := result.(map[string]any)
	if !ok {
		panic("invalid cmd response type")
	}

	previewServerPort := cmdResp["staticServerPort"].(float64)

	return int(previewServerPort), nil
}

func (p *PreviewService) killLspPreview(ctx context.Context) error {
	if p.task == nil {
		return nil
	}

	// tinymist.doKillPreview has a bug in its argument handling.
	// By design it accepts empty args which indicates killing all previews. But this 
	// does not work, so we have to pass a task id here. The primary preview server has 
	// the default task id 'default_preview'.
	_, err := p.client.ExecuteCommand(ctx, "tinymist.doKillPreview", []any{"default_preview"})
	if err != nil {
		log.Printf("kill preview failed: %v", err)
		return err
	}
	p.task = nil

	return nil
}

func (p *PreviewService) scollLspPreview(ctx context.Context) error {
	// With no arguments, this scroll all preview instances to the server’s inferred
	// current cursor/focus position.
	_, err := p.client.ExecuteCommand(ctx, "tinymist.scrollPreview", []any{})
	if err != nil {
		log.Println("scroll previewer failed: ", err)
		return err
	}

	return nil
}

func (p *PreviewService) ScrollOnSelectionChange(ctx context.Context) {
	if p.task == nil {
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	p.scollLspPreview(ctx)
}
