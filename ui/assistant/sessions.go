package assistant

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets/icons"
)

var (
	historyIcon = icons.NewSvgIcon(icons.History)
)

// SessionHistory implements [navpanel.NavSection]
type SessionHistory struct {
	srv *service.ServiceFacade

	list       widget.List
	sessions   []*agent.ACPSession
	fetched    atomic.Bool
	fetchErr   error
	loadBtns   []widget.Clickable
	resumeBtns []widget.Clickable

	// OnSessionSelected is called when the user clicks Load or Resume.
	// sessionID is the selected session ID; load is true for Load, false for Resume.
	OnSessionSelected func(session *agent.ACPSession, load bool)
}

func NewSessionHistory(srv *service.ServiceFacade) *SessionHistory {
	s := &SessionHistory{
		srv: srv,
		list: widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
	}

	s.OnSessionSelected = s.onSessionSelected
	return s
}

// Icon implements [NavSection].
func (s *SessionHistory) Icon() *icons.SvgIcon {
	return historyIcon
}

// LayoutHeader implements [NavSection].
func (s *SessionHistory) LayoutHeader(gtx C, th *theme.Theme) D {
	return layout.Inset{
		Top:    unit.Dp(2),
		Bottom: unit.Dp(2),
		Left:   unit.Dp(4),
		Right:  unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return material.Subtitle2(th.Theme, strings.ToUpper(s.Title())).Layout(gtx)
	})
}

// OnClose implements [NavSection].
func (s *SessionHistory) OnClose() {
}

// Title implements [NavSection].
func (s *SessionHistory) Title() string {
	return i18n.Translate("Sessions")
}

func (s *SessionHistory) Layout(gtx C, th *theme.Theme) D {
	s.fetchSessions()
	s.ensureButtons()

	if s.fetchErr != nil {
		return layout.Center.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, s.fetchErr.Error())
			label.Color = misc.WithAlpha(th.Fg, 0x60)
			return label.Layout(gtx)
		})

	}
	if len(s.sessions) == 0 {
		return layout.Center.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, "No previous sessions")
			label.Color = misc.WithAlpha(th.Fg, 0x60)
			return label.Layout(gtx)
		})
	}

	list := material.List(th.Theme, &s.list)
	list.AnchorStrategy = material.Overlay
	list.ScrollbarStyle = utils.MakeScrollbar(th.Theme, list.Scrollbar, misc.WithAlpha(th.Fg, 0x30))

	return list.Layout(gtx, len(s.sessions), func(gtx C, index int) D {
		return s.layoutItem(gtx, th, index)
	})
}

func (s *SessionHistory) layoutItem(gtx C, th *theme.Theme, index int) D {
	sn := s.sessions[index]
	title := sn.Title()
	if title == "" {
		title = string(sn.SessionID)
	}

	updated := sn.UpdatedAt()

	loadClicked := false
	resumeClicked := false
	if index < len(s.loadBtns) && s.loadBtns[index].Clicked(gtx) {
		loadClicked = true
	}
	if index < len(s.resumeBtns) && s.resumeBtns[index].Clicked(gtx) {
		resumeClicked = true
	}

	if loadClicked && s.OnSessionSelected != nil {
		s.OnSessionSelected(sn, true)
	}
	if resumeClicked && s.OnSessionSelected != nil {
		s.OnSessionSelected(sn, false)
	}

	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
		Left:   unit.Dp(8),
		Right:  unit.Dp(8),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				label := material.Label(th.Theme, th.TextSize, title)
				label.Color = th.Fg
				label.MaxLines = 2
				return label.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx C) D {
				if updated == "" {
					return D{}
				}
				label := material.Label(th.Theme, th.TextSize*0.9, updated)
				label.Color = misc.WithAlpha(th.Fg, 0xb0)
				return label.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						if index >= len(s.loadBtns) {
							return D{}
						}

						btn := material.Button(th.Theme, &s.loadBtns[index], i18n.Translate("Load"))
						btn.Inset = layout.Inset{
							Top: unit.Dp(2), Bottom: unit.Dp(2),
							Left: unit.Dp(8), Right: unit.Dp(8),
						}
						btn.Color = th.Fg
						btn.Background = th.Bg
						return btn.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx C) D {
						if index >= len(s.resumeBtns) {
							return D{}
						}

						btn := material.Button(th.Theme, &s.resumeBtns[index], i18n.Translate("Resume"))
						btn.Inset = layout.Inset{
							Top: unit.Dp(2), Bottom: unit.Dp(2),
							Left: unit.Dp(8), Right: unit.Dp(8),
						}
						btn.Color = th.Fg
						btn.Background = th.Bg
						return btn.Layout(gtx)

					}),
				)
			}),
		)
	})
}

func (s *SessionHistory) ensureButtons() {
	if len(s.loadBtns) < len(s.sessions) {
		for i := len(s.loadBtns); i < len(s.sessions); i++ {
			s.loadBtns = append(s.loadBtns, widget.Clickable{})
			s.resumeBtns = append(s.resumeBtns, widget.Clickable{})
		}
	}
}

func (s *SessionHistory) fetchSessions() {
	if s.fetched.CompareAndSwap(false, true) {
		cwd := s.srv.CurrentProjectDir()
		if cwd == "" {
			s.fetched.Store(false)
			return
		}

		manager := s.srv.AcpSessionManager()
		if manager == nil {
			s.fetchErr = errors.New("no session manager.")
			s.fetched.Store(false)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sessions, err := manager.ListSessions(ctx, cwd)
		if err != nil {
			s.fetchErr = err
			// s.fetched.Store(false)
			return
		}

		s.sessions = sessions
	}
}

func (s *SessionHistory) onSessionSelected(session *agent.ACPSession, load bool) {
	manager := s.srv.AcpSessionManager()
	if manager == nil {
		return
	}

	intent := view.Intent{
		Target:      AgentChatViewID,
		ShowAsModal: false,
		RequireNew:  true, // Don't overwrite existing chats.
		Params: map[string]any{
			"session": session,
		},
	}

	s.srv.RequestSwitch(intent)
}
