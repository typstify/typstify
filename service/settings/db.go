package settings

import (
	"log"
	"os"
	"path/filepath"

	"looz.ws/typstify/service/bus"
)

type Settings struct {
	store    *settingsStore
	eventbus *bus.EventBus

	general  *GeneralSettings
	editor   *EditorSettings
	typst    *TypstSettings
	tpix     *TpixSettings
	acpAgent *AcpAgentSettings
}

func configRoot() string {
	path, err := os.UserConfigDir()
	if err != nil {
		log.Println("Cannot determine system config dir, use user home instead.", err)
		if home, err2 := os.UserHomeDir(); err2 == nil {
			path = home
		}
	}

	configPath := filepath.Join(path, "/typstify")

	err = os.MkdirAll(configPath, 0700)
	if err != nil {
		log.Fatalln("init failed: ", err)
	}

	return configPath
}

func NewSettings(bus *bus.EventBus) *Settings {
	return newSettings(configRoot(), bus)
}

func newSettings(rootDir string, bus *bus.EventBus) *Settings {
	return &Settings{
		store:    newSettingsStore(rootDir),
		eventbus: bus,
	}
}

func (s *Settings) Close() {
}

func (s *Settings) General() *GeneralSettings {
	if s.general == nil {
		s.general = &GeneralSettings{
			baseModel: s.initModel("general"),
		}
	}

	s.general.Load()
	return s.general
}

func (s *Settings) Editor() *EditorSettings {
	if s.editor == nil {
		s.editor = &EditorSettings{
			baseModel: s.initModel("editor"),
		}
	}

	s.editor.Load()

	return s.editor
}

func (s *Settings) Typst() *TypstSettings {
	if s.typst == nil {
		s.typst = &TypstSettings{
			baseModel: s.initModel("typst"),
		}
	}

	s.typst.Load()
	return s.typst
}

func (s *Settings) Tpix() *TpixSettings {
	if s.tpix == nil {
		s.tpix = &TpixSettings{
			baseModel: s.initModel("tpix"),
		}
	}

	s.tpix.Load()
	return s.tpix
}

func (s *Settings) AcpAgent() *AcpAgentSettings {
	if s.acpAgent == nil {
		s.acpAgent = &AcpAgentSettings{
			baseModel: s.initModel("acpAgent"),
		}
	}

	s.acpAgent.Load()
	return s.acpAgent
}

func (s *Settings) initModel(name string) baseModel {
	return baseModel{name: name, store: s.store, onSave: func(model Model) {
		if s.eventbus != nil {
			s.eventbus.Emit(bus.TopicSettingsUpdated, model)
		}
	}}
}
