package typst

import (
	"fmt"
	"strings"
)

type OutFormat string

// possible output format
const (
	PDF  OutFormat = "pdf"
	PNG  OutFormat = "png"
	SVG  OutFormat = "svg"
	HTML OutFormat = "html"
)

// Command line compiling options for Typst. The fields always contains all the
// options of the latest version supported, and Build method exports fields supported
// by the current configured Typst version. Be noted that it uses Typst v0.11.0 as
// the base version.
type CompileCmdOptions struct {
	// Configures the project root (for absolute paths) [env: TYPST_ROOT=]
	RootDir string
	// Adds additional directories that are recursively searched for fonts
	// [env: TYPST_FONT_PATHS=]
	FontPaths []string
	// Add a string key-value pair visible through `sys.inputs`
	Input map[string]string
	// Ensures system fonts won't be searched, unless explicitly included via
	//      `--font-path`
	IgnoreSystemFonts bool
	// Ensures fonts embedded into Typst won't be considered [env: TYPST_IGNORE_EMBEDDED_FONTS=]
	IgnoreEmbeddedFonts bool
	// The document's creation date formatted as a UNIX timestamp [env: SOURCE_DATE_EPOCH=]
	CreationTimestamp uint64

	// The format to emit diagnostics in [default: human] [possible values: human, short]
	DiagnosticFormat string

	// Custom path to local packages, defaults to system-dependent location [env: TYPST_PACKAGE_PATH=]
	PackagePath string

	// Custom path to package cache, defaults to system-dependent location [env: TYPST_PACKAGE_CACHE_PATH=]
	PackageCachePath string

	// Number of parallel jobs spawned during compilation, defaults to number
	// of CPUs. Setting it to 1 disables parallelism
	Jobs int

	// Which pages to export. When unspecified, all document pages are exported
	Pages string

	// File path to which a list of current compilation's dependencies will
	// be written. Use `-` to write to stdout
	Deps string
	// File format to use for dependencies [default: json] [possible values: json, zero, make]
	DepsFormat string
	//The format of the output file, inferred from the extension by default
	// [possible values: pdf, png, svg, html]
	Format OutFormat
	// The PPI (pixels per inch) to use for PNG export [default: 144].
	PPI int

	// Produces performance timings of the compilation process (experimental)
	Timings string

	//One (or multiple comma-separated) PDF standards that Typst will
	//enforce conformance with [possible values: 1.4, 1.5, 1.6, 1.7, 2.0, a-1b, a-1a, a-2b, a-2u, a-2a, a-3b, a-3u, a-3a, a-4, a-4f, a-4e, ua-1]
	PdfStandards []PdfSpec
	// By default, even when not producing a `PDF/UA-1` document, a tagged
	// PDF document is written to provide a baseline of accessibility. In
	// some circumstances (for example when trying to reduce the size of a
	// document) it can be desirable to disable tagged PDF
	NoPdfTags bool
	// Enables in-development features that may be changed or removed at any
	// time [env: TYPST_FEATURES=] [possible values: html, a11y-extras]
	Features string
}

type InitCmdOptions struct {
	// Custom path to local packages, defaults to system-dependent location [env: TYPST_PACKAGE_PATH=]
	PackagePath string

	// Custom path to package cache, defaults to system-dependent location [env: TYPST_PACKAGE_CACHE_PATH=]
	PackageCachePath string
}

