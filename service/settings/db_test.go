package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"

	"looz.ws/typstify/utils"
)

func TestModelSave(t *testing.T) {
	root := t.TempDir()
	db := newSettings(root, nil)
	general := db.General()

	t.Log("general: ", general)

	general.Language = "cn/zh"

	err := general.Save()
	if err != nil {
		t.Log(err)
		t.Fail()
	}

	db.Close()

	data, err := os.ReadFile(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}

	var persisted GeneralSettings
	if err := json.Unmarshal(doc["general"], &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Language != "cn/zh" {
		t.Fatalf("Language = %q, want %q", persisted.Language, "cn/zh")
	}

	reloaded := newSettings(root, nil)
	if got := reloaded.General().Language; got != "cn/zh" {
		t.Fatalf("reloaded Language = %q, want %q", got, "cn/zh")
	}

}

func TestModelGetDefault(t *testing.T) {
	db := newSettings(t.TempDir(), nil)
	general := db.General()
	typst := db.Typst()

	t.Log("general: ", general)
	t.Log("typst: ", typst)
	t.Log("editor: ", db.Editor())

	t.Cleanup(func() {
		db.Close()
	})

}

func TestLegacySettingsMigration(t *testing.T) {
	root := t.TempDir()
	db, err := bolt.Open(filepath.Join(root, "settings.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	bkt := utils.NewBucket[utils.SKey]("tpix", db, &utils.BinaryEncoder[any]{})
	saveLegacyValue(t, bkt, "accessToken", "access-token")
	saveLegacyValue(t, bkt, "refreshToken", "refresh-token")
	saveLegacyValue(t, bkt, "loginAt", int64(1234))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	settings := newSettings(root, nil)
	tpix := settings.Tpix()
	if tpix.AccessToken != "access-token" {
		t.Fatalf("AccessToken = %q, want %q", tpix.AccessToken, "access-token")
	}
	if tpix.RefreshToken != "refresh-token" {
		t.Fatalf("RefreshToken = %q, want %q", tpix.RefreshToken, "refresh-token")
	}
	if tpix.LoginAt != 1234 {
		t.Fatalf("LoginAt = %d, want %d", tpix.LoginAt, int64(1234))
	}

	if _, err := os.Stat(filepath.Join(root, "settings.json")); err != nil {
		t.Fatal(err)
	}
}

func saveLegacyValue(t *testing.T, bkt *utils.Bucket[utils.SKey, any], key string, val any) {
	t.Helper()

	fieldVal := val
	if err := bkt.Save(utils.SKey(key), &fieldVal); err != nil {
		t.Fatal(err)
	}
}
