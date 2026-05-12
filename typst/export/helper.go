package export

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst"
)

type CompileHeler struct {
	projectDir string
	settings   *settings.TypstSettings

	Pages       string // pages range as string
	PPI         int
	PdfVersion  typst.PdfVersion
	PdfStandard typst.PdfStandard
	NoPdfTags   bool
	Format      typst.OutFormat
	CmdOutput   io.Writer
}

func NewCompileHelper(projectDir string, settings *settings.TypstSettings) *CompileHeler {
	return &CompileHeler{
		projectDir: projectDir,
		settings:   settings,
	}
}

func (ch *CompileHeler) BuildParams(targetFile string, outFilename string) (*typst.CompileParams, error) {
	if ch.Format == "" {
		ch.Format = typst.PDF
	}

	params := &typst.CompileParams{
		OutFilename: outFilename,
		Options: typst.CompileCmdOptions{
			Format: ch.Format,
			Pages:  ch.Pages,
			PPI:    ch.PPI,
		},
	}

	if params.Options.Format == typst.PDF {
		params.Options.PdfStandards = []typst.PdfSpec{ch.PdfVersion}
		if ch.PdfStandard != "" {
			if ch.PdfStandard.Compatible(ch.PdfVersion) {
				params.Options.PdfStandards = append(params.Options.PdfStandards, ch.PdfStandard)
			}
		}

		params.Options.NoPdfTags = ch.NoPdfTags
	}

	// Other options
	if ch.settings.UseSysInputs != 0 {
		inputs, err := typst.LoadInputs(ch.projectDir, false)
		if err != nil {
			return nil, err
		}
		if len(inputs) > 0 {
			params.Options.Input = inputs
		}
	}

	params.Options.PackagePath = ch.settings.PackageDir
	params.Options.PackageCachePath = ch.settings.PackageCacheDir
	params.Options.FontPaths = ch.fontPaths()
	params.Options.IgnoreSystemFonts = ch.settings.IgnoreSystemFonts == 1
	params.Options.IgnoreEmbeddedFonts = ch.settings.IgnoreEmbeddedFonts == 1

	params.InputFile = targetFile
	params.OutDir = filepath.Join(filepath.Dir(targetFile), "output")
	if ch.settings.OutputDir != "" {
		params.OutDir = ch.settings.OutputDir
	}

	if ch.settings.BuildDeps == 1 {
		params.Options.Deps = filepath.Join(params.OutDir, "deps.json")
		params.Options.DepsFormat = "json"
	}

	params.CmdOut = ch.CmdOutput
	params.Options.Features = "html" // enable HTML export

	if params.OutFilename == "" {
		params.OutFilename = strings.TrimSuffix(filepath.Base(targetFile), filepath.Ext(targetFile))
	} else {
		name := params.OutFilename
		params.OutFilename = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}

	return params, nil
}

func (ch *CompileHeler) Compile(params *typst.CompileParams) error {
	if params == nil || params.InputFile == "" {
		return errors.New("invalid compile params")
	}

	err := ch.onExportFile(params)
	if err != nil && ch.CmdOutput != nil {
		fmt.Fprintf(ch.CmdOutput, "export %s error: %v\n", strings.ToUpper(string(params.Options.Format)), err)
	}

	return err
}

func (ch *CompileHeler) fontPaths() []string {
	fontPaths := []string{ch.projectDir}
	if ch.settings.ExtraFontPath != "" {
		fontPaths = append(fontPaths, ch.settings.ExtraFontPath)
	}

	return fontPaths
}

func (ch *CompileHeler) onExportFile(params *typst.CompileParams) error {
	// use project dir as work dir for Typst to properly resolve imported resources.
	compiler, err := typst.NewCompiler(ch.projectDir)
	if err != nil {
		return err
	}
	defer compiler.Close()

	return compiler.Compile(context.Background(), params, nil)
}
