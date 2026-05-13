package extensions

import (
	"bytes"
	"encoding/json"

	"looz.ws/typstify/agent"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst"
	"looz.ws/typstify/typst/export"
)

type CompileParams struct {
	Pages       string            `json:"pages"`
	PPI         int               `json:"ppi"`
	PdfVersion  typst.PdfVersion  `json:"pdf_version"`
	PdfStandard typst.PdfStandard `json:"pdf_standard"`
	NoPdfTags   bool              `json:"no_pdf_tags"`
	Format      typst.OutFormat   `json:"format"`
	InputFile   string            `json:"input_file"`
}

// TypstCompilerExt is a ACP extension provider that compile the InputFile and return command output.
func TypstCompilerExt(projectDir string, settings *settings.TypstSettings) agent.ExtentionHandler {
	return func(params json.RawMessage) (any, error) {
		var compileParams CompileParams
		if err := json.Unmarshal(params, &compileParams); err != nil {
			return nil, err
		}

		compiler := export.NewCompileHelper(projectDir, settings)
		compiler.Pages = compileParams.Pages
		compiler.PPI = compileParams.PPI
		compiler.PdfVersion = compileParams.PdfVersion
		compiler.PdfStandard = compileParams.PdfStandard
		compiler.NoPdfTags = compileParams.NoPdfTags
		compiler.Format = compileParams.Format

		outbuf := &bytes.Buffer{}
		compiler.CmdOutput = outbuf

		p, err := compiler.BuildParams(compileParams.InputFile, "")
		if err != nil {
			return nil, err
		}

		compiler.Compile(p)

		return outbuf.String(), nil
	}
}