func (opt *CompileCmdOptions) Build() []string {
	opts := make([]string, 0)
	v := ParseVersion(CurrentVersion())

	setPair := func(key string, value any) {
		opts = append(opts, "--"+key)
		if val, ok := value.(string); ok {
			if val != "" {
				opts = append(opts, val)
			}
		} else {
			opts = append(opts, fmt.Sprintf("%v", value))
		}
	}

	setPair("root", opt.RootDir)

	if opt.Input != nil {
		for k, v := range opt.Input {
			opts = append(opts, fmt.Sprintf("--input=%s=%s", k, v))
		}
	}

	if len(opt.FontPaths) > 0 {
		for _, fontPath := range opt.FontPaths {
			if fontPath != "" {
				setPair("font-path", fontPath)
			}
		}
	}

	if opt.DiagnosticFormat != "" {
		setPair("diagnostic-format", opt.DiagnosticFormat)
	}

	if opt.Format != "" {
		setPair("format", opt.Format)
	}

	if opt.PPI > 0 {
		setPair("ppi", opt.PPI)
	}

	if opt.Timings != "" {
		setPair("timings", opt.Timings)
	}

	// --pages added in v0.12.0
	if v.AtLeast(0, 12, 0) {
		if opt.Pages != "" {
			setPair("pages", opt.Pages)
		}

		if opt.PackagePath != "" {
			setPair("package-path", opt.PackagePath)
		}

		if opt.PackageCachePath != "" {
			setPair("package-cache-path", opt.PackageCachePath)
		}

		if opt.IgnoreSystemFonts {
			setPair("ignore-system-fonts", "")
		}

		if len(opt.PdfStandards) > 0 {
			allStds := make([]string, 0)
			for _, std := range opt.PdfStandards {
				if std.Argument() != "" {
					allStds = append(allStds, std.Argument())
				}
			}
			if len(allStds) > 0 {
				setPair("pdf-standard", strings.Join(allStds, ","))
			}
		}

		if opt.Jobs > 0 {
			setPair("jobs", opt.Jobs)
		}

		if opt.CreationTimestamp > 0 {
			setPair("creation-timestamp", opt.CreationTimestamp)
		}
	}

	// --features added in v0.13.0
	if v.AtLeast(0, 13, 0) {
		if opt.Features != "" {
			setPair("features", opt.Features)
		}
	}

	// --deps and --deps-format added in v0.14.0
	if v.AtLeast(0, 14, 0) {
		if opt.NoPdfTags {
			setPair("no-pdf-tags", "")
		}
		if opt.Deps != "" {
			setPair("deps", opt.Deps)
		}

		if opt.DepsFormat != "" {
			setPair("deps-format", opt.DepsFormat)
		}

		if opt.IgnoreEmbeddedFonts {
			setPair("ignore-embedded-fonts", "")
		}
	}

	return opts
}

func (opt *InitCmdOptions) Build() []string {
	opts := make([]string, 0)
	v := ParseVersion(CurrentVersion())

	setPair := func(key string, val any) {
		opts = append(opts, "--"+key, fmt.Sprintf("%v", val))
	}

	// --package-path and --package-cache-path added in v0.12
	if v.AtLeast(0, 12, 0) {
		if opt.PackagePath != "" {
			setPair("package-path", opt.PackagePath)
		}

		if opt.PackageCachePath != "" {
			setPair("package-cache-path", opt.PackageCachePath)
		}
	}

	return opts
}

type FontCmdOptions struct {
	// Adds additional directories that are recursively searched for fonts
	// [env: TYPST_FONT_PATHS=]
	FontPaths []string

	// Ensures system fonts won't be searched, unless explicitly included via
	//      `--font-path`
	IgnoreSystemFonts bool
	// Ensures fonts embedded into Typst won't be considered [env: TYPST_IGNORE_EMBEDDED_FONTS=]
	IgnoreEmbeddedFonts bool

	// Lists style variants of each font family too.
	Variants bool
}

func (opt *FontCmdOptions) Build() []string {
	opts := make([]string, 0)
	v := ParseVersion(CurrentVersion())

	setPair := func(key string, value any) {
		opts = append(opts, "--"+key)
		if val, ok := value.(string); ok {
			if val != "" {
				opts = append(opts, val)
			}
		} else {
			opts = append(opts, fmt.Sprintf("%v", value))
		}
	}

	if len(opt.FontPaths) > 0 {
		for _, fontPath := range opt.FontPaths {
			if fontPath != "" {
				setPair("font-path", fontPath)
			}
		}
	}

	if opt.Variants {
		setPair("variants", "")
	}

	if v.AtLeast(0, 12, 0) {
		if opt.IgnoreSystemFonts {
			setPair("ignore-system-fonts", "")
		}
	}

	if v.AtLeast(0, 14, 0) {
		if opt.IgnoreEmbeddedFonts {
			setPair("ignore-embedded-fonts", "")
		}
	}

	return opts
}
