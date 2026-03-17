package service

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/oligo/gioview/explorer"
	"github.com/oligo/gioview/image"
	"github.com/oligo/gioview/view"
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
	fileChooserBuilder func() *explorer.FileChooser
	consoleState       *console.ConsoleState

	currentProjectDir string

	// Window layout metrics for native webview positioning.
	// Set by the home view each frame.
	WindowContentWidth int
	ViewAreaTopOffset  int
}

func NewService(ctx context.Context) *ServiceFacade {
	eventbus := bus.NewEventBus(ctx, false)
	st := settings.NewSettings(eventbus)

	// init executable lookup path.
	lsp.Init(st.General().ExternalTinymist)
	typst.Init(st.General().ExternalTypst)

	s := &ServiceFacade{
		eventbus:      eventbus,
		settings:      st,
		pkgService:    pkg.NewTypstPkgService(st.Typst()),
		workspaceSrv: NewWorkspaceService(st.General().RootDir),
		windowSrv:     NewWindowService(ctx, st),
		consoleState:  console.NewConsoleState(1000),
	}

	eventbus.Subscribe(s, "service.onSettingUpdate", bus.TopicSettingsUpdated, func(topic string, data interface{}) {
		s.pkgService = pkg.NewTypstPkgService(st.Typst())
	})

	s.RegisterDevice()

	return s
}

func (s *ServiceFacade) EventBus() *bus.EventBus {
	return s.eventbus
}

func (s *ServiceFacade) Settings() *settings.Settings {
	return s.settings
}

func (s *ServiceFacade) PkgService() *pkg.TypstPkgService {
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

func (s *ServiceFacade) Close(ctx context.Context) {
	image.ClearCache()
	s.workspaceSrv.Close()
	s.windowSrv.Shutdown()
	s.windowSrv.Wait()
	lsp.StopLsp()
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
	s.currentProjectDir = dir
	// connect to LSP server in an eager way.
	client := lsp.GetLspClient(s.currentProjectDir, s.Settings())
	if s.settings.General().EnableLSPLogs != 0 {
		client.SetServreLogStreamer(s.consoleState)
	} else {
		client.SetServreLogStreamer(io.Discard)
	}
}

func (s *ServiceFacade) CurrentProjectDir() string {
	return s.currentProjectDir
}

func (s *ServiceFacade) Console() *console.ConsoleState {
	return s.consoleState
}
