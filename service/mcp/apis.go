package mcp

import (
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
