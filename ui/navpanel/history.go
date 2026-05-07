package navpanel

import (
	"fmt"
	"path/filepath"
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/widgets"
)

type RecentProject struct {
	srv   *service.ServiceFacade
	state service.WorkspaceState
}

func (rp *RecentProject) OnClicked() {
	projectDir := rp.state.Path
	if !dirExists(rp.state.Path) {
		rp.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: fmt.Sprintf("%s does not exists", projectDir), Duration: time.Second * 5})
		return
	}
	rp.srv.EventBus().Emit(bus.TopicProjectSwitched, projectDir)
}

func (rp *RecentProject) Layout(gtx C, th *theme.Theme) D {
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
				lb := material.Label(th.Theme, th.TextSize, filepath.Base(rp.state.Path))
				lb.Color = th.Fg
				return lb.Layout(gtx)

			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx C) D {
				lb := material.Label(th.Theme, th.TextSize*0.9, filepath.Dir(rp.state.RelPath))
				lb.MaxLines = 1
				lb.Color = misc.WithAlpha(th.Fg, 0xb6)
				return lb.Layout(gtx)
			}),
		)
	})

}

type RecentProjects struct {
	srv          *service.ServiceFacade
	recentList   []service.WorkspaceState
	items        []widgets.PopupWidget
	historyPopup *widgets.Popup
}

func NewRecentProjects(srv *service.ServiceFacade) *RecentProjects {
	historyPopup := &widgets.Popup{
		MaxHeight: unit.Dp(400),
		Width:     unit.Dp(350),
		Direction: layout.S,
	}
	return &RecentProjects{srv: srv, historyPopup: historyPopup}
}

func (rp *RecentProjects) update() {
	rebuild := false
	recent := rp.srv.Workspace().GetHistory(100)
	if len(rp.recentList) != len(recent) {
		rp.recentList = recent
		rebuild = true
	} else if len(recent) > 0 {
		if !recent[0].LastAccessAt.Equal(rp.recentList[0].LastAccessAt) {
			rp.recentList = recent
			rebuild = true
		}
	}

	if rebuild {
		rp.items = rp.items[:0]
		for i := range rp.recentList {
			rp.items = append(rp.items, &RecentProject{
				srv:   rp.srv,
				state: rp.recentList[i],
			})
		}
	}

}

func (rp *RecentProjects) Show() {
	rp.historyPopup.SetOpen()
}

func (rp *RecentProjects) Layout(gtx C, th *theme.Theme, btn layout.Widget) D {
	rp.update()

	return rp.historyPopup.Layout(gtx, th,
		btn,
		rp.items...,
	)
}
