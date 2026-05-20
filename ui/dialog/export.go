package dialog

import (
	"fmt"
	"io"
	"strings"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/outlay"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	gw "github.com/oligo/gioview/widget"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/typst"
	"looz.ws/typstify/typst/export"
	"looz.ws/typstify/ui/settings/form"
	"looz.ws/typstify/ui/statusbar"
)

var ExportDialogViewID = view.NewViewID("ExportDialogView")

var (
	formatTip = "The format of the output file."
)

var ppiRange = []float32{72, 300}

type ExportDialog struct {
	srv             *service.ServiceFacade
	formatEnum      widget.Enum
	pagesInput      gw.TextField
	ppiInput        *form.FloatBinder
	pdfVersion      widget.Enum
	pdfStandard     widget.Enum
	pdfVersionFlow  *outlay.Flow
	pdfStandardFlow *outlay.Flow
	noPdfTags       widget.Bool
	nameInput       gw.TextField

	formatChoices  []layout.FlexChild
	targetFile     string
	compilerOutput io.Writer
}

func NewExportDialog(srv *service.ServiceFacade) view.View {
	d := &ExportDialog{
		srv:            srv,
		compilerOutput: srv.Console(),
	}

	dialog := NewDialogModal(ExportDialogViewID, i18n.Translate("Build And Export"), i18n.Translate("Export"))
	dialog.Dialog = d

	return dialog
}

func (d *ExportDialog) OnInit(intent view.Intent) error {
	targetFile := intent.Params["targetFile"]
	if targetFile == nil {
		panic("no targetFile provided!")
	}

	d.targetFile = targetFile.(string)
	d.formatEnum.Value = string(typst.PDF)
	d.pdfVersion.Value = string(typst.PDF1_7)

	d.pdfVersionFlow = &outlay.Flow{
		Num:       5,
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}

	d.pdfStandardFlow = &outlay.Flow{
		Num:       4,
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}
	return nil
}

func (d *ExportDialog) OnConfirm() error {
	compiler := export.NewCompileHelper(d.srv.CurrentProjectDir(), d.srv.Settings().Typst())

	compiler.Pages = d.pagesInput.Text()
	compiler.PPI = int(d.ppiInput.Value())
	compiler.Format = typst.OutFormat(d.formatEnum.Value)
	compiler.PdfVersion = typst.PdfVersion(d.pdfVersion.Value)
	compiler.PdfStandard = typst.PdfStandard(d.pdfStandard.Value)
	compiler.NoPdfTags = d.noPdfTags.Value
	compiler.CmdOutput = d.compilerOutput

	params, err := compiler.BuildParams(d.targetFile, d.nameInput.Text())
	if err != nil {
		d.compilerOutput.Write([]byte(err.Error()))
		return nil
	}

	go func() {
		d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Exporting file...")})
		err := compiler.Compile(params)
		if err != nil {
			d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: "File export error: " + err.Error()})
			return
		}

		msg := fmt.Sprintf("Files exported to %s", params.OutDir)
		d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: msg})

	}()

	return nil
}

