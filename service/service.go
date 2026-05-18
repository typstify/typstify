package service

import (
	"context"
	"errors"
	"io"
	"log"
	"time"

	"github.com/oligo/gioview/explorer"
	"github.com/oligo/gioview/image"
	"github.com/oligo/gioview/view"
	"github.com/typstify/tpix-cli/api"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/agent/extensions"
	"looz.ws/typstify/lsp"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/service/net"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst"
	"looz.ws/typstify/typst/pkg"
	"looz.ws/typstify/ui/console"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/version"
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

	currentProjectDir string

	// Window layout metrics for native webview positioning.
	// Set by the home view each frame.
	WindowContentWidth int
	ViewAreaTopOffset  int
}

func NewService(ctx context.Context) *ServiceFacade {
	eventbus := bus.NewEventBus(ctx, false)
	st := settings.NewSettings(eventbus)

	s := &ServiceFacade{
		eventbus:     eventbus,
		settings:     st,
		pkgService:   pkg.NewTypstPkgService(st.Typst(), st.Tpix()),
		workspaceSrv: NewWorkspaceService(st.General().RootDir, eventbus),
		windowSrv:    NewWindowService(ctx, st),
		consoleState: console.NewConsoleState(1000),
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
	s.pkgService.SetReporter(tpixCliReporter{c: s.consoleState}.Report)
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

func (s *ServiceFacade) StartACPSession(ctx context.Context, projectDir string) (*agent.ACPSession, error) {
	if s.acpSessionManager == nil {
		err := s.startAcpSessionManager(ctx)
		if err != nil {
			return nil, err
		}
	}

	return s.acpSessionManager.NewSession(ctx, projectDir)
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

func (s *ServiceFacade) startAcpSessionManager(ctx context.Context) error {
	cwd := s.CurrentProjectDir()
	if cwd == "" {
		return errors.New("No project dir is open")
	}

	childCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	mgr := &agent.SessionManager{}
	client := agent.NewACPClient(mgr)

	compilerExt := extensions.TypstCompilerExt(s.CurrentProjectDir(), s.Settings().Typst())
	client.RegisterExtension("typstify/compileTypst", compilerExt)

	if err := mgr.Start(childCtx,
		agent.AgentConfig{
			Name: "Claude Code",
			Cmd:  "npx",
			Args: []string{"-y", "@agentclientprotocol/claude-agent-acp@0.35.0"},
		},
		// agent.AgentConfig{
		// 	Name: "Codex",
		// 	Cmd:  "npx",
		// 	Args: []string{"-y", "@zed-industries/codex-acp"},
		// },
		client,
	); err != nil {
		return err
	}

	s.acpSessionManager = mgr
	return nil
}

func (s *ServiceFacade) stopAcpSessionManager(ctx context.Context) {
	childCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if s.acpSessionManager != nil {
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
