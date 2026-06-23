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

// Package API
type DownloadPackageParams struct {
	PkgSpec string `json:"pkgSpec" jsonschema:"Package specifier, e.g. @namespace/name:version. If version is omitted, the latest version is downloaded."`
}

type DownloadPackageResult struct {
	Path string `json:"path" jsonschema:"Local path of the downloaded package"`
}

type QueryPackageDetailParams struct {
	PkgSpec string `json:"pkgSpec" jsonschema:"Package specifier, e.g. @namespace/name:version"`
}

type SearchPackagesParams struct {
	Queries []string `json:"queries" jsonschema:"Search query strings. Multiple queries are run in parallel and results are deduplicated."`
	Limit   int      `json:"limit,omitempty" jsonschema:"Maximum number of results to return"`
}

type PublishPackageParams struct {
	PkgDir    string `json:"pkgDir" jsonschema:"Directory of the package to publish"`
	Namespace string `json:"namespace" jsonschema:"Target namespace to publish to"`
}

type PublishPackageResult struct {
	Success bool   `json:"success" jsonschema:"Whether the publish succeeded"`
	Log     string `json:"log" jsonschema:"Result message"`
}

type PackageInfo struct {
	Name          string   `json:"name" jsonschema:"Package name"`
	Namespace     string   `json:"namespace" jsonschema:"Package namespace"`
	Description   string   `json:"description" jsonschema:"Package description"`
	LatestVersion string   `json:"latestVersion" jsonschema:"Latest available version"`
	License       string   `json:"license,omitempty" jsonschema:"Package license"`
	IsTemplate    bool     `json:"isTemplate" jsonschema:"Whether this is a template"`
	IsCached      bool     `json:"isCached" jsonschema:"Whether the package is cached locally"`
	ImportPath    string   `json:"importPath" jsonschema:"Import path for use in Typst, e.g. @namespace/name:version"`
	Authors       []string `json:"authors,omitempty" jsonschema:"Package authors"`
	Categories    []string `json:"categories,omitempty" jsonschema:"Package categories"`
	Versions      []string `json:"versions,omitempty" jsonschema:"Available versions"`
}

type ListLocalPackagesResult struct {
	Packages []PackageInfo `json:"packages" jsonschema:"List of cached Typst packages"`
}

type SearchPackagesResult struct {
	Results []PackageInfo `json:"results" jsonschema:"Search results"`
}

var listLocalPackagesTool = &mcpsdk.Tool{
	Name:        "listLocalPackages",
	Description: "List locally cached Typst packages.",
	Title:       "List Local Packages",
}

var downloadPackageTool = &mcpsdk.Tool{
	Name:        "downloadPackage",
	Description: "Download a Typst package from the registry. Use pkgSpec like @namespace/name:version.",
	Title:       "Download Package",
}

var queryPackageDetailTool = &mcpsdk.Tool{
	Name:        "queryPackageDetail",
	Description: "Query details of a Typst package from the registry.",
	Title:       "Query Package Detail",
}

var searchPackagesTool = &mcpsdk.Tool{
	Name:        "searchPackages",
	Description: "Search Typst packages in the registry.",
	Title:       "Search Packages",
}

var publishPackageTool = &mcpsdk.Tool{
	Name:        "publishPackage",
	Description: "Publish a Typst package to a namespace.",
	Title:       "Publish Package",
}

var getUserInfoTool = &mcpsdk.Tool{
	Name:        "getUserInfo",
	Description: "Get the current logged-in user's TPIX profile.",
	Title:       "Get User Info",
}

type ReadPackageMetadataParams struct {
	Offset int `json:"offset" jsonschema:"0-based starting line offset to read from"`
	Limit  int `json:"limit,omitempty" jsonschema:"Maximum number of lines to read. 0 means read to the end."`
}

type ReadPackageMetadataResult struct {
	Text       string `json:"text" jsonschema:"The lines of index text"`
	Offset     int    `json:"offset" jsonschema:"The offset that was read from"`
	TotalLines int    `json:"totalLines" jsonschema:"Total number of lines in the full index"`
}

var readPackageIndexTool = &mcpsdk.Tool{
	Name:        "readPackageIndex",
	Description: "Read accessible Typst package index by line range. The result is very large, so paginate with offset and limit.",
	Title:       "Read Package Index",
}

var userProfileResource = &mcpsdk.Resource{
	Name:        "user-profile",
	Description: "The current logged-in user's TPIX profile",
	Title:       "User Profile",
	URI:         "typstify://user-profile",
}

var activeDocumentResource = &mcpsdk.Resource{
	Name:        "active-document",
	Description: "The active document being edited",
	Title:       "Active-Document",
	URI:         "typstify://active-document",
}
