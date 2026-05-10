package statusbar

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/console"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets"
	"looz.ws/typstify/widgets/icons"
	"looz.ws/typstify/widgets/menu"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var defaultActiveDuration = time.Second * 5
var (
	consoleIcon   = icons.NewSvgIcon(icons.Terminal)
	chatIcon      = icons.NewSvgIcon(icons.SquareFunction)
	infoIcon      = icons.NewSvgIcon(icons.Info)
	warnIcon      = icons.NewSvgIcon(icons.CircleAlert)
	errorIcon     = icons.NewSvgIcon(icons.CircleX)
	gitBranchIcon = icons.NewSvgIcon(icons.GitBranch)
	gitTagIcon    = icons.NewSvgIcon(icons.Tag)
)

// Views can implement StatusLine interface to let StatusBar render their
// status indicator.
type StatusIndicator interface {
	LayoutStatus(gtx C, th *theme.Theme) D
}

type NotificationBar struct {
	lastMessage *Notification
	// last notification update time
	lastUpdateTime time.Time
}

type Notification struct {
	Level    int
	Content  string
	Duration time.Duration
}

type StatusBar struct {
	vm                 view.ViewManager
	notification       *NotificationBar
	gitStatusIndicator *GitStatusIndicator
	consoleState       *console.ConsoleState
	showConsoleBtn     widget.Clickable
	showChatBtn        widget.Clickable
}

func (n *NotificationBar) Layout(gtx C, th *theme.Theme) D {
	if n.lastMessage == nil {
		return D{}
	}

	// If idleDuration has zero value, the message will not expire.
	if n.lastUpdateTime.IsZero() && n.lastMessage.Duration > 0 {
		n.lastUpdateTime = gtx.Now
		gtx.Execute(op.InvalidateCmd{At: n.lastUpdateTime.Add(n.lastMessage.Duration)})
	} else if n.lastMessage.Duration > 0 && gtx.Now.Sub(n.lastUpdateTime) > n.lastMessage.Duration {
		defer func() {
			n.lastUpdateTime = time.Time{}
			n.lastMessage = nil
		}()
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			var icon *icons.SvgIcon
			switch n.lastMessage.Level {
			case 0:
				icon = infoIcon
			case 1:
				icon = warnIcon

			case 2:
				icon = errorIcon
			default:
				icon = errorIcon
			}

			return icon.Layout(gtx, th.Fg, th.TextSize)
		}),

		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx C) D {
			return material.Label(th.Theme, th.TextSize*0.9, n.lastMessage.Content).Layout(gtx)
		}),
	)
}

func (s *StatusBar) Update(gtx C) (consoleClicked, chatClicked bool) {
	if s.showConsoleBtn.Clicked(gtx) {
		consoleClicked = true
	}
	if s.showChatBtn.Clicked(gtx) {
		chatClicked = true
	}
	return
}

func (s *StatusBar) Layout(gtx C, th *theme.Theme) D {
	s.Update(gtx)

	if s.notification.lastMessage == nil && s.vm.CurrentView() == nil {
		return D{}
	}

	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
		Left:   unit.Dp(12),
		Right:  unit.Dp(12),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceBetween,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return s.gitStatusIndicator.Layout(gtx, th)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(32)}.Layout),
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
					Spacing:   layout.SpaceStart,
				}.Layout(gtx,

					layout.Flexed(1, func(gtx C) D {
						return s.notification.Layout(gtx, th)
					}),

					layout.Rigid(func(gtx C) D {
						vw := s.vm.CurrentView()
						if vw == nil {
							return D{}
						}

						status, ok := vw.(StatusIndicator)
						if !ok {
							return D{}
						}

						return status.LayoutStatus(gtx, th)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx C) D {
						return material.Clickable(gtx, &s.showChatBtn, func(gtx C) D {
							fillColor := th.Fg
							if s.showChatBtn.Hovered() {
								fillColor = th.ContrastBg
							}
							return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
								return chatIcon.Layout(gtx, fillColor, th.TextSize)
							})
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx C) D {
						return material.Clickable(gtx, &s.showConsoleBtn, func(gtx C) D {
							fillColor := th.Fg
							if s.consoleState.HasMore() {
								fillColor = th.ContrastBg
							}
							return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
								return consoleIcon.Layout(gtx, fillColor, th.TextSize)
							})
						})
					}),
				)
			}),
		)

	})

}

func NewStatusBar(srv *service.ServiceFacade, vm view.ViewManager) *StatusBar {
	sb := &StatusBar{
		vm:           vm,
		notification: &NotificationBar{},
		consoleState: srv.Console(),
		gitStatusIndicator: &GitStatusIndicator{
			workspaceSrv:   srv.Workspace(),
			eventBus:       srv.EventBus(),
			maxPopupHeight: unit.Dp(450),
			popupWidth:     unit.Dp(350),
		},
	}
	eventbus := srv.EventBus()
	eventbus.Subscribe(sb, "statusbar.event", `statusbar\..*`, func(topic string, data interface{}) {
		switch topic {
		case bus.TopicStatusbarNotifyEvent:
			msg := data.(Notification)
			if msg.Duration <= 0 {
				msg.Duration = defaultActiveDuration
			}
			sb.notification.lastMessage = &msg
			vm.Invalidate()
		}
	})

	return sb
}

