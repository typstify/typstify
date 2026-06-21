package mcp

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"looz.ws/typstify/agent"
	"looz.ws/typstify/lsp"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst/export"
)

var _ agent.McpToolProvider = (*EditorMcpService)(nil)

type EditorMcpService struct {
	projectDir string
	settings   *settings.Settings
	previewSrv *lsp.PreviewService
	lspClient  *lsp.Client
	eventBus   *bus.EventBus

	mu sync.RWMutex
	// state data
	previewFile    string
	previewVisible bool
}

func NewEditorMcpService(projectDir string, settings *settings.Settings, lspClient *lsp.Client, previewSrv *lsp.PreviewService, eventBus *bus.EventBus) *EditorMcpService {
	return &EditorMcpService{
		projectDir: projectDir,
		settings:   settings,
		previewSrv: previewSrv,
		lspClient:  lspClient,
		eventBus:   eventBus,
	}
}

func (es *EditorMcpService) Compile(ctx context.Context, input CompileParams) (CompileResult, error) {
	compiler := export.NewCompileHelper(es.projectDir, es.settings.Typst())
	compiler.Pages = input.Pages
	compiler.PPI = input.PPI
	compiler.PdfVersion = input.PdfVersion
	compiler.PdfStandard = input.PdfStandard
	compiler.NoPdfTags = input.NoPdfTags
	compiler.Format = input.Format

	outbuf := &bytes.Buffer{}
	compiler.CmdOutput = outbuf

	p, err := compiler.BuildParams(input.InputFile, "")
	if err != nil {
		return CompileResult{}, err
	}

	if err := compiler.Compile(p); err != nil {
		return CompileResult{
			CompilerOut: outbuf.String(),
		}, nil
	}

	return CompileResult{
		CompilerOut: outbuf.String(),
		OutputFile:  filepath.Join(p.OutDir, p.OutFilename+"."+string(input.Format)),
	}, nil
}

func (es *EditorMcpService) Preview(ctx context.Context, input PreviewParams) error {
	es.eventBus.Emit(bus.TopicPreviewToggle, input)
	return nil
}

func (es *EditorMcpService) RegisterTools(s *agent.McpServer) error {
	agent.AddMcpTool(s, compilerTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input CompileParams) (*mcpsdk.CallToolResult, CompileResult, error) {
		r, err := es.Compile(ctx, input)
		return nil, r, err
	})

	agent.AddMcpTool(s, previewTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input PreviewParams) (*mcpsdk.CallToolResult, PreviewResult, error) {
		err := es.Preview(ctx, input)
		if err != nil {
			return nil, PreviewResult{}, err
		}

		es.previewFile = input.TargetFile

		if input.Action == "show" {
			return nil, PreviewResult{Log: "Previewer is shown"}, nil

		} else {
			return nil, PreviewResult{Log: "Previewer is closed"}, nil
		}

	})

	return nil
}
