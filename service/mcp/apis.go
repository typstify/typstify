package mcp

import (
	"looz.ws/typstify/lsp/protocol"
	"looz.ws/typstify/typst"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Compiler API
type CompileParams struct {
	Pages       string            `json:"pages,omitempty" jsonschema:"Which pages to export.Valid value can be comma seperated page numbers and page ranges, for example, 1,3,5,6-9. When unspecified, all pages are exported"`
	PPI         int               `json:"ppi,omitempty" jsonschema:"The PPI (pixels per inch) to use for PNG export."`
	PdfVersion  typst.PdfVersion  `json:"pdf_version,omitempty" jsonschema:"PDF version that the compiler will enforce conformance with."`
	PdfStandard typst.PdfStandard `json:"pdf_standard,omitempty" jsonschema:"PDF standards that Typstify will enforce conformance with."`
	NoPdfTags   bool              `json:"no_pdf_tags,omitempty" jsonschema:"Disable PDF tags"`
	Format      typst.OutFormat   `json:"format" jsonschema:"Compiler output format, must be one of pdf, png, svg, html"`
	InputFile   string            `json:"input_file" jsonschema:"The source typst file"`
}

type CompileResult struct {
	CompilerOut string `json:"compilerOut" jsonschema:"Output of compiler"`
	OutputFile  string `json:"outputFile" jsonschema:"Path of output file. When exporting PNG or SVG, multiple files may be generated. If there are multiple output files, this is the prefix of the output files."`
}

// Preview API
type PreviewParams struct {
	TargetFile string `json:"targetFile" jsonschema:"The target typst file to be rendered by the previewer. Must be absolute path"`
	Action     string `json:"action" jsonschema:"show or close the previewer, values: show, close"`
}

type PreviewResult struct {
	Log string `json:"log" jsonschema:"log of preview"`
}

type DocumentOutlineParams struct {
	InputFile string `json:"input_file" jsonschema:"The source typst file. Must be absolute path"`
}

type DocumentOutline struct {
	Symbols []protocol.DocumentSymbol `json:"symbols" jsonschema:"Outline symbols, use LSP document symbol format."`
}

type QueryDiagnosticParams struct {
	InputFile string `json:"input_file" jsonschema:"The source typst file. Must be absolute path"`
}

type QueryDiagnosticResult struct {
	Diagnostics []protocol.Diagnostic `json:"diagnostics" jsonschema:"LSP diagnostics result for the input file."`
}

type DiscoverFontsParams struct {
	FontName     string `json:"familyName,omitempty" jsonschema:"Font family name to filter the result"`
	Style        string `json:"style,omitempty" jsonschema:"font variant style to filter the result"`
	Weight       int    `json:"weight,omitempty" jsonschema:"font variant weight to filter the result"`
	ListVariants bool   `json:"listVariants" jsonschema:"List font variants of font family or not"`
}

type DiscoverFontsResult struct {
	Families []typst.FontFamily `json:"families" jsonschema:"All font families can be used in typst document"`
}

// Active document state. All positions are rune offsets.
type ActiveDocument struct {
	File      string `json:"file" jsonschema:"The file path of the document being edited"`
	CursorPos int    `json:"cursorPos" jsonschema:"The cursor position in the document, as a rune offset"`
	Selection struct {
		Start   int    `json:"start" jsonschema:"start position of the selection, as a rune offset"`
		End     int    `json:"end" jsonschema:"end position of the selection, as a rune offset"`
		Content string `json:"content" jsonschema:"content of the selection"`
	} `json:"selection" jsonschema:"The selection in the document"`
}

var compilerTool = &mcpsdk.Tool{
	Name:        "typstCompiler",
	Description: "Compile typst file in the editor environment.",
	Title:       "Typst Compiler",
}

var previewTool = &mcpsdk.Tool{
	Name:        "typst-previewer",
	Description: "Instant preview typst file in the editor environment.",
	Title:       "Typst Previewer",
}

var outlineTool = &mcpsdk.Tool{
	Name:        "getDocumentOutline",
	Description: "Query LSP document symbols of a typst document",
	Title:       "Get Document Outline",
}

var queryDiagnosticTool = &mcpsdk.Tool{
	Name:        "queryDiagnostics",
	Description: "Query LSP diagnostics for a typst document.",
	Title:       "Query Diagnostics",
}

var discoverFontsTool = &mcpsdk.Tool{
	Name:        "discoverFonts",
	Description: "Discover usable font families for a typst document. Results can be filtered by various fields.",
	Title:       "Discover Fonts",
}

var getActiveDocumentTool = &mcpsdk.Tool{
	Name:        "getActiveDocument",
	Description: "Get the active document state in the editor, including file, cursor position, and selection.",
	Title:       "Get Active Document",
}

var activeDocumentResource = &mcpsdk.Resource{
	Name:        "active-document",
	Description: "The active document being edited",
	Title:       "Active-Document",
	URI:         "typstify://active-document",
}
