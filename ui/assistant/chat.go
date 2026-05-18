package assistant

import (
	"context"
	"log"
	"runtime"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/agent"
	agentview "looz.ws/typstify/agent/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	uipreview "looz.ws/typstify/ui/preview"
	"looz.ws/typstify/utils"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	AgentChatViewID = view.NewViewID("AgentChatView")
)

var _ view.View = (*AgentChatView)(nil)

type AgentChatView struct {
	*view.BaseView
	srv       *service.ServiceFacade
	chat      *agentview.AgentChat
	chatReady atomic.Bool

	// Preview
	uiPreviewer    *uipreview.Previewer
	previewVisible bool
}

func (cv *AgentChatView) ID() view.ViewID {
	return AgentChatViewID
}

func (cv *AgentChatView) Title() string {
	return "AI Assistant"
}

func (cv *AgentChatView) OnNavTo(intent view.Intent) error {
	cv.BaseView.OnNavTo(intent)
	sn, ok := intent.Params["session"].(*agent.ACPSession)
	if !ok {
		cv.init()
	} else {
		cv.loadExisting(sn)
	}

	return nil
}

func (cv *AgentChatView) OnResume() {
	previewSrv := cv.srv.PreviewService()
	if previewSrv == nil {
		return
	}

	serverAddr := previewSrv.Address()
	if serverAddr == "" {
		return
	}

	openInBrowser := cv.srv.Settings().General().OpenPreviewInBrowser != 0
	var isLinux = runtime.GOOS == "linux"
	if openInBrowser || isLinux {
		return
	}

	if !cv.previewVisible {
		return
	}

	if serverAddr != "" && cv.uiPreviewer != nil {
		cv.focusPreviewer()
		cv.uiPreviewer.Navigate(serverAddr)
	}
}

func (cv *AgentChatView) init() {
	projectDir := cv.srv.CurrentProjectDir()
	if projectDir == "" {
		return
	}

	// If we already have a chat view for this project, just show it.
	if cv.chat != nil && cv.chat.Session() != nil && cv.chat.Session().Cwd == projectDir {
		return
	}

	// Close old chat view if switching projects.
	if cv.chat != nil {
		cv.closeChat()
		cv.chatReady.Store(false)
	}

	if cv.chatReady.CompareAndSwap(false, true) {
		go func() {
			session, err := cv.srv.StartACPSession(context.Background(), projectDir)
			if err != nil {
				log.Printf("chat: failed to start ACP session: %v", err)
				cv.chatReady.Store(false)
				return
			}

			cv.chat = agentview.NewAgentChat(session)
			cv.chat.SetInvalidator(func() {
				cv.srv.RefreshWindow()
			})
			cv.chatReady.Store(true)
			cv.srv.RefreshWindow()
		}()
	}
}

func (cv *AgentChatView) loadExisting(sn *agent.ACPSession) {
	if sn == nil {
		log.Println("Invalid session")
		return
	}

	// Close old chat view.
	if cv.chat != nil {
		cv.closeChat()
	}

	if cv.chatReady.CompareAndSwap(false, true) {
		go func() {
			session, err := cv.srv.AcpSessionManager().LoadSession(context.Background(), sn)
			if err != nil {
				log.Printf("chat: failed to load ACP session: %v", err)
				cv.chatReady.Store(false)
				return
			}

			cv.chat = agentview.NewAgentChat(session)
			cv.chat.SetInvalidator(func() {
				cv.srv.RefreshWindow()
			})
			cv.chatReady.Store(true)
			cv.srv.RefreshWindow()
		}()
	}
}

func (cv *AgentChatView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			// TODO
			//return te.header.Layout(gtx, th)
			return D{}
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Flexed(1, func(gtx C) D {
			if cv.chatReady.Load() && cv.chat != nil {
				return cv.chat.Layout(gtx, th)
			} else {
				return layout.Center.Layout(gtx, func(gtx C) D {
					return material.Label(th.Theme, th.TextSize, i18n.Translate("Starting AI Assistant...")).Layout(gtx)
				})
			}
		}),
	)

}

func (cv *AgentChatView) SetPreviewer(previewer *uipreview.Previewer) {
	cv.uiPreviewer = previewer
}

// IsPreviewVisible returns whether the inline preview panel should be shown.
func (cv *AgentChatView) IsVisible() bool {
	return cv.previewVisible
}

// LayoutPreview renders the preview panel. Called by home.go when preview is active.
func (cv *AgentChatView) LayoutPreview(gtx C, th *theme.Theme) D {
	return cv.uiPreviewer.Layout(gtx, th)
}

func (cv *AgentChatView) togglePreview(gtx C) {
	previewSrv := cv.srv.PreviewService()
	if previewSrv == nil {
		return
	}

	serverAddr := previewSrv.Address()
	if serverAddr == "" {
		log.Println("preview ERR: no preview server address")
		return
	}

	// focus LSP triggers a refresh of the preview server.
	cv.focusPreviewer()

	openInBrowser := cv.srv.Settings().General().OpenPreviewInBrowser != 0
	var isLinux = runtime.GOOS == "linux"
	if (openInBrowser || isLinux) && serverAddr != "" {
		utils.OpenInExternalApp(serverAddr)
		cv.previewVisible = false
		return
	}

	// built-in previewer
	cv.previewVisible = !cv.previewVisible

	if !cv.previewVisible {
		return
	}

	if serverAddr != "" && cv.uiPreviewer != nil {
		cv.uiPreviewer.Navigate(serverAddr)
	}
}

func (cv *AgentChatView) focusPreviewer() {

}

func (cv *AgentChatView) closeChat() {
	// Close chat view.
	if cv.chat != nil {
		cv.chat.Close()
		cv.srv.CloseACPSession(context.Background(), cv.chat.Session().SessionID)
		cv.chat = nil
	}
}

func (cv *AgentChatView) OnFinish() {
	cv.BaseView.OnFinish()

	// Close chat view.
	cv.closeChat()
}

func NewAgentChatView(srv *service.ServiceFacade) *AgentChatView {
	return &AgentChatView{
		BaseView: &view.BaseView{},
		srv:      srv,
	}
}
