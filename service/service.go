package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/oligo/gioview/explorer"
	"github.com/oligo/gioview/image"
	"github.com/oligo/gioview/view"
	"github.com/typstify/tpix-cli/api"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/lsp"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/service/mcp"
	"looz.ws/typstify/service/net"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst"
	"looz.ws/typstify/typst/pkg"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/version"
	"looz.ws/typstify/widgets/console"
)

const (
	staticMcpServerPort = 5322
)

type ServiceFacade struct {
	vm                 view.ViewManager
	settings           *settings.Settings
	eventbus           *bus.EventBus
	workspaceSrv       *WorkspaceService
	pkgService         *pkg.TypstPkgService
	windowSrv          *WindowService
	previewSrv         *lsp.PreviewService
	fileChooserBuilder func() *explorer.FileChooser
	consoleState       *console.ConsoleState
	acpSessionManager  *agent.SessionManager
	acpMu              sync.Mutex
	mcpServer          *agent.McpServer // the built-in mcp server

	tpixSessionSrv *TpixSessionService

	currentProjectDir string

	// Window layout metrics for native webview positioning.
	// Set by the home view each frame.
	WindowContentWidth int
	ViewAreaTopOffset  int
}

func NewService(ctx context.Context) *ServiceFacade {
	eventbus := bus.NewEventBus(ctx, false)
	st := settings.NewSettings(eventbus)
	tpixSessionSrv := &TpixSessionService{setting: st.Tpix()}

	s := &ServiceFacade{
		eventbus:       eventbus,
		settings:       st,
		pkgService:     pkg.NewTypstPkgService(st.Typst(), st.Tpix()),
		workspaceSrv:   NewWorkspaceService(st.General().RootDir, eventbus),
		windowSrv:      NewWindowService(ctx, st),
		consoleState:   console.NewConsoleState(1000),
		tpixSessionSrv: tpixSessionSrv,
	}

	eventbus.Subscribe(s, "service.onSettingUpdate", bus.TopicSettingsUpdated, func(topic string, data interface{}) {
		s.pkgService = pkg.NewTypstPkgService(st.Typst(), st.Tpix())
	})

	// init executable lookup path.
	lsp.SetupCmdBuilder(s.settings.General().ExternalTinymist)
	typst.SetupCmdBuilder(s.settings.General().ExternalTypst)

	s.RegisterDevice()

	api.Init(&tpixCredentialProvider{setting: st.Tpix()})

	return s
}

func (s *ServiceFacade) EventBus() *bus.EventBus {
	return s.eventbus
}

func (s *ServiceFacade) Settings() *settings.Settings {
	return s.settings
}

func (s *ServiceFacade) PkgService() *pkg.TypstPkgService {
	s.pkgService.SetReporter(tpixCliReporter{w: s.consoleState}.Report)
	return s.pkgService
}

func (s *ServiceFacade) Workspace() *WorkspaceService {
	return s.workspaceSrv
}

func (s *ServiceFacade) WindowService() *WindowService {
	return s.windowSrv
}

func (s *ServiceFacade) InitFileChooser(builder func() *explorer.FileChooser) {
	s.fileChooserBuilder = builder
}

func (s *ServiceFacade) FileChooser() *explorer.FileChooser {
	return s.fileChooserBuilder()
}

func (s *ServiceFacade) SetViewManager(vm view.ViewManager) {
	s.vm = vm
}

func (s *ServiceFacade) RequestSwitch(intent view.Intent) {
	s.vm.RequestSwitch(intent)
}

func (s *ServiceFacade) RefreshWindow() {
	s.vm.Invalidate()
}

func (s *ServiceFacade) Close(ctx context.Context) {
	image.ClearCache()
	s.workspaceSrv.Close()
	s.windowSrv.Shutdown()
	s.windowSrv.Wait()
	lsp.StopLsp()
	if s.previewSrv != nil {
		s.previewSrv.Destroy(ctx)
	}

	s.stopAcpSessionManager(ctx)
	if s.mcpServer != nil {
		s.mcpServer.Shutdown(ctx)
	}
	log.Println("service down")
}

func (s *ServiceFacade) RegisterDevice() {
	api := net.NewRemote()
	go func(dev settings.Device) {
		req := &net.DeviceInfo{
			DeviceID:   dev.ID,
			Hostname:   dev.Hostname,
			OS:         dev.OS,
			Platform:   dev.Platform,
			AppVersion: version.BinVersion,
		}

		tz, _ := time.Now().Zone()
		req.Timezone = tz

		err := api.RegisterDevice(req)
		if err != nil {
			log.Println("Register device failed: ", err)
		}
	}(s.settings.General().GetDeviceInfo())

}

