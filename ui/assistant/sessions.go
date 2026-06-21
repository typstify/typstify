package assistant

import (
	"context"
	"errors"
	"image/color"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	gvwidget "github.com/oligo/gioview/widget"
	"github.com/sahilm/fuzzy"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets"
	"looz.ws/typstify/widgets/icons"
)

var (
	historyIcon = icons.NewSvgIcon(icons.MessagesSquare)
	searchIcon  = icons.NewSvgIcon(icons.Search)
)

// SessionHistory implements [navpanel.NavSection]
type SessionHistory struct {
	srv *service.ServiceFacade

	list             widget.List
	projectDir       string
	sessions         []*agent.ACPSession
	filteredSessions []*agent.ACPSession
	fetched          atomic.Bool
	fetchErr         error
	labels           []widgets.InteractiveLabel
	selectedIdx      int

	// OnSessionSelected is called when the user clicks Load or Resume.
	// sessionID is the selected session ID; load is true for Load, false for Resume.
	OnSessionSelected func(session *agent.ACPSession, load bool)

	searchInput gvwidget.TextField
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

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			s.searchInput.Leading = func(gtx C) D {
				return searchIcon.Layout(gtx, th.Fg, th.TextSize)
			}
			s.searchInput.Alignment = text.Start
			s.searchInput.MaxChars = 128
			s.searchInput.SingleLine = true
			s.searchInput.Padding = unit.Dp(6)

			return s.searchInput.Layout(gtx, th, "")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Flexed(1, func(gtx C) D {
			return s.layout(gtx, th)
		}),
	)
}

func (s *SessionHistory) layout(gtx C, th *theme.Theme) D {
	if s.fetchErr != nil {
		return layout.Center.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, s.fetchErr.Error())
			label.Color = misc.WithAlpha(th.Fg, 0x60)
			return label.Layout(gtx)
		})

	}

	if s.searchInput.Changed() {
		pattern := strings.TrimSpace(s.searchInput.Text())
		if pattern != "" {
			s.filteredSessions = filterSessions(s.sessions, pattern)
			s.labels = s.labels[:0]
		}
	}

	if len(s.filteredSessions) == 0 {
		return layout.Center.Layout(gtx, func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, "No previous sessions")
			label.Color = misc.WithAlpha(th.Fg, 0x60)
			return label.Layout(gtx)
		})
	}

	list := material.List(th.Theme, &s.list)
	list.AnchorStrategy = material.Overlay
	list.ScrollbarStyle = utils.MakeScrollbar(th.Theme, list.Scrollbar, misc.WithAlpha(th.Fg, 0x30))

	return list.Layout(gtx, len(s.filteredSessions), func(gtx C, index int) D {
		s.ensureButtons(index)
		return s.layoutItem(gtx, th, index)
	})
}

func (s *SessionHistory) layoutItem(gtx C, th *theme.Theme, index int) D {
	sn := s.filteredSessions[index]
	title := sn.Title()
	if title == "" {
		title = string(sn.SessionID)
	}

	updated := sn.UpdatedAt()
	label := &s.labels[index]
	clicked := false
	lastIdx := s.selectedIdx

	if label.Update(gtx) {
		clicked = true
		s.selectedIdx = index

		if lastIdx < len(s.labels) && s.labels[lastIdx].IsSelected() {
			s.labels[lastIdx].Unselect()
		}
	}

	if clicked && s.OnSessionSelected != nil {
		s.OnSessionSelected(sn, true)
	}

	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
		Left:   unit.Dp(12),
		Right:  unit.Dp(8),
	}.Layout(gtx, func(gtx C) D {
		return label.Layout(gtx, th, func(gtx C, textColor color.NRGBA) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize, title)
					label.Color = th.Fg
					label.MaxLines = 2
					return label.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx C) D {
					if updated.IsZero() {
						return D{}
					}
					label := material.Label(th.Theme, th.TextSize*0.9, updated.Format(time.DateTime))
					label.Color = misc.WithAlpha(th.Fg, 0xb0)
					return label.Layout(gtx)
				}),
			)
		})

	})
}

func (s *SessionHistory) ensureButtons(index int) {
	for len(s.labels) <= index {
		s.labels = append(s.labels, widgets.InteractiveLabel{})
	}
}

func (s *SessionHistory) fetchSessions() {
	cwd := s.srv.CurrentProjectDir()
	if cwd == "" {
		return
	}

	if cwd != "" && cwd != s.projectDir {
		s.fetched.Store(false)
		s.projectDir = cwd
		return
	}

	if s.fetched.CompareAndSwap(false, true) {
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
		s.filteredSessions = s.sessions
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
		RequireNew:  false, // Replace existing chat view, to ensure there is only one global AgentChatView.
		Params: map[string]any{
			"session": session,
		},
	}

	s.srv.RequestSwitch(intent)
}

type chatSessions []*agent.ACPSession

func (src chatSessions) String(i int) string {
	return src[i].Title()
}

func (src chatSessions) Len() int {
	return len(src)
}

func filterSessions(src []*agent.ACPSession, pattern string) []*agent.ACPSession {
	matches := fuzzy.FindFrom(pattern, chatSessions(src))
	matched := make([]*agent.ACPSession, 0, matches.Len())
	for _, m := range matches {
		matched = append(matched, src[m.Index])
	}

	return matched
}
