package dialog

import (
	"errors"
	"fmt"
	"image/color"
	"sync"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	gw "github.com/oligo/gioview/widget"
	cli "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/widgets"
)

type SyncBibDialog struct {
	srv       *service.ServiceFacade
	parentDir string

	nameInput gw.TextField
	isLoading atomic.Bool

	mu                     sync.Mutex
	libraries              []cli.ZoteroLibrary
	zoteroCollectionChoice *widgets.Dropdown
	loadErr                error
}

var (
	SyncBibDialogViewID     = view.NewViewID("SyncBibDialogView")
	ViewBibInfoDialogViewID = view.NewViewID("ViewBibInfoDialogView")
)

func NewSyncBibDialog(srv *service.ServiceFacade) view.View {
	bibDialog := &SyncBibDialog{srv: srv}

	dialog := NewDialogModal(SyncBibDialogViewID, i18n.Translate("Sync Bibliography"), i18n.Translate("Submit"))
	dialog.Dialog = bibDialog
	return dialog
}

func (d *SyncBibDialog) OnInit(intent view.Intent) error {
	parentDir, ok := intent.Params["parentDir"].(string)
	if !ok {
		return errors.New("missing parent directory")
	}

	d.parentDir = parentDir
	go func() {
		d.isLoading.Store(true)
		defer d.isLoading.Store(false)
		libraries, err := d.srv.Bibliography().ListZoteroLibraries()
		if err != nil {
			d.mu.Lock()
			d.loadErr = err
			d.mu.Unlock()
			return
		}

		opts := make(map[string]any)

		for _, lib := range libraries {
			for _, collection := range lib.Collections {
				label := fmt.Sprintf("%s | %s", lib.Library.Name, collection.Name)
				if lib.Namespace != "" {
					label = fmt.Sprintf("@%s | %s | %s", lib.Namespace, lib.Library.Name, collection.Name)
				}
				opts[collection.Key] = label
			}

		}

		d.mu.Lock()
		d.libraries = libraries
		d.zoteroCollectionChoice = widgets.NewDropDown(opts)
		d.mu.Unlock()
	}()

	return nil
}

// loaded returns the state populated by the background loader.
func (d *SyncBibDialog) loaded() ([]cli.ZoteroLibrary, *widgets.Dropdown, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.libraries, d.zoteroCollectionChoice, d.loadErr
}

func (d *SyncBibDialog) OnConfirm() error {
	libraries, choice, loadErr := d.loaded()

	if loadErr != nil {
		return loadErr
	}

	if choice == nil {
		return nil
	}

	selectedCollectionKey := choice.Value()
	if selectedCollectionKey == "" {
		return errors.New("No collection is selected")
	}

	var library api.ZoteroLibrary
findLibrary:
	for _, lib := range libraries {
		for _, collection := range lib.Collections {
			if collection.Key == selectedCollectionKey {
				library = lib
				break findLibrary
			}
		}
	}

	filename := d.nameInput.Text()

	// TODO: should call this asynchronously.
	err := d.srv.Bibliography().CreateZoteroExport(d.parentDir, filename, library, selectedCollectionKey)
	if err != nil {
		d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{
			Content: i18n.Translate("Creating managed bibliography error: %s", err.Error()),
			Level:   2,
		})
		return err
	}

	d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{
		Content: i18n.Translate("Creating managed bibliography succeeded: %s", filename),
	})

	return err
}

func (d *SyncBibDialog) LayoutBody(gtx C, th *theme.Theme) D {

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,

		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Bibliography File Name"),
				i18n.Translate("The name of the managed bibliography file, such as 'bib-file-name.bib'. You can leave it empty to use a auto generated one."),
				func(gtx C) D {
					d.nameInput.Alignment = text.Start
					d.nameInput.SingleLine = true
					return d.nameInput.Layout(gtx, th, "")
				})
		}),

		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Collections"),
				i18n.Translate("Select the collection to sync with. Make sure you or your team have added Zotero API key on TPIX server. "),
				func(gtx C) D {
					_, choice, loadErr := d.loaded()

					if loadErr != nil {
						return d.layoutErr(gtx, th, loadErr)
					}

					if choice == nil && d.isLoading.Load() {
						label := material.Label(th.Theme, th.TextSize, i18n.Translate("Loading collecions..."))
						label.Color = misc.WithAlpha(th.Fg, 0xb6)
						return label.Layout(gtx)
					}

					if choice != nil {
						return choice.Layout(gtx, th)
					}

					lb := material.Label(th.Theme, th.TextSize, i18n.Translate("No collections found"))
					lb.Color = misc.WithAlpha(th.Fg, 0xb6)
					return lb.Layout(gtx)
				})
		}),
	)

}

