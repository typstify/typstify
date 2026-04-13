package dialog

import (
	"errors"
	"image/color"
	"sync/atomic"
	"time"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/widgets"
)

type PublishPkgDialog struct {
	srv        *service.ServiceFacade
	projectDir string
	bundlePath string
	fetchErr   error

	namespaceChoice *widgets.Dropdown
	isLoading       atomic.Bool

	bundleBtn widget.Clickable
	bundleErr error
}

var PublishPkgDialogViewID = view.NewViewID("PublishPkgDialogView")

func NewPublishPkgDialog(srv *service.ServiceFacade) view.View {
	createDialog := &PublishPkgDialog{srv: srv}

	dialog := NewDialogModal(CreateProjectDialogViewID, i18n.Translate("Publish Package"), i18n.Translate("Submit"))
	dialog.Dialog = createDialog
	return dialog
}

func (d *PublishPkgDialog) OnInit(intent view.Intent) error {
	val, ok := intent.Params["projectDir"]
	if !ok {
		return nil
	}

	d.projectDir = val.(string)
	go func() {
		d.isLoading.Store(true)
		defer d.isLoading.Store(false)
		namespaces, err := d.srv.PkgService().AccessibleNamesapces()
		if err != nil {
			d.fetchErr = err
			return
		}

		nsOpts := make(map[string]any)

		for _, ns := range namespaces {
			if ns.Permission == "READ" {
				continue
			}
			nsOpts[ns.Name] = ns.Name
		}
		d.namespaceChoice = widgets.NewDropDown(nsOpts)
	}()

	return nil
}

func (d *PublishPkgDialog) OnConfirm() error {
	if d.bundleErr != nil {
		return d.bundleErr
	}

	if d.fetchErr != nil {
		return d.fetchErr
	}

	if d.namespaceChoice == nil || d.bundlePath == "" {
		return nil
	}

	selectedNamespace := d.namespaceChoice.Value()
	if selectedNamespace == "" {
		return errors.New("No namespace is selected")
	}

	// TODO: should call this asynchronously.
	err := d.srv.PkgService().Push(d.bundlePath, selectedNamespace)
	if err != nil {
		d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{
			Content:  i18n.Translate("publish package error: %s", err.Error()),
			Level:    2,
			Duration: time.Second * 8,
		})
	} else {
		d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{
			Content: i18n.Translate("publish package succeeded: %s", d.bundlePath),
		})
	}

	return err
}

func (d *PublishPkgDialog) LayoutBody(gtx C, th *theme.Theme) D {
	if d.bundleBtn.Clicked(gtx) {
		d.bundlePath, d.bundleErr = d.srv.PkgService().Bundle(d.projectDir, d.projectDir)
	}

	if d.namespaceChoice != nil && d.namespaceChoice.Update(gtx) {
	}

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Bundle"), i18n.Translate("Bundle the project files to a valid Typst package/template."),
				func(gtx C) D {
					return layout.Flex{
						Axis:      layout.Horizontal,
						Alignment: layout.Middle,
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return material.Button(th.Theme, &d.bundleBtn, i18n.Translate("Build")).Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx C) D {
							if d.bundleErr != nil {
								return d.layoutErr(gtx, th, d.bundleErr)
							}

							return material.Label(th.Theme, th.TextSize, i18n.Translate("Created Bundle: %s", d.bundlePath)).Layout(gtx)
						}),
					)
				})
		}),

		layout.Rigid(func(gtx C) D {
			if d.bundlePath == "" || d.bundleErr != nil {
				return D{}
			}

			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Namespace"),
				i18n.Translate("Select the namespace to publish to. Make sure you have logged in TPIX in the app and have accessible namespaces of your TPIX account. "),
				func(gtx C) D {
					if d.fetchErr != nil {
						return d.layoutErr(gtx, th, d.fetchErr)
					}

					if d.namespaceChoice == nil && d.isLoading.Load() {
						label := material.Label(th.Theme, th.TextSize, i18n.Translate("Loading namespaces..."))
						label.Color = misc.WithAlpha(th.Fg, 0xb6)
						return label.Layout(gtx)
					}

					if d.namespaceChoice != nil {
						return d.namespaceChoice.Layout(gtx, th)
					}

					lb := material.Label(th.Theme, th.TextSize, i18n.Translate("No writable namespaces"))
					lb.Color = misc.WithAlpha(th.Fg, 0xb6)
					return lb.Layout(gtx)
				})
		}),
	)

}

func (d *PublishPkgDialog) layoutErr(gtx C, th *theme.Theme, err error) D {
	label := material.Label(th.Theme, th.TextSize, err.Error())
	label.Color = color.NRGBA{R: 255, A: 255}
	label.Alignment = text.Start
	return label.Layout(gtx)
}
