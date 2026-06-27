package settings

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	bolt "go.etcd.io/bbolt"

	"looz.ws/typstify/utils"
)

const (
	settingsFileName    = "settings.json"
	legacySettingsDB    = "settings.db"
	settingsFileVersion = 1
	settingsVersionKey  = "version"
)

type settingsStore struct {
	mu     sync.Mutex
	root   string
	path   string
	loaded bool
	data   map[string]json.RawMessage
}

func newSettingsStore(root string) *settingsStore {
	return &settingsStore{
		root: root,
		path: filepath.Join(root, settingsFileName),
	}
}

func (s *settingsStore) load(name string, model Model) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureLoadedLocked(); err != nil {
		return false, err
	}

	raw, ok := s.data[name]
	if !ok {
		return false, nil
	}

	if err := json.Unmarshal(raw, model); err != nil {
		return false, err
	}
	return true, nil
}

func (s *settingsStore) save(name string, model Model) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureLoadedLocked(); err != nil {
		return err
	}

	raw, err := json.Marshal(model)
	if err != nil {
		return err
	}
	s.data[name] = raw

	return s.writeLocked()
}

func (s *settingsStore) ensureLoadedLocked() error {
	if s.loaded {
		return nil
	}

	s.data = make(map[string]json.RawMessage)

	data, err := os.ReadFile(s.path)
	if err == nil {
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &s.data); err != nil {
				return err
			}
			delete(s.data, settingsVersionKey)
		}
		s.loaded = true
		return nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	legacyData, err := loadLegacySettings(filepath.Join(s.root, legacySettingsDB))
	if err != nil {
		log.Printf("migrate legacy settings failed: %v", err)
	}
	for name, raw := range legacyData {
		s.data[name] = raw
	}

	s.loaded = true
	if len(s.data) > 0 {
		return s.writeLocked()
	}
	return nil
}

func (s *settingsStore) writeLocked() error {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return err
	}

	doc := make(map[string]json.RawMessage, len(s.data)+1)
	for key, val := range s.data {
		doc[key] = val
	}
	version, err := json.Marshal(settingsFileVersion)
	if err != nil {
		return err
	}
	doc[settingsVersionKey] = version

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.root, ".settings-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, s.path)
}

func loadLegacySettings(dbFile string) (map[string]json.RawMessage, error) {
	if _, err := os.Stat(dbFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	sections := []struct {
		name  string
		model Model
	}{
		{name: "general", model: &GeneralSettings{}},
		{name: "editor", model: &EditorSettings{}},
		{name: "typst", model: &TypstSettings{}},
		{name: "tpix", model: &TpixSettings{}},
		{name: "acpAgent", model: &AcpAgentSettings{}},
	}

	result := make(map[string]json.RawMessage)
	for _, section := range sections {
		if loadLegacyModel(db, section.name, section.model) {
			raw, err := json.Marshal(section.model)
			if err != nil {
				return nil, err
			}
			result[section.name] = raw
		}
	}

	return result, nil
}

func loadLegacyModel(db *bolt.DB, name string, model Model) bool {
	values, ok := readLegacyValues(db, name, model)
	if !ok {
		return false
	}

	loaded := loadLegacyValuesIndividually(model, values)
	if loadLegacyValuesAsStream(model, values) {
		loaded = true
	}
	return loaded
}

func readLegacyValues(db *bolt.DB, name string, model Model) ([][]byte, bool) {
	t := reflect.ValueOf(model).Elem()
	values := make([][]byte, t.NumField())
	found := false

	err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(name))
		if bkt == nil {
			return nil
		}

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.CanSet() {
				continue
			}

			keyName := t.Type().Field(i).Tag.Get("key")
			if keyName == "" {
				continue
			}

			raw := bkt.Get([]byte(keyName))
			if raw == nil {
				continue
			}

			values[i] = append([]byte(nil), raw...)
			found = true
		}
		return nil
	})
	if err != nil {
		log.Printf("read legacy settings bucket %s failed: %v", name, err)
		return nil, false
	}

	return values, found
}

func loadLegacyValuesIndividually(model Model, values [][]byte) bool {
	t := reflect.ValueOf(model).Elem()
	loaded := false

	for i, raw := range values {
		if raw == nil {
			continue
		}

		var val any
		if err := (&utils.BinaryEncoder[any]{}).Decode(raw, &val); err != nil {
			continue
		}

		if err := setFieldValue(t.Field(i), val); err != nil {
			continue
		}
		loaded = true
	}

	return loaded
}

func loadLegacyValuesAsStream(model Model, values [][]byte) bool {
	var stream bytes.Buffer
	for _, raw := range values {
		if raw == nil {
			continue
		}
		stream.Write(raw)
	}
	if stream.Len() == 0 {
		return false
	}

	decoder := gob.NewDecoder(&stream)
	t := reflect.ValueOf(model).Elem()
	loaded := false

	for i, raw := range values {
		if raw == nil {
			continue
		}

		var val any
		if err := decoder.Decode(&val); err != nil {
			return loaded
		}

		if err := setFieldValue(t.Field(i), val); err != nil {
			continue
		}
		loaded = true
	}

	return loaded
}