func (d *SyncBibDialog) layoutErr(gtx C, th *theme.Theme, err error) D {
	label := material.Label(th.Theme, th.TextSize, err.Error())
	label.Color = color.NRGBA{R: 255, A: 255}
	label.Alignment = text.Start
	return label.Layout(gtx)
}

type BibInfoDialog struct {
	srv       *service.ServiceFacade
	meta      service.ManagedBibliography
	pathInput gw.TextField

	err error
}

func NewBibInfoDialog(srv *service.ServiceFacade) view.View {
	bibDialog := &BibInfoDialog{srv: srv}

	dialog := NewDialogModal(ViewBibInfoDialogViewID, i18n.Translate("View Managed Bibliography"), i18n.Translate("Unlink"))
	dialog.Dialog = bibDialog
	return dialog
}

func (d *BibInfoDialog) OnInit(intent view.Intent) error {
	meta, ok := intent.Params["meta"].(service.ManagedBibliography)
	if !ok {
		return errors.New("missing bibliography info")
	}

	d.meta = meta
	d.pathInput.SetText(d.meta.File)

	return nil
}

func (d *BibInfoDialog) OnConfirm() error {
	go d.unlinkRemote()
	return nil
}

func (d *BibInfoDialog) unlinkRemote() error {
	err := d.srv.Bibliography().RemoveBibliography(d.meta)
	if err != nil {
		d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{
			Content: i18n.Translate("Unlink managed bibliography error: %s", err.Error()),
			Level:   2,
		})
		return err
	}

	d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{
		Content: i18n.Translate("Unlink managed bibliography succeeded: %s", d.meta.File),
	})

	return err
}

func (d *BibInfoDialog) LayoutBody(gtx C, th *theme.Theme) D {

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,

		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Bibliography File Name"),
				i18n.Translate("The path of the managed bibliography file. Clicking unlink below will turn it to a regular file, and it will not sync with remote Zotero collection anymore."),
				func(gtx C) D {
					d.pathInput.Alignment = text.Start
					d.pathInput.SingleLine = true
					d.pathInput.State().ReadOnly = true
					return d.pathInput.Layout(gtx, th, "")
				})
		}),

		layout.Rigid(func(gtx C) D {
			return formItem{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
			}.Layout(gtx, th, i18n.Translate("Format:"),
				"",
				func(gtx C) D {
					return material.Label(th.Theme, th.TextSize, d.meta.Meta.Format).Layout(gtx)
				})
		}),

		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Zotero Collection Info"),
				"",
				func(gtx C) D {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							if d.meta.Meta.Namespace == "" {
								return D{}
							}

							return material.Label(th.Theme, th.TextSize, i18n.Translate("Namespace: %s", d.meta.Meta.Namespace)).Layout(gtx)
						}),
						layout.Rigid(func(gtx C) D {
							return material.Label(th.Theme, th.TextSize, i18n.Translate("Library: %s", d.meta.Meta.Library)).Layout(gtx)
						}),
						layout.Rigid(func(gtx C) D {
							return material.Label(th.Theme, th.TextSize, i18n.Translate("Collection: %s", d.meta.Meta.Collection)).Layout(gtx)
						}),
					)
				})
		}),
	)

}

func (d *BibInfoDialog) layoutErr(gtx C, th *theme.Theme, err error) D {
	label := material.Label(th.Theme, th.TextSize, err.Error())
	label.Color = color.NRGBA{R: 255, A: 255}
	label.Alignment = text.Start
	return label.Layout(gtx)
}
