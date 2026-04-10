package navpanel

import (
	"log"
	"path/filepath"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"

	// "golang.org/x/exp/shiny/materialdesign/icons"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/dialog"
	"looz.ws/typstify/ui/pkgmgmt"
	"looz.ws/typstify/ui/settings"
	wg "looz.ws/typstify/widgets"
	"looz.ws/typstify/widgets/icons"
)

var (
	openFolder     = icons.NewSvgIcon(icons.FolderOpen)
	newFolder      = icons.NewSvgIcon(icons.FolderPlus)
	pkgManagerIcon = icons.NewSvgIcon(icons.PackageOpen)
	settingsIcon   = icons.NewSvgIcon(icons.Cog)
)

type CommandPanel struct {
	openDirBtn        widget.Clickable
	openDirTip        wg.TipArea
	openPkgManagerBtn widget.Clickable
	openPkgManagerTip wg.TipArea
	newProjectBtn     widget.Clickable
	newProjectTip     wg.TipArea
	openSettingBtn    widget.Clickable
	openSettingTip    wg.TipArea

	vm  view.ViewManager
	srv *service.ServiceFacade
}

func (cp *CommandPanel) Layout(gtx C, th *theme.Theme) D {
	cp.update(gtx)

	return layout.Inset{
		Left:   unit.Dp(8),
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:    layout.Horizontal,
			Spacing: layout.SpaceEnd,
			Gap:     gtx.Dp(unit.Dp(16)),
		}.Layout(gtx,

			layout.Rigid(func(gtx C) D {
				btn := wg.TipIconButton(th, &cp.openDirTip, i18n.Translate("Open folder"))

				return btn.Layout(gtx, func(gtx C) D {
					return cp.layoutBtn(gtx, th, &cp.openDirBtn, openFolder)
				})
			}),

			layout.Rigid(func(gtx C) D {
				btn := wg.TipIconButton(th, &cp.newProjectTip, i18n.Translate("Create new project"))

				return btn.Layout(gtx, func(gtx C) D {
					return cp.layoutBtn(gtx, th, &cp.newProjectBtn, newFolder)
				})
			}),

			layout.Rigid(func(gtx C) D {
				btn := wg.TipIconButton(th, &cp.openPkgManagerTip, i18n.Translate("Open Typst package center"))
				return btn.Layout(gtx, func(gtx C) D {
					return cp.layoutBtn(gtx, th, &cp.openPkgManagerBtn, pkgManagerIcon)
				})
			}),

			layout.Rigid(func(gtx C) D {
				btn := wg.TipIconButton(th, &cp.openSettingTip, i18n.Translate("Open settings"))
				return btn.Layout(gtx, func(gtx C) D {
					return cp.layoutBtn(gtx, th, &cp.openSettingBtn, settingsIcon)
				})
			}),
		)
	})
}

func (cp *CommandPanel) layoutBtn(gtx C, th *theme.Theme, btn *widget.Clickable, icon *icons.SvgIcon) D {
	return btn.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
			return icon.Layout(gtx, th.Fg, th.TextSize)
		})
	})
}

func (cp *CommandPanel) update(gtx C) {
	cp.openDirTip.Direction = layout.E
	cp.newProjectTip.Direction = layout.E
	cp.openPkgManagerTip.Direction = layout.E
	cp.openSettingTip.Direction = layout.E

	if cp.openSettingBtn.Clicked(gtx) {
		cp.vm.RequestSwitch(view.Intent{
			Target:     settings.SettingViewID,
			RequireNew: true,
		})
	}

	if cp.newProjectBtn.Clicked(gtx) {
		cp.vm.RequestSwitch(view.Intent{
			Target:      dialog.CreateProjectDialogViewID,
			ShowAsModal: true,
		})
	}

	if cp.openDirBtn.Clicked(gtx) {
		go func() {
			projectDir, err := cp.srv.FileChooser().ChooseFolder()
			if err != nil {
				log.Println("failed to choose folder: ", projectDir, err)
				return
			}
			if isFile(projectDir) {
				projectDir = filepath.Dir(projectDir)
			}

			log.Println("choosed folder: ", projectDir)
			cp.srv.EventBus().Emit(bus.TopicProjectSwitched, projectDir)
		}()
	}

	if cp.openPkgManagerBtn.Clicked(gtx) {
		cp.vm.RequestSwitch(view.Intent{
			Target:     pkgmgmt.PkgListViewID,
			RequireNew: true,
		})
	}
}

func NewCommandPanel(vm view.ViewManager, srv *service.ServiceFacade) *CommandPanel {
	return &CommandPanel{
		vm:  vm,
		srv: srv,
	}
}
