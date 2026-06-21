package assistant

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
	"github.com/fsnotify/fsnotify"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	"looz.ws/typstify/agent"
	agentview "looz.ws/typstify/agent/view"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/lsp"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/service/mcp"
	"looz.ws/typstify/ui/preview"
	"looz.ws/typstify/utils"
	appIcons "looz.ws/typstify/widgets/icons"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	AgentChatViewID = view.NewViewID("AgentChatView")
	chatIcon        = appIcons.NewSvgIcon(appIcons.Sparkles)
)

var _ view.View = (*AgentChatView)(nil)

type AgentChatView struct {
	*view.BaseView
	srv       *service.ServiceFacade
	chat      *agentview.AgentChat
	chatReady atomic.Bool

	lspClient      *lsp.Client
	previewVisible bool
	previewer      *preview.Previewer
	previewFile    string
	mu             sync.Mutex
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

	client := lsp.GetLspClient(cv.srv.CurrentProjectDir(), cv.srv.Settings())
	if client == nil {
		log.Println("LSP client is not initialized!")
		return errors.New("LSP client is not initialized")
	}
	cv.lspClient = client

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

	if !cv.previewVisible || cv.previewFile == "" {
		return
	}

	if cv.previewer != nil {
		cv.makeFileFocused(cv.previewFile)
		cv.previewer.Navigate(serverAddr)
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
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, th.Bg2, clip.Rect{Max: gtx.Constraints.Max}.Op())

	if !cv.chatReady.Load() || cv.chat == nil {
		return layout.Center.Layout(gtx, func(gtx C) D {
			return material.Label(th.Theme, th.TextSize, i18n.Translate("Starting AI Assistant...")).Layout(gtx)
		})
	}

	return cv.chat.Layout(gtx, th)
}

func (cv *AgentChatView) SetPreviewer(previewer *preview.Previewer) {
	cv.previewer = previewer
}

// IsPreviewVisible returns whether the inline preview panel should be shown.
func (cv *AgentChatView) IsVisible() bool {
	return cv.previewVisible
}

// LayoutPreview renders the preview panel. Called by home.go when preview is active.
func (cv *AgentChatView) LayoutPreview(gtx C, th *theme.Theme) D {
	return cv.previewer.Layout(gtx, th)
}

func (cv *AgentChatView) togglePreview(previewFile string) error {
	previewSrv := cv.srv.PreviewService()
	if previewSrv == nil {
		return errors.New("no preview service")
	}

	serverAddr := previewSrv.Address()
	if serverAddr == "" {
		log.Println("preview ERR: no preview server address")
		return errors.New("no preview server address")
	}

	// focus LSP triggers a refresh of the preview server.
	err := cv.makeFileFocused(previewFile)
	if err != nil {
		return err
	}
	cv.previewVisible = true
	cv.previewFile = previewFile

	openInBrowser := cv.srv.Settings().General().OpenPreviewInBrowser != 0
	var isLinux = runtime.GOOS == "linux"
	if openInBrowser || isLinux {
		utils.OpenInExternalApp(serverAddr)
		return nil
	}

	// built-in previewer
	if cv.previewer != nil {
		cv.previewer.Navigate(serverAddr)
		log.Printf("refreshing preview at %s: %s", serverAddr, cv.previewFile)
	}
	return nil
}

func (cv *AgentChatView) makeFileFocused(previewFile string) error {
	absFile, err := filepath.Abs(previewFile)
	if err != nil {
		log.Printf("invalid file path %s: %v", previewFile, err)
		return err
	}

	fileContent, err := os.ReadFile(absFile)
	if err != nil {
		log.Println("read file failed:", err)
		return err
	}

	if cv.lspClient != nil {
		cv.lspClient.OnEditorUpdated(absFile, bytes.NewReader(fileContent))
	}
	return nil
}

func (cv *AgentChatView) watchPreviewFile(previewFile string) error {
	if err := cv.srv.Workspace().WatchFile(previewFile); err != nil {
		return err
	}

	// unsubscribe first to prevent eventbus complains 'already subscribed'
	cv.srv.EventBus().UnsubscribeByName(cv, "agent.previewfile.changed")
	cv.srv.EventBus().Subscribe(cv, "agent.previewfile.changed", `workspace\.file\.changed`, func(topic string, data interface{}) {
		evt, ok := data.(bus.FileChangedEvent)
		if !ok || filepath.Clean(evt.Path) != filepath.Clean(cv.previewFile) {
			return
		}

		cv.makeFileFocused(cv.previewFile)
		cv.srv.RefreshWindow()
	})

	return nil
}

func (cv *AgentChatView) unwatchPreviewFile() {
	if err := cv.srv.Workspace().UnwatchFile(cv.previewFile); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
		log.Println("unwatch file failed: ", err)
	}
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
	cv.srv.EventBus().Unsubscribe(cv)

	// Close chat view.
	cv.closeChat()
	cv.unwatchPreviewFile()
}

func NewAgentChatView(srv *service.ServiceFacade) *AgentChatView {
	cv := &AgentChatView{
		BaseView:  &view.BaseView{},
		srv:       srv,
		previewer: preview.NewPreviewer(srv),
	}

	srv.EventBus().Subscribe(cv, "agentChatView", `preview\.toggle`, func(topic string, data interface{}) {
		previewParams, ok := data.(mcp.PreviewParams)
		if !ok {
			return
		}

		lastPreviewFile := cv.previewFile
		if lastPreviewFile != previewParams.TargetFile {
			cv.unwatchPreviewFile()
		}

		if previewParams.Action == "show" && previewParams.TargetFile != "" {
			cv.togglePreview(previewParams.TargetFile)
			cv.watchPreviewFile(previewParams.TargetFile)
		}

		if previewParams.Action == "close" {
			cv.previewVisible = false
			cv.previewFile = ""
		}

	})

	return cv
}
