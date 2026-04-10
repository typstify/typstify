package navpanel

import (
	"errors"
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/list"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/utils"
	appicons "looz.ws/typstify/widgets/icons"
)

var (

	//infoIcon, _       = widget.NewIcon(icons.ActionInfoOutline)
	infoIcon       = appicons.NewSvgIcon(appicons.Info)
	viewListIcon   = appicons.NewSvgIcon(appicons.List)
	historyIcon    = appicons.NewSvgIcon(appicons.History)
	arrowRightIcon = appicons.NewSvgIcon(appicons.ChevronRight)
	arrowDownIcon  = appicons.NewSvgIcon(appicons.ChevronDown)
)

type RecentProjects struct {
	vm         view.ViewManager
	srv        *service.ServiceFacade
	list       widget.List
	recentList []service.WorkspaceState
	labels     []*list.InteractiveLabel
	selected   int
}

type UpdateTips struct {
	srv           *service.ServiceFacade
	once          sync.Once
	latestVersion string
	hide          bool
	confirmBtn    widget.Clickable
	cancelBtn     widget.Clickable
}

func NewRecentProjects(vm view.ViewManager, srv *service.ServiceFacade) *RecentProjects {
	return &RecentProjects{
		vm:  vm,
		srv: srv,
		list: widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
	}
}

func (rp *RecentProjects) Title() string {
	return i18n.Translate("Recent")
}

func (rp *RecentProjects) openSelected(projectDir string) {
	if !dirExists(projectDir) {
		rp.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: fmt.Sprintf("%s does not exists", projectDir), Duration: time.Second * 3})
		return
	}
	rp.srv.EventBus().Emit(bus.TopicProjectSwitched, projectDir)
}

func (rp *RecentProjects) Update(gtx C) bool {
	recent := rp.srv.Workspace().GetHistory(100)
	if len(rp.recentList) != len(recent) {
		rp.recentList = recent
	} else if len(recent) > 0 {
		if !recent[0].LastAccessAt.Equal(rp.recentList[0].LastAccessAt) {
			rp.recentList = recent
		}
	}

	selected := false
	for index, proj := range rp.recentList {
		if len(rp.labels) < index+1 {
			rp.labels = append(rp.labels, &list.InteractiveLabel{})
		}
		label := rp.labels[index]

		if label.Update(gtx) {
			rp.selected = index
			rp.openSelected(proj.Path)
			selected = true
		} else if label.IsSelected() {
			label.Unselect()
		}
	}

	return selected
}

func (rp *RecentProjects) Layout(gtx C, th *theme.Theme) D {
	rp.Update(gtx)
	list := material.List(th.Theme, &rp.list)
	list.AnchorStrategy = material.Overlay
	list.ScrollbarStyle = utils.MakeScrollbar(th.Theme, list.Scrollbar, misc.WithAlpha(th.Fg, 0x30))

	return list.Layout(gtx, len(rp.recentList), func(gtx C, index int) D {
		label := rp.labels[index]
		proj := rp.recentList[index]

		return label.Layout(gtx, th, func(gtx C, textColor color.NRGBA) D {
			return layout.Inset{
				Left:   unit.Dp(16),
				Right:  unit.Dp(16),
				Top:    unit.Dp(3),
				Bottom: unit.Dp(3),
			}.Layout(gtx, func(gtx C) D {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
					Spacing:   layout.SpaceEnd,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						lb := material.Label(th.Theme, th.TextSize, filepath.Base(proj.Path))
						lb.Color = th.Fg
						return lb.Layout(gtx)

					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx C) D {
						lb := material.Label(th.Theme, th.TextSize*0.9, filepath.Dir(proj.RelPath))
						lb.MaxLines = 1
						lb.Color = misc.WithAlpha(th.Fg, 0xb6)
						return lb.Layout(gtx)
					}),
				)
			})
		})

	})
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
