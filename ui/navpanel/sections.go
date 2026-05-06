package navpanel

import (
	"errors"
	"log"
	"os"
	"sync"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	appicons "looz.ws/typstify/widgets/icons"
)

var (

	//infoIcon, _       = widget.NewIcon(icons.ActionInfoOutline)
	infoIcon       = appicons.NewSvgIcon(appicons.Info)
	explorerIcon   = appicons.NewSvgIcon(appicons.FolderTree)
	viewListIcon   = appicons.NewSvgIcon(appicons.List)
	arrowRightIcon = appicons.NewSvgIcon(appicons.ChevronRight)
	arrowDownIcon  = appicons.NewSvgIcon(appicons.ChevronDown)
	tocIcon        = appicons.NewSvgIcon(appicons.TableOfContents)
	ellipsisIcon   = appicons.NewSvgIcon(appicons.Ellipsis)
	functionIcon   = appicons.NewSvgIcon(appicons.SquareFunction)
	variableIcon   = appicons.NewSvgIcon(appicons.Variable)
	classIcon      = appicons.NewSvgIcon(appicons.Package)
)

type UpdateTips struct {
	srv           *service.ServiceFacade
	once          sync.Once
	latestVersion string
	hide          bool
	confirmBtn    widget.Clickable
	cancelBtn     widget.Clickable
}

func (ut *UpdateTips) Layout(gtx C, th *theme.Theme) D {
	if ut.srv.Settings().General().CheckUpdate != "true" || ut.hide {
		return D{}
	}

	ut.once.Do(func() {
		go func() {
			releaseInfo := ut.srv.CheckUpdate()
			if releaseInfo != nil {
				ut.latestVersion = releaseInfo.AppVersion
			}
		}()
	})

	if ut.cancelBtn.Clicked(gtx) {
		ut.hide = true
	}
	if ut.confirmBtn.Clicked(gtx) {
		err := giohyperlink.Open("https://typstify.com/download")
		if err != nil {
			log.Printf("error: opening hyperlink: %v", err)
		}
		ut.hide = true
	}

	if ut.latestVersion == "" {
		return D{}
	}

	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
		return widget.Border{Width: unit.Dp(1), CornerRadius: unit.Dp(8), Color: misc.WithAlpha(th.Fg, th.SelectedAlpha)}.Layout(gtx, func(gtx C) D {
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx C) D {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Middle,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return infoIcon.Layout(gtx, th.Fg, th.TextSize)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx C) D {
						lb := material.Label(th.Theme, th.TextSize*0.8, i18n.Translate("A new version is avaliable: %s", ut.latestVersion))
						lb.Alignment = text.Middle
						return lb.Layout(gtx)
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx C) D {
						return layout.Flex{
							Axis:    layout.Horizontal,
							Spacing: layout.SpaceBetween,
						}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								btn := material.Button(th.Theme, &ut.cancelBtn, i18n.Translate("Later"))
								btn.Inset = layout.UniformInset(unit.Dp(6))
								btn.Background = misc.WithAlpha(th.ContrastBg, 0xff)
								btn.TextSize = th.TextSize * 0.8
								return btn.Layout(gtx)
							}),
							layout.Rigid(func(gtx C) D {
								btn := material.Button(th.Theme, &ut.confirmBtn, i18n.Translate("Download"))
								btn.Inset = layout.UniformInset(unit.Dp(6))
								btn.Background = misc.WithAlpha(th.ContrastBg, 0xff)
								btn.TextSize = th.TextSize * 0.8
								return btn.Layout(gtx)
							}),
						)

					}),
				)
			})
		})
	})

}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist) && st.IsDir()
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !st.IsDir()
}
