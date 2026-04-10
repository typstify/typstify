package statusbar

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/ui/console"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var defaultActiveDuration = time.Second * 5
var (
	notifyInfoIcon, _  = widget.NewIcon(icons.ActionInfoOutline)
	notifyWarnIcon, _  = widget.NewIcon(icons.AlertWarning)
	notifyErrorIcon, _ = widget.NewIcon(icons.AlertError)

	consoleInfoIcon, _   = widget.NewIcon(icons.ActionInfoOutline)
	consoleUpdateIcon, _ = widget.NewIcon(icons.ActionInfo)
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
	vm             view.ViewManager
	notification   *NotificationBar
	consoleState   *console.ConsoleState
	showConsoleBtn widget.Clickable
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
			var icon *widget.Icon
			switch n.lastMessage.Level {
			case 0:
				icon = notifyInfoIcon
			case 1:
				icon = notifyWarnIcon

			case 2:
				icon = notifyErrorIcon
			default:
				icon = notifyErrorIcon
			}

			return misc.Icon{Icon: icon, Size: unit.Dp(16)}.Layout(gtx, th)
		}),

		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx C) D {
			return material.Label(th.Theme, th.TextSize*0.9, n.lastMessage.Content).Layout(gtx)
		}),
	)
}

func (s *StatusBar) Update(gtx C) bool {
	if s.showConsoleBtn.Clicked(gtx) {
		return true
	}

	return false
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
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx C) D {
				return material.Clickable(gtx, &s.showConsoleBtn, func(gtx C) D {
					icon := misc.Icon{Icon: consoleInfoIcon, Color: th.Fg, Size: unit.Dp(th.TextSize * 1.2)}
					if s.consoleState.HasMore() {
						icon.Icon = consoleUpdateIcon
						// icon.Color = misc.WithAlpha(icon.Color, 0xb6)
					}
					return icon.Layout(gtx, th)
				})
			}),
		)
	})

}

func NewStatusBar(srv *service.ServiceFacade, vm view.ViewManager) *StatusBar {
	sb := &StatusBar{
		vm:           vm,
		notification: &NotificationBar{},
		consoleState: srv.Console(),
	}
	eventbus := srv.EventBus()
	eventbus.Subscribe(sb, "statusbar.event", `statusbar\.*`, func(topic string, data interface{}) {
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
