package dialog

import (
	"errors"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	gw "github.com/oligo/gioview/widget"
	cli "github.com/typstify/tpix-cli"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/widgets"
)

type SyncBibDialog struct {
	srv       *service.ServiceFacade
	parentDir string

	libraries []cli.ZoteroLibrary

	nameInput              gw.TextField
	zoteroCollectionChoice *widgets.Dropdown
	isLoading              atomic.Bool
	loadErr                error
}

var SyncBibDialogViewID = view.NewViewID("SyncBibDialogView")

func NewSyncBibDialog(srv *service.ServiceFacade) view.View {
	bibDialog := &SyncBibDialog{srv: srv}

	dialog := NewDialogModal(CreateProjectDialogViewID, i18n.Translate("Sync Bibliography"), i18n.Translate("Submit"))
	dialog.Dialog = bibDialog
	return dialog
}

func (d *SyncBibDialog) OnInit(intent view.Intent) error {
	val, ok := intent.Params["parentDir"]
	if !ok {
		return nil
	}

	d.parentDir = val.(string)
	go func() {
		d.isLoading.Store(true)
		defer d.isLoading.Store(false)
		libraries, err := cli.ListZoteroLibraries()
		if err != nil {
			d.loadErr = err
			return
		}

		opts := make(map[string]any)

		for _, lib := range libraries {
			for _, collection := range lib.Collections {
				label := fmt.Sprintf("%s | %s", lib.Library.Name, collection.Name)
				if lib.Namespace != "" {
					label = fmt.Sprintf("(@%s)%s | %s", lib.Namespace, lib.Library.Name, collection.Name)
				}
				opts[collection.Key] = label
			}

		}
		d.libraries = libraries
		d.zoteroCollectionChoice = widgets.NewDropDown(opts)
	}()

	return nil
}

func (d *SyncBibDialog) OnConfirm() error {
	if d.loadErr != nil {
		return d.loadErr
	}

	if d.zoteroCollectionChoice == nil {
		return nil
	}

	selectedCollectionKey := d.zoteroCollectionChoice.Value()
	if selectedCollectionKey == "" {
		return errors.New("No collection is selected")
	}

	var libraryID int
	var namespaceID, namespaceName, libraryName, collectionName string
	var scope string
	for _, lib := range d.libraries {
		for _, collection := range lib.Collections {
			if collection.Key == selectedCollectionKey {
				namespaceID = lib.NamespaceID
				libraryID = lib.Library.ID
				scope = lib.Scope
				namespaceName = lib.Namespace
				libraryName = lib.Library.Name
				collectionName = collection.Name
			}
		}

	}

	filename := d.nameInput.Text()
	if filename == "" {
		filename = fmt.Sprintf("%s-%s-%s.bib", namespaceName, libraryName, collectionName)
	}

	var format = "biblatex"
	if strings.HasSuffix(filename, ".yml") || strings.HasSuffix(filename, ".yaml") {
		format = "hayagriva"
	}

	// TODO: should call this asynchronously.
	exportID, err := cli.CreateZoteroExport(filename, namespaceID, scope, int64(libraryID), selectedCollectionKey, format, nil)
	if err != nil {
		d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{
			Content: i18n.Translate("Creating managed bibliography error: %s", err.Error()),
			Level:   2,
		})
		return err
	}

	settings := service.WorkspaceSettings{}
	settings.BibFiles = []service.ManagedBibliography{
		{
			ExportID: exportID,
			File:     filepath.Join(d.parentDir, filename),
			Meta: service.BibliographyExportMeta{
				Namespace:  namespaceName,
				Library:    libraryName,
				Collection: collectionName,
				Format:     format,
			},
		},
	}
	d.srv.Workspace().SaveWorkspaceSetting(settings)
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
				i18n.Translate("The name of the managed bibliography file."),
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
					if d.loadErr != nil {
						return d.layoutErr(gtx, th, d.loadErr)
					}

					if d.zoteroCollectionChoice == nil && d.isLoading.Load() {
						label := material.Label(th.Theme, th.TextSize, i18n.Translate("Loading collecions..."))
						label.Color = misc.WithAlpha(th.Fg, 0xb6)
						return label.Layout(gtx)
					}

					if d.zoteroCollectionChoice != nil {
						return d.zoteroCollectionChoice.Layout(gtx, th)
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
