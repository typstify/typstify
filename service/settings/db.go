package settings

import (
	"log"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/utils"
)

type Settings struct {
	db       *bolt.DB
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
	db := openDB(filepath.Join(rootDir, "settings.db"))

	return &Settings{
		db:       db,
		eventbus: bus,
	}
}

func (s *Settings) Close() {
	s.db.Close()
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
	bkt := utils.NewBucket[utils.SKey](name, s.db, &utils.BinaryEncoder[any]{})
	return baseModel{bucket: bkt, onSave: func(model Model) {
		if s.eventbus != nil {
			s.eventbus.Emit(bus.TopicSettingsUpdated, model)
		}
	}}
}

func openDB(dbFile string) *bolt.DB {
	baseDir := filepath.Dir(dbFile)
	err := os.MkdirAll(baseDir, 0700)
	if err != nil {
		log.Fatal(err)
	}

	// It will be created if it doesn't exist.
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}

	// don't forget to close it if needed
	return db
}