func (d *ExportDialog) LayoutBody(gtx C, th *theme.Theme) D {
	if d.ppiInput == nil {
		d.ppiInput = form.NewFloatBinder(float32(144), ppiRange)
	}

	if d.formatChoices == nil {
		for _, name := range []typst.OutFormat{typst.PDF, typst.PNG, typst.SVG, typst.HTML} {
			name := name
			d.formatChoices = append(d.formatChoices, layout.Rigid(func(gtx C) D {
				return material.RadioButton(th.Theme, &d.formatEnum, string(name), strings.ToUpper(string(name))).Layout(gtx)
			}))
		}
	}

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Exported File Format"), i18n.Translate(formatTip),
				func(gtx C) D {
					return layout.Flex{
						Axis: layout.Horizontal,
					}.Layout(gtx, d.formatChoices...)
				})
		}),

		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Pages"),
				i18n.Translate("Which pages to export. Valid value can be comma seperated page numbers and page ranges, for example, 1,3,5,6-9. When unspecified, all document pages are exported."),
				func(gtx C) D {
					d.pagesInput.Alignment = text.Start
					return d.pagesInput.Layout(gtx, th, "")
				})
		}),

		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("File Name"),
				i18n.Translate("This specifies the output file name. If multiple files is generated, the file name will be suffixed with a number. When not specified, the name of the source file is used."),
				func(gtx C) D {
					d.nameInput.Alignment = text.Start
					return d.nameInput.Layout(gtx, th, "")
				})
		}),

		layout.Rigid(func(gtx C) D {
			if d.formatEnum.Value != string(typst.PNG) {
				return D{}
			}

			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("PPI"),
				i18n.Translate("The PPI (pixels per inch) to use for PNG export."),
				func(gtx C) D {
					return layout.Flex{
						Axis:      layout.Horizontal,
						Alignment: layout.Middle,
					}.Layout(gtx,
						layout.Flexed(1, material.Slider(th.Theme, d.ppiInput.GetWidget(gtx)).Layout),
						layout.Rigid(material.Label(th.Theme, th.TextSize, fmt.Sprintf("%.0f", d.ppiInput.Value())).Layout),
					)
				})
		}),

		layout.Rigid(func(gtx C) D {
			if d.formatEnum.Value != string(typst.PDF) {
				return D{}
			}

			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("PDF Versions"),
				i18n.Translate("PDF version that Typstify will enforce conformance with."),
				func(gtx C) D {
					return d.pdfVersionFlow.Layout(gtx, len(typst.PdfVersions), func(gtx C, i int) D {
						radio := material.RadioButton(th.Theme, &d.pdfVersion, string(typst.PdfVersions[i]), string(typst.PdfVersions[i]))
						return radio.Layout(gtx)
					})
				})
		}),

		layout.Rigid(func(gtx C) D {
			if d.formatEnum.Value != string(typst.PDF) {
				return D{}
			}

			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("PDF Standards"),
				i18n.Translate("PDF standards that Typstify will enforce conformance with."),
				func(gtx C) D {
					return layout.Flex{
						Axis:      layout.Horizontal,
						Alignment: layout.Start,
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return d.pdfStandardFlow.Layout(gtx, len(typst.PdfStandards), func(gtx C, i int) D {
								return layout.Inset{Right: unit.Dp(10)}.Layout(gtx,
									func(gtx C) D {
										selectedVersion := typst.PdfVersion(d.pdfVersion.Value)
										if !typst.PdfStandards[i].Compatible(selectedVersion) {
											gtx = gtx.Disabled()
										}
										radio := material.RadioButton(th.Theme, &d.pdfStandard, string(typst.PdfStandards[i]), string(typst.PdfStandards[i]))
										return radio.Layout(gtx)
									})
							})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(30)}.Layout),
						layout.Rigid(func(gtx C) D {
							selected := typst.PdfStandard(d.pdfStandard.Value)
							if selected == "" {
								return D{}
							}
							features := selected.Features()
							featureLabels := make([]layout.FlexChild, 0)
							for _, feat := range features {
								featureLabels = append(featureLabels,
									layout.Rigid(func(gtx C) D {
										return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
											label := material.Label(th.Theme, th.TextSize-1, "\u2713 "+feat)
											// label.Color = misc.WithAlpha(th.Fg, 0xb6)
											return label.Layout(gtx)
										})
									}))
							}

							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, featureLabels...)
						}),
					)
				})
		}),

		layout.Rigid(func(gtx C) D {
			if d.formatEnum.Value != string(typst.PDF) {
				return D{}
			}

			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Disable PDF Tags"),
				i18n.Translate("By default a tagged PDF is generated to provide base accessibility. In some cases this may be not desired. You can check this to disable it."),
				func(gtx C) D {
					checkbox := material.CheckBox(th.Theme, &d.noPdfTags, i18n.Translate("No PDF Tags"))
					return checkbox.Layout(gtx)
				})
		}),
	)
}