func (s *ServiceFacade) CheckUpdate() *net.ReleaseInfo {
	api := net.NewRemote()
	device := s.settings.General().GetDeviceInfo()
	release, err := api.CheckUpdate(&net.UpdateCheckReq{
		DeviceID:       device.ID,
		CurrentVersion: version.BinVersion,
		UseBeta:        false,
	})

	if err != nil {
		log.Println("check update failed: ", err)
		return nil
	}

	latestVer := utils.ParseVersion(release.AppVersion)
	currentVer := utils.ParseVersion(version.BinVersion)

	if latestVer == nil {
		return nil
	}
	if currentVer == nil {
		return release
	}

	if currentVer.Compare(latestVer) < 0 {
		return release
	}

	return nil
}

func (s *ServiceFacade) SetProjectDir(dir string) {
	if dir == "" {
		return
	}

	s.currentProjectDir = dir

	s.Workspace().SwitchWorkspace(s.currentProjectDir)

	// init executable lookup path.
	lsp.SetupCmdBuilder(s.settings.General().ExternalTinymist)
	typst.SetupCmdBuilder(s.settings.General().ExternalTypst)

	// connect to LSP server in an eager way.
	client := lsp.GetLspClient(s.currentProjectDir, s.Settings())
	if s.settings.General().EnableLSPLogs != 0 {
		client.SetServreLogStreamer(s.consoleState)
	} else {
		client.SetServreLogStreamer(io.Discard)
	}

	previewMode := lsp.PreviewMode(s.Workspace().LoadWorkspaceSettings().PreviewMode)
	if previewMode == "" {
		previewMode = lsp.DocumentPreviewMode
	}

	s.previewSrv = lsp.NewPreviwService(client)
	go func() {
		s.previewSrv.Start(context.Background(),
			lsp.PreviewOptions{
				Mode:          previewMode,
				InvertColor:   "never",
				PartialRender: false,
			}, nil)
	}()

	// stop the last acpSessionManager
	s.stopAcpSessionManager(context.Background())
}

func (s *ServiceFacade) RestartPreview(ctx context.Context, onFinish func()) {
	if s.previewSrv == nil {
		return
	}

	previewMode := lsp.PreviewMode(s.Workspace().LoadWorkspaceSettings().PreviewMode)
	if previewMode == "" {
		previewMode = lsp.DocumentPreviewMode
	}

	go func() {
		s.previewSrv.Start(context.Background(),
			lsp.PreviewOptions{
				Mode:          previewMode,
				ProjectRoot:   s.currentProjectDir,
				InvertColor:   "never",
				PartialRender: false,
			}, onFinish)
	}()
}

func (s *ServiceFacade) PreviewService() *lsp.PreviewService {
	return s.previewSrv
}

func (s *ServiceFacade) CurrentProjectDir() string {
	return s.currentProjectDir
}

func (s *ServiceFacade) Console() *console.ConsoleState {
	return s.consoleState
}

func (s *ServiceFacade) initMcpServer(ctx context.Context) {
	if !s.tpixSessionSrv.Authenticated() {
		return
	}
	if s.mcpServer != nil {
		s.mcpServer.Shutdown(ctx)
	}

	serverPort := 0
	useStaticPort := s.settings.AcpAgent().UseStaticMcpPort == 1
	if useStaticPort {
		serverPort = staticMcpServerPort
	}
	s.mcpServer = agent.NewMcpServer(serverPort)

	client := lsp.GetLspClient(s.currentProjectDir, s.Settings())

	// compilerTool := mcp.TypstCompilerHandler(s.CurrentProjectDir(), s.Settings().Typst())
	// agent.AddMcpTool(s.mcpServer, mcp.TypstCompilerTool, compilerTool)
	activeDocQuerier := func() *mcp.ActiveDocument {
		if s.vm == nil {
			return nil
		}
		cv := s.vm.CurrentView()
		if cv == nil {
			return nil
		}
		if provider, ok := cv.(mcp.ActiveDocProvider); ok {
			doc := provider.GetActiveDocument()
			return &doc
		}
		return nil
	}
	editorToolSrv := mcp.NewEditorMcpService(s.currentProjectDir, s.settings, client, s.previewSrv, s.eventbus, activeDocQuerier)
	s.mcpServer.RegisterToolProvider(editorToolSrv)

	pkgToolSrv := mcp.NewPackageMcpService(s.currentProjectDir, s.settings.Tpix(), s.PkgService())
	s.mcpServer.RegisterToolProvider(pkgToolSrv)
	s.mcpServer.RegisterResourceProvider(pkgToolSrv)

	s.mcpServer.Run()
}

