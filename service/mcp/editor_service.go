package mcp

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"looz.ws/typstify/agent"
	"looz.ws/typstify/lsp"
	"looz.ws/typstify/lsp/protocol"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst"
	"looz.ws/typstify/typst/export"
)

var _ agent.McpToolProvider = (*EditorMcpService)(nil)

// ActiveDocProvider is implemented by editor views to report their current state.
type ActiveDocProvider interface {
	GetActiveDocument() ActiveDocument
}

// ActiveDocQuerier queries the active editor view for its current document state.
type ActiveDocQuerier func() *ActiveDocument

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

	activeDocQuerier ActiveDocQuerier
}

func NewEditorMcpService(projectDir string, settings *settings.Settings, lspClient *lsp.Client, previewSrv *lsp.PreviewService, eventBus *bus.EventBus, querier ActiveDocQuerier) *EditorMcpService {
	return &EditorMcpService{
		projectDir:       projectDir,
		settings:         settings,
		previewSrv:       previewSrv,
		lspClient:        lspClient,
		eventBus:         eventBus,
		activeDocQuerier: querier,
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

func (es *EditorMcpService) fontPaths() []string {
	fontPaths := []string{es.projectDir}
	if es.settings.Typst().ExtraFontPath != "" {
		fontPaths = append(fontPaths, es.settings.Typst().ExtraFontPath)
	}

	return fontPaths
}

func (es *EditorMcpService) updateFileLSPState(filePath string) error {
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}

	fileContent, err := os.ReadFile(absFile)
	if err != nil {
		log.Println("read file failed:", err)
		return err
	}

	if es.lspClient != nil && es.lspClient.IsReady() {
		es.lspClient.OnEditorUpdated(absFile, bytes.NewReader(fileContent))
	}
	return nil
}

func (es *EditorMcpService) DiscoverFonts(ctx context.Context, showVariants bool) ([]typst.FontFamily, error) {
	return typst.FontsCmd(ctx, &typst.FontCmdOptions{
		FontPaths:           es.fontPaths(),
		IgnoreSystemFonts:   es.settings.Typst().IgnoreSystemFonts == 1,
		IgnoreEmbeddedFonts: es.settings.Typst().IgnoreEmbeddedFonts == 1,
		Variants:            showVariants,
	})

}

func (es *EditorMcpService) GetDocumentOutline(ctx context.Context, input DocumentOutlineParams) (DocumentOutline, error) {
	if !filepath.IsAbs(input.InputFile) {
		return DocumentOutline{}, errors.New("input file is not an absolute path")
	}

	if es.lspClient != nil && es.lspClient.IsReady() {
		es.updateFileLSPState(input.InputFile)
		symbols, err := es.lspClient.DocumentSymbols(context.Background(), input.InputFile)
		if err != nil {
			return DocumentOutline{}, err
		}

		return DocumentOutline{Symbols: symbols}, nil
	}

	return DocumentOutline{}, errors.New("LSP not ready")
}

func (es *EditorMcpService) QueryDiagnostics(ctx context.Context, file string) ([]protocol.Diagnostic, error) {
	if es.lspClient == nil || !es.lspClient.IsReady() {
		return nil, errors.New("LSP not ready")
	}

	es.updateFileLSPState(file)
	diagnostics := es.lspClient.Diagnostics(file)
	if diagnostics == nil {
		return nil, nil
	}

	return diagnostics.Diagnostics, nil

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

	agent.AddMcpTool(s, outlineTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input DocumentOutlineParams) (*mcpsdk.CallToolResult, any, error) {
		result, err := es.GetDocumentOutline(ctx, input)
		// Use any as output type to avoid the MCP SDK's JSON schema generator
		// hitting a cycle on protocol.DocumentSymbol.Children.
		return nil, result, err
	})

	agent.AddMcpTool(s, discoverFontsTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input DiscoverFontsParams) (*mcpsdk.CallToolResult, DiscoverFontsResult, error) {
		result, err := es.DiscoverFonts(ctx, input.ListVariants)
		if err != nil {
			return nil, DiscoverFontsResult{}, err
		}

		if input.FontName == "" && input.Style == "" && input.Weight == 0 {
			return nil, DiscoverFontsResult{Families: result}, nil
		}

		filtered := make([]typst.FontFamily, 0)

		for _, ft := range result {
			if input.FontName != "" && input.FontName == ft.Name {
				filtered = append(filtered, ft)
				continue
			}

			// keep the whole font family, not just the matched variants
			idx := slices.IndexFunc(ft.Variants, func(variant typst.FontVariant) bool {
				return variant.Style == input.Style || variant.Weight == input.Weight
			})
			if idx >= 0 {
				filtered = append(filtered, ft)
			}
		}

		return nil, DiscoverFontsResult{Families: filtered}, err
	})

	agent.AddMcpTool(s, queryDiagnosticTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input QueryDiagnosticParams) (*mcpsdk.CallToolResult, QueryDiagnosticResult, error) {
		result, err := es.QueryDiagnostics(ctx, input.InputFile)
		if err != nil {
			return nil, QueryDiagnosticResult{}, err
		}

		return nil, QueryDiagnosticResult{Diagnostics: result}, nil
	})

	agent.AddMcpTool(s, getActiveDocumentTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ struct{}) (*mcpsdk.CallToolResult, ActiveDocument, error) {
		if es.activeDocQuerier == nil {
			return nil, ActiveDocument{}, errors.New("no view manager")
		}
		doc := es.activeDocQuerier()
		if doc == nil {
			return nil, ActiveDocument{}, errors.New("no active editor view")
		}
		return nil, *doc, nil
	})

	return nil
}
