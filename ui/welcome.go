package ui

import (
	"log"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/page"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/dialog"
	"looz.ws/typstify/ui/pkgmgmt"
	"looz.ws/typstify/widgets/icons"
)

var (
	openFolderIcon    = icons.NewSvgIcon(icons.FolderOpen)
	createProjectIcon = icons.NewSvgIcon(icons.FolderPlus)
	browseIcon        = icons.NewSvgIcon(icons.PackageSearch)
)

type WelcomeView struct {
	vm view.ViewManager
	page.PageStyle
	srv          *service.ServiceFacade
	createBtn    widget.Clickable
	openBtn      widget.Clickable
	browsePkgBtn widget.Clickable
}

// func (vw *WelcomeView) ID() view.ViewID {
// 	return HelpViewID
// }

func (vw *WelcomeView) Title() string {
	return i18n.Translate("Start")
}

func (vw *WelcomeView) Layout(gtx C, th *theme.Theme) D {
	vw.update(gtx)

	return vw.PageStyle.Layout(gtx, th, func(gtx C) D {
		return layout.Inset{
			Left:  unit.Dp(80),
			Right: unit.Dp(80),
			Top:   unit.Dp(200),
		}.Layout(gtx, func(gtx C) D {
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,
				layout.Rigid(
					func(gtx C) D {
						label := material.H5(th.Theme, i18n.Translate("Getting Started"))
						label.Color = th.Fg
						return label.Layout(gtx)
					}),

				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
				layout.Rigid(func(gtx C) D {
					return layoutOp(gtx, th, &vw.createBtn, i18n.Translate("New project..."), createProjectIcon)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

				layout.Rigid(func(gtx C) D {
					return layoutOp(gtx, th, &vw.openBtn, i18n.Translate("Open exiting project..."), openFolderIcon)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

				layout.Rigid(func(gtx C) D {
					return layoutOp(gtx, th, &vw.browsePkgBtn, i18n.Translate("Browse Packages/Templates..."), browseIcon)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

				layout.Rigid(func(gtx C) D {
					return material.Label(th.Theme, th.TextSize, i18n.Translate("Or open a recent project by checking the Recent panel.")).Layout(gtx)
				}),
			)
		})
	})

}

func layoutOp(gtx C, th *theme.Theme, btn *widget.Clickable, text string, icon *icons.SvgIcon) D {
	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
				return icon.Layout(gtx, th.Fg, th.TextSize*1.4)
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Flexed(1, func(gtx C) D {
			return material.Clickable(gtx, btn, func(gtx C) D {
				return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize, text)
					label.Font.Weight = font.Medium
					label.Color = misc.WithAlpha(th.ContrastBg, 0xff)
					return label.Layout(gtx)
				})
			})
		}),
	)

}

func (vw *WelcomeView) update(gtx C) {
	if vw.createBtn.Clicked(gtx) {
		vw.vm.RequestSwitch(view.Intent{
			Target:      dialog.CreateProjectDialogViewID,
			ShowAsModal: true,
		})
	}

	if vw.openBtn.Clicked(gtx) {
		go func() {
			projectDir, err := vw.srv.FileChooser().ChooseFolder()
			if err != nil {
				log.Println("failed to choose folder: ", projectDir, err)
				return
			}

			log.Println("choosed folder: ", projectDir)
			vw.srv.EventBus().Emit(bus.TopicProjectSwitched, projectDir)
		}()
	}

	if vw.browsePkgBtn.Clicked(gtx) {
		vw.vm.RequestSwitch(view.Intent{
			Target:     pkgmgmt.PkgListViewID,
			RequireNew: true,
		})
	}
}