func (s *ServiceFacade) listMcpServer() []acp.McpServer {
	mcpServers := make([]acp.McpServer, 0)

	if s.tpixSessionSrv.Authenticated() {
		// built-in mcp server
		ip, port := s.mcpServer.Addr()
		mcpServers = append(mcpServers, acp.McpServer{
			Http: &acp.McpServerHttpInline{
				Name:    agent.ServerName,
				Type:    "http",
				Url:     fmt.Sprintf("http://%s:%d", ip, port),
				Headers: []acp.HttpHeader{},
			},
		})
	}

	return mcpServers
}

func (s *ServiceFacade) StartACPSession(ctx context.Context, projectDir string) (*agent.ACPSession, error) {
	as := s.settings.AcpAgent()

	s.acpMu.Lock()
	mgr := s.acpSessionManager
	// If the configured agent changed, stop the running one.
	if mgr != nil && !configEqual(mgr.Config(), as) {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mgr.Close(closeCtx); err != nil {
			log.Printf("close old ACP session manager: %v", err)
		}
		s.acpSessionManager = nil
		mgr = nil
		log.Println("agent config changed, restarting session manager...")
	}
	s.acpMu.Unlock()

	if mgr == nil {
		if err := s.startAcpSessionManager(ctx); err != nil {
			return nil, err
		}
		s.acpMu.Lock()
		mgr = s.acpSessionManager
		s.acpMu.Unlock()
	}

	return mgr.NewSession(ctx, projectDir)
}

func configEqual(a agent.AgentConfig, as *settings.AcpAgentSettings) bool {
	return a.Name == as.AgentName && a.Cmd == as.Cmd && strings.Join(a.Args, " ") == as.Args && strings.Join(a.Env, " ") == as.Env
}

func (s *ServiceFacade) CloseACPSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if s.acpSessionManager == nil {
		return nil
	}

	return s.acpSessionManager.CloseSession(ctx, sessionID)
}

// defaultAgentConfig is the fallback when no agent is configured.
var defaultAgentConfig = agent.AgentConfig{
	Name: "Claude Code",
	Cmd:  "npx",
	Args: []string{"-y", "@agentclientprotocol/claude-agent-acp@0.50.0"},
}

func (s *ServiceFacade) buildAgentConfig() agent.AgentConfig {
	as := s.settings.AcpAgent()
	if as.Cmd == "" {
		return defaultAgentConfig
	}
	return agent.AgentConfig{
		Name: as.AgentName,
		Cmd:  as.Cmd,
		Args: strings.Fields(as.Args),
		Env:  strings.Fields(as.Env),
	}
}

func (s *ServiceFacade) startAcpSessionManager(ctx context.Context) error {
	cwd := s.CurrentProjectDir()
	if cwd == "" {
		return errors.New("No project dir is open")
	}

	childCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// start the mcp server
	s.initMcpServer(ctx)

	mgr := agent.NewSessionManager(s.listMcpServer())

	// Setting env ACP_DEBUG=1 will turn Typstify into ACP debug mode.
	acpDebug := os.Getenv("ACP_DEBUG") == "1"
	agentConfig := s.buildAgentConfig()

	// stream agent logs(usually streamed via stderr) to console. Some agents like Cline
	// write Device-auth flow guide to the console log stream, so user can complete the authentication flow.
	if err := mgr.Start(childCtx, agentConfig, acpDebug, s.consoleState); err != nil {
		return err
	}

	s.acpSessionManager = mgr
	return nil
}

func (s *ServiceFacade) stopAcpSessionManager(ctx context.Context) {
	s.acpMu.Lock()
	defer s.acpMu.Unlock()

	if s.acpSessionManager != nil {
		childCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		s.acpSessionManager.Close(childCtx)
		s.acpSessionManager = nil
	}
}

func (s *ServiceFacade) AcpSessionManager() *agent.SessionManager {
	if s.acpSessionManager != nil {
		return s.acpSessionManager
	}

	childCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := s.startAcpSessionManager(childCtx)
	if err != nil {
		log.Println("start ACP session manager failed: ", err)
		return nil
	}

	return s.acpSessionManager
}

func (s *ServiceFacade) TpixSessionService() *TpixSessionService {
	return s.tpixSessionSrv
}
