package preview

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"time"

	"github.com/oligo/gvcode"

	"looz.ws/typstify/lsp"
)

type PreviewOptions struct {
	PreviewMode      string
	ProjectRoot      string
	FontPath         string
	PackagePath      string
	PackageCachePath string
	InvertColor      string
	PartialRender    bool
	OpenInBrowser    bool
}

type previewTask struct {
	taskID     string
	targetFile string
	opts       PreviewOptions
	serverAddr string
}

// PreviewClient is the previewer client.
type PreviewClient struct {
	client     *lsp.Client
	task       *previewTask
	targetFile string
}

func NewPreviwClient(client *lsp.Client, targetFile string) *PreviewClient {
	return &PreviewClient{
		client:     client,
		targetFile: targetFile,
	}
}

func (p *PreviewClient) New(ctx context.Context, opts PreviewOptions) (string, error) {
	if p.task != nil {
		if opts == p.task.opts {
			return "", nil
		}
		// else kill existing one and create a new previewer
		err := p.killLspPreview(ctx)
		if err != nil {
			return "", err
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	taskID, previewServerPort, err := p.requestLspPreview(ctx, p.targetFile, opts)
	if err != nil {
		return "", err
	}

	task := &previewTask{
		taskID:     taskID,
		targetFile: p.targetFile,
		opts:       opts,
		serverAddr: fmt.Sprintf("http://127.0.0.1:%d", previewServerPort),
	}
	p.task = task
	return task.serverAddr, nil
}

func (p *PreviewClient) Close(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	err := p.killLspPreview(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (p *PreviewClient) Destroy(ctx context.Context) {
	p.Close(ctx)
}

func (p *PreviewClient) requestLspPreview(ctx context.Context, targetFile string, opts PreviewOptions) (string, int, error) {
	taskID := rand.Text()[:8]
	args := []any{
		"--task-id", taskID,
		"--data-plane-host", "127.0.0.1:0",
		"--preview-mode", opts.PreviewMode,
		"--root", opts.ProjectRoot,
		"--invert-colors", opts.InvertColor,
	}

	// if opts.PackagePath != "" {
	// 	args = append(args, "--package-path", opts.PackagePath)
	// }
	// if opts.PackageCachePath != "" {
	// 	args = append(args, "--package-cache-path", opts.PackageCachePath)
	// }

	// if len(opts.SysInputs) != 0 {
	// 	var inputsBuilder strings.Builder
	// 	for k, v := range opts.SysInputs {
	// 		inputsBuilder.WriteString(fmt.Sprintf("%s=%s", k, v))
	// 	}
	// 	args = append(args, "--input", inputsBuilder.String())
	// }

	if opts.PartialRender {
		args = append(args, "--partial-rendering")
	}
	if opts.OpenInBrowser {
		args = append(args, "--open", "--not-primary")
	} else {
		args = append(args, "--no-open")
	}

	args = append(args, targetFile)
	cmd := "tinymist.doStartPreview"
	if opts.OpenInBrowser {
		cmd = "tinymist.doStartBrowsingPreview"
	}

	result, err := p.client.ExecuteCommand(ctx, cmd, []any{args})
	if err != nil {
		log.Println("start previewer failed: ", err)
		return "", 0, err
	}

	// Try to open in built-in webview
	cmdResp, ok := result.(map[string]any)
	if !ok {
		panic("invalid cmd response type")
	}

	previewServerPort := cmdResp["staticServerPort"].(float64)

	return taskID, int(previewServerPort), nil
}

func (p *PreviewClient) killLspPreview(ctx context.Context) error {
	if p.task == nil || p.task.taskID == "" {
		return nil
	}

	_, err := p.client.ExecuteCommand(ctx, "tinymist.doKillPreview", []any{p.task.taskID})
	if err != nil {
		log.Printf("kill preview tasks %v failed: %v", p.task.taskID, err)
		return err
	}
	p.task = nil

	return nil
}

func (p *PreviewClient) scollLspPreview(ctx context.Context, taskID string, req map[string]any) error {
	_, err := p.client.ExecuteCommand(ctx, "tinymist.scrollPreview", []any{taskID, req})
	if err != nil {
		log.Println("scroll previewer failed: ", err)
		return err
	}

	return nil
}

func (p *PreviewClient) ScrollOnSelectionChange(ctx context.Context, pos gvcode.Position) {
	if p.task == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	req := map[string]any{
		"event":     "panelScrollTo",
		"filepath":  p.targetFile,
		"line":      pos.Line,
		"character": pos.Column,
	}
	p.scollLspPreview(ctx, p.task.taskID, req)

	// req2 := map[string]any{
	// 	"event":     "changeCursorPosition",
	// 	"filepath":  p.targetFile,
	// 	"line":      pos.Line,
	// 	"character": pos.Column,
	// }
	// p.scollLspPreview(ctx, req2)
}
