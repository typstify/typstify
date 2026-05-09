package ui

import (
	"log"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/inkeliz/giohyperlink"
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

const (
	typstifyUrl = "https://typstify.com"
	tpixUrl     = "https://tpix.typstify.com"
)

var (
	openFolderIcon    = icons.NewSvgIcon(icons.FolderOpen)
	createProjectIcon = icons.NewSvgIcon(icons.FolderPlus)
	browseIcon        = icons.NewSvgIcon(icons.PackageSearch)
	userIcon          = icons.NewSvgIcon(icons.User)
)

type WelcomeView struct {
	vm view.ViewManager
	page.PageStyle
	srv             *service.ServiceFacade
	createBtn       widget.Clickable
	openBtn         widget.Clickable
	browsePkgBtn    widget.Clickable
	tpixWebsiteLink widget.Clickable
	typstifyLink    widget.Clickable
}

// func (vw *WelcomeView) ID() view.ViewID {
// 	return HelpViewID
// }

func (vw *WelcomeView) Title() string {
	return i18n.Translate("Start")
}

func (vw *WelcomeView) Layout(gtx C, th *theme.Theme) D {
	vw.update(gtx)
	viewport := gtx.Constraints

	return vw.PageStyle.Layout(gtx, th, func(gtx C) D {
		gtx.Constraints.Min.Y = viewport.Max.Y
		gtx.Constraints.Max.Y = viewport.Max.Y

		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				return vw.layoutMain(gtx, th)
			}),
			layout.Rigid(func(gtx C) D {
				return vw.layoutShortcuts(gtx, th)

			}),
		)
	})

}

func (vw *WelcomeView) layoutMain(gtx C, th *theme.Theme) D {
	top := gtx.Metric.PxToDp(int(float32(gtx.Constraints.Max.Y) * 0.2))

	return layout.Inset{
		Left:  unit.Dp(80),
		Right: unit.Dp(80),
		Top:   top,
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				label := material.H3(th.Theme, i18n.Translate("Typstify"))
				label.Color = th.Fg
				return label.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

			layout.Rigid(func(gtx C) D {
				slogan := material.Label(th.Theme, th.TextSize*1.2, i18n.Translate("Crafting Typst documents at the speed of thought."))
				slogan.Color = misc.WithAlpha(th.Fg, 0xb6)
				slogan.Font.Weight = font.Medium
				slogan.Font.Style = font.Italic
				return slogan.Layout(gtx)
			}),

			layout.Rigid(layout.Spacer{Height: unit.Dp(36)}.Layout),

			layout.Rigid(func(gtx C) D {
				if vw.srv.Settings().Tpix().Username == "" {
					return D{}
				}

				return layout.Flex{
					Gap: gtx.Dp(unit.Dp(8)),
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return userIcon.Layout(gtx, th.Fg, th.TextSize)
					}),
					layout.Rigid(func(gtx C) D {
						return material.Label(th.Theme, th.TextSize, i18n.Translate("Welcome, %s!", vw.srv.Settings().Tpix().Username)).Layout(gtx)
					}),
				)
			}),

			layout.Rigid(layout.Spacer{Height: unit.Dp(36)}.Layout),

			layout.Rigid(func(gtx C) D {
				label := material.H5(th.Theme, i18n.Translate("Getting Started"))
				label.Color = th.Fg
				return label.Layout(gtx)
			}),

			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(gtx C) D {
				return layoutOp(gtx, th, &vw.createBtn, i18n.Translate("New project..."), createProjectIcon)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

			layout.Rigid(func(gtx C) D {
				return layoutOp(gtx, th, &vw.openBtn, i18n.Translate("Open existing project..."), openFolderIcon)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

			layout.Rigid(func(gtx C) D {
				return layoutOp(gtx, th, &vw.browsePkgBtn, i18n.Translate("Browse Packages/Templates..."), browseIcon)
			}),

			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

			layout.Rigid(func(gtx C) D {
				label := material.H5(th.Theme, i18n.Translate("Learn more"))
				label.Color = th.Fg
				return label.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

			layout.Rigid(func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return material.Label(th.Theme, th.TextSize, i18n.Translate("To learn more about Typstify, go to ")).Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						return material.Clickable(gtx, &vw.typstifyLink, func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize, typstifyUrl)
							label.Color = th.ContrastBg
							return label.Layout(gtx)
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return material.Label(th.Theme, th.TextSize, i18n.Translate("To learn more about TPIX, click ")).Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						return material.Clickable(gtx, &vw.tpixWebsiteLink, func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize, tpixUrl)
							label.Color = th.ContrastBg
							return label.Layout(gtx)
						})
					}),
				)

			}),
		)
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

	if vw.tpixWebsiteLink.Clicked(gtx) {
		if err := giohyperlink.Open(tpixUrl); err != nil {
			log.Printf("error: opening hyperlink: %v", err)
		}
	}

	if vw.typstifyLink.Clicked(gtx) {
		if err := giohyperlink.Open(typstifyUrl); err != nil {
			log.Printf("error: opening hyperlink: %v", err)
		}
	}
}

func (vw *WelcomeView) layoutShortcuts(gtx C, th *theme.Theme) D {
	shortcutLabel := func(gtx C, text string) D {
		label := material.Caption(th.Theme, text)
		label.Color = misc.WithAlpha(th.Fg, 0xb0)
		return label.Layout(gtx)
	}

	return layout.Inset{
		Left:  unit.Dp(80),
		Right: unit.Dp(80),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Horizontal,
			Gap:  gtx.Dp(unit.Dp(8)),
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return shortcutLabel(gtx, i18n.Translate("Open/Hide File Explorer: %s + D", key.ModShortcut.String()))
			}),
			layout.Rigid(func(gtx C) D {
				return shortcutLabel(gtx, i18n.Translate("Open/Hide Console: %s + K", key.ModShortcut.String()))
			}),
			layout.Rigid(func(gtx C) D {
				return shortcutLabel(gtx, i18n.Translate("Open/Hide Previewer: %s + P", key.ModShortcut.String()))
			}),
		)
	})
}
