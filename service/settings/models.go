package settings

import (
	"fmt"
	"os"

	"gioui.org/font"
)

type Model interface {
	Load() error
	Save() error
	Validate() error
	// Keys() []string
}

var (
	_ Model = (*GeneralSettings)(nil)
	_ Model = (*EditorSettings)(nil)
	_ Model = (*TypstSettings)(nil)
)

var (
	defaultGeneralSettings *GeneralSettings
	defaultEditorSettings  *EditorSettings
	defaultTypstSettings   *TypstSettings
)

type GeneralSettings struct {
	baseModel

	// root dir for user data
	RootDir     string  `key:"rootDir"`
	Language    string  `key:"language"`
	TextSize    float32 `key:"textSize"`
	TypeFace    string  `key:"fontType"`
	Theme       string  `key:"theme"`
	CheckUpdate string  `key:"checkUpdate"`
	DeviceID    string  `key:"deviceId"`

	EnableLSPLogs        int    `key:"enableLspLogs"`
	EnablePowerSaving    int    `key:"enablePowerSaving"`
	ExternalTypst        string `key:"externalTypst"`    // typst executable path
	ExternalTinymist     string `key:"externalTinymist"` // tinymist executable path
	OpenPreviewInBrowser int    `key:"openPreviewInBrowser"`
}

type EditorSettings struct {
	baseModel
	// typeface for editing
	TypeFace         string  `key:"fontType"`
	TextSize         float32 `key:"fontSize"`
	Weight           int     `key:"fontWeight"`
	LineHeightScale  float32 `key:"lineHeightScale"`
	TabSize          int     `key:"tabSize"`
	UseSoftTab       string  `key:"softTab"`
	WrapLine         string  `key:"wrapLine"`
	AutoSaveInterval int     `key:"autoSaveInterval"`
}

type TypstSettings struct {
	baseModel
	Version             string `key:"version"`
	PackageCacheDir     string `key:"cacheDir"`
	PackageDir          string `key:"localPkgDir"`
	ExtraFontPath       string `key:"extraFontPath"`
	UseSysInputs        int    `key:"useSysInputs"`
	IgnoreSystemFonts   int    `key:"ignoreSystemFonts"`
	IgnoreEmbeddedFonts int    `key:"ignoreEmbeddedFonts"`
	BuildDeps           int    `key:"buildDeps"`
	OutputDir           string `key:"outputDir"`
}

type TpixSettings struct {
	baseModel

	Username     string `key:"username"`
	Email        string `key:"email"`
	AccessToken  string `key:"accessToken"`
	RefreshToken string `key:"refreshToken"`
	LoginAt      int64  `key:"loginAt"`
}

func (g *GeneralSettings) Save() error {
	if err := g.Validate(); err != nil {
		return err
	}

	return g.baseModel.save(g)
}

func (g *GeneralSettings) Load() error {
	return g.baseModel.load(g, defaultGeneralSettings)
}

func (g *GeneralSettings) Validate() error {
	if err := isDir(g.RootDir); err != nil {
		return fmt.Errorf("%s is not a directory", g.RootDir)
	}

	if g.Language == "" {
		return fmt.Errorf("Language is not set")
	}

	if g.DeviceID == "" {
		return fmt.Errorf("DeviceID is missing")
	}

	if g.ExternalTypst != "" {
		if err := isFile(g.ExternalTypst); err != nil {
			return err
		}
	}

	if g.ExternalTinymist != "" {
		if err := isFile(g.ExternalTinymist); err != nil {
			return err
		}
	}

	return nil
}

func (e *EditorSettings) Save() error {
	if err := e.Validate(); err != nil {
		return err
	}

	return e.baseModel.save(e)
}

func (e *EditorSettings) Load() error {
	return e.baseModel.load(e, defaultEditorSettings)
}

func (e *EditorSettings) Validate() error {
	e2 := defaultEditorSettings

	if e.LineHeightScale <= 0 {
		e.LineHeightScale = e2.LineHeightScale
	}

	if e.TextSize <= 0 {
		e.TextSize = e2.TextSize
	}

	if font.Weight(e.Weight) < font.Thin || font.Weight(e.Weight) > font.Black {
		e.Weight = e2.Weight
	}

	if e.TypeFace == "" {
		e.TypeFace = e2.TypeFace
	}

	if e.TabSize <= 0 {
		e.TabSize = e2.TabSize
	}

	if e.UseSoftTab == "" {
		e.UseSoftTab = e2.UseSoftTab
	}
	if e.WrapLine == "" {
		e.WrapLine = e2.WrapLine
	}

	if e.AutoSaveInterval <= 0 {
		return fmt.Errorf("AutoSaveInterval should > 0")
	}

	return nil
}

func (t *TypstSettings) Save() error {
	if err := t.Validate(); err != nil {
		return err
	}
	return t.baseModel.save(t)
}

func (t *TypstSettings) Load() error {
	return t.baseModel.load(t, defaultTypstSettings)
}

func (t *TypstSettings) Validate() error {
	if t.PackageCacheDir != "" {
		if err := isDir(t.PackageCacheDir); err != nil {
			return err
		}
	}

	if t.PackageDir != "" {
		if err := isDir(t.PackageDir); err != nil {
			return err
		}
	}

	if t.ExtraFontPath != "" {
		if err := isDir(t.ExtraFontPath); err != nil {
			return err
		}
	}

	if t.OutputDir != "" {
		if err := isDir(t.OutputDir); err != nil {
			return err
		}
	}

	return nil
}

func (t *TpixSettings) Save() error {
	if err := t.Validate(); err != nil {
		return err
	}

	return t.baseModel.save(t)
}

func (t *TpixSettings) Load() error {
	return t.baseModel.load(t, &TpixSettings{})
}

func (t *TpixSettings) Validate() error {
	return nil
}

func (t *TpixSettings) Clear() {
	t.Username = ""
	t.Email = ""
	t.AccessToken = ""
	t.RefreshToken = ""
	t.LoginAt = 0
	t.Save()
}

func init() {
	// do a initialize here:
	defaultGeneralSettings = &GeneralSettings{
		RootDir:              configRoot(),
		Language:             "en-US",
		DeviceID:             genDeviceID(),
		Theme:                "Default Light",
		TextSize:             13,
		TypeFace:             "",
		CheckUpdate:          "true",
		EnableLSPLogs:        0,
		EnablePowerSaving:    0,
		OpenPreviewInBrowser: 0,
	}

	defaultEditorSettings = &EditorSettings{
		TypeFace:         "Hack, Roboto Mono, Go Mono, Noto Emoji, monospace",
		TextSize:         13,
		Weight:           int(font.Normal),
		LineHeightScale:  1.6,
		UseSoftTab:       "true",
		TabSize:          4,
		WrapLine:         "true",
		AutoSaveInterval: 3,
	}

	defaultTypstSettings = &TypstSettings{
		Version:             "",
		PackageCacheDir:     "",
		PackageDir:          "",
		IgnoreSystemFonts:   0,
		IgnoreEmbeddedFonts: 0,
		UseSysInputs:        1,
		ExtraFontPath:       "",
		BuildDeps:           0,
		OutputDir:           "",
	}
}

func isDir(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	return nil
}

func isFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s cannot be accessed", path)
	}

	if st.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}

	return nil
}