type GitStatusIndicator struct {
	workspaceSrv *service.WorkspaceService
	eventBus     *bus.EventBus

	switchBranchBtn widget.Clickable
	branchList      layout.List
	branchLabels    []*widgets.InteractiveLabel
	popupArea       menu.ContextArea
	maxPopupHeight  unit.Dp
	popupWidth      unit.Dp
	pendingOpen     bool
}

func (gs *GitStatusIndicator) Layout(gtx C, th *theme.Theme) D {
	gitRepo := gs.workspaceSrv.GitRepo()

	if gitRepo.Branch == "" {
		return D{}
	}

	hasChanges := len(gitRepo.Changes) > 0
	labelText := gitRepo.Branch
	if hasChanges {
		labelText = fmt.Sprintf("%s*", gitRepo.Branch)
	}

	fill := misc.WithAlpha(th.Fg, 0xb6)
	if gs.switchBranchBtn.Hovered() || gs.popupArea.Active() {
		fill = th.ContrastBg
	}

	if gs.switchBranchBtn.Clicked(gtx) {
		gs.pendingOpen = true
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			return gs.layoutButton(gtx, th, fill, labelText)
		}),

		layout.Expanded(func(gtx C) D {
			defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Min}).Push(gtx.Ops).Pop()

			//originalMin := gtx.Constraints.Min
			gtx.Constraints.Min = image.Point{}

			originalOps := gtx.Ops
			gtx.Ops = &op.Ops{}
			dims := gs.layoutBranchPopup(gtx, th)
			gtx.Ops = originalOps

			offset := image.Pt(0, -dims.Size.Y)

			macro := op.Record(gtx.Ops)
			op.Offset(offset).Add(gtx.Ops)
			if gs.pendingOpen {
				gs.popupArea.PositionHint = layout.N
				gs.popupArea.Show(f32.Pt(0, -float32(offset.Y)))
				gs.pendingOpen = false
			}

			gs.popupArea.Layout(gtx, func(gtx C) D {
				return gs.layoutBranchPopup(gtx, th)
			})

			call := macro.Stop()
			op.Defer(gtx.Ops, call)

			return D{}

		}),
	)

}

func (gs *GitStatusIndicator) layoutButton(gtx C, th *theme.Theme, fill color.NRGBA, labelText string) D {
	return gs.switchBranchBtn.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return gitBranchIcon.Layout(gtx, fill, th.TextSize)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
				layout.Rigid(func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize, labelText)
					label.Color = fill
					return label.Layout(gtx)
				}),
			)
		})
	})
}

func (gs *GitStatusIndicator) layoutBranchPopup(gtx C, th *theme.Theme) D {

	gitRepo := gs.workspaceSrv.GitRepo()
	branches := gitRepo.AllBranches
	currentBranch := gitRepo.Branch

	if len(branches) == 0 {
		return D{}
	}

	if gs.maxPopupHeight > 0 {
		gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(gs.maxPopupHeight))
	}

	if gs.popupWidth > 0 {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(gs.popupWidth))
	}

	surface := component.Surface(th.Theme)
	surface.Fill = misc.WithAlpha(th.Bg, 0xff)
	surface.CornerRadius = unit.Dp(4)

	gs.branchList.Axis = layout.Vertical
	return surface.Layout(gtx, func(gtx C) D {
		return gs.branchList.Layout(gtx, len(branches), func(gtx C, index int) D {
			if len(gs.branchLabels) <= index {
				gs.branchLabels = append(gs.branchLabels, &widgets.InteractiveLabel{})
			}

			return gs.layoutBranchItem(gtx, th, gs.branchLabels[index], branches[index], currentBranch)
		})
	})
}

func (gs *GitStatusIndicator) layoutBranchItem(gtx C, th *theme.Theme, state *widgets.InteractiveLabel, info utils.BranchInfo, currentBranch string) D {
	if state.Update(gtx) {
		gs.popupArea.Dismiss()
		if info.Name != currentBranch {
			go func() {
				if err := utils.SwitchGitBranch(gs.workspaceSrv.Current().Path, info.Name); err != nil {
					log.Printf("git checkout %s failed: %v", info.Name, err)
					gs.eventBus.Emit(bus.TopicStatusbarNotifyEvent,
						Notification{Level: 2, Content: i18n.Translate("git checkout %s failed: %v", info.Name, err)})
				}
			}()
		}
	} else if state.IsSelected() {
		state.Unselect()
	}

	gtx.Constraints.Min.X = gtx.Constraints.Max.X

	return state.Layout(gtx, th, func(gtx C, textColor color.NRGBA) D {
		return layout.Inset{
			Top:    unit.Dp(4),
			Bottom: unit.Dp(4),
			Left:   unit.Dp(12),
			Right:  unit.Dp(12),
		}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							icon := gitBranchIcon
							if info.Kind == utils.RefTag {
								icon = gitTagIcon
							}
							return icon.Layout(gtx, th.Fg, th.TextSize)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx C) D {
							return material.Label(th.Theme, th.TextSize, info.Name).Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx C) D {
					detail := fmt.Sprintf("%s  %s  %s  %s", info.Commit, info.Author, info.Date, info.Message)
					label := material.Label(th.Theme, th.TextSize*0.9, detail)
					label.Color = misc.WithAlpha(textColor, 0xb6)
					return label.Layout(gtx)
				}),
			)
		})
	})
}
